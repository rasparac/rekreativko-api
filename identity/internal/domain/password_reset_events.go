package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
)

const IdentityPasswordResetRequestedEvent = "identity.password_reset.requested"

// PasswordResetRequestedEvent sends a forgot-password code to the account's
// address (notifications delivers it on code_type's channel).
type PasswordResetRequestedEvent struct {
	domainevent.BaseEvent
	AccountID uuid.UUID `json:"account_id"`
	CodeType  CodeType  `json:"code_type"`
	Code      string    `json:"code"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
}

func NewPasswordResetRequestedEvent(reset *PasswordReset, account *Account) *PasswordResetRequestedEvent {
	return &PasswordResetRequestedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   IdentityPasswordResetRequestedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: account.ID(),
		},
		AccountID: account.ID(),
		CodeType:  reset.CodeType(),
		Code:      reset.Code(),
		Email:     account.Email().String(),
		Phone:     account.PhoneNumber().String(),
	}
}
