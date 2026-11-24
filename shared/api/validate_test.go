package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type validateItem struct {
	Name string `json:"name" validate:"required,min=2"`
}

// validateRequest mirrors the tags the real request DTOs use.
type validateRequest struct {
	Type     string         `json:"type" validate:"required,oneof=email phone"`
	Code     string         `json:"code" validate:"required,len=6"`
	Title    string         `json:"title" validate:"required,min=3,max=10"`
	Lat      *float64       `json:"lat" validate:"required,min=-90,max=90"`
	Capacity *int           `json:"capacity,omitempty" validate:"omitempty,min=1,max=1000"`
	Contact  string         `json:"contact,omitempty" validate:"omitempty,email"`
	Zone     string         `json:"zone,omitempty" validate:"omitempty,timezone"`
	Items    []validateItem `json:"items" validate:"required,min=1,max=2,dive"`
}

func validRequest() map[string]any {
	return map[string]any{
		"type": "email", "code": "123456", "title": "Padel",
		"lat": 0, "items": []map[string]string{{"name": "ab"}},
	}
}

func decodeInto(t *testing.T, payload map[string]any) (*validateRequest, error) {
	t.Helper()

	var sb strings.Builder
	sb.WriteString("{")
	first := true
	for k, v := range payload {
		if !first {
			sb.WriteString(",")
		}
		first = false
		sb.WriteString(`"` + k + `":` + mustJSON(t, v))
	}
	sb.WriteString("}")

	var req validateRequest
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(sb.String()))

	return &req, DecodeJSONBody(r, &req)
}

func TestDecodeJSONBody_ValidatesTheTags(t *testing.T) {
	t.Run("a valid body passes, and a latitude of exactly 0 is a value, not a missing field", func(t *testing.T) {
		req, err := decodeInto(t, validRequest())

		require.NoError(t, err)
		require.NotNil(t, req.Lat)
		assert.Zero(t, *req.Lat)
	})

	for name, tc := range map[string]struct {
		change  func(map[string]any)
		field   string
		message string
	}{
		"missing required":    {func(p map[string]any) { delete(p, "type") }, "type", "This field is required"},
		"a missing pointer":   {func(p map[string]any) { delete(p, "lat") }, "lat", "This field is required"},
		"not in oneof":        {func(p map[string]any) { p["type"] = "fax" }, "type", "Must be one of: email, phone"},
		"wrong length":        {func(p map[string]any) { p["code"] = "12" }, "code", "Must be exactly 6 characters"},
		"text too short":      {func(p map[string]any) { p["title"] = "ab" }, "title", "Must be at least 3 characters"},
		"text too long":       {func(p map[string]any) { p["title"] = strings.Repeat("a", 11) }, "title", "Must be at most 10 characters"},
		"number out of range": {func(p map[string]any) { p["lat"] = 91 }, "lat", "Must be at most 90"},
		"optional but bad":    {func(p map[string]any) { p["capacity"] = 0 }, "capacity", "Must be at least 1"},
		"bad email":           {func(p map[string]any) { p["contact"] = "nope" }, "contact", "Must be a valid email address"},
		"bad time zone":       {func(p map[string]any) { p["zone"] = "Mars/Base" }, "zone", "Must be a valid IANA time zone, e.g. Europe/Belgrade"},
		"too few list items":  {func(p map[string]any) { p["items"] = []map[string]string{} }, "items", "Must be at least 1 items"},
		"too many items": {func(p map[string]any) {
			p["items"] = []map[string]string{{"name": "aa"}, {"name": "bb"}, {"name": "cc"}}
		}, "items", "Must be at most 2 items"},
		"a nested field is named by its path": {func(p map[string]any) {
			p["items"] = []map[string]string{{"name": "ok"}, {"name": "x"}}
		}, "items[1].name", "Must be at least 2 characters"},
	} {
		t.Run(name, func(t *testing.T) {
			payload := validRequest()
			tc.change(payload)

			_, err := decodeInto(t, payload)

			var validation *ValidationError
			require.ErrorAs(t, err, &validation)
			assert.Equal(t, "Validation failed", validation.Message)
			assert.Equal(t, tc.message, validation.Fields[tc.field])
		})
	}

	t.Run("every failing field is reported, keyed by its JSON name", func(t *testing.T) {
		_, err := decodeInto(t, map[string]any{"type": "fax", "items": []map[string]string{{"name": "ab"}}})

		var validation *ValidationError
		require.ErrorAs(t, err, &validation)
		assert.Contains(t, validation.Fields, "type")
		assert.Contains(t, validation.Fields, "code")
		assert.Contains(t, validation.Fields, "title")
		assert.Contains(t, validation.Fields, "lat")
		assert.NotContains(t, validation.Fields, "Type", "the Go field name is never exposed")
	})

	t.Run("a body that is not a struct has no tags to check", func(t *testing.T) {
		var m map[string]any
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"a":1}`))

		require.NoError(t, DecodeJSONBody(r, &m))
	})
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()

	b, err := json.Marshal(v)
	require.NoError(t, err)

	return string(b)
}
