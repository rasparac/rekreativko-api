package dtos

import (
	"time"

	"github.com/google/uuid"
)

type UpdateProfileRequest struct {
	FullName *string `json:"full_name"`
	Nickname *string `json:"nickname"`
	Bio      *string `json:"bio"`

	Location *Location `json:"location"`

	ProfilePicture *ProfilePicture `json:"profile_picture"`

	ActivityInterest []ActivityInterest `json:"activity_interest"`
}

// swagger:model Location
type Location struct {
	City        string       `json:"city"`
	Country     string       `json:"country"`
	Coordinates *Coordinates `json:"coordinates"`
}

type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type ProfilePicture struct {
	URL string `json:"url"`
}

type ActivityInterest struct {
	Name  string `json:"name"`
	Level string `json:"level"`
}

type AccountProfileResponse struct {
	ID               uuid.UUID          `json:"id"`
	FullName         string             `json:"full_name"`
	Nickname         string             `json:"nickname"`
	Bio              string             `json:"bio"`
	Location         *Location          `json:"location"`
	ProfilePicture   *ProfilePicture    `json:"profile_picture"`
	ActivityInterest []ActivityInterest `json:"activity_interest"`
	CreatedAt        time.Time          `json:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at"`
}
