package mapper

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/account-profile/internal/application"
	"github.com/rasparac/rekreativko-api/shared/api"
)

var (
	ErrMissingAccountProfilesFilter = errors.New("missing account profile filter")
	ErrTooManyProfileLookups        = fmt.Errorf("too many account_id and nickname values, at most %d per request", MaxProfileLookups)
)

// MaxProfileLookups is how many account_id and nickname values one request may
// carry. A request with any of them is a lookup: every match is returned, not
// a page. The cap bounds the answer and the URL (about 49 characters per id,
// and proxies refuse a request line past about 8 KB).
const MaxProfileLookups = 100

// QueryToProfilesFilter reads GET /profiles. Only the criteria that select
// profiles (account_id, nickname, dob_gt, dob_lt, country, city) count as a
// filter; limit, page_token, sort_by, sort_order and include_deleted only shape
// the answer, so on their own they must not list everyone.
func QueryToProfilesFilter(q url.Values) (application.ProfilesFilter, error) {
	var filter application.ProfilesFilter
	var hasFilter bool

	// Account IDs
	for _, v := range q["account_id"] {
		id, err := uuid.Parse(v)
		if err != nil {
			return filter, fmt.Errorf("invalid account_id: %w", err)
		}
		filter.AccountIDs = append(filter.AccountIDs, id)
		hasFilter = true
	}

	// Nicknames
	filter.Nicknames = q["nickname"]

	if len(filter.Nicknames) > 0 {
		hasFilter = true
	}

	// Date of birth filters
	if v := q.Get("dob_gt"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return filter, fmt.Errorf("invalid dob_gt")
		}
		filter.DateOfBirthOver = &t
		hasFilter = true
	}

	if v := q.Get("dob_lt"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return filter, fmt.Errorf("invalid dob_lt")
		}
		filter.DateOfBirthUnder = &t
		hasFilter = true
	}

	// Include deleted
	if v := q.Get("include_deleted"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return filter, fmt.Errorf("invalid include_deleted")
		}
		filter.IncludeDeleted = &b
	}

	// Sorting
	if v := q.Get("sort_by"); v != "" {
		filter.SortBy = &v
	}

	if v := q.Get("sort_order"); v != "" {
		filter.SortOrder = &v
	}

	// Location
	if v := q.Get("country"); v != "" {
		filter.LocationCountry = &v
		hasFilter = true
	}

	if v := q.Get("city"); v != "" {
		filter.LocationCity = &v
		hasFilter = true
	}

	// Pagination (with defaults)
	limit, pageToken, err := api.ParsePageParams(q, 20)
	if err != nil {
		return filter, fmt.Errorf("invalid limit")
	}
	filter.Limit = limit
	filter.PageToken = pageToken

	if !hasFilter {
		return filter, ErrMissingAccountProfilesFilter
	}

	// Asking for given accounts is a lookup, not a search: the caller wants all
	// of them, so it is not paged (a roster over the page size would lose
	// profiles the client never learns about). The cap keeps it bounded.
	if lookups := len(filter.AccountIDs) + len(filter.Nicknames); lookups > 0 {
		if lookups > MaxProfileLookups {
			return filter, ErrTooManyProfileLookups
		}
		filter.Limit = MaxProfileLookups
		filter.PageToken = ""
	}

	return filter, nil
}
