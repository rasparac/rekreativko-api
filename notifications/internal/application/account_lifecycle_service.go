package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
	"github.com/rasparac/rekreativko-api/notifications/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// notificationsSchema is where this service's outbox lives.
const notificationsSchema = "notifications"

type (
	accountStates interface {
		MarkDeleted(ctx context.Context, accountID uuid.UUID, deletedAt time.Time) error
		Erase(ctx context.Context, accountID uuid.UUID, erasedAt time.Time) (bool, error)
		IsInactive(ctx context.Context, accountID uuid.UUID) (bool, error)
	}

	transactor interface {
		WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error
	}

	// AccountLifecycleService follows identity's account deletion and
	// erasure: a deleted account gets nothing more, an erased one loses its
	// feed, confirmed with notifications.account.erased.
	AccountLifecycleService struct {
		states      accountStates
		txManager   transactor
		eventWriter domainevent.EventWriter
		logger      *logger.Logger
		metrics     *metrics.Metrics
		tracer      trace.Tracer
	}
)

func NewAccountLifecycleService(
	states accountStates,
	txManager transactor,
	eventWriter domainevent.EventWriter,
	logger *logger.Logger,
	metrics *metrics.Metrics,
) *AccountLifecycleService {
	return &AccountLifecycleService{
		states:      states,
		txManager:   txManager,
		eventWriter: eventWriter,
		logger:      logger.WithName("notifications.account_lifecycle"),
		metrics:     metrics,
		tracer:      telemetry.Tracer(telemetry.TracerNotificationsService),
	}
}

// DeactivateAccount stops email/SMS delivery and feed notifications for a
// deleted account. Its feed stays until the erasure. Idempotent.
func (s *AccountLifecycleService) DeactivateAccount(ctx context.Context, accountID uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "notifications.account_lifecycle.DeactivateAccount")
	defer span.End()

	span.SetAttributes(attribute.String("account_id", accountID.String()))

	if err := s.states.MarkDeleted(ctx, accountID, time.Now().UTC()); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("mark deleted: %w", err)
	}

	s.logger.Debug(ctx, "account deactivated", "account_id", accountID)
	span.SetStatus(codes.Ok, "account deactivated")

	return nil
}

// EraseAccount deletes every notification the account received and raises
// notifications.account.erased, in one transaction. Idempotent: an account
// already erased is left alone and not confirmed again - its confirmation was
// written with the first erasure.
func (s *AccountLifecycleService) EraseAccount(ctx context.Context, accountID uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "notifications.account_lifecycle.EraseAccount")
	defer span.End()

	span.SetAttributes(attribute.String("account_id", accountID.String()))

	var erased bool
	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		var err error
		erased, err = s.states.Erase(tCtx, accountID, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("erase account: %w", err)
		}

		if !erased {
			return nil
		}

		err = s.eventWriter.InsertEvents(
			tCtx,
			notificationsSchema,
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

// IsInactive reports whether the account was deleted or erased, so nothing is
// delivered to it and no notification is created for it.
func (s *AccountLifecycleService) IsInactive(ctx context.Context, accountID uuid.UUID) (bool, error) {
	return s.states.IsInactive(ctx, accountID)
}
