package events

import (
	"context"

	"github.com/rasparac/rekreativko-api/internal/shared/events"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	"github.com/rasparac/rekreativko-api/internal/shared/store/postgres"
)

type Subscriber struct {
	broker                    events.MessageBroker
	createProfileEventHandler *createProfileEventHandler
}

func NewSubscriber(
	broker events.MessageBroker,
	txManager *postgres.TransactionManager,
	accountProfileCreator accountProfileCreator,
	accountSettingsCreator accountSettingsCreator,
	logger *logger.Logger,
	// metrics *metrics.Metrics,
) *Subscriber {
	return &Subscriber{
		broker: broker,
		createProfileEventHandler: NewCreateProfileEventHandler(
			txManager,
			accountProfileCreator,
			accountSettingsCreator,
			logger,
			//metrics,
		),
	}
}

func (s *Subscriber) Subscribe(ctx context.Context) error {
	err := s.broker.Subscribe(
		ctx,
		"identity.account.verified",
		s.createProfileEventHandler.Handle,
	)

	return err
}
