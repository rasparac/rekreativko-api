//go:build integration

package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/live"
	"github.com/rasparac/rekreativko-api/shared/events"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamInstance is one activity process as far as live updates go: its own
// NATS connection, its own hub, subscribed to every session event.
type streamInstance struct {
	hub *live.Hub
}

func startStreamInstance(t *testing.T, env *sessionTeamTestEnv, natsURL, name string) *streamInstance {
	t.Helper()

	log := testutil.CreateLogger()

	broker, err := events.NewNatsBroker(natsURL, name, log)
	require.NoError(t, err)
	t.Cleanup(func() { _ = broker.Close(context.Background()) })

	// The same wiring as cmd/api/main.go.
	hub := live.NewHub(env.formation, env.sessionSvc, env.formation, log)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, broker.SubscribeBroadcast(ctx, live.TopicPattern, hub.HandleEvent))

	return &streamInstance{hub: hub}
}

// connect opens a stream for viewer on this instance and swallows the first
// snapshot.
func (i *streamInstance) connect(t *testing.T, sessionID, viewer uuid.UUID) *live.Client {
	t.Helper()

	client, err := i.hub.Connect(context.Background(), sessionID, viewer)
	require.NoError(t, err)
	t.Cleanup(func() { i.hub.Disconnect(client) })

	first := nextUpdate(t, client, "snapshot")
	require.Nil(t, first.Change)

	return client
}

// nextUpdate waits for the client's next update named event, skipping any
// others (a session also emits created/updated events and the like).
func nextUpdate(t *testing.T, c *live.Client, event string) live.Update {
	t.Helper()

	deadline := time.After(10 * time.Second)
	for {
		select {
		case u := <-c.Updates():
			if u.Event == event {
				return u
			}
		case <-c.Done():
			t.Fatalf("stream closed (%q) while waiting for %q", c.Reason(), event)
		case <-deadline:
			t.Fatalf("no %q update arrived", event)
		}
	}
}

// publishOutbox does what the outbox publisher does: every unpublished outbox
// row goes to NATS as its event type, then is marked published.
func publishOutbox(t *testing.T, env *sessionTeamTestEnv, publisher events.MessageBroker) {
	t.Helper()

	type row struct {
		id      uuid.UUID
		topic   string
		payload []byte
	}
	var pending []row

	rows, err := env.txManager.Querier(env.ctx).Query(env.ctx,
		`SELECT event_id, event_type, payload FROM activity.event_outbox WHERE published_at IS NULL ORDER BY created_at`)
	require.NoError(t, err)
	for rows.Next() {
		var r row
		require.NoError(t, rows.Scan(&r.id, &r.topic, &r.payload))
		pending = append(pending, r)
	}
	rows.Close()
	require.NoError(t, rows.Err())

	for _, r := range pending {
		require.NoError(t, publisher.Publish(env.ctx, r.topic, r.payload))
		_, err := env.txManager.Querier(env.ctx).Exec(env.ctx,
			`UPDATE activity.event_outbox SET published_at = NOW() WHERE event_id = $1`, r.id)
		require.NoError(t, err)
	}
}

// A change made on one instance reaches clients connected to any instance: the
// events go out over NATS to every instance, and each builds the snapshot for
// its own clients from the shared database. Run with -race.
func TestStream_ChangesReachClientsOnEveryInstance(t *testing.T) {
	env := setupSessionTeamTest(t)
	natsURL := testutil.StartNATS(t)
	session, u := env.draftSession(t, 4)

	instanceA := startStreamInstance(t, env, natsURL, "activity-a")
	instanceB := startStreamInstance(t, env, natsURL, "activity-b")

	publisher, err := events.NewNatsBroker(natsURL, "outbox-publisher", testutil.CreateLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = publisher.Close(context.Background()) })

	// Two viewers, one on each instance.
	onA := instanceA.connect(t, session.ID(), u[2])
	onB := instanceB.connect(t, session.ID(), u[3])

	versions := map[string]int64{} // the newest snapshot each instance's client has seen

	t.Run("a change reaches the clients of both instances", func(t *testing.T) {
		_, err := env.startDraft(session, u[0], u[1], nil)
		require.NoError(t, err)
		publishOutbox(t, env, publisher)

		for name, client := range map[string]*live.Client{"A": onA, "B": onB} {
			update := nextUpdate(t, client, "draft.started")
			require.NotNil(t, update.Snapshot.Draft, "instance %s: the snapshot already has the draft", name)
			assert.True(t, update.Snapshot.Draft.Draft.IsRunning(), "instance %s", name)
			versions[name] = update.Snapshot.Version
		}
	})

	t.Run("the next change too, with a newer snapshot on each", func(t *testing.T) {
		_, err := env.pick(session, u[0], u[2])
		require.NoError(t, err)
		publishOutbox(t, env, publisher)

		for name, client := range map[string]*live.Client{"A": onA, "B": onB} {
			update := nextUpdate(t, client, "draft.player_picked")
			assert.Greater(t, update.Snapshot.Version, versions[name], "instance %s: snapshots only move forward", name)
			assert.Equal(t, []uuid.UUID{u[0], u[2]}, update.Snapshot.Draft.Draft.Roster(domain.DraftSideA), "instance %s", name)
		}
	})

	t.Run("one instance shutting down leaves the other's clients alone", func(t *testing.T) {
		instanceB.hub.Shutdown()

		select {
		case <-onB.Done():
			assert.Equal(t, live.ReasonShutdown, onB.Reason(), "told to reconnect elsewhere")
		case <-time.After(5 * time.Second):
			t.Fatal("the client on the stopped instance was not disconnected")
		}

		_, err := env.pick(session, u[1], u[3])
		require.NoError(t, err)
		publishOutbox(t, env, publisher)

		update := nextUpdate(t, onA, "draft.player_picked")
		assert.Equal(t, []uuid.UUID{u[1], u[3]}, update.Snapshot.Draft.Draft.Roster(domain.DraftSideB))
	})

	t.Run("a client that reconnects to the other instance resyncs from a fresh snapshot", func(t *testing.T) {
		client, err := instanceA.hub.Connect(context.Background(), session.ID(), u[3])
		require.NoError(t, err)
		t.Cleanup(func() { instanceA.hub.Disconnect(client) })

		// Usually a "snapshot" event. An event still in flight from the last
		// publish can beat it - the hub never sends a client an older snapshot
		// than one it already has - so what matters is that the first update,
		// whatever its name, already holds everything that happened.
		var first live.Update
		select {
		case first = <-client.Updates():
		case <-time.After(10 * time.Second):
			t.Fatal("the reconnected client got no update")
		}
		require.NotNil(t, first.Snapshot.Draft)
		assert.True(t, first.Snapshot.Draft.Draft.IsCompleted(), "everything that happened is in it, no replay needed")
	})
}
