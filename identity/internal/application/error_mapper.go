package application

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rasparac/rekreativko-api/identity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
)

// mapDomainErr maps the domain's errors to the API error the handlers write.
// Sentinels are matched with errors.Is, so one wrapped with context
// ("get account: %w") keeps its code; anything unknown is a 500.
func mapDomainErr(err error) *domainerror.AppError {
	// Handle pgx.ErrNoRows first - it's a sentinel error, not a *pgconn.PgError
	if errors.Is(err, pgx.ErrNoRows) {
		return domainerror.NotFound("not_found", "Resource not found", err)
	}

	if appErr, ok := errors.AsType[*domain.PasswordRequirementError](err); ok {
		return domainerror.BadRequest(
			"identity_weak_password",
			"Password does not meet complexity requirements: "+strings.Join(appErr.Requirements, ", "),
			err,
		)
	}

	// CanLogin returns a *AccountLockedError (the lock's end is in it), which the
	// sentinel switch below can't match.
	if lockedErr, ok := errors.AsType[*domain.AccountLockedError](err); ok {
		return domainerror.Forbidden(
			"identity_account_locked",
			"Account is locked until "+lockedErr.LockedUntil,
			err,
		)
	}

	// Account errors
	switch {
	case errors.Is(err, domain.ErrAccountNotFound):
		return domainerror.NotFound("identity_account_not_found", "Account not found", err)
	case errors.Is(err, domain.ErrAccountLocked):
		return domainerror.Forbidden("identity_account_locked", "Account is locked", err)
	case errors.Is(err, domain.ErrAccountSuspended):
		return domainerror.Forbidden("identity_account_suspended", "Account is suspended", err)
	case errors.Is(err, domain.ErrAccountDeleted):
		return domainerror.Forbidden("identity_account_deleted", "Account is deleted", err)
	case errors.Is(err, domain.ErrAccountNotVerified):
		return domainerror.Forbidden("identity_account_not_verified", "Account is not verified", err)
	case errors.Is(err, domain.ErrAccountAlredyVerified):
		return domainerror.BadRequest("identity_account_already_verified", "Account is already verified", err)
	}

	// Credential errors
	switch {
	case errors.Is(err, domain.ErrInvalidCredentials):
		return domainerror.Unauthorized("identity_invalid_credentials", "Invalid credentials", err)
	case errors.Is(err, domain.ErrInvalidEmailFormat):
		return domainerror.BadRequest("identity_invalid_email_format", "Invalid email format", err)
	case errors.Is(err, domain.ErrInvalidPhoneNumberFormat):
		return domainerror.BadRequest("identity_invalid_phone_number_format", "Invalid phone number format", err)
	}

	// Verification errors
	switch {
	case errors.Is(err, domain.ErrInvalidVerificationCode):
		return domainerror.BadRequest("identity_invalid_verification_code", "Invalid verification code", err)
	case errors.Is(err, domain.ErrVerificationCodeNotFound):
		return domainerror.NotFound("identity_verification_code_not_found", "Verification code not found", err)
	case errors.Is(err, domain.ErrVerificationExpired):
		return domainerror.BadRequest("identity_verification_code_expired", "Verification code has expired", err)
	case errors.Is(err, domain.ErrVerificationCodeUsed):
		return domainerror.BadRequest("identity_verification_code_used", "Verification code has already been used", err)
	case errors.Is(err, domain.ErrTooManyVerificationAttempts):
		return domainerror.BadRequest("identity_too_many_verification_attempts", "Too many verification attempts", err)
	}

	// Token errors
	switch {
	case errors.Is(err, domain.ErrRefreshTokenNotFound):
		return domainerror.Unauthorized("identity_refresh_token_not_found", "Refresh token not found", err)
	case errors.Is(err, domain.ErrRefreshTokenInvalid):
		return domainerror.Unauthorized("identity_refresh_token_invalid", "Refresh token is invalid", err)
	case errors.Is(err, domain.ErrRefreshTokenExpired):
		return domainerror.Unauthorized("identity_refresh_token_expired", "Refresh token has expired", err)
	case errors.Is(err, domain.ErrRefreshTokenRevoked):
		return domainerror.Unauthorized("identity_refresh_token_revoked", "Refresh token has been revoked", err)
	}

	// Default to internal error with wrapped error for debugging
	return domainerror.InternalWithErr(err)
}
