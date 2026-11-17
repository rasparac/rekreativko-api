//go:build integration

package persistence_test

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/account-profile/internal/application"
	"github.com/rasparac/rekreativko-api/account-profile/internal/domain"
	"github.com/rasparac/rekreativko-api/account-profile/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/account-profile/internal/interfaces/http/mapper"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type profileTestEnv struct {
	ctx      context.Context
	profiles interface {
		CreateProfile(context.Context, application.CreateProfileParams) (*domain.AccountProfile, error)
		UpdateProfile(context.Context, uuid.UUID, application.UpdateProfileParams) error
		GetProfile(context.Context, application.ProfileFilter) (*domain.AccountProfile, error)
		GetProfiles(context.Context, application.ProfilesFilter) ([]*domain.AccountProfile, error)
	}
	settings interface {
		CreateSettings(context.Context, application.CreateAccountSettingsParams) error
		UpdateSettings(context.Context, uuid.UUID, application.UpdateAccountSettingsParams) error
	}
}

func setupProfileTest(t *testing.T) *profileTestEnv {
	t.Helper()

	db := testutil.NewTestDatabase(t)
	db.RunMigrationsForService()

	txManager := db.CreateTransactionManager()
	logger := testutil.CreateLogger()
	settingsRepo := persistence.NewAccountProfileSettingsManager(txManager)

	return &profileTestEnv{
		ctx:      context.Background(),
		profiles: application.NewService(logger, txManager, persistence.NewAccountProfileManager(txManager, logger), settingsRepo, domainevent.MustNewEventWriter(txManager, logger, "account_profile"), testMetrics()),
		settings: application.NewAccountSettingsService(settingsRepo, txManager, logger, testMetrics()),
	}
}

// filledProfile is an account with every profile field set and default
// settings, as after account.verified and a full profile edit.
func (e *profileTestEnv) filledProfile(t *testing.T, nickname string) uuid.UUID {
	t.Helper()

	id := uuid.New()
	_, err := e.profiles.CreateProfile(e.ctx, application.CreateProfileParams{AccountID: id})
	require.NoError(t, err)
	require.NoError(t, e.settings.CreateSettings(e.ctx, application.CreateAccountSettingsParams{AccountID: id}))

	fullName, bio, city, country := "Ana Anić", "I run", "Belgrade", "Serbia"
	lat, lng := 44.81, 20.46
	dob := time.Date(1990, 5, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, e.profiles.UpdateProfile(e.ctx, id, application.UpdateProfileParams{
		Nickname:         &nickname,
		FullName:         &fullName,
		Bio:              &bio,
		DateOfBirth:      &dob,
		Location:         &application.Location{City: &city, Country: &country, Latitude: &lat, Longitude: &lng},
		ProfilePicture:   &application.ProfilePicture{URL: "https://example.com/a.jpg"},
		ActivityInterest: []application.ActivityInterest{{Name: "running", Level: "beginner"}},
	}))

	return id
}

func (e *profileTestEnv) getAs(t *testing.T, viewer, owner uuid.UUID) *domain.AccountProfile {
	t.Helper()

	p, err := e.profiles.GetProfile(e.ctx, application.ProfileFilter{AccountID: &owner, ViewerID: viewer})
	require.NoError(t, err)
	return p
}

// The app resolves a whole roster with GET /profiles?account_id=a&account_id=b...
// and reads only items: every requested profile comes back in one answer.
func TestGetProfiles_LookupReturnsEveryRequestedProfile(t *testing.T) {
	env := setupProfileTest(t)

	const roster = 25
	query := url.Values{}
	for range roster {
		id := uuid.New()
		_, err := env.profiles.CreateProfile(env.ctx, application.CreateProfileParams{AccountID: id})
		require.NoError(t, err)
		query.Add("account_id", id.String())
	}

	// Through the same parsing the endpoint uses.
	filter, err := mapper.QueryToProfilesFilter(query)
	require.NoError(t, err)

	profiles, err := env.profiles.GetProfiles(env.ctx, filter)
	require.NoError(t, err)
	assert.Len(t, profiles, roster)
}

func TestProfilePrivacy(t *testing.T) {
	env := setupProfileTest(t)
	viewer := uuid.New()

	t.Run("by default another account sees only the nickname and picture", func(t *testing.T) {
		owner := env.filledProfile(t, "private_ana")

		p := env.getAs(t, viewer, owner)

		require.NotNil(t, p.Nickname())
		assert.Equal(t, "private_ana", p.Nickname().Value())
		require.NotNil(t, p.ProfilePicture())
		assert.Nil(t, p.FullName())
		assert.Empty(t, p.Bio())
		assert.Empty(t, p.ActivityInterests())
		assert.Nil(t, p.Location())
		assert.Nil(t, p.DateOfBirth())
	})

	t.Run("a public profile with its location shown still hides the date of birth and coordinates", func(t *testing.T) {
		owner := env.filledProfile(t, "public_ana")
		require.NoError(t, env.settings.UpdateSettings(env.ctx, owner, application.UpdateAccountSettingsParams{Settings: map[string]any{
			domain.SettingPrivacyProfilePublic: true,
			domain.SettingPrivacyShowLocation:  true,
		}}))

		p := env.getAs(t, viewer, owner)

		require.NotNil(t, p.FullName())
		assert.Equal(t, "I run", p.Bio())
		assert.Len(t, p.ActivityInterests(), 1)
		require.NotNil(t, p.Location())
		assert.Equal(t, "Belgrade", p.Location().City())
		assert.False(t, p.Location().HasCoordinates())
		assert.Nil(t, p.DateOfBirth())
	})

	t.Run("the owner sees its whole profile", func(t *testing.T) {
		owner := env.filledProfile(t, "own_ana")

		p := env.getAs(t, owner, owner)

		assert.NotNil(t, p.FullName())
		assert.NotNil(t, p.DateOfBirth())
		require.NotNil(t, p.Location())
		assert.True(t, p.Location().HasCoordinates())
	})

	t.Run("an account without settings is private", func(t *testing.T) {
		owner := uuid.New()
		_, err := env.profiles.CreateProfile(env.ctx, application.CreateProfileParams{AccountID: owner})
		require.NoError(t, err)
		fullName := "No Settings"
		require.NoError(t, env.profiles.UpdateProfile(env.ctx, owner, application.UpdateProfileParams{FullName: &fullName}))

		assert.Nil(t, env.getAs(t, viewer, owner).FullName())
	})

	t.Run("a roster lookup applies each owner's settings and leaves the viewer's own profile whole", func(t *testing.T) {
		me := env.filledProfile(t, "roster_me")
		private := env.filledProfile(t, "roster_private")
		public := env.filledProfile(t, "roster_public")
		require.NoError(t, env.settings.UpdateSettings(env.ctx, public, application.UpdateAccountSettingsParams{Settings: map[string]any{
			domain.SettingPrivacyProfilePublic: true,
		}}))

		profiles, err := env.profiles.GetProfiles(env.ctx, application.ProfilesFilter{
			AccountIDs: []uuid.UUID{me, private, public},
			ViewerID:   me,
		})
		require.NoError(t, err)
		require.Len(t, profiles, 3)

		byID := make(map[uuid.UUID]*domain.AccountProfile, len(profiles))
		for _, p := range profiles {
			byID[p.ID()] = p
		}
		assert.NotNil(t, byID[me].DateOfBirth(), "own profile")
		assert.Nil(t, byID[private].FullName())
		assert.NotNil(t, byID[public].FullName())
		assert.Nil(t, byID[public].Location(), "show_location is still off")
	})
}
