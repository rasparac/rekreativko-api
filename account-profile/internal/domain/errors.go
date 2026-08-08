package domain

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
)

var (
	// Profile errors
	ErrAccountProfileNotFound   = errors.New("account profile not found")
	ErrAccountProfileExists     = errors.New("account profile already exists")
	ErrAccountProfileDeleted    = errors.New("account profile is deleted")
	ErrAccountProfileBioTooLong = errors.New("account profile bio is too long")

	// Profile picture errors
	ErrProfilePictureNotFound = errors.New("profile picture not found")
	ErrProfilePictureRequired = errors.New("profile picture is required")

	// Nickname errors
	ErrNicknameTooShort      = errors.New("nickname must be at least 3 characters long")
	ErrNicknameTooLong       = errors.New("nickname must be at most 50 characters long")
	ErrInvalidNickname       = errors.New("invalid nickname")
	ErrNickNameAlreadyExists = errors.New("nickname already exists")

	// Date of birth errors
	ErrDateOfBirthRequired = errors.New("date of birth is required")
	ErrDateOfBirthInvalid  = errors.New("invalid date of birth")
	ErrAgeTooYoung         = errors.New("user is too young")

	// Location errors
	ErrLocationRequired = errors.New("location is required")
	ErrLocationInvalid  = errors.New("invalid location")

	// Activity Interests errors
	ErrInvalidInterests          = errors.New("invalid interests")
	ErrDuplicateInterests        = errors.New("duplicate interests")
	ErrTooManyInterests          = errors.New("too many interests")
	ErrActivityInterestsNotFound = errors.New("interests not found")

	// Settings errors
	ErrInvalidSettingsKey   = errors.New("invalid settings key")
	ErrInvalidSettingsValue = errors.New("invalid settings value")
	ErrSettingsNotFound     = errors.New("settings not found")

	// Profile picture errors
	ErrInvalidProfilePictureURL = errors.New("invalid profile picture URL")
	ErrProfilePictureURLTooLong = errors.New("profile picture URL is too long")

	// Activity level errors
	ErrInvalidActivityLevel = errors.New("invalid activity level")
	ErrInvalidActivityType  = errors.New("invalid activity type")

	// Account statistics errors

	ErrInvalidActivityCount = errors.New("invalid activity count")
	ErrInvalidMonthFormat   = errors.New("invalid month format")
)

func MapErrToAppError(err error) *domainerror.AppError {
	// Handle pgx.ErrNoRows first - it's a sentinel error, not a *pgconn.PgError
	if errors.Is(err, pgx.ErrNoRows) {
		return domainerror.NotFound("not_found", "Resource not found", err)
	}

	// Profile errors
	switch {
	case errors.Is(err, ErrAccountProfileNotFound):
		return domainerror.NotFound("profile_not_found", "Account profile not found", err)
	case errors.Is(err, ErrAccountProfileExists):
		return domainerror.Conflict("profile_exists", "Account profile already exists", err)
	case errors.Is(err, ErrAccountProfileDeleted):
		return domainerror.NotFound("profile_deleted", "Account profile is deleted", err)
	case errors.Is(err, ErrAccountProfileBioTooLong):
		return domainerror.ValidationError("bio_too_long", "Account profile bio is too long", err)
	}

	// Profile picture errors
	switch {
	case errors.Is(err, ErrProfilePictureNotFound):
		return domainerror.NotFound("profile_picture_not_found", "Profile picture not found", err)
	case errors.Is(err, ErrProfilePictureRequired):
		return domainerror.ValidationError("profile_picture_required", "Profile picture is required", err)
	case errors.Is(err, ErrInvalidProfilePictureURL):
		return domainerror.ValidationError("invalid_profile_picture_url", "Invalid profile picture URL", err)
	case errors.Is(err, ErrProfilePictureURLTooLong):
		return domainerror.ValidationError("profile_picture_url_too_long", "Profile picture URL is too long", err)
	}

	// Nickname errors
	switch {
	case errors.Is(err, ErrNicknameTooShort):
		return domainerror.ValidationError("nickname_too_short", "Nickname must be at least 3 characters long", err)
	case errors.Is(err, ErrNicknameTooLong):
		return domainerror.ValidationError("nickname_too_long", "Nickname must be at most 50 characters long", err)
	case errors.Is(err, ErrInvalidNickname):
		return domainerror.ValidationError("invalid_nickname", "Invalid nickname", err)
	case errors.Is(err, ErrNickNameAlreadyExists):
		return domainerror.Conflict("nickname_exists", "Nickname already exists", err)
	}

	// Date of birth errors
	switch {
	case errors.Is(err, ErrDateOfBirthRequired):
		return domainerror.ValidationError("date_of_birth_required", "Date of birth is required", err)
	case errors.Is(err, ErrDateOfBirthInvalid):
		return domainerror.ValidationError("date_of_birth_invalid", "Invalid date of birth", err)
	case errors.Is(err, ErrAgeTooYoung):
		return domainerror.ValidationError("age_too_young", "User is too young", err)
	}

	// Location errors
	switch {
	case errors.Is(err, ErrLocationRequired):
		return domainerror.ValidationError("location_required", "Location is required", err)
	case errors.Is(err, ErrLocationInvalid):
		return domainerror.ValidationError("location_invalid", "Invalid location", err)
	}

	// Activity interests errors
	switch {
	case errors.Is(err, ErrInvalidInterests):
		return domainerror.ValidationError("invalid_interests", "Invalid interests", err)
	case errors.Is(err, ErrDuplicateInterests):
		return domainerror.ValidationError("duplicate_interests", "Duplicate interests", err)
	case errors.Is(err, ErrTooManyInterests):
		return domainerror.ValidationError("too_many_interests", "Too many interests", err)
	case errors.Is(err, ErrActivityInterestsNotFound):
		return domainerror.NotFound("interests_not_found", "Interests not found", err)
	}

	// Settings errors
	switch {
	case errors.Is(err, ErrInvalidSettingsKey):
		return domainerror.ValidationError("invalid_settings_key", "Invalid settings key", err)
	case errors.Is(err, ErrInvalidSettingsValue):
		return domainerror.ValidationError("invalid_settings_value", "Invalid settings value", err)
	case errors.Is(err, ErrSettingsNotFound):
		return domainerror.NotFound("settings_not_found", "Settings not found", err)
	}

	// Activity level errors
	switch {
	case errors.Is(err, ErrInvalidActivityLevel):
		return domainerror.ValidationError("invalid_activity_level", "Invalid activity level", err)
	case errors.Is(err, ErrInvalidActivityType):
		return domainerror.ValidationError("invalid_activity_type", "Invalid activity type", err)
	}

	// Account statistics errors
	switch {
	case errors.Is(err, ErrInvalidActivityCount):
		return domainerror.ValidationError("invalid_activity_count", "Invalid activity count", err)
	case errors.Is(err, ErrInvalidMonthFormat):
		return domainerror.ValidationError("invalid_month_format", "Invalid month format", err)
	}

	// Default to internal error with wrapped error for debugging
	return domainerror.InternalWithErr(err)
}
