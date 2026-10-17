package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serve sends a request through RequireAppKey and reports the status, whether
// the handler behind it ran, the reasons reported, and the header it saw.
func serve(t *testing.T, keys []string, enforce bool, header map[string]string) (code int, reached bool, reasons []string, seen string, body string) {
	t.Helper()

	handler := RequireAppKey(keys, enforce, func(reason string) { reasons = append(reasons, reason) })(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			reached = true
			seen = r.Header.Get(AppKeyHeader)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/activity/api/v1/groups", nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	return w.Code, reached, reasons, seen, w.Body.String()
}

func TestRequireAppKey_EnforcedAcceptsAValidKey(t *testing.T) {
	code, reached, reasons, seen, _ := serve(t, []string{"k1"}, true, map[string]string{AppKeyHeader: "k1"})

	assert.Equal(t, http.StatusOK, code)
	assert.True(t, reached)
	assert.Empty(t, reasons)
	assert.Empty(t, seen, "the backends never see the key")
}

func TestRequireAppKey_EnforcedRefusesMissingAndWrongKeys(t *testing.T) {
	for name, tc := range map[string]struct {
		header map[string]string
		reason string
	}{
		"no header":    {nil, AppKeyMissing},
		"empty header": {map[string]string{AppKeyHeader: ""}, AppKeyMissing},
		"wrong key":    {map[string]string{AppKeyHeader: "nope"}, AppKeyInvalid},
		"a prefix":     {map[string]string{AppKeyHeader: "k"}, AppKeyInvalid},
		"a longer key": {map[string]string{AppKeyHeader: "k11"}, AppKeyInvalid},
	} {
		t.Run(name, func(t *testing.T) {
			code, reached, reasons, _, body := serve(t, []string{"k1"}, true, tc.header)

			assert.Equal(t, http.StatusForbidden, code, "403, not 401: the app answers 401 by refreshing its token")
			assert.False(t, reached)
			assert.Equal(t, []string{tc.reason}, reasons)
			assert.Contains(t, body, "app_key_invalid")
		})
	}
}

func TestRequireAppKey_AnyConfiguredKeyIsAccepted(t *testing.T) {
	keys := []string{"old", " new "}

	for _, key := range []string{"old", "new"} {
		code, reached, _, _, _ := serve(t, keys, true, map[string]string{AppKeyHeader: key})
		assert.Equal(t, http.StatusOK, code, key)
		assert.True(t, reached, key)
	}

	code, _, _, _, _ := serve(t, keys, true, map[string]string{AppKeyHeader: "older"})
	assert.Equal(t, http.StatusForbidden, code, "a retired key stops working once removed")
}

func TestRequireAppKey_ReportOnlyCountsButLetsEverythingThrough(t *testing.T) {
	code, reached, reasons, seen, _ := serve(t, []string{"k1"}, false, nil)
	assert.Equal(t, http.StatusOK, code)
	assert.True(t, reached)
	assert.Equal(t, []string{AppKeyMissing}, reasons)
	assert.Empty(t, seen)

	code, reached, reasons, seen, _ = serve(t, []string{"k1"}, false, map[string]string{AppKeyHeader: "wrong"})
	assert.Equal(t, http.StatusOK, code)
	assert.True(t, reached)
	assert.Equal(t, []string{AppKeyInvalid}, reasons)
	assert.Empty(t, seen, "a wrong key is not forwarded either")

	_, _, reasons, _, _ = serve(t, []string{"k1"}, false, map[string]string{AppKeyHeader: "k1"})
	assert.Empty(t, reasons, "a valid key reports nothing")
}

func TestRequireAppKey_NoKeysMeansOff(t *testing.T) {
	for _, keys := range [][]string{nil, {}, {""}, {" ", ""}} {
		// Even "enforce" has nothing to enforce without a key.
		code, reached, reasons, _, _ := serve(t, keys, true, nil)

		assert.Equal(t, http.StatusOK, code, "%q", keys)
		assert.True(t, reached)
		assert.Empty(t, reasons)
	}
}

func TestRequireAppKey_NilReporterIsFine(t *testing.T) {
	handler := RequireAppKey([]string{"k1"}, true, nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	w := httptest.NewRecorder()
	require.NotPanics(t, func() { handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil)) })
	assert.Equal(t, http.StatusForbidden, w.Code)
}
