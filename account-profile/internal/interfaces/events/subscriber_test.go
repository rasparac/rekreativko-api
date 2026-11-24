package events

import (
	"context"
	"errors"
	"testing"

	"github.com/rasparac/rekreativko-api/shared/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBroker records the topics subscribed to and fails the ones in failOn.
type fakeBroker struct {
	subscribed []string
	failOn     map[string]error
}

func (b *fakeBroker) Publish(context.Context, string, string, []byte) error { return nil }
func (b *fakeBroker) Close(context.Context) error                          { return nil }

func (b *fakeBroker) Subscribe(_ context.Context, topic string, _ events.MessageHandler) error {
	if err := b.failOn[topic]; err != nil {
		return err
	}

	b.subscribed = append(b.subscribed, topic)

	return nil
}

func newTestSubscriber(broker events.MessageBroker) *Subscriber {
	return &Subscriber{
		broker:                    broker,
		createProfileEventHandler: &createProfileEventHandler{},
		accountErasedEventHandler: &accountErasedEventHandler{},
	}
}

func TestSubscribe_SubscribesToBothAccountEvents(t *testing.T) {
	broker := &fakeBroker{}

	require.NoError(t, newTestSubscriber(broker).Subscribe(context.Background()))

	assert.Equal(t, []string{"identity.account.verified", "identity.account.erased"}, broker.subscribed)
}

// main stops the service when Subscribe fails (the pod restarts), so it must
// return the broker's error instead of swallowing it, for either subscription.
func TestSubscribe_ReturnsAFailedSubscription(t *testing.T) {
	for _, topic := range []string{"identity.account.verified", "identity.account.erased"} {
		t.Run(topic, func(t *testing.T) {
			boom := errors.New("jetstream unavailable")
			broker := &fakeBroker{failOn: map[string]error{topic: boom}}

			err := newTestSubscriber(broker).Subscribe(context.Background())

			require.ErrorIs(t, err, boom)
			assert.ErrorContains(t, err, topic)
		})
	}
}
