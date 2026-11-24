package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rasparac/rekreativko-api/account-profile/internal/application"
	"github.com/rasparac/rekreativko-api/account-profile/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

// fakeProfiles and fakeSettings record what the handlers hand to the services.
type fakeProfiles struct {
	updated []application.UpdateProfileParams
}

func (f *fakeProfiles) GetProfile(context.Context, application.ProfileFilter) (*domain.AccountProfile, error) {
	return nil, nil
}

func (f *fakeProfiles) UpdateProfile(_ context.Context, _ uuid.UUID, p application.UpdateProfileParams) error {
	f.updated = append(f.updated, p)

	return nil
}

func (f *fakeProfiles) GetProfiles(context.Context, application.ProfilesFilter) ([]*domain.AccountProfile, error) {
	return nil, nil
}

type fakeSettings struct {
	updated []application.UpdateAccountSettingsParams
}

func (f *fakeSettings) GetSettings(context.Context, uuid.UUID) (*domain.AccountProfileSettings, error) {
	return nil, nil
}

func (f *fakeSettings) UpdateSettings(_ context.Context, _ uuid.UUID, p application.UpdateAccountSettingsParams) error {
	f.updated = append(f.updated, p)

	return nil
}

func newTestHandler() (*accountPorfileHandler, *fakeProfiles, *fakeSettings) {
	profiles, settings := &fakeProfiles{}, &fakeSettings{}

	return NewHandler(profiles, settings, api.MustNewDecoder(), logger.New("error", "json")), profiles, settings
}

// put runs handle as an authenticated PUT with body and returns the recorded response.
func put(t *testing.T, handle http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequest(http.MethodPut, "/x", strings.NewReader(body))
	r = r.WithContext(authcontext.WithAccountID(r.Context(), uuid.New()))

	rec := httptest.NewRecorder()
	handle(rec, r)

	return rec
}

type errorBody struct {
	Error struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	} `json:"error"`
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()

	var body errorBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	return body
}

func TestUpdateProfile_PassesAValidBodyToTheService(t *testing.T) {
	h, profiles, _ := newTestHandler()

	rec := put(t, h.UpdateProfile, `{"nickname":"jd_1","bio":"hi","location_city":"Accra","location_country":"Ghana","location_latitude":0,"location_longitude":0}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, profiles.updated, 1)
	require.NotNil(t, profiles.updated[0].Location)
	require.NotNil(t, profiles.updated[0].Location.Latitude, "a latitude of exactly 0 is a value")
	assert.Zero(t, *profiles.updated[0].Location.Latitude)
}

// What a client gets for a body the shared decoder refuses: 400 validation_error with details,
// and the service is never called.
func TestUpdateProfile_RefusesABadBodyBeforeTheService(t *testing.T) {
	for name, tc := range map[string]struct {
		body  string
		field string // a key expected in details
	}{
		"an unknown field":             {`{"nickname":"jd_1","favourite_colour":"red"}`, "favourite_colour"},
		"malformed JSON":               {`{"nickname":`, "_json"},
		"a field of the wrong type":    {`{"full_name":5}`, "full_name"},
		"a latitude over 90":           {`{"location_latitude":91,"location_longitude":0}`, "location_latitude"},
		"a longitude under -180":       {`{"location_latitude":0,"location_longitude":-181}`, "location_longitude"},
		"an empty body":                {``, "_body"},
		"a body that is not an object": {`[]`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			h, profiles, _ := newTestHandler()

			rec := put(t, h.UpdateProfile, tc.body)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, "validation_error", decodeError(t, rec).Error.Code)
			if tc.field != "" {
				assert.Contains(t, decodeError(t, rec).Error.Details, tc.field)
			}
			assert.Empty(t, profiles.updated, "the service never sees it")
		})
	}
}

func TestUpdateAccountSettings_PassesAValidBodyToTheService(t *testing.T) {
	h, _, settings := newTestHandler()

	rec := put(t, h.UpdateAccountSettings, `{"settings":{"notification.email.enabled":true}}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, settings.updated, 1)
	assert.Equal(t, true, settings.updated[0].Settings["notification.email.enabled"])
}

func TestUpdateAccountSettings_RefusesABadBodyBeforeTheService(t *testing.T) {
	for name, body := range map[string]string{
		"an unknown top-level field": `{"settings":{},"extra":1}`,
		"malformed JSON":             `{"settings":`,
		"settings of the wrong type": `{"settings":"yes"}`,
	} {
		t.Run(name, func(t *testing.T) {
			h, _, settings := newTestHandler()

			rec := put(t, h.UpdateAccountSettings, body)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, "validation_error", decodeError(t, rec).Error.Code)
			assert.Empty(t, settings.updated)
		})
	}
}
