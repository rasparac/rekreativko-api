package dtos

import (
	"time"

	"github.com/google/uuid"
)

// SendSessionInviteRequest is a request to invite a user to a standalone session
type SendSessionInviteRequest struct {
	UserID uuid.UUID `json:"user_id" validate:"required" example:"123e4567-e89b-12d3-a456-426655440000"`
}

// SendSessionInviteResponse is a response after sending a session invite
type SendSessionInviteResponse struct {
	ID uuid.UUID `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
}

// SessionInviteResponse is a response containing session invite data
type SessionInviteResponse struct {
	ID            uuid.UUID  `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
	SessionID     uuid.UUID  `json:"session_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	InvitedUserID uuid.UUID  `json:"invited_user_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	InvitedByID   uuid.UUID  `json:"invited_by_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	Status        string     `json:"status" example:"pending"`
	CreatedAt     time.Time  `json:"created_at" example:"2024-01-01T00:00:00Z"`
	ExpiresAt     time.Time  `json:"expires_at" example:"2024-01-08T00:00:00Z"`
	RespondedAt   *time.Time `json:"responded_at,omitempty" example:"2024-01-02T00:00:00Z"`
}
