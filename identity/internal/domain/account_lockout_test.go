package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLockoutTestAccount(t *testing.T) *Account {
	t.Helper()

	email, err := NewEmail("user@example.com")
	require.NoError(t, err)
	phone, err := NewPhoneNumber("")
	require.NoError(t, err)

	account, err := NewAccount(email, phone, NewPasswordFromHash("hash"))
	require.NoError(t, err)
	return account
}

func TestFailedLoginLockDuration(t *testing.T) {
	for attempts, want := range map[int]time.Duration{
		1: 0, 4: 0,
		5: 15 * time.Minute, 9: 15 * time.Minute,
		10: time.Hour, 14: time.Hour,
		15: 24 * time.Hour, 100: 24 * time.Hour,
	} {
		assert.Equal(t, want, failedLoginLockDuration(attempts), "after %d failed logins", attempts)
	}
}

// Wrong passwords back off 15 minutes -> 1 hour -> 1 day, and never lock for good.
func TestAccount_RecordFailedLoginAttempt_BacksOffButNeverLocksForGood(t *testing.T) {
	account := newLockoutTestAccount(t)

	lockFor := func() time.Duration {
		if account.LockedUntil() == nil {
			return 0
		}
		return time.Until(*account.LockedUntil()).Round(time.Minute)
	}

	// Each pass is one wrong password after the previous lock has run out.
	for attempt := 1; attempt <= 40; attempt++ {
		require.NoError(t, account.RecordFailedLoginAttempt("203.0.113.7"))

		want := failedLoginLockDuration(min(attempt, maxCountedFailedLogins))
		assert.Equal(t, want, lockFor(), "after wrong password %d", attempt)
		assert.LessOrEqual(t, lockFor(), 24*time.Hour, "no lock is longer than a day")

		// The lock runs out before the next try.
		account.lockedUntil = nil
	}

	assert.Equal(t, maxCountedFailedLogins, account.FailedLoginAttempts(), "the count stops at the longest tier")

	account.RecordSuccessfulLogin("203.0.113.7")
	assert.Zero(t, account.FailedLoginAttempts())
	assert.Nil(t, account.LockedUntil())
}
