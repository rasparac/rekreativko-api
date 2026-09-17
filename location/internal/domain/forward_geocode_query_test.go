package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewForwardGeocodeQuery(t *testing.T) {
	testCases := []struct {
		name        string
		city        string
		country     string
		street      string
		wantErr     bool
		wantCity    string
		wantCountry string
		wantStreet  string
	}{
		{name: "valid city+country", city: "Zagreb", country: "Croatia", wantCity: "Zagreb", wantCountry: "Croatia"},
		{name: "valid with street", city: "Zagreb", country: "Croatia", street: "Ilica 1", wantCity: "Zagreb", wantCountry: "Croatia", wantStreet: "Ilica 1"},
		{name: "trims whitespace", city: "  Zagreb  ", country: " Croatia ", street: " Ilica 1 ", wantCity: "Zagreb", wantCountry: "Croatia", wantStreet: "Ilica 1"},
		{name: "missing city", city: "", country: "Croatia", wantErr: true},
		{name: "blank city", city: "   ", country: "Croatia", wantErr: true},
		{name: "missing country", city: "Zagreb", country: "", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			query, err := NewForwardGeocodeQuery(tc.city, tc.country, tc.street)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantCity, query.City)
			assert.Equal(t, tc.wantCountry, query.Country)
			assert.Equal(t, tc.wantStreet, query.Street)
		})
	}
}

func TestForwardGeocodeQuery_IsFreeText(t *testing.T) {
	withStreet, err := NewForwardGeocodeQuery("Zagreb", "Croatia", "Ilica 1")
	require.NoError(t, err)
	assert.True(t, withStreet.IsFreeText())

	withoutStreet, err := NewForwardGeocodeQuery("Zagreb", "Croatia", "")
	require.NoError(t, err)
	assert.False(t, withoutStreet.IsFreeText())
}

func TestForwardGeocodeQuery_FreeTextSearch(t *testing.T) {
	query, err := NewForwardGeocodeQuery("Zagreb", "Croatia", "Ilica 1")
	require.NoError(t, err)

	assert.Equal(t, "Ilica 1, Zagreb, Croatia", query.FreeTextSearch())
}

func TestForwardGeocodeQuery_CacheKey(t *testing.T) {
	a, err := NewForwardGeocodeQuery("Zagreb", "Croatia", "")
	require.NoError(t, err)
	b, err := NewForwardGeocodeQuery("ZAGREB", "croatia", "")
	require.NoError(t, err)

	assert.Equal(t, a.CacheKey(), b.CacheKey(), "cache key must be case-insensitive")

	withStreet, err := NewForwardGeocodeQuery("Zagreb", "Croatia", "Ilica 1")
	require.NoError(t, err)
	assert.NotEqual(t, a.CacheKey(), withStreet.CacheKey(), "street must be part of the cache key")
}
