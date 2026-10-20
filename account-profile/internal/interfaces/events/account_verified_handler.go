package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/account-profile/internal/application"
	"github.com/rasparac/rekreativko-api/account-profile/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/events"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
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

	erasedAccountChecker interface {
		IsErased(ctx context.Context, accountID uuid.UUID) (bool, error)
	}

	createProfileEventHandler struct {
		accountProfile  accountProfileCreator
		accountSettings accountSettingsCreator
		erasedAccounts  erasedAccountChecker
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
	erasedAccounts erasedAccountChecker,
	logger *logger.Logger,
	//metrics *metrics.Metrics,
) *createProfileEventHandler {
	return &createProfileEventHandler{
		accountProfile:  accountProfile,
		accountSettings: accountSettings,
		erasedAccounts:  erasedAccounts,
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
		return events.Permanent(fmt.Errorf("decode payload: %w", err))
	}

	ctx = api.WithEventID(ctx, event.EventID.String())

	log := avh.logger.WithValues(
		"event_id", event.EventID,
		"delivery_type", event.DeliveryType,
		"account_id", event.AccountID,
	)

	log.Debug(ctx, "handling account verified event")

	span.SetAttributes(
		attribute.String("event.id", event.EventID.String()),
		attribute.String("event.delivery_type", event.DeliveryType),
		attribute.String("event.account_id", event.AccountID.String()),
	)

	// Events are delivered at least once, so a redelivery finds the profile
	// already there - that's success, not a conflict. Settings are still
	// created (idempotently) so a half-created account gets repaired.
	var erased bool
	err = avh.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		// A late or redelivered event for an account already erased must not
		// re-create its profile.
		var err error
		erased, err = avh.erasedAccounts.IsErased(tCtx, event.AccountID)
		if err != nil {
			return fmt.Errorf("check erased: %w", err)
		}
		if erased {
			return nil
		}

		_, err = avh.accountProfile.CreateProfile(tCtx, application.CreateProfileParams{
			AccountID: event.AccountID,
		})
		switch {
		case errors.Is(err, domain.ErrAccountProfileExists):
			log.Debug(tCtx, "account profile already exists, redelivered event")
		case err != nil:
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

	if erased {
		log.Info(ctx, "account already erased, profile not created")
		span.SetStatus(codes.Ok, "account already erased")
		return nil
	}

	log.Debug(ctx, "account profile and settings created")

	span.SetStatus(codes.Ok, "account profile and settings created")

	return nil
}
