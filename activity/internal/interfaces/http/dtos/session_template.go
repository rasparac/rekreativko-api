package dtos

import (
	"time"

	"github.com/google/uuid"
)

// CreateSessionTemplateRequest is a request to create a new session template
type CreateSessionTemplateRequest struct {
	Title       string `json:"title" validate:"required,min=3,max=200" example:"Weekly Running Session"`
	Description string `json:"description" validate:"max=1000" example:"Every Monday morning run in the park"`

	// Recurrence configuration (all optional - if not provided, template is not recurring)
	RecurrenceFrequency  *string `json:"recurrence_frequency" validate:"omitempty,oneof=daily weekly monthly" example:"weekly"`
	RecurrenceInterval   *int    `json:"recurrence_interval" validate:"omitempty,min=1" example:"1"`
	RecurrenceDayOfWeek  *int    `json:"recurrence_day_of_week" validate:"omitempty,min=0,max=6" example:"1"` // 0=Sun, 1=Mon, etc.
	RecurrenceDayOfMonth *int    `json:"recurrence_day_of_month" validate:"omitempty,min=1,max=31" example:"15"`
	RecurrenceTimeHour   int     `json:"recurrence_time_hour" validate:"min=0,max=23" example:"7"`
	RecurrenceTimeMinute int     `json:"recurrence_time_minute" validate:"min=0,max=59" example:"0"`
	RecurrenceEndsAt     *string `json:"recurrence_ends_at" validate:"omitempty,datetime=2006-01-02T15:04:05Z07:00" example:"2025-12-31T23:59:59Z"` // RFC3339

	// Default session settings, inherited by every session generated from this template
	DefaultCapacity *int   `json:"default_capacity" validate:"omitempty,min=1" example:"20"`
	LocationCity    string `json:"location_city" validate:"required,min=2,max=100" example:"Belgrade"`
	LocationCountry string `json:"location_country" validate:"required,min=2,max=100" example:"Serbia"`
	// LocationStreet is an optional venue/address line, inherited by every
	// session generated from this template.
	LocationStreet string  `json:"location_street,omitempty" validate:"omitempty,max=255" example:"Ada Ciganlija bb, Court 3"`
	LocationLat    float64 `json:"location_lat" validate:"required,min=-90,max=90" example:"44.8176"`
	LocationLng    float64 `json:"location_lng" validate:"required,min=-180,max=180" example:"20.4633"`
	// Teams is the optional team setup inherited by every generated session -
	// only allowed when the group's activity is a team sport. Changing it
	// only affects sessions generated afterwards.
	Teams *TeamConfigRequest `json:"teams,omitempty"`
}

// UpdateSessionTemplateRequest is a request to update a session template
type UpdateSessionTemplateRequest struct {
	Title       string `json:"title" validate:"required,min=3,max=200" example:"Weekly Running Session"`
	Description string `json:"description" validate:"max=1000" example:"Every Monday morning run in the park"`

	// Recurrence configuration
	RecurrenceFrequency  *string `json:"recurrence_frequency" validate:"omitempty,oneof=daily weekly monthly" example:"weekly"`
	RecurrenceInterval   *int    `json:"recurrence_interval" validate:"omitempty,min=1" example:"1"`
	RecurrenceDayOfWeek  *int    `json:"recurrence_day_of_week" validate:"omitempty,min=0,max=6" example:"1"`
	RecurrenceDayOfMonth *int    `json:"recurrence_day_of_month" validate:"omitempty,min=1,max=31" example:"15"`
	RecurrenceTimeHour   int     `json:"recurrence_time_hour" validate:"min=0,max=23" example:"7"`
	RecurrenceTimeMinute int     `json:"recurrence_time_minute" validate:"min=0,max=59" example:"0"`
	RecurrenceEndsAt     *string `json:"recurrence_ends_at" validate:"omitempty,datetime=2006-01-02T15:04:05Z07:00" example:"2025-12-31T23:59:59Z"`

	// Default session settings, inherited by every session generated from this template
	DefaultCapacity *int   `json:"default_capacity" validate:"omitempty,min=1" example:"20"`
	LocationCity    string `json:"location_city" validate:"required,min=2,max=100" example:"Belgrade"`
	LocationCountry string `json:"location_country" validate:"required,min=2,max=100" example:"Serbia"`
	// LocationStreet is an optional venue/address line, inherited by every
	// session generated from this template.
	LocationStreet string  `json:"location_street,omitempty" validate:"omitempty,max=255" example:"Ada Ciganlija bb, Court 3"`
	LocationLat    float64 `json:"location_lat" validate:"required,min=-90,max=90" example:"44.8176"`
	LocationLng    float64 `json:"location_lng" validate:"required,min=-180,max=180" example:"20.4633"`
	// Teams is the optional team setup inherited by every generated session -
	// only allowed when the group's activity is a team sport. Changing it
	// only affects sessions generated afterwards.
	Teams *TeamConfigRequest `json:"teams,omitempty"`
}

