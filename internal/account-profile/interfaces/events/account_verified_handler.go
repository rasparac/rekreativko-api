package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rasparac/rekreativko-api/internal/account-profile/application"
	"github.com/rasparac/rekreativko-api/internal/account-profile/domain"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	"github.com/rasparac/rekreativko-api/internal/shared/store/postgres"
	"github.com/rasparac/rekreativko-api/internal/shared/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type (
	accountProfileCreator interface {
		CreateProfile(ctx context.Context, createProfile application.CreateProfileParams) (*domain.AccountProfile, error)
	}

	accountSettingsCreator interface {
		CreateSettings(ctx context.Context, settings application.CreateAccountSettingsParams) error
	}

	createProfileEventHandler struct {
		accountProfile  accountProfileCreator
		accountSettings accountSettingsCreator
		txManager       *postgres.TransactionManager
		tracer          trace.Tracer
		logger          *logger.Logger
		//metrics         *metrics.Metrics
	}
)

func NewCreateProfileEventHandler(
	txManager *postgres.TransactionManager,
	accountProfile accountProfileCreator,
	accountSettings accountSettingsCreator,
	logger *logger.Logger,
	//metrics *metrics.Metrics,
) *createProfileEventHandler {
	return &createProfileEventHandler{
		accountProfile:  accountProfile,
		accountSettings: accountSettings,
		txManager:       txManager,
		tracer:          telemetry.Tracer("createProfileEventHandler"),
		logger:          logger,
		// metrics:         metrics,
	}
}

func (avh *createProfileEventHandler) Handle(ctx context.Context, payload []byte) error {
	ctx, span := avh.tracer.Start(ctx, "createProfileEventHandler.Handle")
	defer span.End()

	var event AccountVerifiedEvent
	err := json.Unmarshal(payload, &event)
	if err != nil {
		avh.logger.Error(ctx, "decode payload", "error", err)
		span.RecordError(err)
		return fmt.Errorf("decode payload: %w", err)
	}

	log := avh.logger.WithValues(
		"event_id", event.EventID,
		"delivery_type", event.DeliveryType,
		"account_id", event.AccountID,
	)

	log.Info(ctx, "handling account verified event")

	span.SetAttributes(
		attribute.String("event.id", event.EventID.String()),
		attribute.String("event.delivery_type", event.DeliveryType),
		attribute.String("event.account_id", event.AccountID.String()),
	)

	err = avh.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		_, err := avh.accountProfile.CreateProfile(tCtx, application.CreateProfileParams{
			AccountID: event.AccountID,
		})
		if err != nil {
			return fmt.Errorf("create profile: %w", err)
		}

		err = avh.accountSettings.CreateSettings(tCtx, application.CreateAccountSettingsParams{
			AccountID: event.AccountID,
		})
		if err != nil {
			return fmt.Errorf("create settings: %w", err)
		}

		return nil
	})
	if err != nil {
		span.RecordError(err)
		return err
	}

	log.Info(ctx, "account profile and settings created")

	span.SetStatus(codes.Ok, "account profile and settings created")

	return nil
}
