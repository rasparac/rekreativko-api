package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/account-profile/internal/domain"
	"github.com/rasparac/rekreativko-api/account-profile/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type (
	accountEraser interface {
		EraseAccount(ctx context.Context, accountID uuid.UUID, erasedAt time.Time) (bool, error)
		IsErased(ctx context.Context, accountID uuid.UUID) (bool, error)
	}

	transactor interface {
		WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error
	}

	// AccountErasureService erases this context's data of an account erased
	// by identity and confirms it with account_profile.account.erased.
	AccountErasureService struct {
		erasures    accountEraser
		txManager   transactor
		eventWriter domainevent.EventWriter
		logger      *logger.Logger
		metrics     *metrics.Metrics
		tracer      trace.Tracer
	}
)

func NewAccountErasureService(
	erasures accountEraser,
	txManager transactor,
	eventWriter domainevent.EventWriter,
	logger *logger.Logger,
	metrics *metrics.Metrics,
) *AccountErasureService {
	return &AccountErasureService{
		erasures:    erasures,
		txManager:   txManager,
		eventWriter: eventWriter,
		logger:      logger.WithName("account_profile.erasure"),
		metrics:     metrics,
		tracer:      telemetry.Tracer(telemetry.TracerAccountProfileService),
	}
}

// EraseAccount deletes the account's profile, interests, settings and
// statistics and raises account_profile.account.erased, in one transaction.
// An account with no profile (never verified) is erased and confirmed all the
// same. Idempotent: an account already erased is left alone and not confirmed
// again - its confirmation was written with the first erasure.
func (s *AccountErasureService) EraseAccount(ctx context.Context, accountID uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "account_profile.erasure.EraseAccount")
	defer span.End()

	span.SetAttributes(attribute.String("account_id", accountID.String()))

	var erased bool
	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		var err error
		erased, err = s.erasures.EraseAccount(tCtx, accountID, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("erase account: %w", err)
		}

		if !erased {
			return nil
		}

		err = s.eventWriter.InsertEvents(
			tCtx,
			accountProfileSchema,
			[]domainevent.Event{domain.NewAccountErasedEvent(accountID)},
		)
		if err != nil {
			return fmt.Errorf("insert events: %w", err)
		}

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if !erased {
		s.logger.Debug(ctx, "account already erased, redelivered event", "account_id", accountID)
		span.SetStatus(codes.Ok, "account already erased")
		return nil
	}

	s.metrics.AccountsErased.Inc()
	s.logger.Info(ctx, "account erased", "account_id", accountID)
	span.SetStatus(codes.Ok, "account erased")

	return nil
}

// IsErased reports whether the account's data was erased, so a late event
// does not re-create it.
func (s *AccountErasureService) IsErased(ctx context.Context, accountID uuid.UUID) (bool, error) {
	return s.erasures.IsErased(ctx, accountID)
}
