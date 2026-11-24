package dtos

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sessionBody = `{"title":"Run","location_city":"Accra","location_country":"Ghana",%s
	"start_time":"2030-01-01T18:00:00Z","visibility":"public"}`

func decodeSession(t *testing.T, coords string) (*CreateSessionRequest, error) {
	t.Helper()

	var req CreateSessionRequest
	body := strings.Replace(sessionBody, "%s", coords, 1)

	return &req, api.MustNewDecoder().Decode(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)), &req)
}

// 0 is a real coordinate (the equator, the prime meridian): a plain `required` on a
// float64 would call it missing. The fields are pointers, so only an absent one fails.
func TestCreateSessionRequest_CoordinatesOfZeroAreValuesNotMissing(t *testing.T) {
	req, err := decodeSession(t, `"location_lat":0,"location_lng":0,`)

	require.NoError(t, err)
	require.NotNil(t, req.LocationLat)
	require.NotNil(t, req.LocationLng)
	assert.Zero(t, *req.LocationLat)
	assert.Zero(t, *req.LocationLng)
}

func TestCreateSessionRequest_MissingOrOutOfRangeCoordinatesAreRejected(t *testing.T) {
	for name, tc := range map[string]struct{ coords, field string }{
		"no latitude":   {`"location_lng":20,`, "location_lat"},
		"no longitude":  {`"location_lat":44,`, "location_lng"},
		"lat too large": {`"location_lat":91,"location_lng":20,`, "location_lat"},
		"lng too small": {`"location_lat":44,"location_lng":-181,`, "location_lng"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := decodeSession(t, tc.coords)

			var validation *api.ValidationError
			require.ErrorAs(t, err, &validation)
			assert.Contains(t, validation.Fields, tc.field)
		})
	}
}
