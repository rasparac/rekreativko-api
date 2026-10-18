package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rasparac/rekreativko-api/notifications/internal/application"
	"github.com/rasparac/rekreativko-api/notifications/internal/infrastructure/templates"
	"github.com/rasparac/rekreativko-api/shared/logger"
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

// fakeMetrics keeps its vectors so a test can read what a handler counted.
type fakeMetrics struct {
	total, failures *prometheus.CounterVec
	duration        *prometheus.HistogramVec
}

func newFakeMetrics() *fakeMetrics {
	return &fakeMetrics{
		total:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: "t"}, []string{"type", "channel"}),
		failures: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "f"}, []string{"type", "channel", "reason"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "d"}, []string{"type", "channel"}),
	}
}

func (m *fakeMetrics) NotificationSendTotal() *prometheus.CounterVec      { return m.total }
func (m *fakeMetrics) NotificationSendFailures() *prometheus.CounterVec   { return m.failures }
func (m *fakeMetrics) NotificationSendDuration() *prometheus.HistogramVec { return m.duration }

// newTestHandlers wires the real delivery layer (and templates) to a fake sender.
func newTestHandlers(t *testing.T, sender interface {
	application.EmailSender
	application.SMSSender
}, metrics *fakeMetrics, log *logger.Logger) *IdentityHandlers {
	t.Helper()

	renderer, err := templates.NewRenderer()
	require.NoError(t, err)

	delivery := application.NewDeliveryService(sender, sender, renderer, metrics, log)

	return NewIdentityHandlers(delivery, metrics, log)
}

type okSender struct{}

func (okSender) SendEmail(context.Context, string, string, string) error { return nil }
func (okSender) SendSMS(context.Context, string, string) error           { return nil }

// A span is exported to Jaeger, readable by anyone who can read traces, so no
// handler may put a verification code or the recipient's email/phone on one.
func TestEventHandlers_SpansCarryNoCodeEmailOrPhone(t *testing.T) {
	handlers := map[string]func(*IdentityHandlers) func(context.Context, []byte) error{
		"verification code": func(h *IdentityHandlers) func(context.Context, []byte) error {
			return h.HandleVerificationCodeGenerated
		},
		"account verified": func(h *IdentityHandlers) func(context.Context, []byte) error { return h.HandleAccountVerified },
		"password changed": func(h *IdentityHandlers) func(context.Context, []byte) error { return h.HandlePasswordChanged },
	}

	// "sms" is no delivery type (identity sends "email" or "phone"), so it takes
	// the unknown-type branch: that path must not leak either.
	for _, deliveryType := range []string{DeliveryTypeEmail, DeliveryTypePhone, "sms"} {
		for name, handler := range handlers {
			t.Run(fmt.Sprintf("%s/%s", name, deliveryType), func(t *testing.T) {
				recorder := tracetest.NewSpanRecorder()
				provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
				h := newTestHandlers(t, okSender{}, newFakeMetrics(), logger.New("error", "json"))
				h.tracer = provider.Tracer("test")

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

				require.NoError(t, handler(h)(context.Background(), payload))

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
