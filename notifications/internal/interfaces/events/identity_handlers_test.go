package events

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	dto "github.com/prometheus/client_model/go"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingSender remembers where each message went.
type recordingSender struct {
	emails []string
	sms    []string
}

func (r *recordingSender) SendEmail(_ context.Context, to, _, _ string) error {
	r.emails = append(r.emails, to)
	return nil
}

func (r *recordingSender) SendSMS(_ context.Context, to, _ string) error {
	r.sms = append(r.sms, to)
	return nil
}

func counterValue(t *testing.T, c interface{ Write(*dto.Metric) error }) float64 {
	t.Helper()

	var m dto.Metric
	require.NoError(t, c.Write(&m))
	return m.GetCounter().GetValue()
}

// Each identity-event handler routes by the event's code_type: email goes to
// the email sender, phone to the SMS sender (or, for the welcome message, to
// nobody - there is no welcome SMS). Anything else is dropped loudly, not
// silently: an error log and a send-failure metric.
func TestEventHandlers_RouteByDeliveryType(t *testing.T) {
	type outcome struct {
		emailsTo, smsTo []string
		dropped         bool
	}
	nobody := outcome{}
	emailed := outcome{emailsTo: []string{secretEmail}}
	texted := outcome{smsTo: []string{secretPhone}}
	dropped := outcome{dropped: true}

	for name, tc := range map[string]struct {
		notificationType string
		handle           func(*IdentityHandlers) func(context.Context, []byte) error
		want             map[string]outcome // by code_type
	}{
		"verification code": {
			notificationType: "verification",
			handle: func(h *IdentityHandlers) func(context.Context, []byte) error {
				return h.HandleVerificationCodeGenerated
			},
			want: map[string]outcome{"email": emailed, "phone": texted, "sms": dropped, "": dropped},
		},
		"account verified": {
			notificationType: "welcome_message",
			handle:           func(h *IdentityHandlers) func(context.Context, []byte) error { return h.HandleAccountVerified },
			want:             map[string]outcome{"email": emailed, "phone": nobody, "sms": dropped, "": dropped},
		},
		"password changed": {
			notificationType: "password_changed",
			handle:           func(h *IdentityHandlers) func(context.Context, []byte) error { return h.HandlePasswordChanged },
			want:             map[string]outcome{"email": emailed, "phone": texted, "sms": dropped, "": dropped},
		},
	} {
		for deliveryType, want := range tc.want {
			t.Run(name+"/code_type="+deliveryType, func(t *testing.T) {
				var (
					sender  recordingSender
					logs    bytes.Buffer
					metrics = newFakeMetrics()
					log     = logger.NewWithWriter("error", "json", &logs)
					h       = newTestHandlers(t, &sender, metrics, log)
				)

				payload, err := json.Marshal(map[string]any{
					"event_id":   uuid.New(),
					"account_id": uuid.New(),
					"code_type":  deliveryType,
					"code":       secretCode,
					"email":      secretEmail,
					"phone":      secretPhone,
				})
				require.NoError(t, err)

				// Even a dropped event is acked: a retry can't fix a bad payload.
				require.NoError(t, tc.handle(h)(context.Background(), payload))

				assert.Equal(t, want.emailsTo, sender.emails)
				assert.Equal(t, want.smsTo, sender.sms)

				unknown := counterValue(t, metrics.failures.WithLabelValues(tc.notificationType, "unknown", "unknown_delivery_type"))
				if want.dropped {
					assert.Equal(t, 1.0, unknown, "the drop is counted")
					assert.Contains(t, logs.String(), "dropping event with unknown delivery type")
					// The log names the type, never the contact details or the code.
					assert.NotContains(t, logs.String(), secretEmail)
					assert.NotContains(t, logs.String(), secretPhone)
					assert.NotContains(t, logs.String(), secretCode)
				} else {
					assert.Zero(t, unknown)
					assert.Empty(t, logs.String())
				}
			})
		}
	}
}

// An account activated without verification (identity's flag off) never proved
// the address is theirs: no welcome email, and the event is acked.
func TestHandleAccountVerified_UnconfirmedSendsNothing(t *testing.T) {
	var (
		sender  recordingSender
		logs    bytes.Buffer
		metrics = newFakeMetrics()
		h       = newTestHandlers(t, &sender, metrics, logger.NewWithWriter("error", "json", &logs))
	)

	payload, err := json.Marshal(map[string]any{
		"event_id":    uuid.New(),
		"account_id":  uuid.New(),
		"code_type":   "email",
		"email":       secretEmail,
		"unconfirmed": true,
	})
	require.NoError(t, err)

	require.NoError(t, h.HandleAccountVerified(context.Background(), payload))

	assert.Empty(t, sender.emails)
	assert.Empty(t, sender.sms)
	assert.Empty(t, logs.String())
}

// A deleted or erased account gets no email or SMS any more; the event is acked.
func TestIdentityHandlers_DeletedAccountGetsNothing(t *testing.T) {
	for name, handle := range map[string]func(*IdentityHandlers) func(context.Context, []byte) error{
		"verification code": func(h *IdentityHandlers) func(context.Context, []byte) error {
			return h.HandleVerificationCodeGenerated
		},
		"account verified": func(h *IdentityHandlers) func(context.Context, []byte) error { return h.HandleAccountVerified },
		"password changed": func(h *IdentityHandlers) func(context.Context, []byte) error { return h.HandlePasswordChanged },
	} {
		t.Run(name, func(t *testing.T) {
			var (
				sender    recordingSender
				accountID = uuid.New()
				h         = newTestHandlers(t, &sender, newFakeMetrics(), logger.New("error", "json"))
			)
			h.accounts = fakeInactiveAccounts{accountID: {}}

			payload, err := json.Marshal(map[string]any{
				"event_id":   uuid.New(),
				"account_id": accountID,
				"code_type":  "email",
				"code":       secretCode,
				"email":      secretEmail,
			})
			require.NoError(t, err)

			require.NoError(t, handle(h)(context.Background(), payload))

			assert.Empty(t, sender.emails)
		})
	}
}
