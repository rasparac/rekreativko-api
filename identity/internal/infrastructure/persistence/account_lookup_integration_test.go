//go:build integration

package persistence_test

import (
	"testing"

	"github.com/rasparac/rekreativko-api/identity/internal/application"
	"github.com/rasparac/rekreativko-api/identity/internal/domain"
	"github.com/rasparac/rekreativko-api/identity/internal/infrastructure/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A deleted account keeps its email and phone until the erasure, but both are
// free to register again. Lookups by address must find the new account.

func TestAccountLookup_LoginAfterReRegistrationIsTheNewAccount(t *testing.T) {
	e := setupErasureTest(t)

	deleted := e.activeAccount(t, "again@example.com", "+38591111111")
	require.NoError(t, e.svc.DeleteAccount(e.ctx, deleted))

	// activeAccount logs in by email itself, so this already needs the fix.
	current := e.activeAccount(t, "again@example.com", "+38591111111")
	require.NotEqual(t, deleted, current)

	byEmail, err := e.svc.Login(e.ctx, application.LoginParams{Email: "again@example.com", Password: correctPassword})
	require.NoError(t, err)
	assert.Equal(t, current, byEmail.AccountID())

	byPhone, err := e.svc.Login(e.ctx, application.LoginParams{PhoneNumber: "+38591111111", Password: correctPassword})
	require.NoError(t, err)
	assert.Equal(t, current, byPhone.AccountID())
}

func TestAccountLookup_ResendAfterReRegistrationGoesToTheNewAccount(t *testing.T) {
	e := setupErasureTest(t)

	deleted := e.activeAccount(t, "again@example.com", "")
	require.NoError(t, e.svc.DeleteAccount(e.ctx, deleted))
	codesOfDeleted := e.stored(t, deleted).codes

	current, err := e.svc.Register(e.ctx, application.RegistrationParams{Email: "again@example.com", Password: correctPassword})
	require.NoError(t, err)
	codesOfCurrent := e.stored(t, current).codes

	_, err = e.svc.ResendVerificationCode(e.ctx, application.ResendVerificationCodeParams{Type: "email", Identity: "again@example.com"})
	require.NoError(t, err)

	assert.Equal(t, codesOfCurrent+1, e.stored(t, current).codes, "a new code for the new account")
	assert.Equal(t, codesOfDeleted, e.stored(t, deleted).codes, "none for the deleted one")
}

func TestAccountLookup_ADeletedAccountIsFoundOnlyByID(t *testing.T) {
	e := setupErasureTest(t)

	deleted := e.activeAccount(t, "gone@example.com", "+38592222222")
	require.NoError(t, e.svc.DeleteAccount(e.ctx, deleted))

	email, phone := "gone@example.com", "+38592222222"
	for name, filter := range map[string]persistence.AccountFilter{
		"by email": {Email: &email},
		"by phone": {PhoneNumber: &phone},
	} {
		_, err := e.accounts.GetBy(e.ctx, filter)
		assert.ErrorIs(t, err, domain.ErrAccountNotFound, name)
	}

	account, err := e.accounts.GetBy(e.ctx, persistence.AccountFilter{UUID: &deleted})
	require.NoError(t, err)
	assert.Equal(t, domain.AccountStatusDeleted, account.Status())

	_, err = e.svc.Login(e.ctx, application.LoginParams{Email: "gone@example.com", Password: correctPassword})
	assert.ErrorIs(t, err, domain.ErrAccountNotFound, "logging in finds no account, like an unknown address")
}
