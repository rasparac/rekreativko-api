package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/events"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeResend answers every request with status and body, keeping the requests.
func fakeResend(t *testing.T, status int, body string) (*ResendEmailSender, *[]*http.Request, *[]resendSendRequest) {
	t.Helper()

	var (
		reqs     []*http.Request
		payloads []resendSendRequest
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p resendSendRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&p))
		reqs = append(reqs, r)
		payloads = append(payloads, p)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	s := NewResendEmailSender("re_test", "Rekreativko <no-reply@mail.example.com>", testutil.CreateLogger())
	s.baseURL = srv.URL
	return s, &reqs, &payloads
}

func TestResendEmailSender_Sends(t *testing.T) {
	s, reqs, payloads := fakeResend(t, http.StatusOK, `{"id":"49a3999c-0ce1-4ea6-ab68-afcd6dc2e794"}`)

	require.NoError(t, s.SendEmail(context.Background(), "ana@example.com", "Your code", "123456"))

	require.Len(t, *reqs, 1)
	r := (*reqs)[0]
	assert.Equal(t, http.MethodPost, r.Method)
	assert.Equal(t, "/emails", r.URL.Path)
	assert.Equal(t, "Bearer re_test", r.Header.Get("Authorization"))
	assert.Empty(t, r.Header.Get("Idempotency-Key"), "no event being handled")
	assert.Equal(t, resendSendRequest{
		From:    "Rekreativko <no-reply@mail.example.com>",
		To:      []string{"ana@example.com"},
		Subject: "Your code",
		Text:    "123456",
	}, (*payloads)[0])
}

// A redelivered event sends the same key, so Resend sends the email once.
func TestResendEmailSender_IdempotencyKeyPerEventAndRecipient(t *testing.T) {
	s, reqs, _ := fakeResend(t, http.StatusOK, `{"id":"x"}`)
	ctx := api.WithEventID(context.Background(), "e1")

	require.NoError(t, s.SendEmail(ctx, "ana@example.com", "s", "b"))
	require.NoError(t, s.SendEmail(ctx, "ana@example.com", "s", "b"))
	require.NoError(t, s.SendEmail(ctx, "ivo@example.com", "s", "b"))
	require.NoError(t, s.SendEmail(api.WithEventID(context.Background(), "e2"), "ana@example.com", "s", "b"))

	keys := make([]string, len(*reqs))
	for i, r := range *reqs {
		keys[i] = r.Header.Get("Idempotency-Key")
	}
	assert.Equal(t, keys[0], keys[1], "the same event and recipient")
	assert.NotEqual(t, keys[0], keys[2], "another recipient")
	assert.NotEqual(t, keys[0], keys[3], "another event")
	assert.True(t, strings.HasPrefix(keys[0], "notifications/e1/"))
	assert.NotContains(t, keys[0], "ana", "the address is hashed")
}

func TestResendEmailSender_Errors(t *testing.T) {
	for name, tc := range map[string]struct {
		status    int
		body      string
		permanent bool
	}{
		"validation error":      {http.StatusUnprocessableEntity, `{"name":"validation_error","message":"Invalid ana@example.com"}`, true},
		"bad api key":           {http.StatusUnauthorized, `{"name":"missing_api_key"}`, true},
		"unverified domain":     {http.StatusForbidden, `{"name":"invalid_from_address"}`, true},
		"rate limited":          {http.StatusTooManyRequests, `{"name":"rate_limit_exceeded"}`, false},
		"same key in flight":    {http.StatusConflict, `{"name":"concurrent_idempotent_requests"}`, false},
		"outage":                {http.StatusServiceUnavailable, `<html>down</html>`, false},
		"internal server error": {http.StatusInternalServerError, `{"name":"internal_server_error"}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			s, _, _ := fakeResend(t, tc.status, tc.body)

			err := s.SendEmail(context.Background(), "ana@example.com", "Your code", "123456")

			require.Error(t, err)
			assert.Equal(t, tc.permanent, errors.Is(err, events.ErrPermanent))
			for _, private := range []string{"ana", "123456", "Your code"} {
				assert.NotContains(t, err.Error(), private, "no personal data in the error")
			}
		})
	}
}

func TestResendEmailSender_NetworkErrorIsRetried(t *testing.T) {
	s := NewResendEmailSender("re_test", "a@b.c", testutil.CreateLogger())
	s.baseURL = "http://127.0.0.1:1" // nothing listens

	err := s.SendEmail(context.Background(), "ana@example.com", "s", "b")

	require.Error(t, err)
	assert.False(t, errors.Is(err, events.ErrPermanent))
}
