package application

import (
	"context"
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

// AttendeeService handles business logic for session RSVPs
type AttendeeService struct {
	logger       *logger.Logger
	txManager    *postgres.TransactionManager
	attendeeRepo AttendeeRepository
	memberRepo   MemberRepository
	sessionRepo  SessionRepository
	eventWriter  domainevent.EventWriter
	tracer       trace.Tracer
	metrics      *metrics.Metrics
}

// NewAttendeeService creates a new attendee service
func NewAttendeeService(
	logger *logger.Logger,
	txManager *postgres.TransactionManager,
	attendeeRepo AttendeeRepository,
	memberRepo MemberRepository,
	sessionRepo SessionRepository,
	eventWriter domainevent.EventWriter,
	metrics *metrics.Metrics,
) *AttendeeService {
	return &AttendeeService{
		logger:       logger.WithName("activity.attendee_service"),
		txManager:    txManager,
		attendeeRepo: attendeeRepo,
		memberRepo:   memberRepo,
		sessionRepo:  sessionRepo,
		eventWriter:  eventWriter,
		tracer:       telemetry.Tracer(telemetry.TracerActivityService),
		metrics:      metrics,
	}
}

// CreateRSVP creates a new RSVP for a session
func (s *AttendeeService) CreateRSVP(
	ctx context.Context,
	params CreateRSVPParams,
) (*domain.Attendee, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.CreateRSVP",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "CreateRSVP",
		"session_id", params.SessionID,
		"user_id", params.UserID,
		"status", params.Status,
	)

	span.SetAttributes(
		attribute.String("session_id", params.SessionID.String()),
		attribute.String("user_id", params.UserID.String()),
		attribute.String("status", params.Status),
	)

	// Parse status
	status := domain.AttendeeStatus(params.Status)
	if !status.IsValid() {
		err := fmt.Errorf("invalid attendee status: %s", params.Status)
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid status", "error", err)
		return nil, err
	}

	var attendee *domain.Attendee

	err := s.txManager.WithTransaction(ctx, func(txCtx context.Context) error {
		// Check if user is a member of the group
		member, err := s.memberRepo.GetMemberByGroupAndUser(
			txCtx,
			params.ActivityGroupID,
			params.UserID,
		)
		if err != nil {
			log.Error(txCtx, "failed to get member", "error", err)
			return fmt.Errorf("failed to get member: %w", err)
		}

		if !member.IsConfirmed() {
			log.Error(txCtx, "member not confirmed")
			return domain.ErrAttendeeNotGroupMember
		}

		// Get session to check capacity and RSVP permissions
		session, err := s.sessionRepo.GetSessionByID(txCtx, params.SessionID)
		if err != nil {
			log.Error(txCtx, "failed to get session", "error", err)
			return fmt.Errorf("failed to get session: %w", err)
		}

		// Check if user can RSVP (priority member check + openAt time check)
		if err := session.CanRSVP(member.IsPriority()); err != nil {
			log.Error(txCtx, "user cannot RSVP", "is_priority", member.IsPriority(), "error", err)
			return err
		}

		// Check if user already has an RSVP
		existing, err := s.attendeeRepo.GetAttendeeBySessionAndUser(
			txCtx,
			params.SessionID,
			params.UserID,
		)
		if err == nil && existing != nil {
			log.Error(txCtx, "user already has RSVP")
			return domain.ErrAttendeeAlreadyAttending
		}

		// Get current confirmed count for capacity check
		confirmedCount, err := s.attendeeRepo.CountConfirmedAttendees(txCtx, params.SessionID)
		if err != nil {
			log.Error(txCtx, "failed to count confirmed attendees", "error", err)
			return fmt.Errorf("failed to count confirmed attendees: %w", err)
		}

		// Create attendee with capacity-aware logic
		attendee, err = domain.NewRSVPManualAttendee(
			session,
			params.ActivityGroupID,
			params.UserID,
			status,
		)
		if err != nil {
			log.Error(txCtx, "failed to create attendee", "error", err)
			return err
		}

		// If RSVPing "going", check capacity and potentially downgrade to pending
		if status == domain.AttendeeStatusGoing {
			if err := attendee.UpdateRSVP(status, session, confirmedCount); err != nil {
				log.Error(txCtx, "failed to update RSVP", "error", err)
				return err
			}
		}

		// Persist attendee
		if err := s.attendeeRepo.CreateAttendee(txCtx, attendee); err != nil {
			log.Error(txCtx, "failed to create attendee", "error", err)
			return fmt.Errorf("failed to create attendee: %w", err)
		}

		// Publish domain events
		if err := s.eventWriter.InsertEvents(txCtx, activitySchema, attendee.Events()); err != nil {
			log.Error(txCtx, "failed to publish events", "error", err)
			return fmt.Errorf("failed to publish events: %w", err)
		}

		attendee.ClearEvents()

		log.Info(
			txCtx,
			"RSVP created",
			"attendee_id", attendee.ID(),
			"final_status", attendee.Status(),
		)

		return nil
	})

	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	span.SetStatus(codes.Ok, "")
	return attendee, nil
}

