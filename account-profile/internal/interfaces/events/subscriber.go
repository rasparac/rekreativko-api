package events

import (
	"context"
	"fmt"

	"github.com/rasparac/rekreativko-api/shared/events"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

type Subscriber struct {
	broker                    events.MessageBroker
	createProfileEventHandler *createProfileEventHandler
	accountErasedEventHandler *accountErasedEventHandler
}

// accountErasure erases an account's data and tells whether it was erased.
type accountErasure interface {
	accountEraser
	erasedAccountChecker
}

func NewSubscriber(
	broker events.MessageBroker,
	txManager *postgres.TransactionManager,
	accountProfileCreator accountProfileCreator,
	accountSettingsCreator accountSettingsCreator,
	accountErasure accountErasure,
	logger *logger.Logger,
	// metrics *metrics.Metrics,
) *Subscriber {
	return &Subscriber{
		broker: broker,
		createProfileEventHandler: NewCreateProfileEventHandler(
			txManager,
			accountProfileCreator,
			accountSettingsCreator,
			accountErasure,
			logger,
			//metrics,
		),
		accountErasedEventHandler: NewAccountErasedEventHandler(accountErasure, logger),
	}
}

func (s *Subscriber) Subscribe(ctx context.Context) error {
	err := s.broker.Subscribe(
		ctx,
		"identity.account.verified",
		s.createProfileEventHandler.Handle,
	)
	if err != nil {
		return fmt.Errorf("subscribe to identity.account.verified: %w", err)
	}

	err = s.broker.Subscribe(
		ctx,
		"identity.account.erased",
		s.accountErasedEventHandler.Handle,
	)
	if err != nil {
		return fmt.Errorf("subscribe to identity.account.erased: %w", err)
	}

	return nil
}
