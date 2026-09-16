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
		// Get session to check visibility, capacity and RSVP permissions
		session, err := s.sessionRepo.GetSessionByID(txCtx, params.SessionID)
		if err != nil {
			log.Error(txCtx, "failed to get session", "error", err)
			return fmt.Errorf("failed to get session: %w", err)
		}

		// Check if user is a member of the group. A public session can also be
		// joined by non-members, who attend the session without becoming a
		// group member.
		isPriorityMember := false
		var member *domain.Member
		if session.ActivityGroupID() != nil {
			member, err = s.memberRepo.GetMemberByGroupAndUser(
				txCtx,
				*session.ActivityGroupID(),
				params.UserID,
			)
		}
		switch {
		case err == nil && member != nil && member.IsConfirmed():
			isPriorityMember = member.IsPriority()
		case session.IsPublic():
			// non-member joining a public session as an attendee only
		default:
			log.Error(txCtx, "user is not a group member and session is not public", "error", err)
			return domain.ErrAttendeeNotGroupMember
		}

		// Check if user can RSVP (priority member check + openAt time check)
		if err := session.CanRSVP(isPriorityMember); err != nil {
			log.Error(txCtx, "user cannot RSVP", "is_priority", isPriorityMember, "error", err)
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

		if status == domain.AttendeeStatusGoing && session.RequiresApproval() {
			// Approval-gated session: skip the auto-accept path entirely and
			// create a pending join request instead. Enforce capacity up front
			// so nobody is left waiting on a request that can never be
			// approved - re-checked again at approval time, since that's the
			// moment a slot is actually consumed.
			if !session.HasCapacity(confirmedCount) {
				log.Error(txCtx, "session is full")
				return domain.ErrSessionFull
			}

			managerUserIDs, err := s.resolveSessionManagers(txCtx, session)
			if err != nil {
				log.Error(txCtx, "failed to resolve session managers", "error", err)
				return err
			}

			attendee = domain.NewRequestedAttendee(
				session,
				session.ActivityGroupID(),
				params.UserID,
				managerUserIDs,
			)
		} else {
			// Create attendee with capacity-aware logic
			attendee, err = domain.NewRSVPManualAttendee(
				session,
				session.ActivityGroupID(),
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

// ApproveAttendee approves a pending join request, mirroring MemberService.ApproveMember
func (s *AttendeeService) ApproveAttendee(
	ctx context.Context,
	params ApproveAttendeeParams,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.ApproveAttendee",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "ApproveAttendee",
		"session_id", params.SessionID,
		"requester_id", params.RequesterID,
		"user_id", params.UserID,
	)

	span.SetAttributes(
		attribute.String("session_id", params.SessionID.String()),
		attribute.String("requester_id", params.RequesterID.String()),
		attribute.String("user_id", params.UserID.String()),
	)

	requesterRole, err := parseMemberRole(params.RequesterRole)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid requester role", "error", err)
		return MapErrToAppError(err)
	}

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		session, err := s.sessionRepo.GetSessionByID(tCtx, params.SessionID)
		if err != nil {
			return fmt.Errorf("get session: %w", err)
		}

		attendee, err := s.attendeeRepo.GetAttendeeBySessionAndUser(tCtx, params.SessionID, params.UserID)
		if err != nil {
			return fmt.Errorf("get attendee: %w", err)
		}

		// Re-check capacity here, not just at request time - approving is the
		// moment a slot is actually consumed, and other approvals may have
		// filled the session in the meantime.
		confirmedCount, err := s.attendeeRepo.CountConfirmedAttendees(tCtx, params.SessionID)
		if err != nil {
			return fmt.Errorf("count confirmed attendees: %w", err)
		}
		if !session.HasCapacity(confirmedCount) {
			return domain.ErrSessionFull
		}

		if err := attendee.Approve(session, params.RequesterID, requesterRole); err != nil {
			return fmt.Errorf("approve attendee: %w", err)
		}

		if err := s.attendeeRepo.UpdateAttendee(tCtx, attendee); err != nil {
			return fmt.Errorf("persist attendee approval: %w", err)
		}

		if err := s.eventWriter.InsertEvents(tCtx, activitySchema, attendee.Events()); err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}
		attendee.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to approve attendee", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "attendee approved")
	log.Debug(ctx, "attendee approved")

	return nil
}

// RejectAttendee rejects a pending join request, mirroring MemberService.RejectMember
func (s *AttendeeService) RejectAttendee(
	ctx context.Context,
	params RejectAttendeeParams,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.RejectAttendee",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "RejectAttendee",
		"session_id", params.SessionID,
		"requester_id", params.RequesterID,
		"user_id", params.UserID,
	)

	span.SetAttributes(
		attribute.String("session_id", params.SessionID.String()),
		attribute.String("requester_id", params.RequesterID.String()),
		attribute.String("user_id", params.UserID.String()),
	)

	requesterRole, err := parseMemberRole(params.RequesterRole)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid requester role", "error", err)
		return MapErrToAppError(err)
	}

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		session, err := s.sessionRepo.GetSessionByID(tCtx, params.SessionID)
		if err != nil {
			return fmt.Errorf("get session: %w", err)
		}

		attendee, err := s.attendeeRepo.GetAttendeeBySessionAndUser(tCtx, params.SessionID, params.UserID)
		if err != nil {
			return fmt.Errorf("get attendee: %w", err)
		}

		if err := attendee.Reject(session, params.RequesterID, requesterRole); err != nil {
			return fmt.Errorf("reject attendee: %w", err)
		}

		if err := s.attendeeRepo.UpdateAttendee(tCtx, attendee); err != nil {
			return fmt.Errorf("persist attendee rejection: %w", err)
		}

		if err := s.eventWriter.InsertEvents(tCtx, activitySchema, attendee.Events()); err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}
		attendee.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to reject attendee", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "attendee rejected")
	log.Debug(ctx, "attendee rejected")

	return nil
}

