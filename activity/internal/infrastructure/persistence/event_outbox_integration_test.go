//go:build integration

package persistence_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertOutboxEvent(t *testing.T, db *testutil.TestDatabase, offsetSeconds int) uuid.UUID {
	t.Helper()

	id := uuid.New()
	db.RunSQL(`
		INSERT INTO activity.event_outbox (event_id, event_type, aggregate_id, payload, created_at)
		VALUES ($1, 'activity.test.event', $2, '{}', NOW() + make_interval(secs => $3))`,
		id, uuid.New(), offsetSeconds,
	)
	return id
}

func TestEventOutbox_ReadEventsSkipsLockedRows(t *testing.T) {
	db := setupCapacityRaceTestDB(t)
	defer db.Cleanup()

	first := insertOutboxEvent(t, db, 0)
	second := insertOutboxEvent(t, db, 1)

	txManager := db.CreateTransactionManager()
	mgr := domainevent.NewDomainEventManager(txManager)
	ctx := context.Background()

	// Publisher A locks the whole pending batch and holds its transaction open.
	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- txManager.WithTransaction(ctx, func(ctx context.Context) error {
			events, err := mgr.ReadEvents(ctx, "activity", 1)
			if err != nil {
				return err
			}
			if len(events) != 1 || events[0].EventID != first {
				return errors.New("publisher A did not get the oldest event")
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked

	// Publisher B must skip A's locked row rather than block or re-read it.
	err := txManager.WithTransaction(ctx, func(ctx context.Context) error {
		events, err := mgr.ReadEvents(ctx, "activity", 10)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, second, events[0].EventID)
		return nil
	})
	require.NoError(t, err)

	close(release)
	require.NoError(t, <-done)
}

func TestEventOutbox_DeadLettersAfterMaxRetries(t *testing.T) {
	db := setupCapacityRaceTestDB(t)
	defer db.Cleanup()

	poison := insertOutboxEvent(t, db, 0)
	healthy := insertOutboxEvent(t, db, 1)

	txManager := db.CreateTransactionManager()
	mgr := domainevent.NewDomainEventManager(txManager)
	ctx := context.Background()
	const maxRetries = 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		deadLettered, err := mgr.MarkEventAsFailed(ctx, "activity", poison, errors.New("broker rejected"), maxRetries)
		require.NoError(t, err)
		assert.Equal(t, attempt == maxRetries, deadLettered, "attempt %d", attempt)
	}

	var retryCount int
	var lastError string
	err := db.Pool.QueryRow(ctx,
		`SELECT retry_count, last_error FROM activity.event_outbox WHERE event_id = $1`, poison,
	).Scan(&retryCount, &lastError)
	require.NoError(t, err)
	assert.Equal(t, maxRetries, retryCount)
	assert.Equal(t, "broker rejected", lastError)

	// The dead-lettered event no longer occupies the head of the queue.
	events, err := mgr.ReadEvents(ctx, "activity", 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, healthy, events[0].EventID)
}
