package dtos

import (
	"time"

	"github.com/google/uuid"
)

// CreateActivityGroupRequest is a request to create a new activity group
type CreateActivityGroupRequest struct {
	Title           string `json:"title" validate:"required,min=3,max=100" example:"Morning Running Group"`
	Description     string `json:"description" validate:"max=1000" example:"A group for early morning runners in Belgrade"`
	ActivityType    string `json:"activity_type" validate:"required,oneof=hiking cycling running swimming yoga gym other" example:"running"`
	DifficultyLevel string `json:"difficulty_level" validate:"required,oneof=beginner intermediate advanced" example:"beginner"`
	Visibility      string `json:"visibility" validate:"required,oneof=public private" example:"public"`
	LocationCity    string `json:"location_city" validate:"required,min=2,max=100" example:"Belgrade"`
	LocationCountry string `json:"location_country" validate:"required,min=2,max=100" example:"Serbia"`
	Timezone        string `json:"timezone" validate:"required,timezone" example:"Europe/Belgrade"`
	DefaultCapacity *int   `json:"default_capacity" validate:"omitempty,min=1,max=1000" example:"30"`
}

// UpdateActivityGroupRequest is a request to update an existing activity group
type UpdateActivityGroupRequest struct {
	Title           string `json:"title" validate:"required,min=3,max=100" example:"Morning Running Group"`
	Description     string `json:"description" validate:"max=1000" example:"A group for early morning runners in Belgrade"`
	ActivityType    string `json:"activity_type" validate:"required,oneof=hiking cycling running swimming yoga gym other" example:"running"`
	DifficultyLevel string `json:"difficulty_level" validate:"required,oneof=beginner intermediate advanced" example:"beginner"`
	LocationCity    string `json:"location_city" validate:"required,min=2,max=100" example:"Belgrade"`
	LocationCountry string `json:"location_country" validate:"required,min=2,max=100" example:"Serbia"`
	Timezone        string `json:"timezone" validate:"required,timezone" example:"Europe/Belgrade"`
}

// ActivityGroupResponse is a response containing activity group data
type ActivityGroupResponse struct {
	ID              uuid.UUID  `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
	CreatorID       uuid.UUID  `json:"creator_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	Title           string     `json:"title" example:"Morning Running Group"`
	Description     string     `json:"description" example:"A group for early morning runners in Belgrade"`
	ActivityType    string     `json:"activity_type" example:"running"`
	DifficultyLevel string     `json:"difficulty_level" example:"beginner"`
	Visibility      string     `json:"visibility" example:"public"`
	Status          string     `json:"status" example:"active"`
	LocationCity    string     `json:"location_city" example:"Belgrade"`
	LocationCountry string     `json:"location_country" example:"Serbia"`
	Timezone        string     `json:"timezone" example:"Europe/Belgrade"`
	DefaultCapacity *int       `json:"default_capacity,omitempty" example:"30"`
	CreatedAt       time.Time  `json:"created_at" example:"2024-01-01T00:00:00Z"`
	UpdatedAt       time.Time  `json:"updated_at" example:"2024-01-01T00:00:00Z"`
	CancelledAt     *time.Time `json:"cancelled_at,omitempty" example:"2024-01-01T00:00:00Z"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty" example:"2024-01-01T00:00:00Z"`
}

// ActivityGroupListResponse is a response containing a list of activity groups
type ActivityGroupListResponse struct {
	Groups []ActivityGroupResponse `json:"groups"`
	Total  int                     `json:"total"`
	Limit  int                     `json:"limit"`
	Offset int                     `json:"offset"`
}

// CreateActivityGroupResponse is a response after creating an activity group
type CreateActivityGroupResponse struct {
	ID uuid.UUID `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
}