// RemoveAttendee removes an already-confirmed attendee from a session (a
// manager kicking someone out) - as opposed to CancelRSVP, which only ever
// lets a user cancel their own RSVP.
func (s *AttendeeService) RemoveAttendee(
	ctx context.Context,
	params RemoveAttendeeParams,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.RemoveAttendee",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "RemoveAttendee",
		"session_id", params.SessionID,
		"requester_id", params.RequesterID,
		"user_id", params.UserID,
	)

	span.SetAttributes(
		attribute.String("session_id", params.SessionID.String()),
		attribute.String("requester_id", params.RequesterID.String()),
		attribute.String("user_id", params.UserID.String()),
	)

	requesterRole, err := parseMemberRole(params.RequesterRole)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid requester role", "error", err)
		return MapErrToAppError(err)
	}

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		session, err := s.sessionRepo.GetSessionByID(tCtx, params.SessionID)
		if err != nil {
			return fmt.Errorf("get session: %w", err)
		}

		attendee, err := s.attendeeRepo.GetAttendeeBySessionAndUser(tCtx, params.SessionID, params.UserID)
		if err != nil {
			return fmt.Errorf("get attendee: %w", err)
		}

		heldSpot := attendee.Status().HoldSpot()

		if err := attendee.Remove(session, params.RequesterID, requesterRole); err != nil {
			return fmt.Errorf("remove attendee: %w", err)
		}

		if err := s.attendeeRepo.DeleteAttendee(tCtx, attendee.ID()); err != nil {
			return fmt.Errorf("delete attendee: %w", err)
		}

		if err := s.eventWriter.InsertEvents(tCtx, activitySchema, attendee.Events()); err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}
		attendee.ClearEvents()

		// If they had a confirmed spot, promote next pending
		if heldSpot {
			if err := s.promoteNextPending(tCtx, params.SessionID); err != nil {
				log.Error(tCtx, "failed to promote next pending", "error", err)
				// Don't fail the whole transaction
			}
		}

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to remove attendee", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "attendee removed")
	log.Debug(ctx, "attendee removed")

	return nil
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
		// Not attending is the common, expected outcome of viewing a session
		// you haven't RSVPed to - not a real error, and returning the raw
		// domain error here (instead of mapping it) used to surface as a 500
		// to the client rather than a 404.
		if !errors.Is(err, domain.ErrAttendeeNotFound) {
			log.Error(ctx, "failed to get attendee", "error", err)
		}
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "")
	return attendee, nil
}

// ListRSVPs lists RSVPs with filtering
func (s *AttendeeService) ListRSVPs(
	ctx context.Context,
	params ListRSVPsParams,
) ([]*domain.Attendee, string, error) {
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
		PageToken:       params.PageToken,
	}

	if params.Status != nil {
		status := domain.AttendeeStatus(*params.Status)
		filter.Status = &status
	}

	attendees, nextPageToken, err := s.attendeeRepo.ListAttendees(ctx, filter)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to list attendees", "error", err)
		return nil, "", fmt.Errorf("failed to list attendees: %w", err)
	}

	span.SetStatus(codes.Ok, "")
	log.Debug(ctx, "RSVPs listed", "count", len(attendees))

	return attendees, nextPageToken, nil
}

// resolveSessionManagers resolves who should be notified of a join request:
// a grouped session's confirmed admins/creator (same resolution
// RequestToJoinGroup uses for group join requests), or just the creator
// directly for a standalone session, which has no membership to query.
func (s *AttendeeService) resolveSessionManagers(
	ctx context.Context,
	session *domain.Session,
) ([]uuid.UUID, error) {
	if session.ActivityGroupID() == nil {
		return []uuid.UUID{session.CreatedByID()}, nil
	}

	confirmedStatus := domain.MemberStatusConfirmed
	confirmedMembers, _, err := s.memberRepo.ListMembers(ctx, persistence.MemberFilter{
		ActivityGroupID: session.ActivityGroupID(),
		Status:          &confirmedStatus,
	})
	if err != nil {
		return nil, fmt.Errorf("list confirmed members: %w", err)
	}

	var managerUserIDs []uuid.UUID
	for _, m := range confirmedMembers {
		if m.Role().CanManageMembers() {
			managerUserIDs = append(managerUserIDs, m.UserID())
		}
	}

	return managerUserIDs, nil
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
		log.Debug(ctx, "no pending attendees to promote")
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

	log.Debug(ctx, "attendee promoted from waitlist", "attendee_id", pending.ID())

	return nil
}
