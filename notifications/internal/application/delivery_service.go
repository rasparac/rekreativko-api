package application

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

type (
	EmailSender interface {
		SendEmail(ctx context.Context, to, subject, body string) error
	}

	SMSSender interface {
		SendSMS(ctx context.Context, to, message string) error
	}

	// MessageRenderer turns a Message into the text of one language.
	MessageRenderer interface {
		Render(msg domain.Message) (domain.RenderedMessage, error)
	}

	DeliveryMetrics interface {
		NotificationSendTotal() *prometheus.CounterVec
		NotificationSendFailures() *prometheus.CounterVec
		NotificationSendDuration() *prometheus.HistogramVec
	}

	// DeliveryService is the delivery layer: render a Message and hand it to
	// the sender of its channel. It keeps no state; the in-app feed is
	// NotificationService.
	DeliveryService struct {
		email    EmailSender
		sms      SMSSender
		renderer MessageRenderer
		metrics  DeliveryMetrics
		logger   *logger.Logger
	}
)

func NewDeliveryService(
	email EmailSender,
	sms SMSSender,
	renderer MessageRenderer,
	metrics DeliveryMetrics,
	log *logger.Logger,
) *DeliveryService {
	return &DeliveryService{
		email:    email,
		sms:      sms,
		renderer: renderer,
		metrics:  metrics,
		logger:   log.WithName("notification.delivery"),
	}
}

// Deliver sends msg. An error is returned for the caller to retry; it never
// contains the recipient or the text.
func (s *DeliveryService) Deliver(ctx context.Context, msg domain.Message) error {
	kind, channel := string(msg.Kind), string(msg.Channel)

	s.metrics.NotificationSendTotal().WithLabelValues(kind, channel).Inc()

	rendered, err := s.renderer.Render(msg)
	if err != nil {
		s.metrics.NotificationSendFailures().WithLabelValues(kind, channel, "render_failed").Inc()
		return fmt.Errorf("render %s %s: %w", kind, channel, err)
	}

	start := time.Now()

	switch msg.Channel {
	case domain.ChannelEmail:
		err = s.email.SendEmail(ctx, msg.Recipient, rendered.Subject, rendered.Body)
	case domain.ChannelSMS:
		err = s.sms.SendSMS(ctx, msg.Recipient, rendered.Body)
	default:
		err = fmt.Errorf("unknown channel %q", channel)
	}
	if err != nil {
		s.metrics.NotificationSendFailures().WithLabelValues(kind, channel, "send_failed").Inc()
		return fmt.Errorf("send %s %s: %w", kind, channel, err)
	}

	s.metrics.NotificationSendDuration().WithLabelValues(kind, channel).Observe(float64(time.Since(start).Milliseconds()))

	return nil
}