// UpdateRSVP updates an existing RSVP
func (s *AttendeeService) UpdateRSVP(
	ctx context.Context,
	params UpdateRSVPParams,
) (*domain.Attendee, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.UpdateRSVP",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "UpdateRSVP",
		"session_id", params.SessionID,
		"user_id", params.UserID,
		"new_status", params.NewStatus,
	)

	span.SetAttributes(
		attribute.String("session_id", params.SessionID.String()),
		attribute.String("user_id", params.UserID.String()),
		attribute.String("new_status", params.NewStatus),
	)

	// Parse status
	newStatus := domain.AttendeeStatus(params.NewStatus)
	if !newStatus.IsValid() {
		err := fmt.Errorf("invalid attendee status: %s", params.NewStatus)
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid status", "error", err)
		return nil, err
	}

	var attendee *domain.Attendee

	err := s.txManager.WithTransaction(ctx, func(txCtx context.Context) error {
		// Get existing RSVP
		var err error
		attendee, err = s.attendeeRepo.GetAttendeeBySessionAndUser(
			txCtx,
			params.SessionID,
			params.UserID,
		)
		if err != nil {
			log.Error(txCtx, "failed to get attendee", "error", err)
			return domain.ErrAttendeeNotFound
		}

		// Get session for capacity check
		session, err := s.sessionRepo.GetSessionByID(txCtx, params.SessionID)
		if err != nil {
			log.Error(txCtx, "failed to get session", "error", err)
			return fmt.Errorf("failed to get session: %w", err)
		}

		// Get current confirmed count
		confirmedCount, err := s.attendeeRepo.CountConfirmedAttendees(txCtx, params.SessionID)
		if err != nil {
			log.Error(txCtx, "failed to count confirmed attendees", "error", err)
			return fmt.Errorf("failed to count confirmed attendees: %w", err)
		}

		oldStatus := attendee.Status()

		// Update RSVP with capacity check
		if err := attendee.UpdateRSVP(newStatus, session, confirmedCount); err != nil {
			log.Error(txCtx, "failed to update RSVP", "error", err)
			return err
		}

		// Persist changes
		if err := s.attendeeRepo.UpdateAttendee(txCtx, attendee); err != nil {
			log.Error(txCtx, "failed to update attendee", "error", err)
			return fmt.Errorf("failed to update attendee: %w", err)
		}

		// Publish domain events
		if err := s.eventWriter.InsertEvents(txCtx, activitySchema, attendee.Events()); err != nil {
			log.Error(txCtx, "failed to publish events", "error", err)
			return fmt.Errorf("failed to publish events: %w", err)
		}

		attendee.ClearEvents()

		// If someone released their spot, promote next pending attendee
		if oldStatus.HoldSpot() && !attendee.Status().HoldSpot() {
			if err := s.promoteNextPending(txCtx, params.SessionID); err != nil {
				log.Error(txCtx, "failed to promote next pending", "error", err)
				// Don't fail the whole transaction, just log it
			}
		}

		log.Info(
			txCtx,
			"RSVP updated",
			"attendee_id", attendee.ID(),
			"old_status", oldStatus,
			"new_status", attendee.Status(),
		)

		return nil
	})

	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	span.SetStatus(codes.Ok, "")
	return attendee, nil
}

