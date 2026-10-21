package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type (
	accountStates interface {
		MarkDeleted(ctx context.Context, accountID uuid.UUID, deletedAt time.Time) error
		IsInactive(ctx context.Context, accountID uuid.UUID) (bool, error)
		ListUpcomingSessions(ctx context.Context, accountID uuid.UUID) ([]uuid.UUID, error)
		ListPendingGroupInvites(ctx context.Context, accountID uuid.UUID) ([]uuid.UUID, error)
		ListPendingSessionInvites(ctx context.Context, accountID uuid.UUID) ([]uuid.UUID, error)
		ListGroupsToLeave(ctx context.Context, accountID uuid.UUID) ([]uuid.UUID, error)
		Erase(ctx context.Context, accountID uuid.UUID, erasedAt time.Time) (bool, error)
	}

	rsvpCanceller interface {
		CancelRSVP(ctx context.Context, sessionID, userID uuid.UUID) error
	}

	inviteDecliner interface {
		DeclineInvite(ctx context.Context, inviteID, userID uuid.UUID) error
	}

	groupLeaver interface {
		LeaveMember(ctx context.Context, activityGroupID, userID uuid.UUID) error
	}

	transactor interface {
		WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error
	}

	// AccountLifecycleService follows identity's account deletion and
	// erasure (epic dh7, decision dh7.3 in .claude/docs/activity.md).
	AccountLifecycleService struct {
		states         accountStates
		rsvps          rsvpCanceller
		groupInvites   inviteDecliner
		sessionInvites inviteDecliner
		groups         groupLeaver
		txManager      transactor
		eventWriter    domainevent.EventWriter
		logger         *logger.Logger
		metrics        *metrics.Metrics
		tracer         trace.Tracer
	}
)

func NewAccountLifecycleService(
	states accountStates,
	rsvps rsvpCanceller,
	groupInvites inviteDecliner,
	sessionInvites inviteDecliner,
	groups groupLeaver,
	txManager transactor,
	eventWriter domainevent.EventWriter,
	logger *logger.Logger,
	metrics *metrics.Metrics,
) *AccountLifecycleService {
	return &AccountLifecycleService{
		states:         states,
		rsvps:          rsvps,
		groupInvites:   groupInvites,
		sessionInvites: sessionInvites,
		groups:         groups,
		txManager:      txManager,
		eventWriter:    eventWriter,
		logger:         logger.WithName("activity.account_lifecycle"),
		metrics:        metrics,
		tracer:         telemetry.Tracer(telemetry.TracerActivityService),
	}
}

// DeactivateAccount takes a deleted account out of what is coming: it cancels
// its RSVPs to scheduled sessions (promoting the waitlist and resetting team
// formation as a cancellation does), declines the invites it can still answer
// and leaves every group it did not create. Each goes through the same
// service call as when the user does it, so the usual events and
// notifications follow. What it created - its groups, templates and sessions -
// keeps working for the other members until the erasure.
//
// The account is recorded as deleted first, so its requests are refused from
// then on. Safe to repeat: whatever was already given up is not listed again,
// so a redelivery after a partial failure finishes the rest.
func (s *AccountLifecycleService) DeactivateAccount(ctx context.Context, accountID uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "activity.account_lifecycle.DeactivateAccount")
	defer span.End()

	span.SetAttributes(attribute.String("account_id", accountID.String()))

	err := s.deactivate(ctx, accountID)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	s.logger.Info(ctx, "account deactivated", "account_id", accountID)
	span.SetStatus(codes.Ok, "account deactivated")

	return nil
}

func (s *AccountLifecycleService) deactivate(ctx context.Context, accountID uuid.UUID) error {
	if err := s.states.MarkDeleted(ctx, accountID, time.Now().UTC()); err != nil {
		return fmt.Errorf("mark deleted: %w", err)
	}

	steps := []struct {
		name string
		list func(context.Context, uuid.UUID) ([]uuid.UUID, error)
		do   func(ctx context.Context, id, accountID uuid.UUID) error
	}{
		{"cancel RSVP", s.states.ListUpcomingSessions, s.rsvps.CancelRSVP},
		{"decline group invite", s.states.ListPendingGroupInvites, s.groupInvites.DeclineInvite},
		{"decline session invite", s.states.ListPendingSessionInvites, s.sessionInvites.DeclineInvite},
		{"leave group", s.states.ListGroupsToLeave, s.groups.LeaveMember},
	}

	for _, step := range steps {
		ids, err := step.list(ctx, accountID)
		if err != nil {
			return fmt.Errorf("list for %s: %w", step.name, err)
		}

		for _, id := range ids {
			if err := step.do(ctx, id, accountID); err != nil {
				return fmt.Errorf("%s %s: %w", step.name, id, err)
			}
		}
	}

	return nil
}

// EraseAccount deletes what the account created and the rows about it in
// what others created, and raises activity.account.erased, in one
// transaction. Idempotent: an account already erased is left alone and not
// confirmed again - its confirmation was written with the first erasure.
func (s *AccountLifecycleService) EraseAccount(ctx context.Context, accountID uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "activity.account_lifecycle.EraseAccount")
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
			activitySchema,
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

	s.metrics.AccountsErasedTotal.Inc()
	s.logger.Info(ctx, "account erased", "account_id", accountID)
	span.SetStatus(codes.Ok, "account erased")

	return nil
}

// inactiveAccountChecker tells whether an account was deleted or erased in
// identity; persistence.AccountStateManager implements it.
type inactiveAccountChecker interface {
	IsInactive(ctx context.Context, accountID uuid.UUID) (bool, error)
}

// ensureAccountActive returns domain.ErrAccountDeleted for an account deleted
// or erased in identity. Called before a request creates data about another
// account - inviting it, approving its request - which would otherwise outlive
// its erasure or bring it back into groups it was taken out of.
func ensureAccountActive(ctx context.Context, accounts inactiveAccountChecker, accountID uuid.UUID) error {
	inactive, err := accounts.IsInactive(ctx, accountID)
	if err != nil {
		return fmt.Errorf("check account %s: %w", accountID, err)
	}
	if inactive {
		return domain.ErrAccountDeleted
	}

	return nil
}

// IsInactive reports whether the account was deleted or erased.
func (s *AccountLifecycleService) IsInactive(ctx context.Context, accountID uuid.UUID) (bool, error) {
	return s.states.IsInactive(ctx, accountID)
}
