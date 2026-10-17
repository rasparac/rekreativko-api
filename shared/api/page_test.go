package api

import (
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPage(t *testing.T) {
	page := NewPage([]int{1, 2, 3}, 10, "next-token", func(n int) string {
		return strconv.Itoa(n * 2)
	})

	assert.Equal(t, []string{"2", "4", "6"}, page.Items)
	assert.Equal(t, 10, page.Limit)
	assert.Equal(t, "next-token", page.NextPageToken)
}

func TestNewPage_EmptyInput(t *testing.T) {
	page := NewPage([]int{}, 10, "", func(n int) string { return "" })

	assert.Empty(t, page.Items)
	assert.Equal(t, "", page.NextPageToken)
}

func TestParsePageParams(t *testing.T) {
	tests := []struct {
		name          string
		query         url.Values
		defaultLimit  int
		wantLimit     int
		wantPageToken string
		wantErr       bool
	}{
		{
			name:         "no params uses default limit",
			query:        url.Values{},
			defaultLimit: 20,
			wantLimit:    20,
		},
		{
			name:          "explicit limit and page_token",
			query:         url.Values{"limit": {"5"}, "page_token": {"abc"}},
			defaultLimit:  20,
			wantLimit:     5,
			wantPageToken: "abc",
		},
		{
			name:         "invalid limit",
			query:        url.Values{"limit": {"not-a-number"}},
			defaultLimit: 20,
			wantErr:      true,
		},
		{
			name:         "negative limit",
			query:        url.Values{"limit": {"-1"}},
			defaultLimit: 20,
			wantErr:      true,
		},
		{
			name:         "limit 0 is the default, not unlimited",
			query:        url.Values{"limit": {"0"}},
			defaultLimit: 20,
			wantLimit:    20,
		},
		{
			name:         "the maximum itself is allowed",
			query:        url.Values{"limit": {"100"}},
			defaultLimit: 20,
			wantLimit:    MaxPageLimit,
		},
		{
			name:         "a limit past the maximum is cut to it",
			query:        url.Values{"limit": {"101"}},
			defaultLimit: 20,
			wantLimit:    MaxPageLimit,
		},
		{
			name:         "a huge limit is cut to the maximum",
			query:        url.Values{"limit": {"100000000"}},
			defaultLimit: 20,
			wantLimit:    MaxPageLimit,
		},
		{
			name:         "a limit too large for an int is rejected",
			query:        url.Values{"limit": {"99999999999999999999"}},
			defaultLimit: 20,
			wantErr:      true,
		},
		{
			name:         "a default past the maximum is cut as well",
			query:        url.Values{},
			defaultLimit: 500,
			wantLimit:    MaxPageLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, pageToken, err := ParsePageParams(tt.query, tt.defaultLimit)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantLimit, limit)
			assert.Equal(t, tt.wantPageToken, pageToken)
		})
	}
}
