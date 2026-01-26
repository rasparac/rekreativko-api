package domain

import (
	"errors"
	"strings"

	"github.com/rasparac/rekreativko-api/internal/shared/domainerror"
)

var (
	// Account errors
	ErrAccountNotFound       = errors.New("account not found")
	ErrAccountAlreadyExists  = errors.New("account already exists")
	ErrAccountSuspended      = errors.New("account is suspended")
	ErrAccountDeleted        = errors.New("account is deleted")
	ErrAccountLocked         = errors.New("account is locked")
	ErrAccountNotVerified    = errors.New("account is not verified")
	ErrAccountAlredyVerified = errors.New("account is already verified")

	// Credential errors
	ErrInvalidCredentials       = errors.New("invalid credentials")
	ErrInvalidEmailFormat       = errors.New("invalid email format")
	ErrInvalidPhoneNumberFormat = errors.New("invalid phone number format")
	ErrWeakPassword             = errors.New("password does not meet complexity requirements")

	// Token errors
	ErrRefreshTokenInvalid  = errors.New("invalid token")
	ErrRefreshTokenExpired  = errors.New("token has expired")
	ErrRefreshTokenNotFound = errors.New("token not found")
	ErrRefreshTokenRevoked  = errors.New("token has been revoked")

	// Verification errors
	ErrInvalidVerificationCode     = errors.New("invalid verification code")
	ErrVerificationCodeNotFound    = errors.New("verification not found")
	ErrVerificationExpired         = errors.New("verification has expired")
	ErrVerificationCodeUsed        = errors.New("verification code has already been used")
	ErrTooManyVerificationAttempts = errors.New("too many verification attempts")
)

type AccountLockedError struct {
	Reason      string
	LockedUntil string
}

func (e *AccountLockedError) Error() string {
	return "account is locked: " + e.Reason + ", until: " + e.LockedUntil
}

func NewAccountLockedError(reason, lockedUntil string) error {
	return &AccountLockedError{
		Reason:      reason,
		LockedUntil: lockedUntil,
	}
}

type PasswordRequirementError struct {
	Requirements []string
}

func (e *PasswordRequirementError) Error() string {
	return "password does not meet the following requirements: " + strings.Join(e.Requirements, ", ")
}

func IsDomainErr(err error) bool {
	return errors.Is(err, ErrAccountNotFound) ||
		errors.Is(err, ErrAccountAlreadyExists) ||
		errors.Is(err, ErrAccountSuspended) ||
		errors.Is(err, ErrAccountDeleted) ||
		errors.Is(err, ErrAccountLocked) ||
		errors.Is(err, ErrAccountNotVerified)
}

func MapErrToAppError(err error) *domainerror.AppError {
	var appErr *PasswordRequirementError
	if errors.As(err, &appErr) {
		return domainerror.BadRequest(
			"identity_weak_password",
			"Password does not meet complexity requirements: "+strings.Join(appErr.Requirements, ", "),
			err,
		)
	}

	switch err {
	case ErrAccountNotFound:
		return domainerror.NotFound(
			"identity_account_not_found",
			"Account not found",
			err,
		)
	case ErrAccountLocked:
		return domainerror.Forbidden(
			"identity_account_locked",
			"Account is locked",
			err,
		)
	case ErrAccountSuspended:
		return domainerror.Forbidden(
			"identity_account_suspended",
			"Account is suspended",
			err,
		)
	case ErrAccountDeleted:
		return domainerror.Forbidden(
			"identity_account_deleted",
			"Account is deleted",
			err,
		)
	case ErrAccountNotVerified:
		return domainerror.Forbidden(
			"identity_account_not_verified",
			"Account is not verified",
			err,
		)
	case ErrAccountAlredyVerified:
		return domainerror.BadRequest(
			"identity_account_already_verified",
			"Account is already verified",
			err,
		)
	case ErrInvalidCredentials:
		return domainerror.Unauthorized(
			"identity_invalid_credentials",
			"Invalid credentials",
			err,
		)
	case ErrInvalidEmailFormat:
		return domainerror.BadRequest(
			"identity_invalid_email_format",
			"Invalid email format",
			err,
		)
	case ErrInvalidPhoneNumberFormat:
		return domainerror.BadRequest(
			"identity_invalid_phone_number_format",
			"Invalid phone number format",
			err,
		)
	case &PasswordRequirementError{}:
		return domainerror.BadRequest(
			"identity_weak_password",
			"Password does not meet complexity requirements",
			err,
		)
	case ErrInvalidVerificationCode:
		return domainerror.BadRequest(
			"identity_invalid_verification_code",
			"Invalid verification code",
			err,
		)
	case ErrVerificationCodeNotFound:
		return domainerror.NotFound(
			"identity_verification_code_not_found",
			"Verification code not found",
			err,
		)
	case ErrVerificationExpired:
		return domainerror.BadRequest(
			"identity_verification_code_expired",
			"Verification code has expired",
			err,
		)
	case ErrVerificationCodeUsed:
		return domainerror.BadRequest(
			"identity_verification_code_used",
			"Verification code has already been used",
			err,
		)
	case ErrTooManyVerificationAttempts:
		return domainerror.BadRequest(
			"identity_too_many_verification_attempts",
			"Too many verification attempts",
			err,
		)
	case ErrRefreshTokenNotFound:
		return domainerror.Unauthorized(
			"identity_refresh_token_not_found",
			"Refresh token not found",
			err,
		)
	case ErrRefreshTokenInvalid:
		return domainerror.Unauthorized(
			"identity_refresh_token_invalid",
			"Refresh token is invalid",
			err,
		)
	case ErrRefreshTokenExpired:
		return domainerror.Unauthorized(
			"identity_refresh_token_expired",
			"Refresh token has expired",
			err,
		)
	case ErrRefreshTokenRevoked:
		return domainerror.Unauthorized(
			"identity_refresh_token_revoked",
			"Refresh token has been revoked",
			err,
		)
	}

	return domainerror.Internal()
}
