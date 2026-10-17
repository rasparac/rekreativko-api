package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
)

type Account struct {
	id                  uuid.UUID
	email               *Email
	phoneNumber         *PhoneNumber
	password            *Password
	status              AccountStatus
	failedLoginAttempts int
	lockedUntil         *time.Time
	createdAt           time.Time
	updatedAt           time.Time
	deletedAt           *time.Time

	events []domainevent.Event
}

func NewAccount(
	email *Email,
	phoneNumber *PhoneNumber,
	password *Password,
) (*Account, error) {
	if (email == nil || email.IsEmpty()) && (phoneNumber == nil || phoneNumber.IsEmpty()) {
		return nil, ErrInvalidCredentials
	}

	now := time.Now().UTC()

	a := &Account{
		id:                  uuid.New(),
		email:               email,
		phoneNumber:         phoneNumber,
		password:            password,
		failedLoginAttempts: 0,
		status:              AccountStatusPending,
		createdAt:           now,
		updatedAt:           now,
	}

	event := NewAccountRegisteredEvent(a)
	a.addEvent(event)

	return a, nil

}

func ReconstructAccount(
	id uuid.UUID,
	email *Email,
	phoneNumber *PhoneNumber,
	password *Password,
	status AccountStatus,
	failedLoginAttempts int,
	lockedUntil *time.Time,
	createdAt time.Time,
	updatedAt time.Time,
	deletedAt *time.Time,
) *Account {
	return &Account{
		id:                  id,
		email:               email,
		phoneNumber:         phoneNumber,
		password:            password,
		status:              status,
		failedLoginAttempts: failedLoginAttempts,
		lockedUntil:         lockedUntil,
		createdAt:           createdAt,
		updatedAt:           updatedAt,
		deletedAt:           deletedAt,
	}
}

func (a *Account) ID() uuid.UUID {
	return a.id
}

func (a *Account) Email() *Email {
	return a.email
}

func (a *Account) PhoneNumber() *PhoneNumber {
	return a.phoneNumber
}

func (a *Account) Password() *Password {
	return a.password
}

func (a *Account) Status() AccountStatus {
	return a.status
}

func (a *Account) LockedUntil() *time.Time {
	return a.lockedUntil
}

func (a *Account) CreatedAt() time.Time {
	return a.createdAt
}

func (a *Account) UpdatedAt() time.Time {
	return a.updatedAt
}

func (a *Account) DeletedAt() *time.Time {
	return a.deletedAt
}

func (a *Account) FailedLoginAttempts() int {
	return a.failedLoginAttempts
}

func (a *Account) Events() []domainevent.Event {
	return a.events
}

func (a *Account) ClearEvents() {
	a.events = make([]domainevent.Event, 0)
}

func (a *Account) HasEmail() bool {
	return a.email != nil && !a.email.IsEmpty()
}

// Business logic methods would go here

func (a *Account) Activate(code *VerificationCode) error {
	if a.status != AccountStatusPending {
		return ErrAccountNotVerified
	}

	a.status = AccountStatusActive
	a.touch()
	a.addEvent(NewAccountActivatedEvent(a))
	a.addEvent(NewAccountVerifiedEvent(a, code))

	return nil
}

func (a *Account) IsActive() bool {
	return a.status == AccountStatusActive
}

func (a *Account) Suspend(reason string) error {
	if !a.status.CanBeSuspended() {
		return ErrAccountSuspended
	}

	a.status = AccountStatusSuspended
	a.touch()
	a.addEvent(NewAccountSuspendedEvent(a, reason))

	return nil
}

func (a *Account) Delete() error {
	if !a.status.CanBeDeleted() {
		return ErrAccountDeleted
	}

	now := time.Now().UTC()

	a.status = AccountStatusDeleted
	a.deletedAt = &now
	a.touch()
	a.addEvent(NewAccountDeletedEvent(a))

	return nil
}

