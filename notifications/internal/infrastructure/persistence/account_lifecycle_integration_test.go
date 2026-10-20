//go:build integration

package persistence_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/notifications/internal/application"
	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
	"github.com/rasparac/rekreativko-api/notifications/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/notifications/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// metrics.New() registers its collectors globally via promauto, so it may
// only run once per test binary.
var testMetrics = metrics.New()

type accountLifecycleTestEnv struct {
	ctx           context.Context
	db            *testutil.TestDatabase
	accounts      *application.AccountLifecycleService
	notifications *application.NotificationService
}

func setupAccountLifecycleTest(t *testing.T) *accountLifecycleTestEnv {
	t.Helper()

	db := testutil.NewTestDatabase(t)
	db.RunMigrationsForService()

	txManager := db.CreateTransactionManager()
	logger := testutil.CreateLogger()

	accounts := application.NewAccountLifecycleService(
		persistence.NewAccountStateManager(txManager),
		txManager,
		domainevent.NewDomainEventManager(txManager, logger),
		logger,
		testMetrics,
	)

	return &accountLifecycleTestEnv{
		ctx:      context.Background(),
		db:       db,
		accounts: accounts,
		notifications: application.NewNotificationService(
			logger, txManager, persistence.NewNotificationRepository(txManager, logger), accounts, testMetrics,
		),
	}
}

// notify creates a notification for recipient the way the feed handlers do;
// it reports whether one was created.
func (e *accountLifecycleTestEnv) notify(t *testing.T, recipient uuid.UUID, data map[string]any) bool {
	t.Helper()

	n, err := e.notifications.CreateNotification(e.ctx, application.CreateNotificationParams{
		EventID:            uuid.New(),
		RecipientAccountID: recipient,
		Type:               string(domain.NotificationTypeInviteSent),
		Data:               data,
	})
	require.NoError(t, err)
	return n != nil
}

func (e *accountLifecycleTestEnv) count(t *testing.T, query string, accountID uuid.UUID) int {
	t.Helper()

	var n int
	require.NoError(t, e.db.Pool.QueryRow(e.ctx, query, accountID).Scan(&n))
	return n
}

func (e *accountLifecycleTestEnv) feed(t *testing.T, accountID uuid.UUID) int {
	return e.count(t, `SELECT count(*) FROM notifications.notification WHERE recipient_account_id = $1`, accountID)
}

func (e *accountLifecycleTestEnv) erasedEvents(t *testing.T, accountID uuid.UUID) []json.RawMessage {
	t.Helper()

	rows, err := e.db.Pool.Query(e.ctx, `SELECT payload FROM notifications.event_outbox
		WHERE event_type = $1 AND aggregate_id = $2`, domain.EventAccountErased, accountID)
	require.NoError(t, err)
	defer rows.Close()

	var payloads []json.RawMessage
	for rows.Next() {
		var p json.RawMessage
		require.NoError(t, rows.Scan(&p))
		payloads = append(payloads, p)
	}
	require.NoError(t, rows.Err())
	return payloads
}

func TestAccountLifecycle_DeletedAccountGetsNoNewNotificationsButKeepsItsFeed(t *testing.T) {
	env := setupAccountLifecycleTest(t)
	deleted, other := uuid.New(), uuid.New()

	require.True(t, env.notify(t, deleted, nil))

	require.NoError(t, env.accounts.DeactivateAccount(env.ctx, deleted))
	require.NoError(t, env.accounts.DeactivateAccount(env.ctx, deleted), "a redelivered deletion is a no-op")

	assert.False(t, env.notify(t, deleted, nil), "nothing new for a deleted account")
	assert.Equal(t, 1, env.feed(t, deleted), "the feed stays until the erasure")
	assert.True(t, env.notify(t, other, nil))

	inactive, err := env.accounts.IsInactive(env.ctx, deleted)
	require.NoError(t, err)
	assert.True(t, inactive)
}

func TestAccountLifecycle_ErasureDeletesTheFeedAndConfirmsOnce(t *testing.T) {
	env := setupAccountLifecycleTest(t)
	erased, other := uuid.New(), uuid.New()

	require.True(t, env.notify(t, erased, nil))
	require.True(t, env.notify(t, erased, nil))
	// another account's notification naming the erased one as the actor
	require.True(t, env.notify(t, other, map[string]any{"user_id": erased}))

	require.NoError(t, env.accounts.DeactivateAccount(env.ctx, erased))
	require.NoError(t, env.accounts.EraseAccount(env.ctx, erased))

	assert.Zero(t, env.feed(t, erased), "the erased account's feed is gone")
	assert.Equal(t, 1, env.feed(t, other), "a notification naming it only by id stays")
	assert.Equal(t, 1, env.count(t, `SELECT count(*) FROM notifications.erased_account WHERE account_id = $1`, erased))

	payloads := env.erasedEvents(t, erased)
	require.Len(t, payloads, 1, "notifications.account.erased in the outbox")
	var payload map[string]any
	require.NoError(t, json.Unmarshal(payloads[0], &payload))
	assert.Equal(t, erased.String(), payload["account_id"])

	// At-least-once delivery: the same event again is a no-op, not a second
	// confirmation.
	require.NoError(t, env.accounts.EraseAccount(env.ctx, erased))
	assert.Len(t, env.erasedEvents(t, erased), 1)

	// A late activity event does not bring the feed back.
	assert.False(t, env.notify(t, erased, nil))
	assert.Zero(t, env.feed(t, erased))
}

func TestAccountLifecycle_ErasureWithoutAPriorDeletionStillStopsNotifications(t *testing.T) {
	env := setupAccountLifecycleTest(t)
	erased := uuid.New()

	// identity.account.deleted lost or not yet processed
	require.NoError(t, env.accounts.EraseAccount(env.ctx, erased))

	assert.False(t, env.notify(t, erased, nil))
	assert.Len(t, env.erasedEvents(t, erased), 1)
}

// Published outbox rows older than the retention window are purged; pending
// and dead-lettered ones are not.
func TestOutboxPurge_Notifications(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	db.RunMigrationsForService()

	events := domainevent.NewDomainEventManager(db.CreateTransactionManager(), testutil.CreateLogger())

	testutil.VerifyOutboxPurge(t, db, "notifications", events.DeletePublishedEvents)
}

// Dead-lettered outbox rows older than their retention window are purged;
// pending and published ones are not.
func TestDeadLetterPurge_Notifications(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	db.RunMigrationsForService()

	events := domainevent.NewDomainEventManager(db.CreateTransactionManager(), testutil.CreateLogger())

	testutil.VerifyDeadLetterPurge(t, db, "notifications", events.DeleteDeadLetteredEvents)
}
