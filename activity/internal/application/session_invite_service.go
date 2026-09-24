package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/activity/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// SessionInviteService handles business logic for invites to standalone
// sessions. A standalone session has no group, so GroupInvite can't cover it
// and, if it is private, nobody but the creator could otherwise ever attend.
type SessionInviteService struct {
	logger            *logger.Logger
	txManager         *postgres.TransactionManager
	sessionInviteRepo SessionInviteRepository
	sessionRepo       SessionRepository
	attendeeRepo      AttendeeRepository
	eventWriter       domainevent.EventWriter
	tracer            trace.Tracer
	metrics           *metrics.Metrics
}

// NewSessionInviteService creates a new session invite service
func NewSessionInviteService(
	logger *logger.Logger,
	txManager *postgres.TransactionManager,
	sessionInviteRepo SessionInviteRepository,
	sessionRepo SessionRepository,
	attendeeRepo AttendeeRepository,
	eventWriter domainevent.EventWriter,
	metrics *metrics.Metrics,
) *SessionInviteService {
	return &SessionInviteService{
		logger:            logger.WithName("activity.session_invite_service"),
		txManager:         txManager,
		sessionInviteRepo: sessionInviteRepo,
		sessionRepo:       sessionRepo,
		attendeeRepo:      attendeeRepo,
		eventWriter:       eventWriter,
		tracer:            telemetry.Tracer(telemetry.TracerActivityService),
		metrics:           metrics,
	}
}

// SendInvite invites a user to a standalone session. Only the session's
// creator can invite.
func (s *SessionInviteService) SendInvite(
	ctx context.Context,
	params SendSessionInviteParams,
) (*domain.SessionInvite, error) {
	ctx, span := s.tracer.Start(ctx, "activity.service.SendSessionInvite")
	defer span.End()

	log := s.logger.WithValues(
		"method", "SendInvite",
		"session_id", params.SessionID,
		"requester_id", params.RequesterID,
		"invited_user_id", params.InvitedUserID,
	)

	span.SetAttributes(
		attribute.String("session_id", params.SessionID.String()),
		attribute.String("requester_id", params.RequesterID.String()),
		attribute.String("invited_user_id", params.InvitedUserID.String()),
	)

	var invite *domain.SessionInvite

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		session, err := s.sessionRepo.GetSessionByID(tCtx, params.SessionID)
		if err != nil {
			return fmt.Errorf("get session: %w", err)
		}

		invite, err = domain.NewSessionInvite(session, params.InvitedUserID, params.RequesterID)
		if err != nil {
			return fmt.Errorf("create session invite: %w", err)
		}

		// Reject if the invited user is already attending
		_, err = s.attendeeRepo.GetAttendeeBySessionAndUser(tCtx, params.SessionID, params.InvitedUserID)
		switch {
		case err == nil:
			return domain.ErrAttendeeAlreadyAttending
		case !errors.Is(err, domain.ErrAttendeeNotFound):
			return fmt.Errorf("get attendee: %w", err)
		}

		// Reject if there's already a pending invite for this user in this session
		_, err = s.sessionInviteRepo.GetPendingInviteBySessionAndUser(tCtx, params.SessionID, params.InvitedUserID)
		switch {
		case err == nil:
			return domain.ErrAlreadyInvited
		case !errors.Is(err, domain.ErrInviteNotFound):
			return fmt.Errorf("get pending invite: %w", err)
		}

		if err := s.sessionInviteRepo.CreateInvite(tCtx, invite); err != nil {
			return fmt.Errorf("persist session invite: %w", err)
		}

		if err := s.eventWriter.InsertEvents(tCtx, activitySchema, invite.Events()); err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}
		invite.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to send session invite", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session invite sent")
	log.Debug(ctx, "session invite sent", "invite_id", invite.ID())

	return invite, nil
}

// AcceptInvite accepts a pending invite and makes the invited user an
// attendee. Like a "going" RSVP, a full session puts them on the waitlist
// (pending) instead of failing the acceptance.
func (s *SessionInviteService) AcceptInvite(
	ctx context.Context,
	inviteID uuid.UUID,
	userID uuid.UUID,
) (*domain.Attendee, error) {
	ctx, span := s.tracer.Start(ctx, "activity.service.AcceptSessionInvite")
	defer span.End()

	log := s.logger.WithValues(
		"method", "AcceptInvite",
		"invite_id", inviteID,
		"user_id", userID,
	)

	span.SetAttributes(
		attribute.String("invite_id", inviteID.String()),
		attribute.String("user_id", userID.String()),
	)

	var attendee *domain.Attendee

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		invite, err := s.sessionInviteRepo.GetInviteByID(tCtx, inviteID)
		if err != nil {
			return fmt.Errorf("get invite: %w", err)
		}

		if err := invite.Accept(userID); err != nil {
			return fmt.Errorf("accept invite: %w", err)
		}

		session, err := s.sessionRepo.GetSessionByID(tCtx, invite.SessionID())
		if err != nil {
			return fmt.Errorf("get session: %w", err)
		}

		if session.Status() != domain.SessionStatusScheduled {
			return domain.ErrSessionNotScheduled
		}

		_, err = s.attendeeRepo.GetAttendeeBySessionAndUser(tCtx, session.ID(), userID)
		switch {
		case err == nil:
			return domain.ErrAttendeeAlreadyAttending
		case !errors.Is(err, domain.ErrAttendeeNotFound):
			return fmt.Errorf("get attendee: %w", err)
		}

		if err := s.sessionInviteRepo.UpdateInvite(tCtx, invite); err != nil {
			return fmt.Errorf("persist session invite acceptance: %w", err)
		}

		if err := s.eventWriter.InsertEvents(tCtx, activitySchema, invite.Events()); err != nil {
			return fmt.Errorf("insert invite events: %w", err)
		}
		invite.ClearEvents()

		// Serialize with concurrent RSVPs/approvals so the capacity check
		// below can't be raced into overbooking the session.
		if err := lockSessionCapacity(tCtx, s.txManager, session.ID()); err != nil {
			return err
		}

		confirmedCount, err := s.attendeeRepo.CountConfirmedAttendees(tCtx, session.ID())
		if err != nil {
			return fmt.Errorf("count confirmed attendees: %w", err)
		}

		attendee = domain.NewInvitedAttendee(session, userID, confirmedCount)

		if err := s.attendeeRepo.CreateAttendee(tCtx, attendee); err != nil {
			return fmt.Errorf("persist attendee: %w", err)
		}

		if err := s.eventWriter.InsertEvents(tCtx, activitySchema, attendee.Events()); err != nil {
			return fmt.Errorf("insert attendee events: %w", err)
		}
		attendee.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to accept session invite", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session invite accepted")
	log.Debug(ctx, "session invite accepted", "attendee_id", attendee.ID(), "status", attendee.Status())

	return attendee, nil
}