func (a *Account) Lock(duration time.Duration, reason string) error {
	lockUntil := time.Now().UTC().Add(duration)
	a.lockedUntil = &lockUntil
	a.touch()
	a.addEvent(NewAccountLockedEvent(a, reason, lockUntil))

	return nil
}

// failedLoginLockTiers is the lockout back-off: from atLeast consecutive failed
// logins on, the account is locked for lockFor. Each wrong password after a lock
// has expired locks it again, for as long as the tier of the new count says, so
// guessing gets slower the more is guessed - but no lock is ever longer than a
// day, and a successful login clears the count. (A permanent lock would let
// anyone who knows an email or phone number lock that person out for good.)
var failedLoginLockTiers = []struct {
	atLeast int
	lockFor time.Duration
}{
	{atLeast: 5, lockFor: 15 * time.Minute},
	{atLeast: 10, lockFor: time.Hour},
	{atLeast: 15, lockFor: 24 * time.Hour},
}

// maxCountedFailedLogins is where the count stops: the longest tier starts
// there, so counting further changes nothing (and the event reports it as int8).
const maxCountedFailedLogins = 15

// failedLoginLockDuration returns how long to lock an account after its
// attempts-th consecutive failed login; zero means not at all yet.
func failedLoginLockDuration(attempts int) time.Duration {
	var lockFor time.Duration
	for _, tier := range failedLoginLockTiers {
		if attempts >= tier.atLeast {
			lockFor = tier.lockFor
		}
	}
	return lockFor
}

func (a *Account) RecordFailedLoginAttempt(ipAddress string) error {
	if a.failedLoginAttempts < maxCountedFailedLogins {
		a.failedLoginAttempts++
	}
	a.touch()

	a.addEvent(
		NewLoginFailedEvent(a, ipAddress, "invalid credentials", int8(a.failedLoginAttempts)),
	)

	if lockFor := failedLoginLockDuration(a.failedLoginAttempts); lockFor > 0 {
		return a.Lock(lockFor, "too many failed login attempts")
	}

	return nil
}

func (a *Account) RecordSuccessfulLogin(ipAddress string) {
	a.failedLoginAttempts = 0
	a.lockedUntil = nil
	a.touch()

	a.addEvent(NewLoginSucceededEvent(a, ipAddress))
}

func (a *Account) Unlock() error {
	if !a.IsLocked() {
		return nil
	}

	a.failedLoginAttempts = 0
	a.lockedUntil = nil
	a.updatedAt = time.Now().UTC()
	a.addEvent(NewAccountUnlockedEvent(a))

	return nil
}

func (a *Account) IsLocked() bool {
	if a.lockedUntil == nil {
		return false
	}

	now := time.Now().UTC()

	if now.After(*a.lockedUntil) {
		a.lockedUntil = nil
		return false
	}

	return true
}

func (a *Account) ChangePassword(newPassword *Password) error {
	if a.status != AccountStatusActive {
		return ErrAccountNotVerified
	}

	a.password = newPassword
	a.updatedAt = time.Now().UTC()
	a.addEvent(NewPasswordChangedEvent(a))

	return nil
}

func (a *Account) CanLogin() error {
	if a.IsLocked() {
		return NewAccountLockedError(
			"account is temporarily locked",
			a.lockedUntil.Format(time.RFC3339),
		)
	}

	switch a.status {
	case AccountStatusActive:
		return nil
	case AccountStatusPending:
		return ErrAccountNotVerified
	case AccountStatusSuspended:
		return ErrAccountSuspended
	case AccountStatusDeleted:
		return ErrAccountDeleted
	default:
		return ErrAccountNotVerified
	}
}

func (a *Account) ValidatePassword(passwordHash string) bool {
	return a.password.String() == passwordHash
}

// helper methods

func (a *Account) touch() {
	a.updatedAt = time.Now().UTC()
}

func (a *Account) addEvent(event domainevent.Event) {
	a.events = append(a.events, event)
}
