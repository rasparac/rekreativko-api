//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

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

// createAged inserts a notification created age ago, read or not.
func createAged(t *testing.T, db *testutil.TestDatabase, repo NotificationRepository, recipientID uuid.UUID, age time.Duration, read bool) *domain.Notification {
	t.Helper()
	ctx := context.Background()

	n := newTestNotification(uuid.New(), recipientID)
	_, err := repo.Create(ctx, n)
	require.NoError(t, err)

	createdAt := time.Now().Add(-age)
	var readAt *time.Time
	if read {
		readAt = &createdAt
	}
	_, err = db.Pool.Exec(ctx,
		`UPDATE notifications.notification SET created_at = $2, read_at = $3 WHERE id = $1`, n.ID(), createdAt, readAt,
	)
	require.NoError(t, err)

	return n
}

func TestNotificationRepository_ListByRecipient(t *testing.T) {
	db := setupTestDB(t)
	repo := NewNotificationRepository(db.CreateTransactionManager(), testutil.CreateLogger())
	ctx := context.Background()

	recipientID := uuid.New()
	oldest := createAged(t, db, repo, recipientID, 3*time.Hour, true)
	middle := createAged(t, db, repo, recipientID, 2*time.Hour, false)
	newest := createAged(t, db, repo, recipientID, time.Hour, false)
	createAged(t, db, repo, uuid.New(), time.Minute, false) // someone else's

	ids := func(ns []*domain.Notification) []uuid.UUID {
		out := make([]uuid.UUID, len(ns))
		for i, n := range ns {
			out[i] = n.ID()
		}
		return out
	}

	t.Run("newest first, only the recipient's", func(t *testing.T) {
		got, err := repo.ListByRecipient(ctx, NotificationFilter{RecipientAccountID: recipientID})
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{newest.ID(), middle.ID(), oldest.ID()}, ids(got))
	})

	t.Run("limit keeps the newest", func(t *testing.T) {
		got, err := repo.ListByRecipient(ctx, NotificationFilter{RecipientAccountID: recipientID, Limit: 2})
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{newest.ID(), middle.ID()}, ids(got))
	})

	t.Run("unread only", func(t *testing.T) {
		got, err := repo.ListByRecipient(ctx, NotificationFilter{RecipientAccountID: recipientID, UnreadOnly: true})
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{newest.ID(), middle.ID()}, ids(got))
	})
}

func TestNotificationRepository_DeleteExpired(t *testing.T) {
	db := setupTestDB(t)
	repo := NewNotificationRepository(db.CreateTransactionManager(), testutil.CreateLogger())
	ctx := context.Background()

	const day = 24 * time.Hour
	recipientID := uuid.New()

	kept := []*domain.Notification{
		createAged(t, db, repo, recipientID, 10*day, true),  // read, but recent
		createAged(t, db, repo, recipientID, 60*day, false), // old, but unread
	}
	gone := []*domain.Notification{
		createAged(t, db, repo, recipientID, 40*day, true),   // read and past the read window
		createAged(t, db, repo, recipientID, 100*day, false), // past the window for everything
		createAged(t, db, repo, recipientID, 100*day, true),
	}

	now := time.Now()
	deleted, err := repo.DeleteExpired(ctx, now.Add(-30*day), now.Add(-90*day))
	require.NoError(t, err)
	assert.EqualValues(t, len(gone), deleted)

	for _, n := range kept {
		_, err := repo.GetByID(ctx, n.ID())
		assert.NoError(t, err)
	}
	for _, n := range gone {
		_, err := repo.GetByID(ctx, n.ID())
		assert.ErrorIs(t, err, domain.ErrNotificationNotFound)
	}

	// Running it again deletes nothing more.
	deleted, err = repo.DeleteExpired(ctx, now.Add(-30*day), now.Add(-90*day))
	require.NoError(t, err)
	assert.Zero(t, deleted)
}
