package events

import "github.com/google/uuid"

type AccountVerifiedEvent struct {
	EventID      uuid.UUID `json:"event_id"`
	AccountID    uuid.UUID `json:"account_id"`
	Email        string    `json:"email"`
	Phone        string    `json:"phone"`
	DeliveryType string    `json:"code_type"`
}
