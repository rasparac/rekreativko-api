package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func (s *Service) HandleVerificationCodeGenerated(ctx context.Context, payload []byte) error {
	ctx, span := s.tracer.Start(ctx, "HandleVerificationCodeGenerated")
	defer span.End()

	var event VerificationCodeGeneratedEvent
	err := json.Unmarshal(payload, &event)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to parse event")
		return fmt.Errorf("decode payload: %w", err)
	}

	span.SetAttributes(
		attribute.String("event.id", event.EventID.String()),
		attribute.String("event.delivery_type", event.DeliveryType),
		attribute.String("event.code", event.Code),
		attribute.String("event.account_id", event.AccountID.String()),
	)

	switch event.DeliveryType {
	case "email":
		span.SetAttributes(attribute.String("event.email", event.Email))
		err = s.sendVerificationCodeEmail(ctx, event.Email, event.Code)
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("send verification code email: %w", err)
		}
	case "phone":
		span.SetAttributes(attribute.String("event.phone", event.Phone))
		err = s.sendVerificationCodeSMS(ctx, event.Phone, event.Code)
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("send verification code sms: %w", err)
		}
	default:
		span.SetAttributes(attribute.String("error.type", "unknow_delivery_method"))
		return nil
	}

	span.SetStatus(codes.Ok, "notification sent")

	return nil
}

func (s *Service) HandleAccountVerified(ctx context.Context, payload []byte) error {
	ctx, span := s.tracer.Start(ctx, "HandleAccountVerified")
	defer span.End()

	var event AccountVerifiedEvent
	err := json.Unmarshal(payload, &event)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to parse event")
		return fmt.Errorf("decode payload: %w", err)
	}

	span.SetAttributes(
		attribute.String("event.id", event.EventID.String()),
		attribute.String("event.delivery_type", event.DeliveryType),
		attribute.String("event.account_id", event.AccountID.String()),
	)

	switch event.DeliveryType {
	case "email":
		span.SetAttributes(attribute.String("event.email", event.Email))
		err = s.sendWelcomeEmail(ctx, event.Email)
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("send welcome email: %w", err)
		}
	default:
		span.RecordError(errors.New("failed to send verification"))
		span.SetAttributes(attribute.String("error.type", "unknow_delivery_method"))
		return nil
	}

	span.SetStatus(codes.Ok, "notification sent")

	return nil
}

func (s *Service) HandlePasswordChanged(ctx context.Context, payload []byte) error {
	ctx, span := s.tracer.Start(ctx, "HandlePasswordChanged")
	defer span.End()

	var event PasswordChangedEvent
	err := json.Unmarshal(payload, &event)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to parse event")
		return fmt.Errorf("decode payload: %w", err)
	}

	span.SetAttributes(
		attribute.String("event.id", event.EventID.String()),
		attribute.String("event.delivery_type", event.DeliveryType),
		attribute.String("event.account_id", event.AccountID.String()),
	)

	switch event.DeliveryType {
	case "email":
		span.SetAttributes(attribute.String("event.email", event.Email))
		err = s.sendPasswordChangedEmail(ctx, event.Email)
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("send password changed email: %w", err)
		}
	case "sms":
		span.SetAttributes(attribute.String("event.phone", event.Phone))
		err = s.sendPasswordChangedSMS(ctx, event.Phone)
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("send password changed sms: %w", err)
		}
	default:
		span.SetAttributes(attribute.String("error.type", "unknow_delivery_method"))
		return nil
	}

	span.SetStatus(codes.Ok, "notification sent")

	return nil
}

func (s *Service) HandleAccountLocked(ctx context.Context, payload []byte) error {
	ctx, span := s.tracer.Start(ctx, "HandleAccountLocked")
	defer span.End()

	var event AccountLockedEvent
	err := json.Unmarshal(payload, &event)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to parse event")
		return fmt.Errorf("decode payload: %w", err)
	}

	span.SetAttributes(
		attribute.String("event.id", event.EventID.String()),
		attribute.String("event.delivery_type", event.DeliveryType),
		attribute.String("event.account_id", event.AccountID.String()),
	)

	switch event.DeliveryType {
	case "email":
		span.SetAttributes(attribute.String("event.email", event.Email))
		err = s.sendAccountLockedEmail(ctx, event.Email, event.Reason)
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("send account locked email: %w", err)
		}
	case "sms":
		span.SetAttributes(attribute.String("event.phone", event.Phone))
		err = s.sendAccountLockedSMS(ctx, event.Phone, event.Reason)
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("send account locked sms: %w", err)
		}
	default:
		span.SetAttributes(attribute.String("error.type", "unknow_delivery_method"))
		return nil
	}

	span.SetStatus(codes.Ok, "notification sent")

	return nil
}
