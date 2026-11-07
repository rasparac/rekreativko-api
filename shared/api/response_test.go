package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteJSONResponse_WritesTheStatusAndBody(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteCreatedResponse(rec, map[string]int{"answer": 42}, "created")

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, byte('\n'), rec.Body.Bytes()[rec.Body.Len()-1], "newline-terminated like json.Encoder")

	var got Response[map[string]int]
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, SuccessMsg, got.Status)
	assert.Equal(t, "created", got.Message)
	assert.Equal(t, 42, got.Data["answer"])
}

// A response that cannot be encoded is a bug; the client must get a 500, not
// the intended status with a truncated body.
func TestWriteJSONResponse_AResponseThatFailsToEncodeBecomesA500(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteOkResponse(rec, math.NaN(), "")

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var got Response[any]
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got), "the body is a complete JSON document")
	assert.Equal(t, ErrorMsg, got.Status)
	require.NotNil(t, got.Error)
	assert.Equal(t, "internal_server_error", got.Error.Code)
}

func TestWriteValidationErrorResponse_ListsTheFields(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteValidationErrorResponse(rec, &ValidationError{
		Message: "Invalid request",
		Fields:  map[string]string{"title": "required"},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var got Response[any]
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.NotNil(t, got.Error)
	assert.Equal(t, "validation_error", got.Error.Code)
	assert.Equal(t, "required", got.Error.Details["title"])
}
