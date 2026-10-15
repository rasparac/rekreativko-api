//go:build integration

package persistence_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/account-profile/internal/application"
	"github.com/rasparac/rekreativko-api/account-profile/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A nickname belongs to one profile at a time, case-insensitively; taking one
// that's in use is a 409 nickname_exists, not a 500.
func TestProfileService_NicknameIsUnique(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	db.RunMigrationsForService()

	txManager := db.CreateTransactionManager()
	logger := testutil.CreateLogger()
	svc := application.NewService(logger, txManager, persistence.NewAccountProfileManager(txManager, logger), domainevent.NewDomainEventManager(txManager, logger), testMetrics())
	ctx := context.Background()

	newProfile := func(t *testing.T) uuid.UUID {
		t.Helper()
		accountID := uuid.New()
		_, err := svc.CreateProfile(ctx, application.CreateProfileParams{AccountID: accountID})
		require.NoError(t, err)
		return accountID
	}
	setNickname := func(accountID uuid.UUID, nickname string) error {
		return svc.UpdateProfile(ctx, accountID, application.UpdateProfileParams{Nickname: &nickname})
	}
	requireCode := func(t *testing.T, err error, code string) {
		t.Helper()
		var appErr *domainerror.AppError
		require.True(t, errors.As(err, &appErr), "expected an AppError, got %v", err)
		assert.Equal(t, code, appErr.Code)
	}

	first, second := newProfile(t), newProfile(t)
	newProfile(t) // profiles without a nickname don't clash with each other

	require.NoError(t, setNickname(first, "igorb"))
	require.NoError(t, setNickname(first, "igorb"), "saving your own nickname again is fine")

	requireCode(t, setNickname(second, "igorb"), "nickname_exists")
	requireCode(t, setNickname(second, "IgorB"), "nickname_exists")

	require.NoError(t, setNickname(first, "igor"), "a nickname can be changed...")
	assert.NoError(t, setNickname(second, "igorb"), "...which frees the old one")

	requireCode(t, setNickname(second, "ab"), "nickname_too_short")
}
