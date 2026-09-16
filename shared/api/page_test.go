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
