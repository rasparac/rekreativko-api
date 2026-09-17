package domain

import (
	"fmt"
	"strings"
)

// ForwardGeocodeQuery is a validated, normalized forward-geocoding input.
// City and Country are always required; Street is optional and, when
// present, switches resolution to Nominatim's free-text search - a
// structured city+country+street search resolves far less reliably than a
// single free-text string for street-level addresses.
type ForwardGeocodeQuery struct {
	City    string
	Country string
	Street  string
}

// NewForwardGeocodeQuery validates and normalizes a forward-geocoding query.
func NewForwardGeocodeQuery(city, country, street string) (ForwardGeocodeQuery, error) {
	city = strings.TrimSpace(city)
	country = strings.TrimSpace(country)
	street = strings.TrimSpace(street)

	if city == "" {
		return ForwardGeocodeQuery{}, fmt.Errorf("city is required")
	}
	if country == "" {
		return ForwardGeocodeQuery{}, fmt.Errorf("country is required")
	}

	return ForwardGeocodeQuery{City: city, Country: country, Street: street}, nil
}

// IsFreeText reports whether this query should be resolved via Nominatim's
// free-text search (Street given) instead of a structured city+country search.
func (q ForwardGeocodeQuery) IsFreeText() bool {
	return q.Street != ""
}

// FreeTextSearch builds the human-readable free-text query string sent to
// Nominatim when Street is given.
func (q ForwardGeocodeQuery) FreeTextSearch() string {
	return strings.Join([]string{q.Street, q.City, q.Country}, ", ")
}

// CacheKey returns a stable, normalized cache key for this query - case and
// whitespace insensitive, so "Belgrade"/"belgrade " hit the same cache entry.
func (q ForwardGeocodeQuery) CacheKey() string {
	return strings.ToLower(q.Street) + "|" + strings.ToLower(q.City) + "|" + strings.ToLower(q.Country)
}
