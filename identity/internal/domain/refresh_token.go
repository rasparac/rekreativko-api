package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
)

type RefreshToken struct {
	id          uuid.UUID
	accessToken string
	tokenType   string
	accountID   uuid.UUID
	// plaintext is the raw, client-facing token value. Only ever populated at
	// creation time (NewRefreshToken) - it is never persisted and reconstructed
	// tokens loaded from the database (ReconstructRefreshToken) never have it,
	// only its hash.
	plaintext string
	// tokenHash is a SHA-256 hex digest of plaintext; this is what gets stored
	// and looked up in the database.
	tokenHash string
	expiresAt time.Time
	createdAt time.Time
	revokedAt *time.Time

	events []domainevent.Event
}

func NewRefreshToken(
	accountID uuid.UUID,
	plaintext string,
	tokenHash string,
	expiresAt time.Time,
) *RefreshToken {
	rt := &RefreshToken{
		id:        uuid.New(),
		accountID: accountID,
		plaintext: plaintext,
		tokenHash: tokenHash,
		expiresAt: expiresAt,
		createdAt: time.Now().UTC(),
	}

	rt.addEvent(NewRefreshTokenCreatedEvent(rt))

	return rt
}

func ReconstructRefreshToken(
	id uuid.UUID,
	accountID uuid.UUID,
	tokenHash string,
	expiresAt time.Time,
	createdAt time.Time,
	revokedAt *time.Time,
) *RefreshToken {
	return &RefreshToken{
		id:        id,
		accountID: accountID,
		tokenHash: tokenHash,
		expiresAt: expiresAt,
		createdAt: createdAt,
		revokedAt: revokedAt,
		tokenType: "Bearer",
	}
}

func (rt *RefreshToken) ID() uuid.UUID {
	return rt.id
}

func (rt *RefreshToken) AccountID() uuid.UUID {
	return rt.accountID
}

// Token returns the raw, client-facing token value. Only meaningful right
// after creation (NewRefreshToken); reconstructed tokens loaded from the
// database never have the plaintext, only TokenHash().
func (rt *RefreshToken) Token() string {
	return rt.plaintext
}

// TokenHash returns the SHA-256 hex digest used for storage/lookup.
func (rt *RefreshToken) TokenHash() string {
	return rt.tokenHash
}

func (rt *RefreshToken) ExpiresAt() time.Time {
	return rt.expiresAt
}

func (rt *RefreshToken) CreatedAt() time.Time {
	return rt.createdAt
}

func (rt *RefreshToken) AccessToken() string {
	return rt.accessToken
}

func (rt *RefreshToken) TokenType() string {
	return rt.tokenType
}

func (rt *RefreshToken) RevokedAt() *time.Time {
	return rt.revokedAt
}

func (rt *RefreshToken) IsValid() bool {
	if rt.revokedAt != nil {
		return false
	}

	now := time.Now().UTC()

	return !now.After(rt.expiresAt)
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

func (rt *RefreshToken) SetAccessToken(accessToken string) {
	rt.accessToken = accessToken
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
