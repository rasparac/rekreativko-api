package mapper

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parse(t *testing.T, raw string) url.Values {
	t.Helper()
	q, err := url.ParseQuery(raw)
	require.NoError(t, err)
	return q
}

// Only account_id and nickname select profiles. The removed search filters
// (date of birth, location) and paging params select nothing, so on their own
// they are refused rather than listing everyone.
func TestQueryToProfilesFilter_OnlyALookupIsAFilter(t *testing.T) {
	for _, raw := range []string{
		"",
		"limit=100000",
		"page_token=abc",
		"sort_by=nickname",
		"country=HR",
		"city=Zagreb",
		"dob_gt=1990-01-01",
		"dob_lt=2000-01-01",
	} {
		_, err := QueryToProfilesFilter(parse(t, raw))
		assert.ErrorIs(t, err, ErrMissingAccountProfilesFilter, "%q", raw)
	}
}

func TestQueryToProfilesFilter_Lookup(t *testing.T) {
	id := uuid.New()

	filter, err := QueryToProfilesFilter(parse(t, "account_id="+id.String()+"&nickname=jd&nickname=ana"))
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{id}, filter.AccountIDs)
	assert.Equal(t, []string{"jd", "ana"}, filter.Nicknames)
}

func TestQueryToProfilesFilter_LookupIsCapped(t *testing.T) {
	ids := url.Values{}
	for range MaxProfileLookups {
		ids.Add("account_id", uuid.NewString())
	}
	_, err := QueryToProfilesFilter(ids)
	assert.NoError(t, err, "the cap itself is allowed")

	ids.Add("account_id", uuid.NewString())
	_, err = QueryToProfilesFilter(ids)
	assert.ErrorIs(t, err, ErrTooManyProfileLookups)

	// Ids and nicknames share the cap.
	mixed := url.Values{}
	for range MaxProfileLookups {
		mixed.Add("account_id", uuid.NewString())
	}
	mixed.Add("nickname", "jd")
	_, err = QueryToProfilesFilter(mixed)
	assert.ErrorIs(t, err, ErrTooManyProfileLookups)
	assert.Contains(t, ErrTooManyProfileLookups.Error(), fmt.Sprint(MaxProfileLookups))
}

func TestQueryToProfilesFilter_RejectsABadAccountID(t *testing.T) {
	_, err := QueryToProfilesFilter(parse(t, "account_id=not-a-uuid"))
	assert.Error(t, err)
}
