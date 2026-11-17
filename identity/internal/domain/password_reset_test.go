package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPasswordReset(t *testing.T) {
	t.Run("an active account gets a code, sent to its address", func(t *testing.T) {
		account := newActiveAccount(t)

		reset, err := NewPasswordReset(account, "123456", CodeTypeEmail)

		require.NoError(t, err)
		assert.WithinDuration(t, time.Now().Add(PasswordResetTTL), reset.ExpiresAt(), time.Second)
		require.Len(t, reset.Events(), 1)
		event, ok := reset.Events()[0].(*PasswordResetRequestedEvent)
		require.True(t, ok)
		assert.Equal(t, IdentityPasswordResetRequestedEvent, event.GetEventType())
		assert.Equal(t, "123456", event.Code)
		assert.Equal(t, CodeTypeEmail, event.CodeType)
		assert.Equal(t, "player@example.com", event.Email)
		assert.Equal(t, account.ID(), event.AccountID)
	})

	t.Run("an account that isn't active gets none", func(t *testing.T) {
		account := newActiveAccount(t)
		require.NoError(t, account.Delete(0))

		_, err := NewPasswordReset(account, "123456", CodeTypeEmail)
		assert.ErrorIs(t, err, ErrAccountNotVerified)
	})
}

func TestPasswordReset_Check(t *testing.T) {
	now := time.Now().UTC()
	reset := func(failed int) *PasswordReset {
		return ReconstructPasswordReset(uuid.New(), "123456", CodeTypeEmail, failed, now.Add(time.Minute), now)
	}

	t.Run("the right code", func(t *testing.T) {
		assert.NoError(t, reset(0).Check("123456", now))
	})

	t.Run("a wrong code counts", func(t *testing.T) {
		r := reset(0)
		assert.ErrorIs(t, r.Check("000000", now), ErrInvalidVerificationCode)
		assert.Equal(t, 1, r.FailedAttempts())
	})

	t.Run("the 5th wrong code burns it, and then even the right one fails", func(t *testing.T) {
		r := reset(maxPasswordResetAttempts - 1)
		assert.ErrorIs(t, r.Check("000000", now), ErrTooManyVerificationAttempts)
		assert.True(t, r.Burned())
		assert.ErrorIs(t, r.Check("123456", now), ErrTooManyVerificationAttempts)
	})

	t.Run("an expired code", func(t *testing.T) {
		assert.ErrorIs(t, reset(0).Check("123456", now.Add(2*time.Minute)), ErrVerificationExpired)
	})
}

func TestPasswordReset_RecentlySent(t *testing.T) {
	sent := time.Now().UTC()
	r := ReconstructPasswordReset(uuid.New(), "123456", CodeTypeEmail, 0, sent.Add(PasswordResetTTL), sent)

	assert.True(t, r.RecentlySent(sent.Add(PasswordResetResendAfter-time.Second)))
	assert.False(t, r.RecentlySent(sent.Add(PasswordResetResendAfter)))
}

// The code proved the address is the user's, so a lock from someone guessing
// their password must not keep them out.
func TestAccount_ResetPassword_LiftsTheLoginLock(t *testing.T) {
	account := newActiveAccount(t)
	for range 5 {
		require.NoError(t, account.RecordFailedLoginAttempt("10.0.0.1"))
	}
	require.True(t, account.IsLocked())
	account.ClearEvents()

	require.NoError(t, account.ResetPassword(NewPasswordFromHash("new-hash")))

	assert.False(t, account.IsLocked())
	assert.Zero(t, account.FailedLoginAttempts())
	assert.Equal(t, "new-hash", account.Password().String())
	require.Len(t, account.Events(), 1)
	changed, ok := account.Events()[0].(*PasswordChangedEvent)
	require.True(t, ok)
	assert.Equal(t, CodeTypeEmail, changed.CodeType, "notifications needs the channel and address")
	assert.Equal(t, "player@example.com", changed.Email)
	assert.Equal(t, account.ID(), changed.AccountID)
}
