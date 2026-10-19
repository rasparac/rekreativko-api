package application

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rasparac/rekreativko-api/identity/internal/domain"
	"github.com/rasparac/rekreativko-api/identity/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

// fakeAccounts serves the due accounts batch by batch.
type fakeAccounts struct {
	due      []*domain.Account
	listErr  error
	limits   []int
	updated  []*domain.Account
	listedAt []time.Time
}

func (f *fakeAccounts) ListDueForErasure(_ context.Context, now time.Time, limit int) ([]*domain.Account, error) {
	f.limits = append(f.limits, limit)
	f.listedAt = append(f.listedAt, now)
	if f.listErr != nil {
		return nil, f.listErr
	}
	n := min(limit, len(f.due))
	batch := f.due[:n]
	f.due = f.due[n:]
	return batch, nil
}

func (f *fakeAccounts) UpdateAccount(_ context.Context, account *domain.Account) error {
	f.updated = append(f.updated, account)
	return nil
}

type fakeDeleter struct{ accounts []uuid.UUID }

func (f *fakeDeleter) DeleteAllForAccount(_ context.Context, accountID uuid.UUID) error {
	f.accounts = append(f.accounts, accountID)
	return nil
}

type fakeTracker struct {
	recorded     map[uuid.UUID][]string
	incomplete   int64
	contexts     []string
	erasedBefore time.Time
}

func (f *fakeTracker) RecordCompleted(_ context.Context, accountID uuid.UUID, context string, _ time.Time) error {
	if f.recorded == nil {
		f.recorded = map[uuid.UUID][]string{}
	}
	f.recorded[accountID] = append(f.recorded[accountID], context)
	return nil
}

func (f *fakeTracker) CountIncomplete(_ context.Context, contexts []string, erasedBefore time.Time) (int64, error) {
	f.contexts = contexts
	f.erasedBefore = erasedBefore
	return f.incomplete, nil
}

// fakeTx runs fn directly; a failed fn is "rolled back" by the caller's checks.
type fakeTx struct{ runs int }

func (f *fakeTx) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	f.runs++
	return fn(ctx)
}

type fakeWriter struct{ events []domainevent.Event }

func (f *fakeWriter) InsertEvents(_ context.Context, _ string, events []domainevent.Event) error {
	f.events = append(f.events, events...)
	return nil
}

type erasureTestEnv struct {
	svc      *ErasureService
	accounts *fakeAccounts
	tokens   *fakeDeleter
	codes    *fakeDeleter
	tracker  *fakeTracker
	tx       *fakeTx
	writer   *fakeWriter
	metrics  *metrics.Metrics
	logs     *bytes.Buffer
}

func newErasureTestEnv(t *testing.T, due ...*domain.Account) *erasureTestEnv {
	t.Helper()

	env := &erasureTestEnv{
		accounts: &fakeAccounts{due: due},
		tokens:   &fakeDeleter{},
		codes:    &fakeDeleter{},
		tracker:  &fakeTracker{},
		tx:       &fakeTx{},
		writer:   &fakeWriter{},
		metrics: &metrics.Metrics{
			AccountsErasedTotal: prometheus.NewCounter(prometheus.CounterOpts{Name: "erased"}),
			ErasuresIncomplete:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "incomplete"}),
		},
		logs: &bytes.Buffer{},
	}

	svc, err := NewErasureService(
		env.accounts, env.tokens, env.codes, env.tracker, env.tx, env.writer,
		logger.NewWithWriter("info", "json", env.logs), env.metrics,
		ErasurePolicy{Batch: 2, Contexts: []string{"activity", "notifications"}, IncompleteAfter: 7 * 24 * time.Hour},
	)
	require.NoError(t, err)
	env.svc = svc

	return env
}

func deletedAccount(t *testing.T) *domain.Account {
	t.Helper()

	email, err := domain.NewEmail(uuid.NewString()[:8] + "@example.com")
	require.NoError(t, err)

	now := time.Now().UTC()
	account := domain.ReconstructAccount(
		uuid.New(), email, nil, domain.NewPasswordFromHash("hash"),
		domain.AccountStatusDeleted, 0, nil, now, now, &now, &now, nil,
	)
	return account
}

func metricValue(t *testing.T, c prometheus.Collector) float64 {
	t.Helper()

	ch := make(chan prometheus.Metric, 1)
	c.Collect(ch)
	var m dto.Metric
	require.NoError(t, (<-ch).Write(&m))
	if m.Counter != nil {
		return m.GetCounter().GetValue()
	}
	return m.GetGauge().GetValue()
}

