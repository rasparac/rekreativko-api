package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
)

type CodeType string

const (
	CodeTypeEmail CodeType = "email"
	CodeTypePhone CodeType = "phone"
)

func (c CodeType) String() string {
	return string(c)
}

func (c CodeType) IsValid() bool {
	return c == CodeTypeEmail || c == CodeTypePhone
}

type VerificationCode struct {
	id        uuid.UUID
	accountID uuid.UUID
	code      string
	codeType  CodeType
	expiresAt time.Time
	createdAt time.Time
	usedAt    *time.Time

	events []domainevent.Event
}

func NewVerificationCode(
	account *Account,
	code string,
	codeType CodeType,
	expiresAt time.Time,
) (*VerificationCode, error) {
	if !codeType.IsValid() {
		return nil, ErrInvalidVerificationCode
	}

	if code == "" {
		return nil, ErrInvalidVerificationCode
	}

	vc := &VerificationCode{
		id:        uuid.New(),
		accountID: account.ID(),
		code:      code,
		codeType:  codeType,
		expiresAt: expiresAt,
		createdAt: time.Now().UTC(),
		events:    []domainevent.Event{},
	}

	createdEvent := NewVerificationCodeCreatedEvent(vc, account)

	vc.events = []domainevent.Event{
		createdEvent,
	}

	return vc, nil
}

func ReconstructVerificationCode(
	id uuid.UUID,
	accountID uuid.UUID,
	code string,
	codeType CodeType,
	expiresAt time.Time,
	createdAt time.Time,
	usedAt *time.Time,
) *VerificationCode {
	return &VerificationCode{
		id:        id,
		accountID: accountID,
		code:      code,
		codeType:  codeType,
		expiresAt: expiresAt,
		createdAt: createdAt,
		usedAt:    usedAt,
	}
}

func (vc *VerificationCode) ID() uuid.UUID {
	return vc.id
}

func (vc *VerificationCode) AccountID() uuid.UUID {
	return vc.accountID
}

func (vc *VerificationCode) Code() string {
	return vc.code
}

func (vc *VerificationCode) CodeType() CodeType {
	return vc.codeType
}

func (vc *VerificationCode) ExpiresAt() time.Time {
	return vc.expiresAt
}

func (vc *VerificationCode) CreatedAt() time.Time {
	return vc.createdAt
}

func (vc *VerificationCode) UsedAt() *time.Time {
	return vc.usedAt
}

func (vc *VerificationCode) IsUsed() bool {
	return vc.usedAt != nil && !vc.usedAt.IsZero()
}

func (vc *VerificationCode) IsExpired() bool {
	return time.Now().UTC().After(vc.expiresAt)
}

func (vc *VerificationCode) IsValid() bool {
	if vc.IsUsed() {
		return false
	}

	if time.Now().UTC().After(vc.expiresAt) {
		return false
	}

	return true
}

func (vc *VerificationCode) Use() error {
	if vc.IsUsed() {
		return ErrVerificationCodeUsed
	}

	if vc.IsExpired() {
		return ErrVerificationExpired
	}

	if !vc.IsValid() {
		return ErrInvalidVerificationCode
	}

	now := time.Now().UTC()
	vc.usedAt = &now

	vc.events = append(vc.events, NewVerificationCodeUsedEvent(vc))

	return nil
}

func (vc *VerificationCode) Verify(code string) error {
	if !vc.IsValid() {
		if vc.IsUsed() {
			return ErrVerificationCodeUsed
		}

		if vc.IsExpired() {
			return ErrVerificationExpired
		}

		return ErrInvalidVerificationCode
	}

	if vc.code != code {
		return ErrInvalidVerificationCode
	}

	return nil
}

func (vc *VerificationCode) TimeUntilExpiration() time.Duration {
	return time.Until(vc.expiresAt)
}

func (vc *VerificationCode) Validate() error {
	if vc.IsUsed() {
		return ErrVerificationCodeUsed
	}

	if vc.IsExpired() {
		return ErrVerificationExpired
	}

	return nil
}

func (vc *VerificationCode) ClearEvents() {
	vc.events = make([]domainevent.Event, 0)
}

func (vc *VerificationCode) Events() []domainevent.Event {
	return vc.events
}
