//go:build integration

package persistence_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/account-profile/internal/application"
	"github.com/rasparac/rekreativko-api/account-profile/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/account-profile/internal/interfaces/events"
	"github.com/rasparac/rekreativko-api/account-profile/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// metrics.New() registers its collectors globally via promauto, so it may
// only run once per test binary.
var (
	sharedTestMetricsOnce sync.Once
	sharedTestMetrics     *metrics.Metrics
)

func testMetrics() *metrics.Metrics {
	sharedTestMetricsOnce.Do(func() {
		sharedTestMetrics = metrics.New()
	})
	return sharedTestMetrics
}

type accountVerifiedTestEnv struct {
	ctx     context.Context
	db      *testutil.TestDatabase
	handler interface {
		Handle(ctx context.Context, payload []byte) error
	}
}

func setupAccountVerifiedTest(t *testing.T) *accountVerifiedTestEnv {
	t.Helper()

	db := testutil.NewTestDatabase(t)
	db.RunMigrationsForService()

	txManager := db.CreateTransactionManager()
	logger := testutil.CreateLogger()
	eventWriter := domainevent.NewDomainEventManager(txManager)

	profileSvc := application.NewService(logger, txManager, persistence.NewAccountProfileManager(txManager, logger), eventWriter, testMetrics())
	settingsSvc := application.NewAccountSettingsService(persistence.NewAccountProfileSettingsManager(txManager), txManager, eventWriter, logger, testMetrics())

	return &accountVerifiedTestEnv{
		ctx:     context.Background(),
		db:      db,
		handler: events.NewCreateProfileEventHandler(txManager, profileSvc, settingsSvc, logger),
	}
}

func (e *accountVerifiedTestEnv) payload(t *testing.T, accountID uuid.UUID) []byte {
	t.Helper()

	payload, err := json.Marshal(events.AccountVerifiedEvent{
		EventID:      uuid.New(),
		AccountID:    accountID,
		Email:        "someone@example.com",
		DeliveryType: "email",
	})
	require.NoError(t, err)

	return payload
}

func (e *accountVerifiedTestEnv) count(t *testing.T, query string, accountID uuid.UUID) int {
	t.Helper()

	var n int
	require.NoError(t, e.db.Pool.QueryRow(e.ctx, query, accountID).Scan(&n))

	return n
}

func (e *accountVerifiedTestEnv) profiles(t *testing.T, accountID uuid.UUID) int {
	return e.count(t, `SELECT count(*) FROM account_profile.profiles WHERE account_id = $1`, accountID)
}

func (e *accountVerifiedTestEnv) settings(t *testing.T, accountID uuid.UUID) int {
	return e.count(t, `SELECT count(*) FROM account_profile.settings WHERE account_id = $1`, accountID)
}

func (e *accountVerifiedTestEnv) settingsMeta(t *testing.T, accountID uuid.UUID) int {
	return e.count(t, `SELECT count(*) FROM account_profile.account_settings_meta WHERE account_id = $1`, accountID)
}

func (e *accountVerifiedTestEnv) outboxEvents(t *testing.T, accountID uuid.UUID) int {
	return e.count(t, `SELECT count(*) FROM account_profile.event_outbox WHERE aggregate_id = $1`, accountID)
}

func TestAccountVerifiedHandler_RedeliveryIsIdempotent(t *testing.T) {
	env := setupAccountVerifiedTest(t)
	accountID := uuid.New()
	payload := env.payload(t, accountID)

	require.NoError(t, env.handler.Handle(env.ctx, payload))

	settingsCount := env.settings(t, accountID)
	require.Positive(t, settingsCount, "default settings are created")
	outboxCount := env.outboxEvents(t, accountID)
	require.Equal(t, 1, outboxCount, "one profile-created event")

	// At-least-once delivery: the same event again must succeed, not conflict.
	require.NoError(t, env.handler.Handle(env.ctx, payload))

	assert.Equal(t, 1, env.profiles(t, accountID))
	assert.Equal(t, settingsCount, env.settings(t, accountID))
	assert.Equal(t, 1, env.settingsMeta(t, accountID))
	assert.Equal(t, outboxCount, env.outboxEvents(t, accountID), "no second profile-created event")
}

func TestAccountVerifiedHandler_RedeliveryRepairsMissingSettings(t *testing.T) {
	env := setupAccountVerifiedTest(t)
	accountID := uuid.New()
	payload := env.payload(t, accountID)

	require.NoError(t, env.handler.Handle(env.ctx, payload))
	settingsCount := env.settings(t, accountID)

	// A profile without its settings: one setting row and the metadata gone.
	env.db.RunSQL(`DELETE FROM account_profile.settings WHERE account_id = $1 AND key = (
		SELECT min(key) FROM account_profile.settings WHERE account_id = $1)`, accountID)
	env.db.RunSQL(`DELETE FROM account_profile.account_settings_meta WHERE account_id = $1`, accountID)
	require.Equal(t, settingsCount-1, env.settings(t, accountID))

	require.NoError(t, env.handler.Handle(env.ctx, payload))

	assert.Equal(t, 1, env.profiles(t, accountID))
	assert.Equal(t, settingsCount, env.settings(t, accountID), "the missing setting is recreated")
	assert.Equal(t, 1, env.settingsMeta(t, accountID))
}
