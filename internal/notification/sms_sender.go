package notification

import (
	"context"

	"github.com/rasparac/rekreativko-api/internal/shared/logger"
)

type SMSSender interface {
	SendSMS(ctx context.Context, to, message string) error
}

type InMemorySMSSender struct {
	logger *logger.Logger
}

func NewInMemorySMSSender(log *logger.Logger) *InMemorySMSSender {
	return &InMemorySMSSender{
		logger: log,
	}
}

func (isms *InMemorySMSSender) SendSMS(ctx context.Context, to, message string) error {
	isms.logger.Info(ctx, "sending sms", "to", to, "message", message)
	return nil
}

// TODO add real sms sender implementation
