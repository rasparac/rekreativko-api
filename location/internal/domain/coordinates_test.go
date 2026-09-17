package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCoordinates(t *testing.T) {
	testCases := []struct {
		name    string
		lat     float64
		lon     float64
		wantErr bool
	}{
		{name: "valid coordinates", lat: 45.815, lon: 15.982, wantErr: false},
		{name: "boundary latitude 90", lat: 90, lon: 0, wantErr: false},
		{name: "boundary latitude -90", lat: -90, lon: 0, wantErr: false},
		{name: "boundary longitude 180", lat: 0, lon: 180, wantErr: false},
		{name: "boundary longitude -180", lat: 0, lon: -180, wantErr: false},
		{name: "latitude too high", lat: 90.1, lon: 0, wantErr: true},
		{name: "latitude too low", lat: -90.1, lon: 0, wantErr: true},
		{name: "longitude too high", lat: 0, lon: 180.1, wantErr: true},
		{name: "longitude too low", lat: 0, lon: -180.1, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			coords, err := NewCoordinates(tc.lat, tc.lon)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.lat, coords.Latitude)
			assert.Equal(t, tc.lon, coords.Longitude)
		})
	}
}

func TestCoordinates_Bucket(t *testing.T) {
	testCases := []struct {
		name     string
		lat, lon float64
		wantLat  float64
		wantLon  float64
	}{
		{name: "already at precision", lat: 45.815, lon: 15.982, wantLat: 45.815, wantLon: 15.982},
		{name: "rounds down", lat: 45.8151, lon: 15.9822, wantLat: 45.815, wantLon: 15.982},
		{name: "rounds up", lat: 45.8156, lon: 15.9827, wantLat: 45.816, wantLon: 15.983},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			coords, err := NewCoordinates(tc.lat, tc.lon)
			require.NoError(t, err)

			gotLat, gotLon := coords.Bucket()
			assert.InDelta(t, tc.wantLat, gotLat, 1e-9)
			assert.InDelta(t, tc.wantLon, gotLon, 1e-9)
		})
	}
}

func TestCoordinates_Bucket_NearbyPointsShareABucket(t *testing.T) {
	a, err := NewCoordinates(45.81501, 15.98199)
	require.NoError(t, err)
	b, err := NewCoordinates(45.81499, 15.98201)
	require.NoError(t, err)

	aLat, aLon := a.Bucket()
	bLat, bLon := b.Bucket()

	assert.Equal(t, aLat, bLat)
	assert.Equal(t, aLon, bLon)
}
