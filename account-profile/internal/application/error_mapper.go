package application

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/rasparac/rekreativko-api/account-profile/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
)

// mapDomainErr maps the domain's sentinel errors to the API error the handlers
// write; anything unknown is a 500.
func mapDomainErr(err error) *domainerror.AppError {
	// Handle pgx.ErrNoRows first - it's a sentinel error, not a *pgconn.PgError
	if errors.Is(err, pgx.ErrNoRows) {
		return domainerror.NotFound("not_found", "Resource not found", err)
	}

	// Profile errors
	switch {
	case errors.Is(err, domain.ErrAccountProfileNotFound):
		return domainerror.NotFound("profile_not_found", "Account profile not found", err)
	case errors.Is(err, domain.ErrAccountProfileExists):
		return domainerror.Conflict("profile_exists", "Account profile already exists", err)
	case errors.Is(err, domain.ErrAccountProfileBioTooLong):
		return domainerror.ValidationError("bio_too_long", "Account profile bio is too long", err)
	}

	// Profile picture errors
	switch {
	case errors.Is(err, domain.ErrProfilePictureNotFound):
		return domainerror.NotFound("profile_picture_not_found", "Profile picture not found", err)
	case errors.Is(err, domain.ErrProfilePictureRequired):
		return domainerror.ValidationError("profile_picture_required", "Profile picture is required", err)
	case errors.Is(err, domain.ErrInvalidProfilePictureURL):
		return domainerror.ValidationError("invalid_profile_picture_url", "Invalid profile picture URL", err)
	case errors.Is(err, domain.ErrProfilePictureURLTooLong):
		return domainerror.ValidationError("profile_picture_url_too_long", "Profile picture URL is too long", err)
	}

	// Nickname errors
	switch {
	case errors.Is(err, domain.ErrNicknameTooShort):
		return domainerror.ValidationError("nickname_too_short", "Nickname must be at least 3 characters long", err)
	case errors.Is(err, domain.ErrNicknameTooLong):
		return domainerror.ValidationError("nickname_too_long", "Nickname must be at most 50 characters long", err)
	case errors.Is(err, domain.ErrInvalidNickname):
		return domainerror.ValidationError("invalid_nickname", "Invalid nickname", err)
	case errors.Is(err, domain.ErrNickNameAlreadyExists):
		return domainerror.Conflict("nickname_exists", "Nickname already exists", err)
	}

	// Date of birth errors
	switch {
	case errors.Is(err, domain.ErrDateOfBirthRequired):
		return domainerror.ValidationError("date_of_birth_required", "Date of birth is required", err)
	case errors.Is(err, domain.ErrDateOfBirthInvalid):
		return domainerror.ValidationError("date_of_birth_invalid", "Invalid date of birth", err)
	case errors.Is(err, domain.ErrAgeTooYoung):
		return domainerror.ValidationError("age_too_young", "User is too young", err)
	}

	// Location errors
	switch {
	case errors.Is(err, domain.ErrLocationRequired):
		return domainerror.ValidationError("location_required", "Location is required", err)
	case errors.Is(err, domain.ErrLocationInvalid):
		return domainerror.ValidationError("location_invalid", "Invalid location", err)
	}

	// Activity interests errors
	switch {
	case errors.Is(err, domain.ErrInvalidInterests):
		return domainerror.ValidationError("invalid_interests", "Invalid interests", err)
	case errors.Is(err, domain.ErrDuplicateInterests):
		return domainerror.ValidationError("duplicate_interests", "Duplicate interests", err)
	case errors.Is(err, domain.ErrTooManyInterests):
		return domainerror.ValidationError("too_many_interests", "Too many interests", err)
	case errors.Is(err, domain.ErrActivityInterestsNotFound):
		return domainerror.NotFound("interests_not_found", "Interests not found", err)
	}

	// Settings errors
	switch {
	case errors.Is(err, domain.ErrInvalidSettingsKey):
		return domainerror.ValidationError("invalid_settings_key", "Invalid settings key", err)
	case errors.Is(err, domain.ErrInvalidSettingsValue):
		return domainerror.ValidationError("invalid_settings_value", "Invalid settings value", err)
	case errors.Is(err, domain.ErrSettingsNotFound):
		return domainerror.NotFound("settings_not_found", "Settings not found", err)
	}

	// Activity level errors
	switch {
	case errors.Is(err, domain.ErrInvalidActivityLevel):
		return domainerror.ValidationError("invalid_activity_level", "Invalid activity level", err)
	case errors.Is(err, domain.ErrInvalidActivityType):
		return domainerror.ValidationError("invalid_activity_type", "Invalid activity type", err)
	}

	// Account statistics errors
	switch {
	case errors.Is(err, domain.ErrInvalidActivityCount):
		return domainerror.ValidationError("invalid_activity_count", "Invalid activity count", err)
	case errors.Is(err, domain.ErrInvalidMonthFormat):
		return domainerror.ValidationError("invalid_month_format", "Invalid month format", err)
	}

	// Default to internal error with wrapped error for debugging
	return domainerror.InternalWithErr(err)
}
