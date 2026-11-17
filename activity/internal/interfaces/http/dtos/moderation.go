package dtos

import (
	"time"

	"github.com/google/uuid"
)

// ReportRequest reports a user, group or session to the operators
type ReportRequest struct {
	TargetType string    `json:"target_type" validate:"required,oneof=user group session" example:"group"`
	TargetID   uuid.UUID `json:"target_id" validate:"required" example:"123e4567-e89b-12d3-a456-426655440000"`
	Reason     string    `json:"reason" validate:"required,oneof=spam harassment hate violence inappropriate other" example:"spam"`
	// Note is optional free text, at most 1000 characters.
	Note string `json:"note,omitempty" validate:"max=1000" example:"Advertises a paid app in every session"`
}

// BlockRequest blocks an account
type BlockRequest struct {
	AccountID uuid.UUID `json:"account_id" validate:"required" example:"123e4567-e89b-12d3-a456-426655440000"`
}

// BlockResponse is one account the caller has blocked
type BlockResponse struct {
	AccountID uuid.UUID `json:"account_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	BlockedAt time.Time `json:"blocked_at" example:"2026-11-17T10:00:00Z"`
}