func TestNewErasureService_RejectsNonPositiveSettings(t *testing.T) {
	for name, policy := range map[string]ErasurePolicy{
		"batch":            {Batch: 0, IncompleteAfter: time.Hour},
		"incomplete after": {Batch: 1, IncompleteAfter: 0},
	} {
		_, err := NewErasureService(nil, nil, nil, nil, nil, nil, logger.New("error", "json"), nil, policy)
		assert.Error(t, err, name)
	}
}

func TestErasureService_RunOnce_ErasesEveryDueAccountBatchByBatch(t *testing.T) {
	due := []*domain.Account{deletedAccount(t), deletedAccount(t), deletedAccount(t)}
	env := newErasureTestEnv(t, due...)

	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	env.svc.RunOnce(context.Background(), now)

	assert.Equal(t, []int{2, 2}, env.accounts.limits, "full batch, short batch: stop")
	assert.Equal(t, 2, env.tx.runs, "one transaction per batch")
	for _, at := range env.accounts.listedAt {
		assert.True(t, at.Equal(now))
	}

	require.Len(t, env.accounts.updated, 3)
	for i, account := range due {
		assert.True(t, account.Email().IsEmpty(), "account %d erased", i)
		assert.Equal(t, domain.ErasedPasswordHash, account.Password().String())
		assert.NotNil(t, account.ErasedAt())
		assert.Empty(t, account.Events(), "events cleared once written")
		assert.Contains(t, env.tokens.accounts, account.ID())
		assert.Contains(t, env.codes.accounts, account.ID())
	}

	require.Len(t, env.writer.events, 3)
	for i, event := range env.writer.events {
		erased, ok := event.(*domain.AccountErasedEvent)
		require.True(t, ok)
		assert.Equal(t, due[i].ID(), erased.AccountID)
	}

	assert.InDelta(t, 3, metricValue(t, env.metrics.AccountsErasedTotal), 0)
	assert.Contains(t, env.logs.String(), "erased deleted accounts")
}

func TestErasureService_RunOnce_NothingDueIsQuiet(t *testing.T) {
	env := newErasureTestEnv(t)

	env.svc.RunOnce(context.Background(), time.Now())

	assert.Len(t, env.accounts.limits, 1)
	assert.Empty(t, env.writer.events)
	assert.Zero(t, metricValue(t, env.metrics.AccountsErasedTotal))
	assert.NotContains(t, env.logs.String(), "erased deleted accounts")
}

func TestErasureService_RunOnce_AFailedBatchIsLoggedAndTheGaugeStillRefreshed(t *testing.T) {
	env := newErasureTestEnv(t)
	env.accounts.listErr = errors.New("boom")
	env.tracker.incomplete = 4

	env.svc.RunOnce(context.Background(), time.Now())

	assert.Len(t, env.accounts.limits, 1, "no retry loop on error")
	assert.Contains(t, env.logs.String(), "failed to erase deleted accounts")
	assert.InDelta(t, 4, metricValue(t, env.metrics.ErasuresIncomplete), 0)
}

func TestErasureService_RunOnce_CountsIncompleteErasuresPastTheWindow(t *testing.T) {
	env := newErasureTestEnv(t)
	env.tracker.incomplete = 2

	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	env.svc.RunOnce(context.Background(), now)

	assert.Equal(t, []string{"activity", "notifications"}, env.tracker.contexts)
	assert.True(t, env.tracker.erasedBefore.Equal(now.Add(-7*24*time.Hour)), "got %s", env.tracker.erasedBefore)
	assert.InDelta(t, 2, metricValue(t, env.metrics.ErasuresIncomplete), 0)
	assert.Contains(t, env.logs.String(), "account erasures not confirmed by every context")
}

func TestErasureService_RunOnce_StopsWhenTheContextEnds(t *testing.T) {
	env := newErasureTestEnv(t, deletedAccount(t))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	env.svc.RunOnce(ctx, time.Now())

	assert.Empty(t, env.accounts.limits)
}

func TestErasureService_RecordContextErasure(t *testing.T) {
	env := newErasureTestEnv(t)
	accountID := uuid.New()

	require.NoError(t, env.svc.RecordContextErasure(context.Background(), accountID, "activity"))

	assert.Equal(t, []string{"activity"}, env.tracker.recorded[accountID])
}
