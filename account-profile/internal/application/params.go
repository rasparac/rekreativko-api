package application

import (
	"time"

	"github.com/google/uuid"
)

type (
	UpdateProfileParams struct {
		Nickname    *string
		FullName    *string
		DateOfBirth *time.Time
		Bio         *string

		Location *Location

		ProfilePicture *ProfilePicture

		ActivityInterest []ActivityInterest
	}

	CreateProfileParams struct {
		AccountID uuid.UUID
	}

	// Location carries only the fields the client actually included in the
	// update request (nil = omitted, left untouched). An explicitly empty
	// City is the sentinel for "clear my location".
	Location struct {
		City      *string
		Country   *string
		Latitude  *float64
		Longitude *float64
	}

	ProfilePicture struct {
		URL string
	}

	ActivityInterest struct {
		Name  string
		Level string
	}

	ProfilesFilter struct {
		AccountIDs []uuid.UUID
		Nicknames  []string

		DateOfBirthOver  *time.Time
		DateOfBirthUnder *time.Time

		IncludeDeleted *bool

		SortBy    *string
		SortOrder *string

		LocationCountry *string
		LocationCity    *string

		Limit     int
		PageToken string
	}

	ProfileFilter struct {
		AccountID *uuid.UUID
		Nickname  *string
	}

	Setting struct {
		Key   string
		Value any
		Type  string
	}

	UpdateAccountSettingsParams struct {
		Settings map[string]any
	}

	CreateAccountSettingsParams struct {
		AccountID uuid.UUID
	}
)
