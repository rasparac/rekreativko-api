package notification

import "github.com/google/uuid"

// The delivery types identity puts in an event's code_type: the channel the
// account registered with (identity's CodeType).
const (
	DeliveryTypeEmail = "email"
	DeliveryTypePhone = "phone"
)

type (
	VerificationCodeGeneratedEvent struct {
		EventID      uuid.UUID `json:"event_id"`
		AccountID    uuid.UUID `json:"account_id"`
		DeliveryType string    `json:"code_type"`
		Code         string    `json:"code"`
		Email        string    `json:"email"`
		Phone        string    `json:"phone"`
	}

	AccountVerifiedEvent struct {
		EventID      uuid.UUID `json:"event_id"`
		AccountID    uuid.UUID `json:"account_id"`
		Email        string    `json:"email"`
		Phone        string    `json:"phone"`
		DeliveryType string    `json:"code_type"`
	}

	PasswordChangedEvent struct {
		EventID      uuid.UUID `json:"event_id"`
		DeliveryType string    `json:"code_type"`
		AccountID    uuid.UUID `json:"account_id"`
		Email        string    `json:"email"`
		Phone        string    `json:"phone"`
	}
)
