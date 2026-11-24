package middleware

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A client that declares a body bigger than the limit is turned away with 413
// before the handler runs or a byte is read.
func TestLimitBodySize_RefusesADeclaredBodyOverTheLimit(t *testing.T) {
	var called bool
	handler := LimitBodySize(10)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(strings.Repeat("a", 11))))

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.False(t, called, "the handler never sees it")

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "request_too_large", body.Error.Code)
}

// A body at the limit, and one with no body at all, go through untouched.
func TestLimitBodySize_LetsASmallBodyThrough(t *testing.T) {
	var got string
	handler := LimitBodySize(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		got = string(b)
	}))

	for name, body := range map[string]string{"at the limit": strings.Repeat("a", 10), "empty": ""} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)))

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, body, got)
		})
	}
}

// A client that does not declare the size (chunked) is cut off once it sends
// more than the limit: reading the body fails instead of buffering all of it.
func TestLimitBodySize_CutsOffAnUndeclaredBodyOverTheLimit(t *testing.T) {
	var readErr error
	handler := LimitBodySize(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
	}))

	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(strings.Repeat("a", 1000)))
	r.ContentLength = -1 // chunked: the size is not declared
	handler.ServeHTTP(httptest.NewRecorder(), r)

	var tooLarge *http.MaxBytesError
	require.ErrorAs(t, readErr, &tooLarge)
	assert.Equal(t, int64(10), tooLarge.Limit)
}
