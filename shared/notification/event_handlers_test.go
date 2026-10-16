package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

const (
	secretCode  = "482913"
	secretEmail = "someone.private@example.com"
	secretPhone = "+385911234567"
)

type fakeMetrics struct{}

func (fakeMetrics) NotificationSendTotal() *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{Name: "t"}, []string{"type", "channel"})
}

func (fakeMetrics) NotificationSendFailures() *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{Name: "f"}, []string{"type", "channel", "reason"})
}

func (fakeMetrics) NotificationSendDuration() *prometheus.HistogramVec {
	return prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "d"}, []string{"type", "channel"})
}

type okSender struct{}

func (okSender) SendEmail(context.Context, string, string, string) error { return nil }
func (okSender) SendSMS(context.Context, string, string) error           { return nil }

// A span is exported to Jaeger, readable by anyone who can read traces, so no
// handler may put a verification code or the recipient's email/phone on one.
func TestEventHandlers_SpansCarryNoCodeEmailOrPhone(t *testing.T) {
	handlers := map[string]func(*service) func(context.Context, []byte) error{
		"verification code": func(s *service) func(context.Context, []byte) error { return s.HandleVerificationCodeGenerated },
		"account verified":  func(s *service) func(context.Context, []byte) error { return s.HandleAccountVerified },
		"password changed":  func(s *service) func(context.Context, []byte) error { return s.HandlePasswordChanged },
		"account locked":    func(s *service) func(context.Context, []byte) error { return s.HandleAccountLocked },
	}

	// "sms" is what password-changed/locked handlers match, "phone" what the
	// verification handlers match - send every spelling so each branch runs.
	for _, deliveryType := range []string{"email", "phone", "sms"} {
		for name, handler := range handlers {
			t.Run(fmt.Sprintf("%s/%s", name, deliveryType), func(t *testing.T) {
				recorder := tracetest.NewSpanRecorder()
				provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
				svc := &service{
					SMSSender:   okSender{},
					EmailSender: okSender{},
					tracer:      provider.Tracer("test"),
					metrics:     fakeMetrics{},
				}

				eventID, accountID := uuid.New(), uuid.New()
				payload, err := json.Marshal(map[string]any{
					"event_id":   eventID,
					"account_id": accountID,
					"code_type":  deliveryType,
					"code":       secretCode,
					"email":      secretEmail,
					"phone":      secretPhone,
					"reason":     "too many attempts",
				})
				require.NoError(t, err)

				require.NoError(t, handler(svc)(context.Background(), payload))

				spans := recorder.Ended()
				require.NotEmpty(t, spans)

				for _, span := range spans {
					var recorded []string
					for _, kv := range span.Attributes() {
						recorded = append(recorded, kv.Value.Emit())
					}
					for _, event := range span.Events() {
						recorded = append(recorded, event.Name)
						for _, kv := range event.Attributes {
							recorded = append(recorded, kv.Value.Emit())
						}
					}
					recorded = append(recorded, span.Status().Description)

					all := strings.Join(recorded, "\n")
					for _, secret := range []string{secretCode, secretEmail, secretPhone} {
						assert.NotContains(t, all, secret, "span %q", span.Name())
					}

					// Still enough to correlate with the event.
					assert.Contains(t, all, eventID.String(), "span %q", span.Name())
					assert.Contains(t, all, accountID.String(), "span %q", span.Name())
				}
			})
		}
	}
}
