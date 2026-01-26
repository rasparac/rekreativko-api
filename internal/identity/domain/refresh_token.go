package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
)

type RefreshToken struct {
	id        uuid.UUID
	accountID uuid.UUID
	value     string
	expiresAt time.Time
	createdAt time.Time
	revokedAt *time.Time

	events []domainevent.Event
}

func NewRefreshToken(
	accountID uuid.UUID,
	value string,
	expiresAt time.Time,
) *RefreshToken {
	rt := &RefreshToken{
		id:        uuid.New(),
		accountID: accountID,
		value:     value,
		expiresAt: expiresAt,
		createdAt: time.Now().UTC(),
	}

	rt.addEvent(NewRefreshTokenCreatedEvent(rt))

	return rt
}

func ReconstructRefreshToken(
	id uuid.UUID,
	accountID uuid.UUID,
	value string,
	expiresAt time.Time,
	createdAt time.Time,
	revokedAt *time.Time,
) *RefreshToken {
	return &RefreshToken{
		id:        id,
		accountID: accountID,
		value:     value,
		expiresAt: expiresAt,
		createdAt: createdAt,
		revokedAt: revokedAt,
	}
}

func (rt *RefreshToken) ID() uuid.UUID {
	return rt.id
}

func (rt *RefreshToken) AccountID() uuid.UUID {
	return rt.accountID
}

func (rt *RefreshToken) Token() string {
	return rt.value
}

func (rt *RefreshToken) ExpiresAt() time.Time {
	return rt.expiresAt
}

func (rt *RefreshToken) CreatedAt() time.Time {
	return rt.createdAt
}

func (rt *RefreshToken) RevokedAt() *time.Time {
	return rt.revokedAt
}

func (rt *RefreshToken) IsValid() bool {
	if rt.revokedAt != nil {
		return false
	}

	now := time.Now().UTC()

	if now.After(rt.expiresAt) {
		return false
	}

	return true
}

func (rt *RefreshToken) IsExpired() bool {
	return time.Now().UTC().After(rt.expiresAt)
}

func (rt *RefreshToken) IsRevoked() bool {
	return rt.revokedAt != nil && !rt.revokedAt.IsZero()
}

func (rt *RefreshToken) Revoke(reason string) error {
	if rt.IsRevoked() {
		return ErrRefreshTokenRevoked
	}

	now := time.Now().UTC()
	rt.revokedAt = &now

	rt.addEvent(NewRefreshTokenRevokedEvent(rt, reason))

	return nil
}

func (rt *RefreshToken) TimeUntilExpiration() time.Duration {
	return time.Until(rt.expiresAt)
}

func (rt *RefreshToken) Validate() error {
	if rt.IsRevoked() {
		return ErrRefreshTokenRevoked
	}

	if rt.IsExpired() {
		return ErrRefreshTokenExpired
	}

	return nil
}

func (rt *RefreshToken) addEvent(event domainevent.Event) {
	rt.events = append(rt.events, event)
}

func (rt *RefreshToken) Events() []domainevent.Event {
	return rt.events
}

func (rt *RefreshToken) ClearEvents() {
	rt.events = nil
}
