package events

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rasparac/rekreativko-api/shared/logger"
)

type purgeCall struct {
	schema string
	before time.Time
	limit  int
}

// fakeDeleter answers each schema's calls from a script of results.
type fakeDeleter struct {
	script map[string][]int64
	errs   map[string]error
	calls  []purgeCall
}

func (f *fakeDeleter) DeletePublishedEvents(_ context.Context, schema string, before time.Time, limit int) (int64, error) {
	f.calls = append(f.calls, purgeCall{schema, before, limit})
	if err := f.errs[schema]; err != nil {
		return 0, err
	}
	results := f.script[schema]
	if len(results) == 0 {
		return 0, nil
	}
	f.script[schema] = results[1:]
	return results[0], nil
}

func counterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		panic(err)
	}
	return m.GetCounter().GetValue()
}

func newTestPurger(t *testing.T, deleter outboxDeleter, schemas ...string) (*outboxPurger, *Metrics, *bytes.Buffer) {
	t.Helper()

	metrics := &Metrics{OutboxPurgedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "purged"}, []string{"schema"})}
	var logs bytes.Buffer
	purger, err := NewOutboxPurger(deleter, logger.NewWithWriter("info", "json", &logs), metrics, schemas, 7*24*time.Hour, time.Hour, 100)
	require.NoError(t, err)

	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	purger.now = func() time.Time { return now }

	return purger, metrics, &logs
}

func TestNewOutboxPurger_RejectsNonPositiveSettings(t *testing.T) {
	for name, args := range map[string][3]any{
		"retention": {time.Duration(0), time.Hour, 100},
		"interval":  {time.Hour, time.Duration(0), 100},
		"batch":     {time.Hour, time.Hour, 0},
	} {
		_, err := NewOutboxPurger(&fakeDeleter{}, logger.New("error", "json"), nil, []string{"a"}, args[0].(time.Duration), args[1].(time.Duration), args[2].(int))
		assert.Error(t, err, name)
	}
}

func TestOutboxPurger_DeletesInBatchesUntilOneComesBackShort(t *testing.T) {
	deleter := &fakeDeleter{script: map[string][]int64{"activity": {100, 100, 37}}}
	purger, metrics, logs := newTestPurger(t, deleter, "activity")

	purger.PurgeOnce(context.Background())

	require.Len(t, deleter.calls, 3, "full batch, full batch, short batch: stop")
	cutoff := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	for _, call := range deleter.calls {
		assert.Equal(t, "activity", call.schema)
		assert.Equal(t, 100, call.limit)
		assert.True(t, call.before.Equal(cutoff), "7 days before now, got %s", call.before)
	}
	assert.InDelta(t, 237, counterValue(metrics.OutboxPurgedTotal.WithLabelValues("activity")), 0)
	assert.Contains(t, logs.String(), "purged published outbox rows")
}

func TestOutboxPurger_NothingToDeleteIsQuiet(t *testing.T) {
	deleter := &fakeDeleter{}
	purger, metrics, logs := newTestPurger(t, deleter, "identity", "activity")

	purger.PurgeOnce(context.Background())

	assert.Len(t, deleter.calls, 2, "one call per schema")
	assert.Zero(t, counterValue(metrics.OutboxPurgedTotal.WithLabelValues("identity")))
	assert.NotContains(t, logs.String(), "purged")
}

func TestOutboxPurger_AFailingSchemaDoesNotStopTheOthers(t *testing.T) {
	deleter := &fakeDeleter{
		errs:   map[string]error{"identity": errors.New("boom")},
		script: map[string][]int64{"activity": {5}},
	}
	purger, metrics, logs := newTestPurger(t, deleter, "identity", "activity")

	purger.PurgeOnce(context.Background())

	assert.Contains(t, logs.String(), "failed to purge published outbox rows")
	assert.InDelta(t, 5, counterValue(metrics.OutboxPurgedTotal.WithLabelValues("activity")), 0)
	assert.Len(t, deleter.calls, 2, "identity once (no retry loop on error), then activity")
}

func TestOutboxPurger_StopsWhenTheContextEnds(t *testing.T) {
	deleter := &fakeDeleter{script: map[string][]int64{"activity": {100, 100, 100}}}
	purger, _, _ := newTestPurger(t, deleter, "activity")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	purger.PurgeOnce(ctx)

	assert.Empty(t, deleter.calls)
}
