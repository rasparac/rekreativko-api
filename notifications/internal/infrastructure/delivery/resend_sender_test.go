package delivery

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/events"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/resend/resend-go/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeResendEmails answers every send with err (or an id), keeping the calls.
type fakeResendEmails struct {
	err      error
	requests []*resend.SendEmailRequest
	options  []*resend.SendEmailOptions
}

func (f *fakeResendEmails) SendWithOptions(_ context.Context, params *resend.SendEmailRequest, options *resend.SendEmailOptions) (*resend.SendEmailResponse, error) {
	f.requests = append(f.requests, params)
	f.options = append(f.options, options)
	if f.err != nil {
		return nil, f.err
	}
	return &resend.SendEmailResponse{Id: "49a3999c-0ce1-4ea6-ab68-afcd6dc2e794"}, nil
}

func newTestSender(err error) (*resendEmailSender, *fakeResendEmails) {
	fake := &fakeResendEmails{err: err}
	return NewResendEmailSender(fake, "Rekreativko <no-reply@mail.example.com>", testutil.CreateLogger()), fake
}

func TestResendEmailSender_Sends(t *testing.T) {
	s, fake := newTestSender(nil)

	require.NoError(t, s.SendEmail(context.Background(), "ana@example.com", "Your code", "123456"))

	require.Len(t, fake.requests, 1)
	assert.Equal(t, &resend.SendEmailRequest{
		From:    "Rekreativko <no-reply@mail.example.com>",
		To:      []string{"ana@example.com"},
		Subject: "Your code",
		Text:    "123456",
	}, fake.requests[0])
	assert.Nil(t, fake.options[0], "no idempotency key outside an event's handling")
}

// A redelivered event sends the same key, so Resend sends the email once.
func TestResendEmailSender_IdempotencyKeyPerEventAndRecipient(t *testing.T) {
	s, fake := newTestSender(nil)
	ctx := api.WithEventID(context.Background(), "e1")

	require.NoError(t, s.SendEmail(ctx, "ana@example.com", "s", "b"))
	require.NoError(t, s.SendEmail(ctx, "ana@example.com", "s", "b"))
	require.NoError(t, s.SendEmail(ctx, "ivo@example.com", "s", "b"))
	require.NoError(t, s.SendEmail(api.WithEventID(context.Background(), "e2"), "ana@example.com", "s", "b"))

	keys := make([]string, len(fake.options))
	for i, o := range fake.options {
		require.NotNil(t, o)
		keys[i] = o.IdempotencyKey
	}
	assert.Equal(t, keys[0], keys[1], "the same event and recipient")
	assert.NotEqual(t, keys[0], keys[2], "another recipient")
	assert.NotEqual(t, keys[0], keys[3], "another event")
	assert.Regexp(t, `^notifications/e1/[0-9a-f]{16}$`, keys[0], "the address is hashed")
}

func TestResendEmailSender_Errors(t *testing.T) {
	// Resend's message may quote the address, so it must not reach the error.
	rejected := func(status int, name string) error {
		return &resend.ResendError{StatusCode: status, Name: name, Message: "Invalid `to`: ana@example.com"}
	}

	for name, tc := range map[string]struct {
		err       error
		permanent bool
	}{
		"validation error":      {rejected(http.StatusUnprocessableEntity, "validation_error"), true},
		"bad api key":           {rejected(http.StatusUnauthorized, "missing_api_key"), true},
		"unverified domain":     {rejected(http.StatusForbidden, "invalid_from_address"), true},
		"same key in flight":    {rejected(http.StatusConflict, "concurrent_idempotent_requests"), false},
		"outage":                {rejected(http.StatusServiceUnavailable, ""), false},
		"internal server error": {rejected(http.StatusInternalServerError, "internal_server_error"), false},
		"rate limited":          {&resend.RateLimitError{Message: "Too many requests", RetryAfter: "1"}, false},
		"network error":         {errors.New("Post \"https://api.resend.com/emails\": dial tcp: connection refused"), false},
	} {
		t.Run(name, func(t *testing.T) {
			s, _ := newTestSender(tc.err)

			err := s.SendEmail(context.Background(), "ana@example.com", "Your code", "123456")

			require.Error(t, err)
			assert.Equal(t, tc.permanent, errors.Is(err, events.ErrPermanent))
			for _, private := range []string{"ana", "123456", "Your code"} {
				assert.NotContains(t, err.Error(), private, "no personal data in the error")
			}
		})
	}
}
