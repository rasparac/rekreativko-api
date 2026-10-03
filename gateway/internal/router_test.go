package gateway

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The gateway's AuthMiddleware is built from Router.PublicPaths, so for every
// path the middleware's view (matches a public pattern) and the router's view
// (its auth rule does not require auth) must agree.
func TestRouter_PublicPaths(t *testing.T) {
	r := NewRouter(nil)

	var public []*regexp.Regexp
	for _, p := range r.PublicPaths() {
		public = append(public, regexp.MustCompile(p))
	}
	middlewareLetsThrough := func(path string) bool {
		for _, re := range public {
			if re.MatchString(path) {
				return true
			}
		}
		return false
	}

	tests := []struct {
		path   string
		public bool
	}{
		{"/identity/api/v1/login", true},
		{"/identity/api/v1/register", true},
		{"/identity/api/v1/verify-account", true},
		{"/identity/api/v1/resend-verification-code", true},
		{"/identity/api/v1/refresh-token", true},
		{"/identity/api/v1/me", false},
		{"/identity/api/v1/login/extra", false},
		{"/account-profile/api/v1/profiles", false},
		{"/activity/api/v1/sessions", false},
		{"/notifications/api/v1/notifications", false},
		{"/location/api/v1/places", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.public, middlewareLetsThrough(tt.path), "middleware")

			route := r.matchRoute(tt.path)
			require.NotNil(t, route)
			assert.Equal(t, !tt.public, r.matchAuthRule(route, tt.path).RequireAuth, "router")
		})
	}
}

func TestRoute_IsStream(t *testing.T) {
	r := NewRouter(nil)
	route := r.matchRoute("/activity/api/v1/sessions/x/events")
	require.NotNil(t, route)

	assert.True(t, route.isStream("/activity/api/v1/sessions/0b6e6f4e-1a2b-4c3d-8e9f-0a1b2c3d4e5f/events"))

	for _, path := range []string{
		"/activity/api/v1/sessions/x/team-formation",
		"/activity/api/v1/sessions/x/events/extra",
		"/activity/api/v1/sessions/events",
		"/activity/api/v1/sessions/x/y/events",
	} {
		assert.Falsef(t, route.isStream(path), "%s must not be a stream", path)
	}
}

// Backends trust X-User-ID as the authenticated account, so only the gateway
// may set it: a value sent by the client must never reach the backend.
func TestRouter_DropsClientSuppliedUserID(t *testing.T) {
	spoofed := uuid.MustParse("0b6e6f4e-1a2b-4c3d-8e9f-0a1b2c3d4e5f")

	tests := []struct {
		name    string
		path    string
		account uuid.UUID
		want    string
	}{
		{"public route, unauthenticated", "/identity/api/v1/login", uuid.Nil, ""},
		{"protected route, authenticated", "/identity/api/v1/me", testAccountID, testAccountID.String()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				got     []string
				reached bool
			)
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached = true
				got = r.Header.Values(authcontext.XUserIDHeader)
			}))
			t.Cleanup(backend.Close)

			proxy, err := NewReverseProxy(map[string]ServiceConfig{
				"identity": {URL: backend.URL, Timeout: testServiceTimeout},
			}, logger.New("error", "json"))
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			req.Header.Set(authcontext.XUserIDHeader, spoofed.String())
			if tt.account != uuid.Nil {
				req = req.WithContext(authcontext.WithAccountID(req.Context(), tt.account))
			}

			rec := httptest.NewRecorder()
			NewRouter(proxy).ServeHTTP(rec, req)

			require.True(t, reached, "request must reach the backend (status %d)", rec.Code)
			if tt.want == "" {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, []string{tt.want}, got)
			}
		})
	}
}
