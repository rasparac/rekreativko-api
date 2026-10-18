package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewAccount(t *testing.T) {

	testCase := []struct {
		name        string
		email       string
		phoneNumber string
		password    string
		wantErr     bool
	}{
		{
			name:     "valid account with email only",
			email:    "user@example.com",
			password: "StrongP@ssw0rd",
		},
		{
			name:        "valid account with phone number only",
			phoneNumber: "+1234567890",
			password:    "StrongP@ssw0rd",
		},
		{
			name:        "with email and phone number",
			email:       "user@example.com",
			phoneNumber: "+1234567890",
			password:    "StrongP@ssw0rd",
		},
		{
			name:     "invalid account with no credentials",
			password: "StrongP@ssw0rd",
			wantErr:  true,
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {

			email, err := NewEmail(tc.email)
			require.NoError(t, err)

			phoneNumber, err := NewPhoneNumber(tc.phoneNumber)
			require.NoError(t, err)

			password, err := NewPassword(tc.password)
			require.NoError(t, err)

			account, err := NewAccount(email, phoneNumber, password)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, account)

			assert.Equal(t, AccountStatusPending, account.Status())
			assert.Equal(t, tc.email, account.Email().String())
			assert.Equal(t, tc.phoneNumber, account.PhoneNumber().String())
			assert.Zero(t, account.FailedLoginAttempts())
		})
	}
}

func Test_Account_ChangePassword(t *testing.T) {
	tests := []struct {
		name        string
		email       string
		newPassword string
		activate    bool
		wantErr     bool
	}{
		{
			name:        "change password for active account",
			email:       "user@example.com",
			newPassword: "StrongP@ssw0rd",
			activate:    true,
		},
		{
			name:        "change password for inactive account",
			email:       "user@example.com",
			newPassword: "StrongP@ssw0rd",
			activate:    false,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			email, err := NewEmail(tt.email)
			require.NoError(t, err)

			password := NewPasswordFromHash("hash")
			require.NoError(t, err)

			account, err := NewAccount(email, nil, password)
			require.NoError(t, err)

			verificationCode, err := NewVerificationCode(account, "test", CodeTypeEmail, time.Now())
			require.NoError(t, err)

			if tt.activate {
				account.Activate(verificationCode)
			}

			newPassword := NewPasswordFromHash(tt.newPassword)
			require.NoError(t, err)

			err = account.ChangePassword(newPassword)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

		})
	}
}

func TestAccount_ActivateUnverified(t *testing.T) {
	newPending := func(t *testing.T) *Account {
		t.Helper()

		email, err := NewEmail("player@example.com")
		require.NoError(t, err)
		phone, err := NewPhoneNumber("")
		require.NoError(t, err)

		account, err := NewAccount(email, phone, NewPasswordFromHash("hash"))
		require.NoError(t, err)
		return account
	}

	t.Run("activates and raises an unconfirmed verified event", func(t *testing.T) {
		account := newPending(t)

		require.NoError(t, account.ActivateUnverified(CodeTypeEmail))

		assert.True(t, account.IsActive())

		var verified *AccountVerifiedEvent
		for _, event := range account.Events() {
			if e, ok := event.(*AccountVerifiedEvent); ok {
				verified = e
			}
		}
		require.NotNil(t, verified)
		assert.True(t, verified.Unconfirmed)
		assert.Equal(t, CodeTypeEmail, verified.CodeType)
		assert.Equal(t, "player@example.com", verified.Email)
	})

	t.Run("an active account cannot be activated again", func(t *testing.T) {
		account := newPending(t)
		require.NoError(t, account.ActivateUnverified(CodeTypeEmail))

		assert.ErrorIs(t, account.ActivateUnverified(CodeTypeEmail), ErrAccountNotVerified)
	})
}
