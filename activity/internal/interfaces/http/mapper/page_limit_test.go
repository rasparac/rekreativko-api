package mapper

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every paginated list reads its limit through the same parsing, so none can be
// asked for more than a page, and limit=0 is not "no limit" (the repositories
// read 0 as "no LIMIT", which returned the whole table).
func TestListEndpoints_LimitIsBounded(t *testing.T) {
	// The discover endpoint also needs a position.
	where := "&lat=45.8&lng=16.0"

	parsers := map[string]func(url.Values) (int, error){
		"activity groups": func(q url.Values) (int, error) {
			p, err := QueryToListActivityGroupsParams(q)
			if err != nil {
				return 0, err
			}
			return p.Limit, nil
		},
		"activity groups discover": func(q url.Values) (int, error) {
			p, err := QueryToDiscoverActivityGroupsParams(q)
			if err != nil {
				return 0, err
			}
			return p.Limit, nil
		},
		"members": func(q url.Values) (int, error) {
			p, err := QueryToListMembersParams(q)
			if err != nil {
				return 0, err
			}
			return p.Limit, nil
		},
		"sessions": func(q url.Values) (int, error) {
			p, err := QueryToListSessionsParams(q)
			if err != nil {
				return 0, err
			}
			return p.Limit, nil
		},
		"sessions discover": func(q url.Values) (int, error) {
			p, err := QueryToDiscoverSessionsParams(q)
			if err != nil {
				return 0, err
			}
			return p.Limit, nil
		},
	}

	for name, parse := range parsers {
		t.Run(name, func(t *testing.T) {
			limitOf := func(raw string) int {
				q, err := url.ParseQuery(raw + where)
				require.NoError(t, err)
				limit, err := parse(q)
				require.NoError(t, err, raw)
				return limit
			}

			assert.Equal(t, 20, limitOf(""), "default")
			assert.Equal(t, 50, limitOf("limit=50"))
			assert.Equal(t, 20, limitOf("limit=0"), "0 is not unlimited")
			assert.Equal(t, 100, limitOf("limit=100"))
			assert.Equal(t, 100, limitOf("limit=101"))
			assert.Equal(t, 100, limitOf("limit=100000000"))

			q, err := url.ParseQuery("limit=-1" + where)
			require.NoError(t, err)
			_, err = parse(q)
			assert.Error(t, err, "negative")
		})
	}
}
