package notification

import (
	"context"

	"github.com/rasparac/rekreativko-api/internal/shared/logger"
)

type EmailSender interface {
	SendEmail(ctx context.Context, to, subject, body string) error
}

type SMTPEmailSender struct {
	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
}

func NewSMTPEmailSender(
	smtpHost string,
	smtpPort int,
	smtpUser string,
	smtpPass string,
) *SMTPEmailSender {
	return &SMTPEmailSender{
		SMTPHost: smtpHost,
		SMTPPort: smtpPort,
		SMTPUser: smtpUser,
		SMTPPass: smtpPass,
	}
}

func (e *SMTPEmailSender) SendEmail(ctx context.Context, to, subject, body string) error {
	return nil
}

type InMemoryEmailSender struct {
	logger *logger.Logger
}

func NewInMemoryEmailSender(log *logger.Logger) *InMemoryEmailSender {
	return &InMemoryEmailSender{
		logger: log,
	}
}

func (imes *InMemoryEmailSender) SendEmail(ctx context.Context, to, subject, body string) error {
	imes.logger.Info(ctx, "sending email", "to", to, "subject", subject, "body", body)
	return nil
}

var _ EmailSender = (*InMemoryEmailSender)(nil)

var _ EmailSender = (*SMTPEmailSender)(nil)
