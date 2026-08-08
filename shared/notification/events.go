package notification

import "github.com/google/uuid"

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

	AccountLockedEvent struct {
		EventID      uuid.UUID `json:"event_id"`
		DeliveryType string    `json:"code_type"`
		AccountID    uuid.UUID `json:"account_id"`
		Email        string    `json:"email"`
		Phone        string    `json:"phone"`
		Reason       string    `json:"reason"`
	}

	PasswordChangedEvent struct {
		EventID      uuid.UUID `json:"event_id"`
		DeliveryType string    `json:"code_type"`
		AccountID    uuid.UUID `json:"account_id"`
		Email        string    `json:"email"`
		Phone        string    `json:"phone"`
	}
)
