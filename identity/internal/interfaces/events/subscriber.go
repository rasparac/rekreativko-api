package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/events"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

type (
	contextErasureRecorder interface {
		RecordContextErasure(ctx context.Context, accountID uuid.UUID, context string) error
	}

	// AccountErasedEvent is the confirmation a context raises once it has
	// erased its data of an account: <context>.account.erased.
	AccountErasedEvent struct {
		EventID   uuid.UUID `json:"event_id"`
		AccountID uuid.UUID `json:"account_id"`
	}

	// Subscriber records the erasure confirmations of the configured contexts.
	Subscriber struct {
		broker   events.MessageBroker
		recorder contextErasureRecorder
		contexts []string
		logger   *logger.Logger
	}
)

func NewSubscriber(
	broker events.MessageBroker,
	recorder contextErasureRecorder,
	contexts []string,
	logger *logger.Logger,
) *Subscriber {
	return &Subscriber{
		broker:   broker,
		recorder: recorder,
		contexts: contexts,
		logger:   logger.WithName("identity.erasure.subscriber"),
	}
}

// AccountErasedTopic is the event a context raises once it has erased its
// data of an account.
func AccountErasedTopic(context string) string {
	return context + ".account.erased"
}

func (s *Subscriber) Subscribe(ctx context.Context) error {
	for _, erasureContext := range s.contexts {
		err := s.broker.Subscribe(ctx, AccountErasedTopic(erasureContext), s.handler(erasureContext))
		if err != nil {
			return fmt.Errorf("subscribe to %s: %w", AccountErasedTopic(erasureContext), err)
		}
	}

	return nil
}

func (s *Subscriber) handler(erasureContext string) events.MessageHandler {
	return func(ctx context.Context, payload []byte) error {
		var event AccountErasedEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return events.Permanent(fmt.Errorf("decode payload: %w", err))
		}

		ctx = api.WithEventID(ctx, event.EventID.String())

		if event.AccountID == uuid.Nil {
			return events.Permanent(errors.New("account_id missing"))
		}

		err := s.recorder.RecordContextErasure(ctx, event.AccountID, erasureContext)
		if pgErr := postgres.GetPgxError(err); pgErr != nil && pgErr.Code == pgerrcode.ForeignKeyViolation {
			// No such account: redelivering will not create it.
			return events.Permanent(fmt.Errorf("unknown account %s: %w", event.AccountID, err))
		}
		if err != nil {
			return fmt.Errorf("record %s erasure: %w", erasureContext, err)
		}

		s.logger.Debug(ctx, "context erasure recorded", "context", erasureContext, "account_id", event.AccountID)

		return nil
	}
}
