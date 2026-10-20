package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/events"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type (
	accountEraser interface {
		EraseAccount(ctx context.Context, accountID uuid.UUID) error
	}

	accountErasedEventHandler struct {
		eraser accountEraser
		tracer trace.Tracer
		logger *logger.Logger
	}
)

func NewAccountErasedEventHandler(eraser accountEraser, logger *logger.Logger) *accountErasedEventHandler {
	return &accountErasedEventHandler{
		eraser: eraser,
		tracer: telemetry.Tracer("accountErasedEventHandler"),
		logger: logger,
	}
}

// Handle erases the account's data on identity.account.erased. A bad payload
// is dead-lettered; any other failure is retried (the erasure is idempotent).
func (h *accountErasedEventHandler) Handle(ctx context.Context, payload []byte) error {
	ctx, span := h.tracer.Start(ctx, "accountErasedEventHandler.Handle")
	defer span.End()

	var event AccountErasedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		span.RecordError(err)
		return events.Permanent(fmt.Errorf("decode payload: %w", err))
	}

	ctx = api.WithEventID(ctx, event.EventID.String())

	if event.AccountID == uuid.Nil {
		err := errors.New("account_id missing")
		span.RecordError(err)
		return events.Permanent(err)
	}

	span.SetAttributes(
		attribute.String("event.id", event.EventID.String()),
		attribute.String("event.account_id", event.AccountID.String()),
	)

	if err := h.eraser.EraseAccount(ctx, event.AccountID); err != nil {
		span.RecordError(err)
		return fmt.Errorf("erase account %s: %w", event.AccountID, err)
	}

	span.SetStatus(codes.Ok, "account erased")

	return nil
}
