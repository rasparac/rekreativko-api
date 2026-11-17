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

	// ProfilesFilter looks profiles up by account id and/or nickname. ViewerID
	// is the caller: other accounts' profiles come back as their privacy
	// settings allow.
	ProfilesFilter struct {
		AccountIDs []uuid.UUID
		Nicknames  []string
		ViewerID   uuid.UUID
	}

	ProfileFilter struct {
		AccountID *uuid.UUID
		Nickname  *string
		ViewerID  uuid.UUID
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
