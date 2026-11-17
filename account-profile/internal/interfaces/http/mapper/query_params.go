package mapper

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/account-profile/internal/application"
)

var (
	ErrMissingAccountProfilesFilter = errors.New("account_id or nickname is required")
	ErrTooManyProfileLookups        = fmt.Errorf("too many account_id and nickname values, at most %d per request", MaxProfileLookups)
)

// MaxProfileLookups is how many account_id and nickname values one request may
// carry; every match is returned, not a page. The cap bounds the answer and
// the URL (about 49 characters per id, and proxies refuse a request line past
// about 8 KB).
const MaxProfileLookups = 100

// QueryToProfilesFilter reads GET /profiles: a lookup by account_id and/or
// nickname (repeatable). There is no search by date of birth or location: it
// would find people by what their privacy settings hide (rekreativko-api-ep8.7).
func QueryToProfilesFilter(q url.Values) (application.ProfilesFilter, error) {
	var filter application.ProfilesFilter

	for _, v := range q["account_id"] {
		id, err := uuid.Parse(v)
		if err != nil {
			return filter, fmt.Errorf("invalid account_id: %w", err)
		}
		filter.AccountIDs = append(filter.AccountIDs, id)
	}

	filter.Nicknames = q["nickname"]

	switch lookups := len(filter.AccountIDs) + len(filter.Nicknames); {
	case lookups == 0:
		return filter, ErrMissingAccountProfilesFilter
	case lookups > MaxProfileLookups:
		return filter, ErrTooManyProfileLookups
	}

	return filter, nil
}
