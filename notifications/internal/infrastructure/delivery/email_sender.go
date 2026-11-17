package delivery

import (
	"context"

	"github.com/rasparac/rekreativko-api/notifications/internal/application"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

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

var _ application.EmailSender = (*InMemoryEmailSender)(nil)
