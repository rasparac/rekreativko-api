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

// Only what selects profiles is a filter. Without one the request would list
// every profile, whatever else it asks for.
func TestQueryToProfilesFilter_ShapingParamsAreNotFilters(t *testing.T) {
	for _, raw := range []string{
		"",
		"limit=100000",
		"limit=0",
		"page_token=abc",
		"sort_by=nickname",
		"sort_order=desc",
		"include_deleted=false",
		"limit=50&sort_by=nickname&sort_order=asc&include_deleted=true",
	} {
		_, err := QueryToProfilesFilter(parse(t, raw))
		assert.ErrorIs(t, err, ErrMissingAccountProfilesFilter, "%q", raw)
	}
}

func TestQueryToProfilesFilter_SelectingParamsAreFilters(t *testing.T) {
	for _, raw := range []string{
		"account_id=" + uuid.NewString(),
		"nickname=jd",
		"country=HR",
		"city=Zagreb",
		"dob_gt=1990-01-01",
		"dob_lt=2000-01-01",
	} {
		_, err := QueryToProfilesFilter(parse(t, raw))
		assert.NoError(t, err, "%q", raw)
	}
}

func TestQueryToProfilesFilter_SearchIsPaged(t *testing.T) {
	filter, err := QueryToProfilesFilter(parse(t, "city=Zagreb"))
	require.NoError(t, err)
	assert.Equal(t, 20, filter.Limit)

	filter, err = QueryToProfilesFilter(parse(t, "city=Zagreb&limit=5&page_token=tok"))
	require.NoError(t, err)
	assert.Equal(t, 5, filter.Limit)
	assert.Equal(t, "tok", filter.PageToken)
}

// A lookup returns everything asked for: the page size must not cut it, and a
// stray page_token must not skip part of it.
func TestQueryToProfilesFilter_LookupIsNotPaged(t *testing.T) {
	ids := url.Values{}
	for range 25 {
		ids.Add("account_id", uuid.NewString())
	}

	filter, err := QueryToProfilesFilter(ids)
	require.NoError(t, err)
	assert.Len(t, filter.AccountIDs, 25)
	assert.Equal(t, MaxProfileLookups, filter.Limit, "not the default 20")

	ids.Set("limit", "5")
	ids.Set("page_token", "tok")
	filter, err = QueryToProfilesFilter(ids)
	require.NoError(t, err)
	assert.Equal(t, MaxProfileLookups, filter.Limit, "a client's limit does not cut a lookup")
	assert.Empty(t, filter.PageToken)

	filter, err = QueryToProfilesFilter(parse(t, "nickname=jd&limit=1"))
	require.NoError(t, err)
	assert.Equal(t, MaxProfileLookups, filter.Limit)
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

// A search is paged and a page is bounded: limit=0 is the default, not
// everything, and a huge limit is cut.
func TestQueryToProfilesFilter_SearchLimitIsBounded(t *testing.T) {
	for raw, want := range map[string]int{
		"city=Zagreb&limit=0":         20,
		"city=Zagreb&limit=100":       100,
		"city=Zagreb&limit=100000":    100,
		"city=Zagreb&limit=100000000": 100,
	} {
		filter, err := QueryToProfilesFilter(parse(t, raw))
		require.NoError(t, err, raw)
		assert.Equal(t, want, filter.Limit, raw)
	}
}
