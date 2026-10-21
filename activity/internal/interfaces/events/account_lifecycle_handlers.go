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
)

type (
	// accountDeletedEvent is identity.account.deleted: the account id is the
	// event's aggregate id.
	accountDeletedEvent struct {
		EventID   uuid.UUID `json:"event_id"`
		AccountID uuid.UUID `json:"aggregate_id"`
	}

	// accountErasedEvent is identity.account.erased.
	accountErasedEvent struct {
		EventID   uuid.UUID `json:"event_id"`
		AccountID uuid.UUID `json:"account_id"`
	}

	// accountLifecycle is the subset of AccountLifecycleService the
	// handlers need.
	accountLifecycle interface {
		DeactivateAccount(ctx context.Context, accountID uuid.UUID) error
		EraseAccount(ctx context.Context, accountID uuid.UUID) error
	}

	// AccountLifecycleHandlers follow identity's account deletion and
	// erasure. A bad payload is dead-lettered; any other failure is retried
	// (both operations are idempotent).
	AccountLifecycleHandlers struct {
		accounts accountLifecycle
		logger   *logger.Logger
	}
)

func NewAccountLifecycleHandlers(accounts accountLifecycle, log *logger.Logger) *AccountLifecycleHandlers {
	return &AccountLifecycleHandlers{
		accounts: accounts,
		logger:   log.WithName("activity.account_lifecycle_handlers"),
	}
}

func (h *AccountLifecycleHandlers) HandleAccountDeleted(ctx context.Context, payload []byte) error {
	var event accountDeletedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return events.Permanent(fmt.Errorf("decode payload: %w", err))
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	if event.AccountID == uuid.Nil {
		return events.Permanent(errors.New("aggregate_id missing"))
	}

	if err := h.accounts.DeactivateAccount(ctx, event.AccountID); err != nil {
		h.logger.Error(ctx, "failed to deactivate account", "error", err, "account_id", event.AccountID)
		return err
	}

	return nil
}

func (h *AccountLifecycleHandlers) HandleAccountErased(ctx context.Context, payload []byte) error {
	var event accountErasedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return events.Permanent(fmt.Errorf("decode payload: %w", err))
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	if event.AccountID == uuid.Nil {
		return events.Permanent(errors.New("account_id missing"))
	}

	if err := h.accounts.EraseAccount(ctx, event.AccountID); err != nil {
		h.logger.Error(ctx, "failed to erase account", "error", err, "account_id", event.AccountID)
		return err
	}

	return nil
}
