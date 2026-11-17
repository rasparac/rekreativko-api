package delivery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rasparac/rekreativko-api/notifications/internal/application"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/events"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

const resendAPIURL = "https://api.resend.com"

// ResendEmailSender sends email through Resend's REST API
// (https://resend.com/docs/api-reference/emails/send-email). Not the Go SDK:
// it reports a 400/422 as a bare string, so a request Resend will never accept
// could not be told from an outage, and its error text may repeat the
// recipient's address.
type ResendEmailSender struct {
	apiKey  string
	from    string
	baseURL string
	client  *http.Client
	logger  *logger.Logger
}

func NewResendEmailSender(apiKey, from string, log *logger.Logger) *ResendEmailSender {
	return &ResendEmailSender{
		apiKey:  apiKey,
		from:    from,
		baseURL: resendAPIURL,
		client:  &http.Client{Timeout: 10 * time.Second},
		logger:  log.WithName("notifications.resend"),
	}
}

type (
	resendSendRequest struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		Text    string   `json:"text"`
	}

	resendSendResponse struct {
		ID string `json:"id"`
	}

	resendErrorResponse struct {
		Name string `json:"name"`
	}
)

// SendEmail sends one plain-text email. Within the handling of one event it
// carries an idempotency key, so a redelivered event (or a retry after a lost
// response) does not send it twice; Resend keeps keys for 24 hours.
//
// A rejection retrying cannot fix (any 4xx but 409 and 429) is an
// events.Permanent error, so the event is dead-lettered instead of redelivered.
// Errors never carry the recipient, the text or Resend's message (it may quote
// the address) - only the status and Resend's error name.
func (s *ResendEmailSender) SendEmail(ctx context.Context, to, subject, body string) error {
	payload, err := json.Marshal(resendSendRequest{From: s.from, To: []string{to}, Subject: subject, Text: body})
	if err != nil {
		return fmt.Errorf("encode resend request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/emails", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if key := idempotencyKey(ctx, to); key != "" {
		req.Header.Set("Idempotency-Key", key)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("resend request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return fmt.Errorf("read resend response: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var sent resendSendResponse
		_ = json.Unmarshal(respBody, &sent) // the id is only logged
		s.logger.Debug(ctx, "email sent", "resend_id", sent.ID)
		return nil
	}

	var rejected resendErrorResponse
	_ = json.Unmarshal(respBody, &rejected)
	err = fmt.Errorf("resend answered %d %s", resp.StatusCode, rejected.Name)

	switch {
	case resp.StatusCode == http.StatusTooManyRequests,
		resp.StatusCode == http.StatusConflict, // the same key still in flight
		resp.StatusCode >= 500:
		return err
	default:
		return events.Permanent(err)
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

var _ application.EmailSender = (*ResendEmailSender)(nil)