// DeclineInvite declines a pending invite; no attendee is created
func (s *SessionInviteService) DeclineInvite(
	ctx context.Context,
	inviteID uuid.UUID,
	userID uuid.UUID,
) error {
	ctx, span := s.tracer.Start(ctx, "activity.service.DeclineSessionInvite")
	defer span.End()

	log := s.logger.WithValues(
		"method", "DeclineInvite",
		"invite_id", inviteID,
		"user_id", userID,
	)

	span.SetAttributes(
		attribute.String("invite_id", inviteID.String()),
		attribute.String("user_id", userID.String()),
	)

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		invite, err := s.sessionInviteRepo.GetInviteByID(tCtx, inviteID)
		if err != nil {
			return fmt.Errorf("get invite: %w", err)
		}

		if err := invite.Decline(userID); err != nil {
			return fmt.Errorf("decline invite: %w", err)
		}

		if err := s.sessionInviteRepo.UpdateInvite(tCtx, invite); err != nil {
			return fmt.Errorf("persist session invite decline: %w", err)
		}

		if err := s.eventWriter.InsertEvents(tCtx, activitySchema, invite.Events()); err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}
		invite.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to decline session invite", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session invite declined")
	log.Debug(ctx, "session invite declined")

	return nil
}

// ExpireStaleInvites marks every pending session invite past its expiry as
// expired. Meant to be run periodically by the cron job - accepting an
// expired invite already fails on its own, but nothing else flips its status,
// which would otherwise leave it "pending" forever and block re-inviting.
func (s *SessionInviteService) ExpireStaleInvites(ctx context.Context) (int, error) {
	ctx, span := s.tracer.Start(ctx, "activity.service.ExpireStaleSessionInvites")
	defer span.End()

	log := s.logger.WithValues("method", "ExpireStaleInvites")

	invites, err := s.sessionInviteRepo.FindExpiredPendingInvites(ctx)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to find expired session invites", "error", err)
		return 0, mapToAppErr(err)
	}

	var expiredCount int
	for _, invite := range invites {
		err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
			if err := invite.Expire(); err != nil {
				return fmt.Errorf("expire invite: %w", err)
			}

			if err := s.sessionInviteRepo.UpdateInvite(tCtx, invite); err != nil {
				return fmt.Errorf("persist session invite expiry: %w", err)
			}

			if err := s.eventWriter.InsertEvents(tCtx, activitySchema, invite.Events()); err != nil {
				return fmt.Errorf("insert domain events: %w", err)
			}
			invite.ClearEvents()

			return nil
		})
		if err != nil {
			log.Error(ctx, "failed to expire session invite", "invite_id", invite.ID(), "error", err)
			continue
		}
		expiredCount++
	}

	span.SetAttributes(attribute.Int("expired_count", expiredCount))
	log.Info(ctx, "expired stale session invites", "count", expiredCount, "found", len(invites))

	return expiredCount, nil
}

// ListMyInvites retrieves the caller's own pending session invites
func (s *SessionInviteService) ListMyInvites(
	ctx context.Context,
	userID uuid.UUID,
	params ListMyInvitesParams,
) ([]*domain.SessionInvite, string, error) {
	ctx, span := s.tracer.Start(ctx, "activity.service.ListMySessionInvites")
	defer span.End()

	log := s.logger.WithValues(
		"method", "ListMyInvites",
		"user_id", userID,
	)

	span.SetAttributes(attribute.String("user_id", userID.String()))

	invites, nextPageToken, err := s.sessionInviteRepo.ListPendingInvitesForUser(ctx, persistence.ListPendingInvitesFilter{
		InvitedUserID: userID,
		Limit:         params.Limit,
		PageToken:     params.PageToken,
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to list session invites", "error", err)
		return nil, "", mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session invites listed")
	log.Debug(ctx, "session invites listed", "count", len(invites))

	return invites, nextPageToken, nil
}
