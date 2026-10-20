package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/notifications/internal/application"
	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// The delivery types identity puts in an event's code_type: the channel the
// account registered with (identity's CodeType).
const (
	DeliveryTypeEmail = "email"
	DeliveryTypePhone = "phone"
)

type (
	verificationCodeGeneratedEvent struct {
		EventID      uuid.UUID `json:"event_id"`
		AccountID    uuid.UUID `json:"account_id"`
		DeliveryType string    `json:"code_type"`
		Code         string    `json:"code"`
		Email        string    `json:"email"`
		Phone        string    `json:"phone"`
	}

	accountVerifiedEvent struct {
		EventID      uuid.UUID `json:"event_id"`
		AccountID    uuid.UUID `json:"account_id"`
		Email        string    `json:"email"`
		Phone        string    `json:"phone"`
		DeliveryType string    `json:"code_type"`
		// Unconfirmed: identity activated the account without verifying the
		// address (ACCOUNT_VERIFICATION_REQUIRED=false).
		Unconfirmed bool `json:"unconfirmed"`
	}

	passwordChangedEvent struct {
		EventID      uuid.UUID `json:"event_id"`
		DeliveryType string    `json:"code_type"`
		AccountID    uuid.UUID `json:"account_id"`
		Email        string    `json:"email"`
		Phone        string    `json:"phone"`
	}

	// messageDeliverer is the subset of DeliveryService the identity handlers need.
	messageDeliverer interface {
		Deliver(ctx context.Context, msg domain.Message) error
	}

	// inactiveAccountChecker tells whether an account was deleted or erased.
	inactiveAccountChecker interface {
		IsInactive(ctx context.Context, accountID uuid.UUID) (bool, error)
	}

	// IdentityHandlers turns identity's account-lifecycle events into emails
	// and SMS, through the delivery layer. Nothing is written to the feed.
	IdentityHandlers struct {
		delivery messageDeliverer
		accounts inactiveAccountChecker
		metrics  application.DeliveryMetrics
		logger   *logger.Logger
		tracer   trace.Tracer
	}
)

func NewIdentityHandlers(
	delivery messageDeliverer,
	accounts inactiveAccountChecker,
	metrics application.DeliveryMetrics,
	log *logger.Logger,
) *IdentityHandlers {
	return &IdentityHandlers{
		delivery: delivery,
		accounts: accounts,
		metrics:  metrics,
		logger:   log.WithName("notification.identity_handlers"),
		tracer:   telemetry.Tracer("notification.identity_handlers"),
	}
}

func (h *IdentityHandlers) HandleVerificationCodeGenerated(ctx context.Context, payload []byte) error {
	ctx, span := h.tracer.Start(ctx, "HandleVerificationCodeGenerated")
	defer span.End()

	var event verificationCodeGeneratedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to parse event")
		return fmt.Errorf("decode payload: %w", err)
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	span.SetAttributes(
		attribute.String("event.id", event.EventID.String()),
		attribute.String("event.delivery_type", event.DeliveryType),
		attribute.String("event.account_id", event.AccountID.String()),
	)

	data := map[string]string{"code": event.Code}

	switch event.DeliveryType {
	case DeliveryTypeEmail:
		return h.deliver(ctx, span, event.AccountID, domain.Message{
			Kind: domain.MessageVerificationCode, Channel: domain.ChannelEmail, Recipient: event.Email, Data: data,
		})
	case DeliveryTypePhone:
		return h.deliver(ctx, span, event.AccountID, domain.Message{
			Kind: domain.MessageVerificationCode, Channel: domain.ChannelSMS, Recipient: event.Phone, Data: data,
		})
	default:
		return h.dropUnknownDeliveryType(ctx, span, domain.MessageVerificationCode, event.DeliveryType)
	}
}

