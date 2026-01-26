package domain

import (
	"errors"

	"github.com/rasparac/rekreativko-api/internal/shared/domainerror"
)

var (
	// Profile errors
	ErrUserProfileNotFound   = errors.New("user profile not found")
	ErrUserProfileExists     = errors.New("user profile already exists")
	ErrUserProfileDeleted    = errors.New("user profile is deleted")
	ErrUserProfileBioTooLong = errors.New("user profile bio is too long")

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

	// User statistics errors

	ErrInvalidActivityCount = errors.New("invalid activity count")
	ErrInvalidMonthFormat   = errors.New("invalid month format")
)

func MapErrToAppError(err error) *domainerror.AppError {
	return nil
}
