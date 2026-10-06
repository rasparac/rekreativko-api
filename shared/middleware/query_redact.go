package middleware

import (
	"net/url"
	"slices"
	"strings"
)

// loggedQueryKeys are the query parameters whose values may be written to logs
// and traces: ids, enums, flags, paging and coarse filters. Every other
// parameter keeps its name but loses its value, so a secret (token, code,
// password) or personal data (coordinates, street, date of birth, free-text
// search) added to some endpoint later is never logged by accident - it has to
// be allowed here on purpose.
var loggedQueryKeys = map[string]struct{}{
	"status":              {},
	"role":                {},
	"limit":               {},
	"sort_by":             {},
	"sort_order":          {},
	"unread_only":         {},
	"include_deleted":     {},
	"is_recurring":        {},
	"activity_type":       {},
	"difficulty_level":    {},
	"country":             {},
	"city":                {},
	"radius_km":           {},
	"start_time_from":     {},
	"start_time_to":       {},
	"activity_group_id":   {},
	"session_id":          {},
	"session_template_id": {},
	"member_id":           {},
	"user_id":             {},
	"created_by_id":       {},
	"creator_id":          {},
	"attendee_id":         {},
}

const (
	redactedValue    = "[redacted]"
	unparseableQuery = "[unparseable]"
)

// RedactQuery returns rawQuery as it may be logged: parameters in
// loggedQueryKeys keep their values, the rest show "[redacted]". Keys come out
// sorted. A query string that can't be parsed is not logged at all, since it
// could hold anything.
func RedactQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}

	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return unparseableQuery
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	var parts []string
	for _, key := range keys {
		_, allowed := loggedQueryKeys[key]
		for _, value := range values[key] {
			shown := redactedValue
			if allowed {
				shown = url.QueryEscape(value)
			}
			parts = append(parts, url.QueryEscape(key)+"="+shown)
		}
	}

	return strings.Join(parts, "&")
}

// redactedTarget is the request target (path and query) as it may be logged or
// traced.
func redactedTarget(u *url.URL) string {
	if query := RedactQuery(u.RawQuery); query != "" {
		return u.EscapedPath() + "?" + query
	}

	return u.EscapedPath()
}
