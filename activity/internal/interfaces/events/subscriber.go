package events

import (
	"context"
	"fmt"

	"github.com/rasparac/rekreativko-api/shared/events"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

// Subscriber wires the events this service consumes from other contexts to
// their handlers. Each Subscribe call creates a durable JetStream consumer
// (named after this service + the subject) shared by every instance, so each
// event is handled once.
type Subscriber struct {
	broker   events.MessageBroker
	accounts *AccountLifecycleHandlers
}

func NewSubscriber(broker events.MessageBroker, accounts accountLifecycle, log *logger.Logger) *Subscriber {
	return &Subscriber{
		broker:   broker,
		accounts: NewAccountLifecycleHandlers(accounts, log),
	}
}

func (s *Subscriber) Subscribe(ctx context.Context) error {
	subscriptions := []struct {
		topic   string
		handler events.MessageHandler
	}{
		{"identity.account.deleted", s.accounts.HandleAccountDeleted},
		{"identity.account.erased", s.accounts.HandleAccountErased},
	}

	for _, sub := range subscriptions {
		if err := s.broker.Subscribe(ctx, sub.topic, sub.handler); err != nil {
			return fmt.Errorf("subscribe to %s: %w", sub.topic, err)
		}
	}

	return nil
}
