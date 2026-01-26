package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
)

const (
	IdentityVerificationCodeCreatedEvent = "identity.verification_code.created"
	IdentityVerificationCodeUsedEvent    = "identity.verification_code.used"
)

type VerificationCodeCreatedEvent struct {
	domainevent.BaseEvent
	CodeType CodeType `json:"code_type"`
	Code     string   `json:"code"`

	AccountID uuid.UUID `json:"account_id"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
}

func NewVerificationCodeCreatedEvent(
	vCode *VerificationCode,
	account *Account,
) *VerificationCodeCreatedEvent {
	return &VerificationCodeCreatedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   IdentityVerificationCodeCreatedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: vCode.ID(),
		},
		CodeType:  vCode.CodeType(),
		Code:      vCode.Code(),
		Email:     account.Email().String(),
		Phone:     account.PhoneNumber().String(),
		AccountID: vCode.AccountID(),
	}
}

type VerificationCodeUsedEvent struct {
	domainevent.BaseEvent
	CodeType  CodeType  `json:"code_type"`
	Code      string    `json:"code"`
	AccountID uuid.UUID `json:"account_id"`
}

func NewVerificationCodeUsedEvent(
	vCode *VerificationCode,
) *VerificationCodeUsedEvent {
	return &VerificationCodeUsedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   IdentityVerificationCodeUsedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: vCode.ID(),
		},
		CodeType:  vCode.CodeType(),
		Code:      vCode.Code(),
		AccountID: vCode.AccountID(),
	}
}
