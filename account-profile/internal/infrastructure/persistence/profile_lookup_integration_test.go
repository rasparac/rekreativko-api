//go:build integration

package persistence_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/account-profile/internal/application"
	"github.com/rasparac/rekreativko-api/account-profile/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/account-profile/internal/interfaces/http/mapper"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The app resolves a whole roster with GET /profiles?account_id=a&account_id=b...
// and reads only items. With the default page size of 20 it lost every profile
// after the 20th, without a sign (it never reads next_page_token).
func TestGetProfiles_LookupReturnsEveryRequestedProfile(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	db.RunMigrationsForService()

	txManager := db.CreateTransactionManager()
	logger := testutil.CreateLogger()
	ctx := context.Background()
	svc := application.NewService(logger, txManager, persistence.NewAccountProfileManager(txManager, logger), domainevent.NewDomainEventManager(txManager, logger), testMetrics())

	const roster = 25
	query := url.Values{}
	for range roster {
		id := uuid.New()
		_, err := svc.CreateProfile(ctx, application.CreateProfileParams{AccountID: id})
		require.NoError(t, err)
		query.Add("account_id", id.String())
	}

	// Through the same parsing the endpoint uses.
	filter, err := mapper.QueryToProfilesFilter(query)
	require.NoError(t, err)

	profiles, nextPageToken, err := svc.GetProfiles(ctx, filter)
	require.NoError(t, err)

	assert.Len(t, profiles, roster)
	assert.Empty(t, nextPageToken, "a lookup is not paged")
}
