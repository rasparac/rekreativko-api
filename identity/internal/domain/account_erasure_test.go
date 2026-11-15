package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newActiveAccount(t *testing.T) *Account {
	t.Helper()

	email, err := NewEmail("player@example.com")
	require.NoError(t, err)
	phone, err := NewPhoneNumber("+38591234567")
	require.NoError(t, err)

	now := time.Now().UTC()
	return ReconstructAccount(
		uuid.New(), email, phone, NewPasswordFromHash("hash"),
		AccountStatusActive, 0, nil, now, now, nil, nil, nil,
	)
}

func TestAccount_Delete_SchedulesTheErasure(t *testing.T) {
	account := newActiveAccount(t)

	before := time.Now().UTC()
	require.NoError(t, account.Delete(30*24*time.Hour))

	assert.Equal(t, AccountStatusDeleted, account.Status())
	require.NotNil(t, account.DeletedAt())
	require.NotNil(t, account.EraseAfter())
	assert.WithinDuration(t, before.Add(30*24*time.Hour), *account.EraseAfter(), time.Second)
	assert.Nil(t, account.ErasedAt(), "erased only once the grace period has passed")
	assert.Equal(t, "player@example.com", account.Email().String(), "data kept during the grace period")

	require.Len(t, account.Events(), 1)
	assert.Equal(t, IdentityAccountDeletedEvent, account.Events()[0].GetEventType())
}

func TestAccount_Delete_ZeroGraceIsDueRightAway(t *testing.T) {
	account := newActiveAccount(t)

	require.NoError(t, account.Delete(0))

	require.NotNil(t, account.EraseAfter())
	assert.Equal(t, *account.DeletedAt(), *account.EraseAfter())
}

// A deletion redone after a backup restore keeps the user's original request
// time, so the grace period isn't restarted.
func TestAccount_DeleteAt_CountsTheGraceFromTheOriginalDeletion(t *testing.T) {
	account := newActiveAccount(t)
	asked := time.Date(2026, 11, 2, 9, 30, 0, 0, time.FixedZone("CET", 3600))

	require.NoError(t, account.DeleteAt(asked, 30*24*time.Hour))

	assert.Equal(t, AccountStatusDeleted, account.Status())
	assert.Equal(t, asked.UTC(), *account.DeletedAt())
	assert.Equal(t, asked.UTC().Add(30*24*time.Hour), *account.EraseAfter())
	require.Len(t, account.Events(), 1)
	assert.Equal(t, IdentityAccountDeletedEvent, account.Events()[0].GetEventType())

	assert.ErrorIs(t, account.DeleteAt(asked, 0), ErrAccountDeleted, "an account is deleted once")
}

func TestAccount_Erase(t *testing.T) {
	t.Run("removes the personal data and raises an erased event with the id only", func(t *testing.T) {
		account := newActiveAccount(t)
		require.NoError(t, account.Delete(0))
		account.ClearEvents()

		require.NoError(t, account.Erase())

		assert.True(t, account.Email().IsEmpty())
		assert.True(t, account.PhoneNumber().IsEmpty())
		assert.Equal(t, ErasedPasswordHash, account.Password().String())
		require.NotNil(t, account.ErasedAt())
		assert.Equal(t, AccountStatusDeleted, account.Status())
		assert.ErrorIs(t, account.CanLogin(), ErrAccountDeleted)

		require.Len(t, account.Events(), 1)
		erased, ok := account.Events()[0].(*AccountErasedEvent)
		require.True(t, ok)
		assert.Equal(t, IdentityAccountErasedEvent, erased.GetEventType())
		assert.Equal(t, account.ID(), erased.AccountID)

		payload, err := json.Marshal(erased)
		require.NoError(t, err)
		assert.NotContains(t, string(payload), "player@example.com")
		assert.NotContains(t, string(payload), "+38591234567")
	})

	t.Run("an account that is not deleted cannot be erased", func(t *testing.T) {
		account := newActiveAccount(t)

		assert.ErrorIs(t, account.Erase(), ErrAccountNotDeleted)
		assert.Equal(t, "player@example.com", account.Email().String())
		assert.Empty(t, account.Events())
	})

	t.Run("an erased account cannot be erased again", func(t *testing.T) {
		account := newActiveAccount(t)
		require.NoError(t, account.Delete(0))
		require.NoError(t, account.Erase())
		account.ClearEvents()

		assert.ErrorIs(t, account.Erase(), ErrAccountAlreadyErased)
		assert.Empty(t, account.Events())
	})
}
