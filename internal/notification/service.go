package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rasparac/rekreativko-api/internal/shared/telemetry"
	"go.opentelemetry.io/otel/trace"
)

type (
	service struct {
		SMSSender   SMSSender
		EmailSender EmailSender

		tracer  trace.Tracer
		metrics Metrics
	}

	Metrics interface {
		NotificationSendTotal() *prometheus.CounterVec
		NotificationSendFailures() *prometheus.CounterVec
		NotificationSendDuration() *prometheus.HistogramVec
	}
)

func NewService(
	smsSender SMSSender,
	emailSender EmailSender,
	metrics Metrics,
) *service {
	return &service{
		SMSSender:   smsSender,
		EmailSender: emailSender,

		metrics: metrics,
		tracer:  telemetry.Tracer("notification.service"),
	}
}

func (s *service) sendVerificationCodeEmail(ctx context.Context, to, code string) error {
	var (
		subject = "Rekreativko verification code"
		body    = fmt.Sprintf(`
		Hello,

		Thank you for registering with Rekreativko. Your verification code is: %s

		This code will expire in 15 minutes.

		Best regards,
		Rekreativko
		`, code)
	)

	s.metrics.NotificationSendTotal().WithLabelValues("verification", "email").Inc()

	start := time.Now()

	err := s.EmailSender.SendEmail(ctx, to, subject, body)
	if err != nil {
		s.metrics.NotificationSendFailures().WithLabelValues("verification", "email", "send_failed").Inc()
		return fmt.Errorf("send verification code email: %w", err)
	}

	duration := time.Since(start).Milliseconds()
	s.metrics.NotificationSendDuration().WithLabelValues("verification", "email").Observe(float64(duration))

	return err
}

func (s *service) sendVerificationCodeSMS(ctx context.Context, to, code string) error {
	message := fmt.Sprintf("Your Rekreativko verification code is: %s", code)
	start := time.Now()

	s.metrics.NotificationSendTotal().WithLabelValues("verification", "sms").Inc()

	err := s.SMSSender.SendSMS(ctx, to, message)
	if err != nil {
		s.metrics.NotificationSendFailures().WithLabelValues("verification", "sms", "send_failed").Inc()
		return fmt.Errorf("send verification code sms: %w", err)
	}

	duration := time.Since(start).Milliseconds()
	s.metrics.NotificationSendDuration().WithLabelValues("verification", "sms").Observe(float64(duration))

	return err
}

func (s *service) sendWelcomeEmail(ctx context.Context, to string) error {
	var (
		subject = `Welcome to Rekreativko!`
		body    = `
		Hello,
		
		Thank you for registering with Rekreativko.
		
		Best regards,
		Rekreativko
		`
	)

	s.metrics.NotificationSendTotal().WithLabelValues("welcome_message", "email").Inc()

	start := time.Now()

	err := s.EmailSender.SendEmail(ctx, to, subject, body)
	if err != nil {
		s.metrics.NotificationSendFailures().WithLabelValues("welcome_message", "email", "send_failed").Inc()
		return fmt.Errorf("send welcome email: %w", err)
	}

	duration := time.Since(start).Milliseconds()
	s.metrics.NotificationSendDuration().WithLabelValues("welcome_message", "email").Observe(float64(duration))
	return err
}

func (s *service) sendPasswordChangedEmail(ctx context.Context, to string) error {
	var (
		subject = "Rekreativko password changed"
		body    = `
		Hello,
		
		Your password has been changed.
		
		If you did not change your password, please contact support.

		Best regards,
		Rekreativko
		`
	)

	s.metrics.NotificationSendTotal().WithLabelValues("password_changed", "email").Inc()

	start := time.Now()

	err := s.EmailSender.SendEmail(ctx, to, subject, body)
	if err != nil {
		s.metrics.NotificationSendFailures().WithLabelValues("password_changed", "email", "send_failed").Inc()
		return fmt.Errorf("send password changed email: %w", err)
	}

	duration := time.Since(start).Milliseconds()
	s.metrics.NotificationSendDuration().WithLabelValues("password_changed", "email").Observe(float64(duration))
	return err
}

func (s *service) sendPasswordChangedSMS(ctx context.Context, to string) error {
	message := "Your Rekreativko password has been changed. If you did not change your password, please contact support."
	s.metrics.NotificationSendTotal().WithLabelValues("password_changed", "sms").Inc()

	start := time.Now()

	err := s.SMSSender.SendSMS(ctx, to, message)
	if err != nil {
		s.metrics.NotificationSendFailures().WithLabelValues("password_changed", "sms", "send_failed").Inc()
		return fmt.Errorf("send password changed sms: %w", err)
	}

	duration := time.Since(start).Milliseconds()
	s.metrics.NotificationSendDuration().WithLabelValues("password_changed", "sms").Observe(float64(duration))
	return err
}

func (s *service) sendAccountLockedEmail(ctx context.Context, to, reason string) error {
	var (
		subject = "Rekreativko account locked"
		body    = fmt.Sprintf(`
		Hello,
		
		Your Rekreativko account has been locked for the following reason: %s
		
		Best regards,
		Rekreativko
		`, reason)
	)

	s.metrics.NotificationSendTotal().WithLabelValues("account_locked", "email").Inc()

	start := time.Now()

	err := s.EmailSender.SendEmail(ctx, to, subject, body)
	if err != nil {
		s.metrics.NotificationSendFailures().WithLabelValues("account_locked", "email", "send_failed").Inc()
		return fmt.Errorf("send password changed email: %w", err)
	}

	duration := time.Since(start).Milliseconds()
	s.metrics.NotificationSendDuration().WithLabelValues("account_locked", "email").Observe(float64(duration))
	return err
}

func (s *service) sendAccountLockedSMS(ctx context.Context, to, reason string) error {
	message := fmt.Sprintf("Your Rekreativko account has been locked for the following reason: %s", reason)
	s.metrics.NotificationSendTotal().WithLabelValues("account_locked", "sms").Inc()

	start := time.Now()

	err := s.SMSSender.SendSMS(ctx, to, message)
	if err != nil {
		s.metrics.NotificationSendFailures().WithLabelValues("account_locked", "sms", "send_failed").Inc()
		return fmt.Errorf("send account locked sms: %w", err)
	}

	duration := time.Since(start).Milliseconds()
	s.metrics.NotificationSendDuration().WithLabelValues("account_locked", "sms").Observe(float64(duration))
	return err
}
