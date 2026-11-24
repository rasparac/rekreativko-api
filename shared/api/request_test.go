package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeJSONBody_ReadsAtMostMaxRequestBodyBytes(t *testing.T) {
	type payload struct {
		Note string `json:"note"`
	}

	t.Run("a normal body decodes", func(t *testing.T) {
		var p payload
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"note":"hi"}`))

		require.NoError(t, DecodeJSONBody(r, &p))
		assert.Equal(t, "hi", p.Note)
	})

	t.Run("a body over the limit is refused as too large, not read to the end", func(t *testing.T) {
		var p payload
		huge := `{"note":"` + strings.Repeat("a", MaxRequestBodyBytes) + `"}`
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(huge))

		err := DecodeJSONBody(r, &p)

		var validation *ValidationError
		require.ErrorAs(t, err, &validation)
		assert.Equal(t, "Request body is too large", validation.Message)
	})
}