func (h *IdentityHandlers) HandleAccountVerified(ctx context.Context, payload []byte) error {
	ctx, span := h.tracer.Start(ctx, "HandleAccountVerified")
	defer span.End()

	var event accountVerifiedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to parse event")
		return fmt.Errorf("decode payload: %w", err)
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	span.SetAttributes(
		attribute.String("event.id", event.EventID.String()),
		attribute.String("event.delivery_type", event.DeliveryType),
		attribute.String("event.account_id", event.AccountID.String()),
	)

	// Nobody proved the address is theirs, so don't mail it.
	if event.Unconfirmed {
		h.logger.Debug(ctx, "no welcome message for an unconfirmed address")
		span.SetStatus(codes.Ok, "skipped: unconfirmed address")
		return nil
	}

	switch event.DeliveryType {
	case DeliveryTypeEmail:
		return h.deliver(ctx, span, event.AccountID, domain.Message{
			Kind: domain.MessageWelcome, Channel: domain.ChannelEmail, Recipient: event.Email,
		})
	case DeliveryTypePhone:
		// Deliberate: there is no welcome SMS, only a welcome email.
		h.logger.Debug(ctx, "no welcome message for a phone account")
		span.SetStatus(codes.Ok, "notification sent")
		return nil
	default:
		return h.dropUnknownDeliveryType(ctx, span, domain.MessageWelcome, event.DeliveryType)
	}
}

func (h *IdentityHandlers) HandlePasswordChanged(ctx context.Context, payload []byte) error {
	ctx, span := h.tracer.Start(ctx, "HandlePasswordChanged")
	defer span.End()

	var event passwordChangedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to parse event")
		return fmt.Errorf("decode payload: %w", err)
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	span.SetAttributes(
		attribute.String("event.id", event.EventID.String()),
		attribute.String("event.delivery_type", event.DeliveryType),
		attribute.String("event.account_id", event.AccountID.String()),
	)

	switch event.DeliveryType {
	case DeliveryTypeEmail:
		return h.deliver(ctx, span, event.AccountID, domain.Message{
			Kind: domain.MessagePasswordChanged, Channel: domain.ChannelEmail, Recipient: event.Email,
		})
	case DeliveryTypePhone:
		return h.deliver(ctx, span, event.AccountID, domain.Message{
			Kind: domain.MessagePasswordChanged, Channel: domain.ChannelSMS, Recipient: event.Phone,
		})
	default:
		return h.dropUnknownDeliveryType(ctx, span, domain.MessagePasswordChanged, event.DeliveryType)
	}
}

// deliver sends msg to the account, unless the account was deleted or erased:
// then nothing is sent and the event is acked.
func (h *IdentityHandlers) deliver(ctx context.Context, span trace.Span, accountID uuid.UUID, msg domain.Message) error {
	inactive, err := h.accounts.IsInactive(ctx, accountID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("check account: %w", err)
	}
	if inactive {
		h.logger.Debug(ctx, "account deleted or erased, nothing delivered", "notification_type", string(msg.Kind))
		span.SetStatus(codes.Ok, "skipped: account deleted")
		return nil
	}

	if err := h.delivery.Deliver(ctx, msg); err != nil {
		span.RecordError(err)
		return err
	}

	span.SetStatus(codes.Ok, "notification sent")

	return nil
}

// dropUnknownDeliveryType handles an event whose delivery type is neither
// email nor phone. Retrying can't fix a bad payload, so the event is acked
// (nil) - but loudly: an error log, an error on the span and a send-failure
// metric, so a producer/consumer mismatch (like identity sending "phone" while
// a handler matched "sms") shows up instead of silently sending nothing.
func (h *IdentityHandlers) dropUnknownDeliveryType(ctx context.Context, span trace.Span, kind domain.MessageKind, deliveryType string) error {
	h.logger.Error(ctx, "dropping event with unknown delivery type",
		"notification_type", string(kind),
		"delivery_type", deliveryType,
	)

	h.metrics.NotificationSendFailures().WithLabelValues(string(kind), "unknown", "unknown_delivery_type").Inc()

	span.SetAttributes(attribute.String("error.type", "unknown_delivery_type"))
	span.SetStatus(codes.Error, "unknown delivery type")

	return nil
}
