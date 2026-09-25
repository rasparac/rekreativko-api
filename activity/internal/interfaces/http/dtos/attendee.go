package dtos

import (
	"time"

	"github.com/google/uuid"
)

// CreateRSVPRequest is a request to create a new RSVP
type CreateRSVPRequest struct {
	Status string `json:"status" validate:"required,oneof=going not_going maybe" example:"going"`
}

// UpdateRSVPRequest is a request to update an existing RSVP
type UpdateRSVPRequest struct {
	Status string `json:"status" validate:"required,oneof=going not_going maybe" example:"not_going"`
}

// AttendeeResponse is a response containing attendee data
type AttendeeResponse struct {
	ID              uuid.UUID  `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
	SessionID       uuid.UUID  `json:"session_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	ActivityGroupID *uuid.UUID `json:"activity_group_id,omitempty" example:"123e4567-e89b-12d3-a456-426655440000"`
	UserID          uuid.UUID  `json:"user_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	Status          string     `json:"status" example:"going"`
	Source          string     `json:"source" example:"rsvp_manual"`
	// TeamID is the team the attendee plays for - omitted when unassigned or
	// the session has no teams.
	TeamID    *uuid.UUID `json:"team_id,omitempty" example:"123e4567-e89b-12d3-a456-426655440000"`
	CreatedAt time.Time  `json:"created_at" example:"2024-01-01T00:00:00Z"`
	UpdatedAt time.Time  `json:"updated_at" example:"2024-01-01T00:00:00Z"`
}

// AssignTeamRequest puts an attendee on a team, or moves them to it
type AssignTeamRequest struct {
	TeamID uuid.UUID `json:"team_id" validate:"required" example:"123e4567-e89b-12d3-a456-426655440000"`
}

// CreateRSVPResponse is a response after creating an RSVP
type CreateRSVPResponse struct {
	ID uuid.UUID `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
}
