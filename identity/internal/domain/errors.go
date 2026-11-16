package domain

import (
	"errors"
	"strings"
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
	ErrAccountNotDeleted     = errors.New("account is not deleted")
	ErrAccountAlreadyErased  = errors.New("account is already erased")

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
