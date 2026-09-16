package notification

import (
	"context"
	"fmt"

	"github.com/rasparac/rekreativko-api/shared/logger"
)

type SMSSender interface {
	SendSMS(ctx context.Context, to, message string) error
}

// DisabledSMSSender is wired in when phone registration is turned off (see
// config.FeaturesConfig.PhoneRegistrationEnabled) and no real SMS provider is
// configured. The SMS path should be unreachable in that state, but this
// fails loudly if it's ever invoked anyway - a silent no-op would hide the
// bug that made it reachable in the first place.
type DisabledSMSSender struct {
	logger *logger.Logger
}

func NewDisabledSMSSender(log *logger.Logger) *DisabledSMSSender {
	return &DisabledSMSSender{
		logger: log,
	}
}

func (dsms *DisabledSMSSender) SendSMS(ctx context.Context, to, message string) error {
	dsms.logger.Error(ctx, "sms send attempted while SMS sending is disabled", "to", to)
	return fmt.Errorf("sms sending is disabled: phone registration is not enabled and no SMS provider is configured")
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
