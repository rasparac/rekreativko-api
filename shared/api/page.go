package api

import (
	"fmt"
	"net/url"
	"strconv"
)

// Page is a generic paginated list envelope, reused across every list/discover endpoint.
type Page[T any] struct {
	Items         []T    `json:"items"`
	Limit         int    `json:"limit"`
	NextPageToken string `json:"next_page_token,omitempty"`
}

// NewPage builds a Page, mapping each domain item to its response type via mapOne.
func NewPage[TIn, TOut any](items []TIn, limit int, nextPageToken string, mapOne func(TIn) TOut) Page[TOut] {
	responses := make([]TOut, len(items))
	for i, item := range items {
		responses[i] = mapOne(item)
	}
	return Page[TOut]{
		Items:         responses,
		Limit:         limit,
		NextPageToken: nextPageToken,
	}
}

// ParsePageParams parses the standard "limit"/"page_token" pagination query params.
func ParsePageParams(q url.Values, defaultLimit int) (limit int, pageToken string, err error) {
	limit = defaultLimit
	if v := q.Get("limit"); v != "" {
		l, convErr := strconv.Atoi(v)
		if convErr != nil || l < 0 {
			return 0, "", fmt.Errorf("invalid limit")
		}
		limit = l
	}
	return limit, q.Get("page_token"), nil
}
