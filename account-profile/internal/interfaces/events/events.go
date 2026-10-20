package events

import "github.com/google/uuid"

type AccountVerifiedEvent struct {
	EventID      uuid.UUID `json:"event_id"`
	AccountID    uuid.UUID `json:"account_id"`
	Email        string    `json:"email"`
	Phone        string    `json:"phone"`
	DeliveryType string    `json:"code_type"`
}

// AccountErasedEvent is identity.account.erased: identity has erased the
// account, and every context erases its own data of it.
type AccountErasedEvent struct {
	EventID   uuid.UUID `json:"event_id"`
	AccountID uuid.UUID `json:"account_id"`
}
