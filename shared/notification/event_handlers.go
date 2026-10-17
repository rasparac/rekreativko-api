package notification

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rasparac/rekreativko-api/shared/api"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func (s *service) HandleVerificationCodeGenerated(ctx context.Context, payload []byte) error {
	ctx, span := s.tracer.Start(ctx, "HandleVerificationCodeGenerated")
	defer span.End()

	var event VerificationCodeGeneratedEvent
	err := json.Unmarshal(payload, &event)
	if err != nil {
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
		err = s.sendVerificationCodeEmail(ctx, event.Email, event.Code)
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("send verification code email: %w", err)
		}
	case DeliveryTypePhone:
		err = s.sendVerificationCodeSMS(ctx, event.Phone, event.Code)
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("send verification code sms: %w", err)
		}
	default:
		return s.dropUnknownDeliveryType(ctx, span, "verification", event.DeliveryType)
	}

	span.SetStatus(codes.Ok, "notification sent")

	return nil
}

func (s *service) HandleAccountVerified(ctx context.Context, payload []byte) error {
	ctx, span := s.tracer.Start(ctx, "HandleAccountVerified")
	defer span.End()

	var event AccountVerifiedEvent
	err := json.Unmarshal(payload, &event)
	if err != nil {
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
		err = s.sendWelcomeEmail(ctx, event.Email)
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("send welcome email: %w", err)
		}
	case DeliveryTypePhone:
		// Deliberate: there is no welcome SMS, only a welcome email.
		s.logger.Debug(ctx, "no welcome message for a phone account")
	default:
		return s.dropUnknownDeliveryType(ctx, span, "welcome_message", event.DeliveryType)
	}

	span.SetStatus(codes.Ok, "notification sent")

	return nil
}

func (s *service) HandlePasswordChanged(ctx context.Context, payload []byte) error {
	ctx, span := s.tracer.Start(ctx, "HandlePasswordChanged")
	defer span.End()

	var event PasswordChangedEvent
	err := json.Unmarshal(payload, &event)
	if err != nil {
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
		err = s.sendPasswordChangedEmail(ctx, event.Email)
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("send password changed email: %w", err)
		}
	case DeliveryTypePhone:
		err = s.sendPasswordChangedSMS(ctx, event.Phone)
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("send password changed sms: %w", err)
		}
	default:
		return s.dropUnknownDeliveryType(ctx, span, "password_changed", event.DeliveryType)
	}

	span.SetStatus(codes.Ok, "notification sent")

	return nil
}

// dropUnknownDeliveryType handles an event whose delivery type is neither
// email nor phone. Retrying can't fix a bad payload, so the event is acked
// (nil) - but loudly: an error log, an error on the span and a send-failure
// metric, so a producer/consumer mismatch (like identity sending "phone" while
// a handler matched "sms") shows up instead of silently sending nothing.
func (s *service) dropUnknownDeliveryType(ctx context.Context, span trace.Span, notificationType, deliveryType string) error {
	s.logger.Error(ctx, "dropping event with unknown delivery type",
		"notification_type", notificationType,
		"delivery_type", deliveryType,
	)

	s.metrics.NotificationSendFailures().WithLabelValues(notificationType, "unknown", "unknown_delivery_type").Inc()

	span.SetAttributes(attribute.String("error.type", "unknown_delivery_type"))
	span.SetStatus(codes.Error, "unknown delivery type")

	return nil
}
