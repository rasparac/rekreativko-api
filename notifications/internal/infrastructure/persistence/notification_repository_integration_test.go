//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *testutil.TestDatabase {
	t.Helper()

	db := testutil.NewTestDatabase(t)
	db.RunMigrationsForService()

	return db
}

func newTestNotification(eventID, recipientID uuid.UUID) *domain.Notification {
	return domain.New(eventID, recipientID, domain.NotificationTypeInviteSent, map[string]any{"group_id": uuid.NewString()})
}

func countForRecipient(t *testing.T, db *testutil.TestDatabase, recipientID uuid.UUID) int {
	t.Helper()

	var count int
	err := db.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM notifications.notification WHERE recipient_account_id = $1`, recipientID,
	).Scan(&count)
	require.NoError(t, err)
	return count
}

func TestNotificationRepository_Create_IsIdempotentPerEventAndRecipient(t *testing.T) {
	db := setupTestDB(t)
	repo := NewNotificationRepository(db.CreateTransactionManager(), testutil.CreateLogger())
	ctx := context.Background()

	eventID, recipientID := uuid.New(), uuid.New()

	created, err := repo.Create(ctx, newTestNotification(eventID, recipientID))
	require.NoError(t, err)
	assert.True(t, created)

	// Redelivery of the same event for the same recipient is a no-op.
	created, err = repo.Create(ctx, newTestNotification(eventID, recipientID))
	require.NoError(t, err)
	assert.False(t, created)

	assert.Equal(t, 1, countForRecipient(t, db, recipientID))
}

func TestNotificationRepository_Create_SameEventDifferentRecipients(t *testing.T) {
	db := setupTestDB(t)
	repo := NewNotificationRepository(db.CreateTransactionManager(), testutil.CreateLogger())
	ctx := context.Background()

	eventID := uuid.New()
	managerA, managerB := uuid.New(), uuid.New()

	// Fan-out: one event notifies both managers.
	for _, id := range []uuid.UUID{managerA, managerB} {
		created, err := repo.Create(ctx, newTestNotification(eventID, id))
		require.NoError(t, err)
		assert.True(t, created)
	}

	// Redelivery after a partial failure: managerA is skipped, no duplicate rows.
	for _, id := range []uuid.UUID{managerA, managerB} {
		created, err := repo.Create(ctx, newTestNotification(eventID, id))
		require.NoError(t, err)
		assert.False(t, created)
	}

	assert.Equal(t, 1, countForRecipient(t, db, managerA))
	assert.Equal(t, 1, countForRecipient(t, db, managerB))
}

func TestNotificationRepository_Create_DifferentEventsSameRecipient(t *testing.T) {
	db := setupTestDB(t)
	repo := NewNotificationRepository(db.CreateTransactionManager(), testutil.CreateLogger())
	ctx := context.Background()

	recipientID := uuid.New()

	for range 2 {
		created, err := repo.Create(ctx, newTestNotification(uuid.New(), recipientID))
		require.NoError(t, err)
		assert.True(t, created)
	}

	assert.Equal(t, 2, countForRecipient(t, db, recipientID))
}

func TestNotificationRepository_GetByID_RoundTripsEventID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewNotificationRepository(db.CreateTransactionManager(), testutil.CreateLogger())
	ctx := context.Background()

	n := newTestNotification(uuid.New(), uuid.New())
	_, err := repo.Create(ctx, n)
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, n.ID())
	require.NoError(t, err)
	assert.Equal(t, n.EventID(), got.EventID())
}
