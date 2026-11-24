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

func decode(t *testing.T, body string, into any) error {
	t.Helper()

	return api.MustNewDecoder().Decode(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)), into)
}

// The domain trims and lowercases an email before checking it, so a phone keyboard's
// "User@Gmail.com " must get past the DTO: a format tag here would see the raw input.
func TestRegisterAndLogin_AcceptAnEmailTheDomainNormalizes(t *testing.T) {
	var register RegisterAccountRequest
	require.NoError(t, decode(t, `{"email":" User@Gmail.com ","password":"Str0ng!Pass"}`, &register))

	var login LoginRequest
	require.NoError(t, decode(t, `{"email":" User@Gmail.com ","password":"x"}`, &login))
}

func TestForgotPassword_RejectsAMissingOrUnknownField(t *testing.T) {
	for name, body := range map[string]string{
		"empty identity": `{"type":"email","identity":""}`,
		"no identity":    `{"type":"email"}`,
		"unknown type":   `{"type":"fax","identity":"a@b.co"}`,
	} {
		t.Run(name, func(t *testing.T) {
			var req ForgotPasswordRequest

			var validation *api.ValidationError
			require.ErrorAs(t, decode(t, body, &req), &validation)
			assert.Equal(t, "Validation failed", validation.Message)
		})
	}

	var ok ForgotPasswordRequest
	require.NoError(t, decode(t, `{"type":"email","identity":"a@b.co"}`, &ok))
}

// Either email or phone is enough: neither is required by the DTO (the service
// refuses a request with neither), so a client that sends only one is fine.
func TestRegisterAndLogin_EmailAloneOrPhoneAloneIsEnough(t *testing.T) {
	for name, body := range map[string]string{
		"email only": `{"email":"a@b.co","password":"Str0ng!Pass"}`,
		"phone only": `{"phone_number":"+381601234567","password":"Str0ng!Pass"}`,
	} {
		t.Run(name, func(t *testing.T) {
			var register RegisterAccountRequest
			require.NoError(t, decode(t, body, &register))

			var login LoginRequest
			require.NoError(t, decode(t, body, &login))
		})
	}
}
