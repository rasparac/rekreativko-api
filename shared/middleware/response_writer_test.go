package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseWriter_FlushReachesTheUnderlyingWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	w := newResponseWriter(rec)

	// Streaming handlers flush through http.ResponseController, which finds
	// the underlying writer via Unwrap.
	require.NoError(t, http.NewResponseController(w).Flush())

	assert.True(t, rec.Flushed)
}
