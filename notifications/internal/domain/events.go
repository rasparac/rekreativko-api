package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
)

// EventAccountErased confirms to identity that this context has erased its
// data of an account (identity.account.erased).
const EventAccountErased = "notifications.account.erased"

// AccountErasedEvent confirms that an erased account's notifications are
// gone. It carries the account id only.
type AccountErasedEvent struct {
	domainevent.BaseEvent
	AccountID uuid.UUID `json:"account_id"`
}

func NewAccountErasedEvent(accountID uuid.UUID) *AccountErasedEvent {
	return &AccountErasedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventAccountErased,
			OccurredAt:  time.Now().UTC(),
			AggregateID: accountID,
		},
		AccountID: accountID,
	}
}
