package mapper

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/account-profile/application"
)

var ErrMissingAccountProfilesFilter = errors.New("missing account profile filter")

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
		hasFilter = true
	}

	// Sorting
	if v := q.Get("sort_by"); v != "" {
		filter.SortBy = &v
		hasFilter = true
	}

	if v := q.Get("sort_order"); v != "" {
		filter.SortOrder = &v
		hasFilter = true
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
	filter.Limit = 20
	filter.Offset = 0

	if v := q.Get("limit"); v != "" {
		limit, err := strconv.Atoi(v)
		if err != nil || limit < 0 {
			return filter, fmt.Errorf("invalid limit")
		}
		filter.Limit = limit
		hasFilter = true
	}

	if v := q.Get("offset"); v != "" {
		offset, err := strconv.Atoi(v)
		if err != nil || offset < 0 {
			return filter, fmt.Errorf("invalid offset")
		}
		filter.Offset = offset
		hasFilter = true
	}

	if !hasFilter {
		return filter, ErrMissingAccountProfilesFilter
	}

	return filter, nil
}