// CancelRSVP cancels a user's RSVP
func (s *AttendeeService) CancelRSVP(
	ctx context.Context,
	sessionID uuid.UUID,
	userID uuid.UUID,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.CancelRSVP",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "CancelRSVP",
		"session_id", sessionID,
		"user_id", userID,
	)

	span.SetAttributes(
		attribute.String("session_id", sessionID.String()),
		attribute.String("user_id", userID.String()),
	)

	err := s.txManager.WithTransaction(ctx, func(txCtx context.Context) error {
		// Get existing RSVP
		attendee, err := s.attendeeRepo.GetAttendeeBySessionAndUser(
			txCtx,
			sessionID,
			userID,
		)
		if err != nil {
			log.Error(txCtx, "failed to get attendee", "error", err)
			return domain.ErrAttendeeNotFound
		}

		oldStatus := attendee.Status()

		// Soft delete
		if err := s.attendeeRepo.DeleteAttendee(txCtx, attendee.ID()); err != nil {
			log.Error(txCtx, "failed to delete attendee", "error", err)
			return fmt.Errorf("failed to delete attendee: %w", err)
		}

		// If they had a confirmed spot, promote next pending
		if oldStatus.HoldSpot() {
			if err := s.promoteNextPending(txCtx, sessionID); err != nil {
				log.Error(txCtx, "failed to promote next pending", "error", err)
				// Don't fail the whole transaction
			}
		}

		log.Info(txCtx, "RSVP cancelled", "attendee_id", attendee.ID())

		return nil
	})

	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetStatus(codes.Ok, "")
	return nil
}

// GetRSVP retrieves a user's RSVP for a session
func (s *AttendeeService) GetRSVP(
	ctx context.Context,
	sessionID uuid.UUID,
	userID uuid.UUID,
) (*domain.Attendee, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.GetRSVP",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "GetRSVP",
		"session_id", sessionID,
		"user_id", userID,
	)

	span.SetAttributes(
		attribute.String("session_id", sessionID.String()),
		attribute.String("user_id", userID.String()),
	)

	attendee, err := s.attendeeRepo.GetAttendeeBySessionAndUser(ctx, sessionID, userID)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to get attendee", "error", err)
		return nil, err
	}

	span.SetStatus(codes.Ok, "")
	return attendee, nil
}

// ListRSVPs lists RSVPs with filtering
func (s *AttendeeService) ListRSVPs(
	ctx context.Context,
	params ListRSVPsParams,
) ([]*domain.Attendee, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.ListRSVPs",
	)
	defer span.End()

	log := s.logger.WithValues("method", "ListRSVPs")

	// Build filter
	filter := persistence.AttendeeFilter{
		SessionID:       params.SessionID,
		ActivityGroupID: params.ActivityGroupID,
		UserID:          params.UserID,
		Limit:           params.Limit,
		Offset:          params.Offset,
	}

	if params.Status != nil {
		status := domain.AttendeeStatus(*params.Status)
		filter.Status = &status
	}

	attendees, err := s.attendeeRepo.ListAttendees(ctx, filter)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to list attendees", "error", err)
		return nil, fmt.Errorf("failed to list attendees: %w", err)
	}

	span.SetStatus(codes.Ok, "")
	log.Info(ctx, "RSVPs listed", "count", len(attendees))

	return attendees, nil
}

// promoteNextPending promotes the next pending attendee from the waitlist
func (s *AttendeeService) promoteNextPending(
	ctx context.Context,
	sessionID uuid.UUID,
) error {
	log := s.logger.WithValues("method", "promoteNextPending", "session_id", sessionID)

	// Get first pending attendee (FIFO)
	pending, err := s.attendeeRepo.GetFirstPendingAttendee(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get first pending attendee: %w", err)
	}

	if pending == nil {
		log.Info(ctx, "no pending attendees to promote")
		return nil
	}

	// Promote to confirmed
	if err := pending.Promote(); err != nil {
		log.Error(ctx, "failed to promote attendee", "error", err)
		return err
	}

	// Persist
	if err := s.attendeeRepo.UpdateAttendee(ctx, pending); err != nil {
		log.Error(ctx, "failed to update promoted attendee", "error", err)
		return err
	}

	// Publish events
	if err := s.eventWriter.InsertEvents(ctx, activitySchema, pending.Events()); err != nil {
		log.Error(ctx, "failed to publish promotion events", "error", err)
		return err
	}

	pending.ClearEvents()

	log.Info(ctx, "attendee promoted from waitlist", "attendee_id", pending.ID())

	return nil
}
