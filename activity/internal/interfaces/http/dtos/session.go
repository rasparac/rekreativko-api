package dtos

import (
	"time"

	"github.com/google/uuid"
)

// CreateSessionRequest is a request to create a new session
type CreateSessionRequest struct {
	ActivityGroupID uuid.UUID `json:"activity_group_id" validate:"required" example:"123e4567-e89b-12d3-a456-426655440000"`
	LocationCity    string    `json:"location_city" validate:"required,min=2,max=100" example:"Belgrade"`
	LocationCountry string    `json:"location_country" validate:"required,min=2,max=100" example:"Serbia"`
	LocationLat     float64   `json:"location_lat" validate:"required,min=-90,max=90" example:"44.8176"`
	LocationLng     float64   `json:"location_lng" validate:"required,min=-180,max=180" example:"20.4633"`
	StartTime       time.Time `json:"start_time" validate:"required" example:"2024-02-01T18:00:00Z"`
	EndTime         *time.Time `json:"end_time,omitempty" example:"2024-02-01T20:00:00Z"`
	Capacity        *int      `json:"capacity,omitempty" validate:"omitempty,min=1,max=1000" example:"20"`
	Note            string    `json:"note,omitempty" validate:"max=500" example:"Bring your own equipment"`
	IsRecurring     bool      `json:"is_recurring,omitempty" example:"false"`
}

// UpdateSessionRequest is a request to update an existing session
type UpdateSessionRequest struct {
	LocationCity    string    `json:"location_city" validate:"required,min=2,max=100" example:"Belgrade"`
	LocationCountry string    `json:"location_country" validate:"required,min=2,max=100" example:"Serbia"`
	LocationLat     float64   `json:"location_lat" validate:"required,min=-90,max=90" example:"44.8176"`
	LocationLng     float64   `json:"location_lng" validate:"required,min=-180,max=180" example:"20.4633"`
	StartTime       time.Time `json:"start_time" validate:"required" example:"2024-02-01T18:00:00Z"`
	EndTime         *time.Time `json:"end_time,omitempty" example:"2024-02-01T20:00:00Z"`
	Capacity        *int      `json:"capacity,omitempty" validate:"omitempty,min=1,max=1000" example:"20"`
	Note            string    `json:"note,omitempty" validate:"max=500" example:"Bring your own equipment"`
}

// SessionResponse is a response containing session data
type SessionResponse struct {
	ID              uuid.UUID  `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
	ActivityGroupID uuid.UUID  `json:"activity_group_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	CreatedByID     uuid.UUID  `json:"created_by_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	TemplateID      *uuid.UUID `json:"template_id,omitempty" example:"123e4567-e89b-12d3-a456-426655440000"`
	LocationCity    string     `json:"location_city" example:"Belgrade"`
	LocationCountry string     `json:"location_country" example:"Serbia"`
	LocationLat     float64    `json:"location_lat" example:"44.8176"`
	LocationLng     float64    `json:"location_lng" example:"20.4633"`
	StartTime       time.Time  `json:"start_time" example:"2024-02-01T18:00:00Z"`
	EndTime         *time.Time `json:"end_time,omitempty" example:"2024-02-01T20:00:00Z"`
	Capacity        *int       `json:"capacity,omitempty" example:"20"`
	Status          string     `json:"status" example:"scheduled"`
	IsRecurring     bool       `json:"is_recurring" example:"false"`
	Note            string     `json:"note,omitempty" example:"Bring your own equipment"`
	OpenAt          *time.Time `json:"open_at,omitempty" example:"2024-02-01T12:00:00Z"`
	CreatedAt       time.Time  `json:"created_at" example:"2024-01-01T00:00:00Z"`
	UpdatedAt       time.Time  `json:"updated_at" example:"2024-01-01T00:00:00Z"`
	CancelledAt     *time.Time `json:"cancelled_at,omitempty" example:"2024-01-01T00:00:00Z"`
	StartedAt       *time.Time `json:"started_at,omitempty" example:"2024-02-01T18:05:00Z"`
	CompletedAt     *time.Time `json:"completed_at,omitempty" example:"2024-02-01T20:10:00Z"`
}

// SessionListResponse is a response containing a list of sessions
type SessionListResponse struct {
	Sessions []SessionResponse `json:"sessions"`
	Total    int               `json:"total"`
	Limit    int               `json:"limit"`
	Offset   int               `json:"offset"`
}

// CreateSessionResponse is a response after creating a session
type CreateSessionResponse struct {
	ID uuid.UUID `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
}
