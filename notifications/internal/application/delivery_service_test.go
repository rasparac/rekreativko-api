package application

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type deliveryMetrics struct {
	total, failures *prometheus.CounterVec
	duration        *prometheus.HistogramVec
}

func newDeliveryMetrics() *deliveryMetrics {
	return &deliveryMetrics{
		total:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: "t"}, []string{"type", "channel"}),
		failures: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "f"}, []string{"type", "channel", "reason"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "d"}, []string{"type", "channel"}),
	}
}

func (m *deliveryMetrics) NotificationSendTotal() *prometheus.CounterVec    { return m.total }
func (m *deliveryMetrics) NotificationSendFailures() *prometheus.CounterVec { return m.failures }
func (m *deliveryMetrics) NotificationSendDuration() *prometheus.HistogramVec {
	return m.duration
}

type fakeSenders struct {
	emails, sms []string // "to|subject|body" and "to|body"
	err         error
}

func (f *fakeSenders) SendEmail(_ context.Context, to, subject, body string) error {
	f.emails = append(f.emails, to+"|"+subject+"|"+body)
	return f.err
}

func (f *fakeSenders) SendSMS(_ context.Context, to, body string) error {
	f.sms = append(f.sms, to+"|"+body)
	return f.err
}

type fakeRenderer struct{ err error }

func (r fakeRenderer) Render(msg domain.Message) (domain.RenderedMessage, error) {
	return domain.RenderedMessage{Subject: "subject", Body: "body " + msg.Data["code"]}, r.err
}

func counter(t *testing.T, c interface{ Write(*dto.Metric) error }) float64 {
	t.Helper()

	var m dto.Metric
	require.NoError(t, c.Write(&m))
	return m.GetCounter().GetValue()
}

func TestDeliveryService_Deliver(t *testing.T) {
	newService := func(senders *fakeSenders, renderer fakeRenderer) (*DeliveryService, *deliveryMetrics) {
		metrics := newDeliveryMetrics()
		return NewDeliveryService(senders, senders, renderer, metrics, logger.New("error", "json")), metrics
	}

	t.Run("email goes to the email sender with subject and body", func(t *testing.T) {
		senders := &fakeSenders{}
		svc, metrics := newService(senders, fakeRenderer{})

		err := svc.Deliver(context.Background(), domain.Message{
			Kind: domain.MessageVerificationCode, Channel: domain.ChannelEmail,
			Recipient: "a@example.com", Data: map[string]string{"code": "1"},
		})

		require.NoError(t, err)
		assert.Equal(t, []string{"a@example.com|subject|body 1"}, senders.emails)
		assert.Empty(t, senders.sms)
		assert.Equal(t, 1.0, counter(t, metrics.total.WithLabelValues("verification", "email")))
	})

	t.Run("sms goes to the sms sender with the body only", func(t *testing.T) {
		senders := &fakeSenders{}
		svc, _ := newService(senders, fakeRenderer{})

		err := svc.Deliver(context.Background(), domain.Message{
			Kind: domain.MessagePasswordChanged, Channel: domain.ChannelSMS, Recipient: "+385",
		})

		require.NoError(t, err)
		assert.Equal(t, []string{"+385|body "}, senders.sms)
		assert.Empty(t, senders.emails)
	})

	t.Run("a failing sender is counted and returned", func(t *testing.T) {
		senders := &fakeSenders{err: errors.New("smtp down")}
		svc, metrics := newService(senders, fakeRenderer{})

		err := svc.Deliver(context.Background(), domain.Message{
			Kind: domain.MessageWelcome, Channel: domain.ChannelEmail, Recipient: "a@example.com",
		})

		require.ErrorContains(t, err, "smtp down")
		assert.NotContains(t, err.Error(), "a@example.com")
		assert.Equal(t, 1.0, counter(t, metrics.failures.WithLabelValues("welcome_message", "email", "send_failed")))
	})

	t.Run("a render failure sends nothing and is counted", func(t *testing.T) {
		senders := &fakeSenders{}
		svc, metrics := newService(senders, fakeRenderer{err: errors.New("no template")})

		err := svc.Deliver(context.Background(), domain.Message{
			Kind: domain.MessageWelcome, Channel: domain.ChannelEmail, Recipient: "a@example.com",
		})

		require.Error(t, err)
		assert.Empty(t, senders.emails)
		assert.Equal(t, 1.0, counter(t, metrics.failures.WithLabelValues("welcome_message", "email", "render_failed")))
	})

	t.Run("an unknown channel is an error", func(t *testing.T) {
		senders := &fakeSenders{}
		svc, _ := newService(senders, fakeRenderer{})

		err := svc.Deliver(context.Background(), domain.Message{Kind: domain.MessageWelcome, Channel: "push"})

		require.Error(t, err)
		assert.Empty(t, senders.emails)
		assert.Empty(t, senders.sms)
	})
}
