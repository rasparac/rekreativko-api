package dtos

import (
	"time"

	"github.com/google/uuid"
)

type UpdateProfileRequest struct {
	FullName *string `json:"full_name" example:"John Doe"`
	Nickname *string `json:"nickname" example:"JD"`
	Bio      *string `json:"bio" example:"I love playing basketball and soccer"`

	// DateOfBirth, format YYYY-MM-DD. Omit to leave the existing value untouched;
	// send an empty string to clear it. Must be in the past and at least 13 years ago.
	DateOfBirth *string `json:"date_of_birth,omitempty" example:"1990-05-15"`

	// Location fields (flattened for swagger compatibility). Omit all four to
	// leave the existing location untouched. Send location_city as an empty
	// string to clear the location entirely. Otherwise, any field you omit
	// keeps its previously saved value (e.g. sending only updated coordinates
	// keeps the existing city/country). location_latitude/location_longitude
	// are only applied together - if you send just one, it's ignored.
	LocationCity      *string  `json:"location_city" example:"Belgrade"`
	LocationCountry   *string  `json:"location_country" example:"Serbia"`
	LocationLatitude  *float64 `json:"location_latitude" example:"44.8176"`
	LocationLongitude *float64 `json:"location_longitude" example:"20.4633"`

	// Profile picture
	ProfilePictureURL *string `json:"profile_picture_url" example:"https://example.com/avatar.jpg"`

	// Activity interests
	ActivityInterests []ActivityInterest `json:"activity_interests"`
}

// Location represents a geographic location
type Location struct {
	City        string       `json:"city" example:"Belgrade"`
	Country     string       `json:"country" example:"Serbia"`
	Coordinates *Coordinates `json:"coordinates"`
}

// Coordinates represents geographic coordinates
type Coordinates struct {
	Latitude  float64 `json:"latitude" example:"44.8176"`
	Longitude float64 `json:"longitude" example:"20.4633"`
}

// ProfilePicture represents a user's profile picture
type ProfilePicture struct {
	URL string `json:"url" example:"https://example.com/avatar.jpg"`
}

// ActivityInterest represents a user's interest in an activity
type ActivityInterest struct {
	Name  string `json:"name" example:"Basketball"`
	Level string `json:"level" example:"intermediate" enums:"beginner,intermediate,advanced"`
}

type AccountProfileResponse struct {
	ID               uuid.UUID          `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
	FullName         string             `json:"full_name" example:"John Doe"`
	Nickname         string             `json:"nickname" example:"JD"`
	Bio              string             `json:"bio" example:"I love playing basketball and soccer"`
	DateOfBirth      *string            `json:"date_of_birth,omitempty" example:"1990-05-15"`
	Location         *Location          `json:"location"`
	ProfilePicture   *ProfilePicture    `json:"profile_picture"`
	ActivityInterest []ActivityInterest `json:"activity_interest"`
	CreatedAt        time.Time          `json:"created_at" example:"2024-01-01T00:00:00Z"`
	UpdatedAt        time.Time          `json:"updated_at" example:"2024-01-01T00:00:00Z"`
}
