package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// VerifyOutboxPurge checks a schema's published-row purge against its real
// event_outbox table: old published rows go, oldest first and a batch at a
// time, while recent, pending and dead-lettered rows stay. purge is the
// DeletePublishedEvents of the code under test.
func VerifyOutboxPurge(
	t *testing.T,
	db *TestDatabase,
	schema string,
	purge func(ctx context.Context, schema string, publishedBefore time.Time, limit int) (int64, error),
) {
	t.Helper()

	insert := func(publishedAgo, failedAgo, createdAgo time.Duration, retries int) uuid.UUID {
		id := uuid.New()
		db.RunSQL(fmt.Sprintf(`
			INSERT INTO %s.event_outbox (event_id, event_type, aggregate_id, payload, created_at, published_at, failed_at, retry_count)
			VALUES ($1, 'test.event', $2, '{}', NOW() - make_interval(secs => $3),
				CASE WHEN $4::float8 >= 0 THEN NOW() - make_interval(secs => $4) END,
				CASE WHEN $5::float8 >= 0 THEN NOW() - make_interval(secs => $5) END, $6)`, schema),
			id, uuid.New(), createdAgo.Seconds(), publishedAgo.Seconds(), failedAgo.Seconds(), retries,
		)
		return id
	}
	const never = -time.Second // a negative age means "NULL"

	day := 24 * time.Hour

	var expired []uuid.UUID
	for i := range 5 {
		// 8 to 12 days ago: past the 7-day window
		expired = append(expired, insert(time.Duration(8+i)*day, never, time.Duration(9+i)*day, 0))
	}
	recent := insert(1*day, never, 2*day, 0)
	justInside := insert(6*day, never, 7*day, 0)
	pending := insert(never, never, 10*day, 0)
	deadLettered := insert(never, 9*day, 10*day, 5)

	remaining := func() map[uuid.UUID]bool {
		rows, err := db.Pool.Query(context.Background(), fmt.Sprintf(`SELECT event_id FROM %s.event_outbox`, schema))
		require.NoError(t, err)
		defer rows.Close()

		ids := map[uuid.UUID]bool{}
		for rows.Next() {
			var id uuid.UUID
			require.NoError(t, rows.Scan(&id))
			ids[id] = true
		}
		require.NoError(t, rows.Err())
		return ids
	}

	ctx := context.Background()
	cutoff := time.Now().Add(-7 * day)

	// A batch at a time, oldest first.
	deleted, err := purge(ctx, schema, cutoff, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 2, deleted)
	left := remaining()
	assert.False(t, left[expired[4]], "the oldest goes first")
	assert.False(t, left[expired[3]])
	assert.True(t, left[expired[2]])

	deleted, err = purge(ctx, schema, cutoff, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 2, deleted)

	deleted, err = purge(ctx, schema, cutoff, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted, "a short batch: nothing more to delete")

	deleted, err = purge(ctx, schema, cutoff, 2)
	require.NoError(t, err)
	assert.Zero(t, deleted, "idempotent")

	left = remaining()
	assert.Len(t, left, 4)
	for _, id := range expired {
		assert.False(t, left[id], "expired published row %s", id)
	}
	assert.True(t, left[recent], "published inside the window")
	assert.True(t, left[justInside], "published 6 days ago")
	assert.True(t, left[pending], "pending rows are never purged, however old")
	assert.True(t, left[deadLettered], "dead-lettered rows are kept until handled")
}
