package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
)

const (
	// PasswordResetTTL is how long a forgot-password code can be used.
	PasswordResetTTL = 15 * time.Minute
	// PasswordResetResendAfter is how soon another code may be sent to the same
	// account, so the endpoint can't be used to flood someone's inbox.
	PasswordResetResendAfter = time.Minute
	// maxPasswordResetAttempts wrong guesses burn the code: a 6-digit code must
	// not be guessable within its lifetime.
	maxPasswordResetAttempts = 5
)

// PasswordReset is an account's outstanding forgot-password code. An account
// has at most one; a new request replaces it.
type PasswordReset struct {
	accountID      uuid.UUID
	code           string
	codeType       CodeType
	failedAttempts int
	expiresAt      time.Time
	createdAt      time.Time

	events []domainevent.Event
}

// NewPasswordReset issues a code for an active account and raises
// identity.password_reset.requested, which sends it to the account's address.
func NewPasswordReset(account *Account, code string, codeType CodeType) (*PasswordReset, error) {
	if !account.IsActive() {
		return nil, ErrAccountNotVerified
	}

	if code == "" || !codeType.IsValid() {
		return nil, ErrInvalidVerificationCode
	}

	now := time.Now().UTC()
	reset := &PasswordReset{
		accountID: account.ID(),
		code:      code,
		codeType:  codeType,
		expiresAt: now.Add(PasswordResetTTL),
		createdAt: now,
	}
	reset.events = []domainevent.Event{NewPasswordResetRequestedEvent(reset, account)}

	return reset, nil
}

func ReconstructPasswordReset(
	accountID uuid.UUID,
	code string,
	codeType CodeType,
	failedAttempts int,
	expiresAt time.Time,
	createdAt time.Time,
) *PasswordReset {
	return &PasswordReset{
		accountID:      accountID,
		code:           code,
		codeType:       codeType,
		failedAttempts: failedAttempts,
		expiresAt:      expiresAt,
		createdAt:      createdAt,
	}
}

func (r *PasswordReset) AccountID() uuid.UUID { return r.accountID }
func (r *PasswordReset) Code() string         { return r.code }
func (r *PasswordReset) CodeType() CodeType   { return r.codeType }
func (r *PasswordReset) FailedAttempts() int  { return r.failedAttempts }
func (r *PasswordReset) ExpiresAt() time.Time { return r.expiresAt }
func (r *PasswordReset) CreatedAt() time.Time { return r.createdAt }

// RecentlySent tells whether this code went out too recently for another one.
func (r *PasswordReset) RecentlySent(now time.Time) bool {
	return now.Before(r.createdAt.Add(PasswordResetResendAfter))
}

// Burned tells whether the code can no longer be used, however it is guessed.
func (r *PasswordReset) Burned() bool {
	return r.failedAttempts >= maxPasswordResetAttempts
}

// Check accepts the code the user typed. A wrong one counts as a failed
// attempt; the last allowed one burns the code (ErrTooManyVerificationAttempts).
func (r *PasswordReset) Check(code string, now time.Time) error {
	if r.Burned() {
		return ErrTooManyVerificationAttempts
	}

	if now.After(r.expiresAt) {
		return ErrVerificationExpired
	}

	if code != r.code {
		r.failedAttempts++
		if r.Burned() {
			return ErrTooManyVerificationAttempts
		}
		return ErrInvalidVerificationCode
	}

	return nil
}

func (r *PasswordReset) Events() []domainevent.Event { return r.events }

func (r *PasswordReset) ClearEvents() { r.events = nil }
