//go:build integration

package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rasparac/rekreativko-api/shared/logger"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
)

// The NATS users of nats/nats-server.conf (rekreativko-api-ywg): every
// connection logs in, each service can do what the broker needs, and only the
// outbox-publisher can publish domain events or manage streams.
func TestNatsAuth_ServerConfig(t *testing.T) {
	passwords := map[string]string{
		"outbox_publisher": "op-secret",
		"identity":         "id-secret",
		"account_profile":  "ap-secret",
		"notifications":    "no-secret",
		"activity":         "ac-secret",
	}
	url := testutil.StartNATSWithConfig(t, "../../nats/nats-server.conf", map[string]string{
		"NATS_OUTBOX_PUBLISHER_PASSWORD": passwords["outbox_publisher"],
		"NATS_IDENTITY_PASSWORD":         passwords["identity"],
		"NATS_ACCOUNT_PROFILE_PASSWORD":  passwords["account_profile"],
		"NATS_NOTIFICATIONS_PASSWORD":    passwords["notifications"],
		"NATS_ACTIVITY_PASSWORD":         passwords["activity"],
	})
	log := logger.New("error", "json")
	ctx := context.Background()

	connect := func(t *testing.T, user string, opts ...BrokerOption) *natsBroker {
		t.Helper()
		b, err := NewNatsBroker(url, user, log, append(opts, WithCredentials(user, passwords[user]))...)
		require.NoError(t, err)
		t.Cleanup(func() { _ = b.Close(context.Background()) })
		return b
	}

	t.Run("a connection without credentials, or with a wrong password, is refused", func(t *testing.T) {
		_, err := NewNatsBroker(url, "nobody", log, WithStreamManagement())
		assert.ErrorIs(t, err, nats.ErrAuthorization)

		_, err = NewNatsBroker(url, "activity", log, WithCredentials("activity", "wrong"))
		assert.ErrorIs(t, err, nats.ErrAuthorization)
	})

	outbox := connect(t, "outbox_publisher", WithStreamManagement())
	dlqCount := func(t *testing.T) uint64 {
		t.Helper()
		stream, err := outbox.js.Stream(ctx, dlqStream)
		require.NoError(t, err)
		info, err := stream.Info(ctx)
		require.NoError(t, err)
		return info.State.Msgs
	}

	t.Run("every consumer service can start and subscribe", func(t *testing.T) {
		for _, user := range []string{"identity", "account_profile", "notifications", "activity"} {
			b := connect(t, user)
			assert.NoError(t, b.Subscribe(ctx, "test.startup."+user, func(context.Context, []byte) error { return nil }), user)
		}
	})

	activity := connect(t, "activity")

	t.Run("a consumer receives what the outbox-publisher publishes", func(t *testing.T) {
		got := make(chan []byte, 1)
		require.NoError(t, activity.Subscribe(ctx, "identity.account.erased", func(_ context.Context, payload []byte) error {
			got <- payload
			return nil
		}))

		require.NoError(t, outbox.Publish(ctx, "identity.account.erased", "e-1", []byte(`{"event_id":"e-1"}`)))

		select {
		case payload := <-got:
			assert.JSONEq(t, `{"event_id":"e-1"}`, string(payload))
		case <-time.After(10 * time.Second):
			t.Fatal("the event never arrived")
		}
	})

	t.Run("a consumer can dead-letter", func(t *testing.T) {
		before := dlqCount(t)
		require.NoError(t, activity.Subscribe(ctx, "identity.account.bad", func(context.Context, []byte) error {
			return Permanent(errors.New("bad payload"))
		}))

		require.NoError(t, outbox.Publish(ctx, "identity.account.bad", "e-2", []byte(`{"event_id":"e-2"}`)))

		assert.Eventually(t, func() bool { return dlqCount(t) == before+1 }, 10*time.Second, 100*time.Millisecond)
	})

	t.Run("a consumer can dead-letter from the max deliveries advisory", func(t *testing.T) {
		before := dlqCount(t)
		stream, err := activity.js.Stream(ctx, eventsStream)
		require.NoError(t, err)
		consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
			Durable: "activity-crashing", FilterSubject: "events.test.crash",
			AckPolicy: jetstream.AckExplicitPolicy, AckWait: time.Second, MaxDeliver: 1,
		})
		require.NoError(t, err)
		require.NoError(t, activity.watchMaxDeliveries("activity-crashing", "test.crash"))

		require.NoError(t, outbox.Publish(ctx, "test.crash", "e-3", []byte(`{"event_id":"e-3"}`)))
		_, err = consumer.Fetch(1, jetstream.FetchMaxWait(10*time.Second)) // delivered, never acked
		require.NoError(t, err)

		assert.Eventually(t, func() bool { return dlqCount(t) == before+1 }, 15*time.Second, 200*time.Millisecond)
	})

	t.Run("activity's live hub gets its broadcast", func(t *testing.T) {
		got := make(chan string, 1)
		bctx, cancel := context.WithCancel(ctx)
		defer cancel()
		require.NoError(t, activity.SubscribeBroadcast(bctx, "activity.session.>", func(_ context.Context, topic string, _ []byte) {
			got <- topic
		}))

		require.NoError(t, outbox.Publish(ctx, "activity.session.cancelled", "e-4", []byte(`{}`)))

		select {
		case topic := <-got:
			assert.Equal(t, "activity.session.cancelled", topic)
		case <-time.After(10 * time.Second):
			t.Fatal("the broadcast never arrived")
		}
	})

	t.Run("a consumer service can't publish events or delete a stream", func(t *testing.T) {
		// a denied request gets no reply, so each attempt has its own deadline
		attempt := func(do func(ctx context.Context) error) error {
			short, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			return do(short)
		}
		before := func() uint64 {
			stream, err := outbox.js.Stream(ctx, eventsStream)
			require.NoError(t, err)
			info, err := stream.Info(ctx)
			require.NoError(t, err)
			return info.State.Msgs
		}()

		assert.Error(t, attempt(func(ctx context.Context) error {
			return activity.Publish(ctx, "identity.account.erased", "forged", []byte(`{}`))
		}), "forging another context's event")
		assert.Error(t, attempt(func(ctx context.Context) error {
			return activity.js.DeleteStream(ctx, eventsStream)
		}), "deleting the stream")

		stream, err := outbox.js.Stream(ctx, eventsStream)
		require.NoError(t, err, "the stream is still there")
		info, err := stream.Info(ctx)
		require.NoError(t, err)
		assert.Equal(t, before, info.State.Msgs, "and the forged event was not stored")
	})
}
