//go:build integration

package persistence_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/events"
	"github.com/rasparac/rekreativko-api/shared/logger"
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

// A committed outbox insert wakes the listener; nothing is sent before commit.
func TestOutboxListener_WakesOnCommittedInsert(t *testing.T) {
	db := setupCapacityRaceTestDB(t)
	defer db.Cleanup()

	connConfig, err := pgx.ParseConfig(db.DSN)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wakeups := events.ListenOutbox(ctx, connConfig, logger.New("error", "json"))

	next := func(within time.Duration) (string, bool) {
		select {
		case schema := <-wakeups:
			return schema, true
		case <-time.After(within):
			return "", false
		}
	}

	schema, ok := next(5 * time.Second)
	require.True(t, ok, "listener never connected")
	assert.Empty(t, schema, "connecting asks for a full pass: notifications sent while down are lost")

	tx, err := db.Pool.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `
		INSERT INTO activity.event_outbox (event_id, event_type, aggregate_id, payload)
		VALUES ($1, 'activity.test.event', $2, '{}')`, uuid.New(), uuid.New())
	require.NoError(t, err)

	_, early := next(300 * time.Millisecond)
	assert.False(t, early, "NOTIFY is delivered on commit, not on insert")

	require.NoError(t, tx.Commit(ctx))

	schema, ok = next(2 * time.Second)
	require.True(t, ok, "no wake-up after commit")
	assert.Equal(t, "activity", schema)
}

// listenerEnv starts a listener against a migrated database; next waits for
// its next wake-up.
type listenerEnv struct {
	db      *testutil.TestDatabase
	cancel  context.CancelFunc
	wakeups <-chan string
}

func startOutboxListener(t *testing.T) *listenerEnv {
	t.Helper()

	db := setupCapacityRaceTestDB(t)
	t.Cleanup(db.Cleanup)

	connConfig, err := pgx.ParseConfig(db.DSN)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	return &listenerEnv{
		db:      db,
		cancel:  cancel,
		wakeups: events.ListenOutbox(ctx, connConfig, logger.New("error", "json")),
	}
}

func (e *listenerEnv) next(t *testing.T, within time.Duration) string {
	t.Helper()

	select {
	case schema, ok := <-e.wakeups:
		require.True(t, ok, "listener stopped")
		return schema
	case <-time.After(within):
		t.Fatal("no wake-up")
		return ""
	}
}

func (e *listenerEnv) insertEvent(t *testing.T) {
	t.Helper()

	e.db.RunSQL(`
		INSERT INTO activity.event_outbox (event_id, event_type, aggregate_id, payload)
		VALUES ($1, 'activity.test.event', $2, '{}')`, uuid.New(), uuid.New())
}

// A dropped LISTEN connection is replaced, and the replacement asks for a full
// pass because anything sent in between was lost.
func TestOutboxListener_ReconnectsAfterTheConnectionDrops(t *testing.T) {
	env := startOutboxListener(t)
	require.Empty(t, env.next(t, 5*time.Second), "connect pass")

	env.db.RunSQL(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity
		WHERE pid <> pg_backend_pid() AND query ILIKE 'listen%'`)

	assert.Empty(t, env.next(t, 10*time.Second), "a reconnect asks for a full pass")

	env.insertEvent(t)
	assert.Equal(t, "activity", env.next(t, 2*time.Second), "the new connection listens")
}

func TestOutboxListener_StopsWhenTheContextEnds(t *testing.T) {
	env := startOutboxListener(t)
	require.Empty(t, env.next(t, 5*time.Second))

	env.cancel()

	deadline := time.After(3 * time.Second)
	for {
		select {
		case _, ok := <-env.wakeups:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("the channel was never closed")
		}
	}
}

// Wake-ups are dropped, never blocked on, when the publisher is behind: the
// listener must keep receiving, and the poll covers what was dropped.
func TestOutboxListener_DoesNotBlockOnASlowConsumer(t *testing.T) {
	env := startOutboxListener(t)
	require.Empty(t, env.next(t, 5*time.Second))

	// Far more commits than the channel holds, with nobody reading.
	for range 200 {
		env.insertEvent(t)
	}

	// Drain what fit, then a fresh commit must still come through.
	for drained := false; !drained; {
		select {
		case <-env.wakeups:
		case <-time.After(300 * time.Millisecond):
			drained = true
		}
	}

	env.insertEvent(t)
	assert.Equal(t, "activity", env.next(t, 2*time.Second))
}
