package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	"github.com/rasparac/rekreativko-api/internal/shared/metrics"
	"github.com/rasparac/rekreativko-api/internal/shared/telemetry"
	"github.com/rasparac/rekreativko-api/internal/user_profile/application"
	"github.com/rasparac/rekreativko-api/internal/user_profile/domain"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type (
	userProfileCreator interface {
		CreateProfile(ctx context.Context, createProfile application.CreateProfileParams) (*domain.UserProfile, error)
	}

	createProfileEventHandler struct {
		userProfile userProfileCreator
		tracer      trace.Tracer
		logger      *logger.Logger
		metrics     *metrics.Metrics
	}
)

func NewAccountVerifiedEventHandler(
	userProfile userProfileCreator,
	tracer trace.Tracer,
	logger *logger.Logger,
	metrics *metrics.Metrics,
) *createProfileEventHandler {
	return &createProfileEventHandler{
		userProfile: userProfile,
		tracer:      telemetry.Tracer(""),
		logger:      logger,
		metrics:     metrics,
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

	span.SetAttributes(
		attribute.String("event.id", event.EventID.String()),
		attribute.String("event.delivery_type", event.DeliveryType),
		attribute.String("event.account_id", event.AccountID.String()),
	)

	_, err = avh.userProfile.CreateProfile(ctx, application.CreateProfileParams{
		AccountID: event.AccountID,
	})
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("create profile: %w", err)
	}

	span.SetStatus(codes.Ok, "account profile created")

	return nil
}
