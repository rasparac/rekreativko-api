package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
)

const (
	IdentityAccountRefreshTokenCreatedEvent = "identity.refresh_token.created"
	IdentityAccountRefreshTokenRevokedEvent = "identity.refresh_token.revoked"
)

type RefreshTokenCreatedEvent struct {
	domainevent.BaseEvent
	AccountID uuid.UUID `json:"account_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewRefreshTokenCreatedEvent(token *RefreshToken) *RefreshTokenCreatedEvent {
	return &RefreshTokenCreatedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   IdentityAccountRefreshTokenCreatedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: token.ID(),
		},
		AccountID: token.AccountID(),
		ExpiresAt: token.ExpiresAt(),
	}
}

type RefreshTokenRevokedEvent struct {
	domainevent.BaseEvent
	TokenID   uuid.UUID `json:"token_id"`
	AccountID uuid.UUID `json:"account_id"`
	Reason    string    `json:"reason"`
}

func NewRefreshTokenRevokedEvent(refreshToken *RefreshToken, reason string) *RefreshTokenRevokedEvent {
	return &RefreshTokenRevokedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   IdentityAccountRefreshTokenRevokedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: refreshToken.ID(),
		},
		TokenID:   refreshToken.ID(),
		AccountID: refreshToken.AccountID(),
		Reason:    reason,
	}
}
