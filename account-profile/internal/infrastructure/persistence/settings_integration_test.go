//go:build integration

package persistence_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/account-profile/internal/application"
	"github.com/rasparac/rekreativko-api/account-profile/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Saving the values a user already has is a no-op, not a version conflict (500).
func TestAccountSettingsService_UpdateSettings(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	db.RunMigrationsForService()

	txManager := db.CreateTransactionManager()
	logger := testutil.CreateLogger()
	svc := application.NewAccountSettingsService(
		persistence.NewAccountProfileSettingsManager(txManager),
		txManager,
		logger,
		testMetrics(),
	)
	ctx := context.Background()

	// Settings belong to a profile (foreign key).
	profileSvc := application.NewService(logger, txManager, persistence.NewAccountProfileManager(txManager, logger), domainevent.NewDomainEventManager(txManager, logger), testMetrics())
	accountID := uuid.New()
	_, err := profileSvc.CreateProfile(ctx, application.CreateProfileParams{AccountID: accountID})
	require.NoError(t, err)

	require.NoError(t, svc.CreateSettings(ctx, application.CreateAccountSettingsParams{AccountID: accountID}))

	current := func(t *testing.T) (version int, emailEnabled string) {
		t.Helper()
		settings, err := svc.GetSettings(ctx, accountID)
		require.NoError(t, err)
		setting, ok := settings.GetSetting("notification.email.enabled")
		require.True(t, ok)
		return settings.Version(), setting.Value()
	}
	update := func(enabled bool) error {
		return svc.UpdateSettings(ctx, accountID, application.UpdateAccountSettingsParams{
			Settings: map[string]any{"notification.email.enabled": enabled},
		})
	}

	version, value := current(t)
	assert.Equal(t, "true", value)

	require.NoError(t, update(true), "the value it already has")
	require.NoError(t, update(true))
	gotVersion, gotValue := current(t)
	assert.Equal(t, version, gotVersion)
	assert.Equal(t, "true", gotValue)

	require.NoError(t, update(false))
	gotVersion, gotValue = current(t)
	assert.Equal(t, version+1, gotVersion)
	assert.Equal(t, "false", gotValue)

	require.NoError(t, update(false), "unchanged again, after a real change")
	gotVersion, _ = current(t)
	assert.Equal(t, version+1, gotVersion)
}
