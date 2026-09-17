package domain

import (
	"fmt"
	"math"
)

// bucketPrecision is the number of decimal places coordinates are rounded to
// when deriving a cache key. 3 decimal places is roughly a 110m grid at the
// equator - close enough that repeated lookups within the same city block
// hit the cache, without collapsing genuinely distinct locations together.
const bucketPrecision = 3

// Coordinates is a validated lat/long pair.
type Coordinates struct {
	Latitude  float64
	Longitude float64
}

// NewCoordinates validates and builds a Coordinates value.
func NewCoordinates(lat, lon float64) (Coordinates, error) {
	if lat < -90 || lat > 90 {
		return Coordinates{}, fmt.Errorf("latitude %.6f is out of range [-90, 90]", lat)
	}
	if lon < -180 || lon > 180 {
		return Coordinates{}, fmt.Errorf("longitude %.6f is out of range [-180, 180]", lon)
	}

	return Coordinates{Latitude: lat, Longitude: lon}, nil
}

// Bucket rounds the coordinates to a fixed-precision grid cell, used as the
// geocode cache key so nearby lookups reuse the same cached result.
func (c Coordinates) Bucket() (latBucket, lngBucket float64) {
	factor := math.Pow(10, bucketPrecision)
	return math.Round(c.Latitude*factor) / factor, math.Round(c.Longitude*factor) / factor
}
