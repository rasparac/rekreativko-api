package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
)

const (
	IdentityAccountRegisterdEvent       = "identity.account.registered"
	IdentityAccountVerifiedEvent        = "identity.account.verified"
	IdentityAccountAcitvatedEvent       = "identity.account.activated"
	IdentityAccountSuspendedEvent       = "identity.account.suspended"
	IdentityAccountLockedEvent          = "identity.account.locked"
	IdentityAccountUnlockedEvent        = "identity.account.unlocked"
	IdentityAccountLoginSucceededEvent  = "identity.account.login.succeeded"
	IdentityAccountLoginFailedEvent     = "identity.account.login.failed"
	IdentityAccountPasswordChangedEvent = "identity.account.password.changed"
	IdentityAccountDeletedEvent         = "identity.account.deleted"
)

type AccountRegisteredEvent struct {
	domainevent.BaseEvent
	Email *string `json:"email,omitempty"`
	Phone *string `json:"phone,omitempty"`
}

func NewAccountRegisteredEvent(account *Account) *AccountRegisteredEvent {
	var email, phone *string
	if account.email != nil {
		e := account.email.String()
		email = &e
	}
	if account.phoneNumber != nil {
		p := account.phoneNumber.String()
		phone = &p
	}

	return &AccountRegisteredEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   IdentityAccountRegisterdEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: account.ID(),
		},
		Email: email,
		Phone: phone,
	}
}

type AccountVerifiedEvent struct {
	domainevent.BaseEvent
	VerifiedBy CodeType  `json:"verified_by"`
	AccountID  uuid.UUID `json:"account_id"`
	CodeType   CodeType  `json:"code_type"`
	Email      string    `json:"email"`
	Phone      string    `json:"phone"`
}

func NewAccountVerifiedEvent(account *Account, code *VerificationCode) *AccountVerifiedEvent {
	return &AccountVerifiedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   IdentityAccountVerifiedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: account.ID(),
		},
		VerifiedBy: code.CodeType(),
		AccountID:  account.ID(),
		CodeType:   code.CodeType(),
		Email:      account.Email().String(),
		Phone:      account.PhoneNumber().String(),
	}
}

type AccountActivatedEvent struct {
	domainevent.BaseEvent
}

func NewAccountActivatedEvent(account *Account) *AccountActivatedEvent {
	return &AccountActivatedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   IdentityAccountAcitvatedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: account.ID(),
		},
	}
}

type AccountSuspendedEvent struct {
	domainevent.BaseEvent
	Reason string
}

func NewAccountSuspendedEvent(account *Account, reason string) *AccountSuspendedEvent {
	return &AccountSuspendedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   IdentityAccountSuspendedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: account.ID(),
		},
		Reason: reason,
	}
}

type AccountLockedEvent struct {
	domainevent.BaseEvent
	Reason      string
	LockedUntil time.Time
}

func NewAccountLockedEvent(account *Account, reason string, lockedUntil time.Time) *AccountLockedEvent {
	return &AccountLockedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   IdentityAccountLockedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: account.ID(),
		},
		Reason:      reason,
		LockedUntil: lockedUntil,
	}
}

type AccountUnlockedEvent struct {
	domainevent.BaseEvent
}

func NewAccountUnlockedEvent(account *Account) *AccountUnlockedEvent {
	return &AccountUnlockedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   IdentityAccountUnlockedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: account.ID(),
		},
	}
}

type LoginSucceededEvent struct {
	domainevent.BaseEvent
	IPAddress string
}

func NewLoginSucceededEvent(account *Account, ipAddress string) *LoginSucceededEvent {
	return &LoginSucceededEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   IdentityAccountLoginSucceededEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: account.ID(),
		},
		IPAddress: ipAddress,
	}
}

type LoginFailedEvent struct {
	domainevent.BaseEvent
	IPAddress string
	Reason    string
	Attempts  int8
}

func NewLoginFailedEvent(account *Account, ipAddress, reason string, attempts int8) *LoginFailedEvent {
	return &LoginFailedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   IdentityAccountLoginFailedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: account.ID(),
		},
		IPAddress: ipAddress,
		Reason:    reason,
		Attempts:  attempts,
	}
}

type PasswordChangedEvent struct {
	domainevent.BaseEvent
}

func NewPasswordChangedEvent(account *Account) *PasswordChangedEvent {
	return &PasswordChangedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   IdentityAccountPasswordChangedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: account.ID(),
		},
	}
}

type AccountDeletedEvent struct {
	domainevent.BaseEvent
}

func NewAccountDeletedEvent(account *Account) *AccountDeletedEvent {
	return &AccountDeletedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   IdentityAccountDeletedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: account.ID(),
		},
	}
}