// RecurrenceInfo contains recurrence rule information
type RecurrenceInfo struct {
	IsRecurring bool       `json:"is_recurring" example:"true"`
	Frequency   *string    `json:"frequency,omitempty" example:"weekly"`
	Interval    *int       `json:"interval,omitempty" example:"1"`
	DayOfWeek   *int       `json:"day_of_week,omitempty" example:"1"`   // 0=Sun, 1=Mon, ..., 6=Sat
	DayOfMonth  *int       `json:"day_of_month,omitempty" example:"15"` // 1-31
	TimeHour    *int       `json:"time_hour,omitempty" example:"7"`     // 0-23
	TimeMinute  *int       `json:"time_minute,omitempty" example:"0"`   // 0-59
	EndsAt      *time.Time `json:"ends_at,omitempty" example:"2025-12-31T23:59:59Z"`
}

// TemplateTeamConfigResponse is the team setup of a session template
type TemplateTeamConfigResponse struct {
	TeamCount      int      `json:"team_count" example:"2"`
	PlayersPerTeam *int     `json:"players_per_team,omitempty" example:"5"`
	Colors         []string `json:"colors,omitempty" example:"#FF0000,#0000FF"`
}

// SessionTemplateResponse is a response containing session template data
type SessionTemplateResponse struct {
	ID              uuid.UUID  `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
	ActivityGroupID uuid.UUID  `json:"activity_group_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	CreatedByID     uuid.UUID  `json:"created_by_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	Title           string     `json:"title" example:"Weekly Running Session"`
	Description     string     `json:"description" example:"Every Monday morning run in the park"`
	Status          string     `json:"status" example:"active"`
	DefaultCapacity *int       `json:"default_capacity,omitempty" example:"20"`
	LocationCity    string     `json:"location_city" example:"Belgrade"`
	LocationCountry string     `json:"location_country" example:"Serbia"`
	LocationStreet  string     `json:"location_street,omitempty" example:"Ada Ciganlija bb, Court 3"`
	LocationLat     float64    `json:"location_lat" example:"44.8176"`
	LocationLng     float64    `json:"location_lng" example:"20.4633"`
	GeneratedUpTo   *time.Time `json:"generated_up_to,omitempty" example:"2025-12-31T23:59:59Z"`

	// Teams is the team setup inherited by generated sessions (omitted if none)
	Teams *TemplateTeamConfigResponse `json:"teams,omitempty"`

	// Recurrence information (omitted if template is not recurring)
	Recurrence *RecurrenceInfo `json:"recurrence,omitempty"`

	CreatedAt time.Time  `json:"created_at" example:"2024-01-01T00:00:00Z"`
	UpdatedAt time.Time  `json:"updated_at" example:"2024-01-01T00:00:00Z"`
	DeletedAt *time.Time `json:"deleted_at,omitempty" example:"2024-01-01T00:00:00Z"`
}

// CreateSessionTemplateResponse is a response after creating a session template
type CreateSessionTemplateResponse struct {
	ID uuid.UUID `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
}

// EmptyResponse is a response with no data
type EmptyResponse struct{}
