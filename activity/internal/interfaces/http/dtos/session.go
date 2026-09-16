package dtos

import (
	"time"

	"github.com/google/uuid"
)

// CreateSessionRequest is a request to create a new session
type CreateSessionRequest struct {
	// ActivityGroupID is optional - omit it to create a standalone session with no
	// group. A standalone session can be public or private just like a grouped
	// one; visibility is controlled by the Visibility field below in both cases.
	ActivityGroupID *uuid.UUID `json:"activity_group_id,omitempty" example:"123e4567-e89b-12d3-a456-426655440000"`
	Title           string     `json:"title" validate:"required,min=1,max=200" example:"Tuesday tempo run"`
	// ActivityType and DifficultyLevel are required only for a standalone session
	// (no activity_group_id) - for a grouped session both are always inherited
	// from the group and any value sent here is ignored.
	ActivityType    string `json:"activity_type,omitempty" validate:"omitempty,oneof=running walking jogging basketball football tennis gym dancing skiing climbing cycling swimming hiking yoga weightlifting other" example:"running"`
	DifficultyLevel string `json:"difficulty_level,omitempty" validate:"omitempty,oneof=beginner intermediate advanced" example:"beginner"`
	LocationCity    string `json:"location_city" validate:"required,min=2,max=100" example:"Belgrade"`
	LocationCountry string `json:"location_country" validate:"required,min=2,max=100" example:"Serbia"`
	// LocationStreet is an optional venue/address line shown to attendees so they
	// know exactly where to go, beyond just the city.
	LocationStreet string     `json:"location_street,omitempty" validate:"omitempty,max=255" example:"Ada Ciganlija bb, Court 3"`
	LocationLat    float64    `json:"location_lat" validate:"required,min=-90,max=90" example:"44.8176"`
	LocationLng    float64    `json:"location_lng" validate:"required,min=-180,max=180" example:"20.4633"`
	StartTime      time.Time  `json:"start_time" validate:"required" example:"2024-02-01T18:00:00Z"`
	EndTime        *time.Time `json:"end_time,omitempty" example:"2024-02-01T20:00:00Z"`
	Capacity       *int       `json:"capacity,omitempty" validate:"omitempty,min=1,max=1000" example:"20"`
	Note           string     `json:"note,omitempty" validate:"max=500" example:"Bring your own equipment"`
	IsRecurring    bool       `json:"is_recurring,omitempty" example:"false"`
	// Visibility is required and applies to both group-scoped and standalone
	// sessions - "public" means anyone can RSVP as an attendee, "private"
	// restricts joining to group members (grouped) or the creator (standalone).
	Visibility string `json:"visibility" validate:"required,oneof=public private" example:"public" enums:"private,public"`
	// RequiresApproval: if true, RSVPing "going" creates a pending join request
	// that the creator/admin must approve rather than joining immediately.
	RequiresApproval bool `json:"requires_approval,omitempty" example:"false"`
}

// UpdateSessionRequest is a request to update an existing session
type UpdateSessionRequest struct {
	LocationCity    string     `json:"location_city" validate:"required,min=2,max=100" example:"Belgrade"`
	LocationCountry string     `json:"location_country" validate:"required,min=2,max=100" example:"Serbia"`
	LocationStreet  string     `json:"location_street,omitempty" validate:"omitempty,max=255" example:"Ada Ciganlija bb, Court 3"`
	LocationLat     float64    `json:"location_lat" validate:"required,min=-90,max=90" example:"44.8176"`
	LocationLng     float64    `json:"location_lng" validate:"required,min=-180,max=180" example:"20.4633"`
	StartTime       time.Time  `json:"start_time" validate:"required" example:"2024-02-01T18:00:00Z"`
	EndTime         *time.Time `json:"end_time,omitempty" example:"2024-02-01T20:00:00Z"`
	Capacity        *int       `json:"capacity,omitempty" validate:"omitempty,min=1,max=1000" example:"20"`
	Note            string     `json:"note,omitempty" validate:"max=500" example:"Bring your own equipment"`
	// Visibility is required and applies to both group-scoped and standalone
	// sessions - see CreateSessionRequest.Visibility for what each value means.
	Visibility string `json:"visibility" validate:"required,oneof=public private" example:"public" enums:"private,public"`
}

// SetSessionVisibilityRequest is a request to change a session's visibility
type SetSessionVisibilityRequest struct {
	Visibility string `json:"visibility" validate:"required,oneof=public private" example:"public" enums:"private,public"`
}

// SessionResponse is a response containing session data
type SessionResponse struct {
	ID               uuid.UUID  `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
	ActivityGroupID  *uuid.UUID `json:"activity_group_id,omitempty" example:"123e4567-e89b-12d3-a456-426655440000"`
	CreatedByID      uuid.UUID  `json:"created_by_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	TemplateID       *uuid.UUID `json:"template_id,omitempty" example:"123e4567-e89b-12d3-a456-426655440000"`
	Title            string     `json:"title" example:"Tuesday tempo run"`
	ActivityType     string     `json:"activity_type" example:"running"`
	DifficultyLevel  string     `json:"difficulty_level" example:"beginner"`
	LocationCity     string     `json:"location_city" example:"Belgrade"`
	LocationCountry  string     `json:"location_country" example:"Serbia"`
	LocationStreet   string     `json:"location_street,omitempty" example:"Ada Ciganlija bb, Court 3"`
	LocationLat      float64    `json:"location_lat" example:"44.8176"`
	LocationLng      float64    `json:"location_lng" example:"20.4633"`
	StartTime        time.Time  `json:"start_time" example:"2024-02-01T18:00:00Z"`
	EndTime          *time.Time `json:"end_time,omitempty" example:"2024-02-01T20:00:00Z"`
	Capacity         *int       `json:"capacity,omitempty" example:"20"`
	Status           string     `json:"status" example:"scheduled"`
	Visibility       string     `json:"visibility" example:"private" enums:"private,public"`
	RequiresApproval bool       `json:"requires_approval" example:"false"`
	IsRecurring      bool       `json:"is_recurring" example:"false"`
	Note             string     `json:"note,omitempty" example:"Bring your own equipment"`
	OpenAt           *time.Time `json:"open_at,omitempty" example:"2024-02-01T12:00:00Z"`
	CreatedAt        time.Time  `json:"created_at" example:"2024-01-01T00:00:00Z"`
	UpdatedAt        time.Time  `json:"updated_at" example:"2024-01-01T00:00:00Z"`
	CancelledAt      *time.Time `json:"cancelled_at,omitempty" example:"2024-01-01T00:00:00Z"`
	StartedAt        *time.Time `json:"started_at,omitempty" example:"2024-02-01T18:05:00Z"`
	CompletedAt      *time.Time `json:"completed_at,omitempty" example:"2024-02-01T20:10:00Z"`
	// AttendeeStatus is the requesting user's own RSVP status for this session
	// (going, pending, not_going, maybe, promoted). Only ever populated on
	// list/discover responses (batched per page); GET /sessions/{id} never
	// sets it - fetch the attendee list separately if you need this for a
	// single session.
	AttendeeStatus *string `json:"attendee_status,omitempty" example:"going"`
}

// CreateSessionResponse is a response after creating a session
type CreateSessionResponse struct {
	ID uuid.UUID `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
}

// NearbySessionResponse is a session result from a location-based discovery query
type NearbySessionResponse struct {
	SessionResponse
	DistanceKm float64 `json:"distance_km" example:"2.3"`
}
