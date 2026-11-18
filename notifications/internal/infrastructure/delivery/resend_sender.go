package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"

	"github.com/rasparac/rekreativko-api/notifications/internal/application"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/events"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/resend/resend-go/v4"
)

type (
	// resendEmails is the part of the Resend SDK's emails service the sender
	// uses (resend.Client.Emails).
	resendEmails interface {
		SendWithOptions(ctx context.Context, params *resend.SendEmailRequest, options *resend.SendEmailOptions) (*resend.SendEmailResponse, error)
	}

	// resendEmailSender sends email through Resend with its official Go SDK.
	resendEmailSender struct {
		emails resendEmails
		from   string
		logger *logger.Logger
	}
)

// NewResendEmailSender sends from the given sender through emails, e.g.
// resend.NewCustomClient(httpClient, apiKey).Emails.
func NewResendEmailSender(emails resendEmails, from string, log *logger.Logger) *resendEmailSender {
	return &resendEmailSender{
		emails: emails,
		from:   from,
		logger: log.WithName("notifications.resend"),
	}
}

// SendEmail sends one plain-text email. Within the handling of one event it
// carries an idempotency key, so a redelivered event (or a retry after a lost
// response) does not send it twice; Resend keeps keys for 24 hours.
//
// A rejection retrying cannot fix (any 4xx but 409 and 429) is an
// events.Permanent error, so the event is dead-lettered instead of redelivered.
// Errors never carry the recipient, the text or Resend's message (it may quote
// the address) - only the status and Resend's error name - so the SDK's error
// is not wrapped.
func (s *resendEmailSender) SendEmail(ctx context.Context, to, subject, body string) error {
	var options *resend.SendEmailOptions
	if key := idempotencyKey(ctx, to); key != "" {
		options = &resend.SendEmailOptions{IdempotencyKey: key}
	}

	sent, err := s.emails.SendWithOptions(ctx, &resend.SendEmailRequest{
		From:    s.from,
		To:      []string{to},
		Subject: subject,
		Text:    body,
	}, options)
	if err == nil {
		s.logger.Debug(ctx, "email sent", "resend_id", sent.Id)
		return nil
	}

	var (
		rateLimited *resend.RateLimitError
		rejected    *resend.ResendError
	)
	switch {
	case errors.As(err, &rateLimited):
		return fmt.Errorf("resend rate limited (retry after %ss)", rateLimited.RetryAfter)
	case errors.As(err, &rejected):
		err := fmt.Errorf("resend answered %d %s", rejected.StatusCode, rejected.Name)
		if rejected.StatusCode == http.StatusConflict || // the same key still in flight
			rejected.StatusCode >= 500 {
			return err
		}
		return events.Permanent(err)
	case ctx.Err() != nil:
		return fmt.Errorf("resend request: %w", ctx.Err())
	default:
		// a transport error: only the method and Resend's URL, no recipient
		return fmt.Errorf("resend request: %w", err)
	}
}

// idempotencyKey is the event being handled and the recipient (hashed, as the
// key is visible in Resend's dashboard). Empty outside an event's handling.
func idempotencyKey(ctx context.Context, to string) string {
	eventID := api.EventIDFromContext(ctx)
	if eventID == "" {
		return ""
	}

	sum := sha256.Sum256([]byte(to))
	return "notifications/" + eventID + "/" + hex.EncodeToString(sum[:8])
}

var _ application.EmailSender = (*resendEmailSender)(nil)
