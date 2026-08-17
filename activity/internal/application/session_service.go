package application

import (
	"context"
	"database/sql"
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

// SessionService handles business logic for sessions
type SessionService struct {
	logger      *logger.Logger
	txManager   *postgres.TransactionManager
	sessionRepo SessionRepository
	eventWriter domainevent.EventWriter
	tracer      trace.Tracer
	metrics     *metrics.Metrics
}

// parseMemberRole parses a string into a MemberRole
func parseMemberRole(roleStr string) (domain.MemberRole, error) {
	role := domain.MemberRole(roleStr)
	if !role.IsValid() {
		return "", fmt.Errorf("invalid member role: %s", roleStr)
	}
	return role, nil
}

// NewSessionService creates a new session service
func NewSessionService(
	logger *logger.Logger,
	txManager *postgres.TransactionManager,
	sessionRepo SessionRepository,
	eventWriter domainevent.EventWriter,
	metrics *metrics.Metrics,
) *SessionService {
	return &SessionService{
		logger:      logger.WithName("activity.session_service"),
		txManager:   txManager,
		sessionRepo: sessionRepo,
		eventWriter: eventWriter,
		tracer:      telemetry.Tracer(telemetry.TracerActivityService),
		metrics:     metrics,
	}
}

// CreateSession creates a new session
func (s *SessionService) CreateSession(
	ctx context.Context,
	params CreateSessionParams,
) (*domain.Session, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.CreateSession",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "CreateSession",
		"activity_group_id", params.ActivityGroupID,
		"created_by_id", params.CreatedByID,
	)

	span.SetAttributes(
		attribute.String("activity_group_id", params.ActivityGroupID.String()),
		attribute.String("created_by_id", params.CreatedByID.String()),
	)

	// Create session location
	location, err := domain.NewSessionLocation(
		params.LocationCity,
		params.LocationCountry,
		params.LocationLat,
		params.LocationLng,
	)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid session location", "error", err)
		return nil, MapErrToAppError(err)
	}

	// Create session schedule
	schedule, err := domain.NewSessionSchedule(params.StartTime, params.EndTime)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid session schedule", "error", err)
		return nil, MapErrToAppError(err)
	}

	var session *domain.Session
	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		// Create the domain object
		input := domain.SessionInput{
			ActivityGroupID: params.ActivityGroupID,
			CreatedByID:     params.CreatedByID,
			Location:        location,
			Schedule:        schedule,
			Capacity:        params.Capacity,
			Note:            params.Note,
			IsRecurring:     params.IsRecurring,
			AutoAttendeeIDs: []uuid.UUID{}, // Empty for manual sessions
		}

		session, _, err = domain.NewSession(input)
		if err != nil {
			return fmt.Errorf("create session domain object: %w", err)
		}

		// Persist to database
		err = s.sessionRepo.CreateSession(tCtx, session)
		if err != nil {
			return fmt.Errorf("persist session: %w", err)
		}

		// Publish domain events
		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			session.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		session.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to create session", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session created")
	log.Info(ctx, "session created", "session_id", session.ID())

	return session, nil
}

// GetSession retrieves a session by ID
func (s *SessionService) GetSession(
	ctx context.Context,
	sessionID uuid.UUID,
) (*domain.Session, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.GetSession",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "GetSession",
		"session_id", sessionID,
	)

	span.SetAttributes(attribute.String("session_id", sessionID.String()))

	session, err := s.sessionRepo.GetSessionByID(ctx, sessionID)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to get session", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session found")
	log.Debug(ctx, "session found")

	return session, nil
}

// UpdateSession updates an existing session
func (s *SessionService) UpdateSession(
	ctx context.Context,
	sessionID uuid.UUID,
	params UpdateSessionParams,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.UpdateSession",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "UpdateSession",
		"session_id", sessionID,
		"requester_id", params.RequesterID,
	)

	span.SetAttributes(
		attribute.String("session_id", sessionID.String()),
		attribute.String("requester_id", params.RequesterID.String()),
	)

	// Parse requester role
	requesterRole, err := parseMemberRole(params.RequesterRole)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid requester role", "error", err)
		return MapErrToAppError(err)
	}

	// Create session location
	location, err := domain.NewSessionLocation(
		params.LocationCity,
		params.LocationCountry,
		params.LocationLat,
		params.LocationLng,
	)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid session location", "error", err)
		return MapErrToAppError(err)
	}

	// Create session schedule
	schedule, err := domain.NewSessionSchedule(params.StartTime, params.EndTime)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid session schedule", "error", err)
		return MapErrToAppError(err)
	}

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		// Fetch existing session
		session, err := s.sessionRepo.GetSessionByID(tCtx, sessionID)
		if err != nil {
			return fmt.Errorf("get session: %w", err)
		}

		// Update the domain object
		err = session.Update(domain.SessionUpdateInput{
			RequesterID:   params.RequesterID,
			RequesterRole: requesterRole,
			Location:      location,
			Schedule:      schedule,
			Capacity:      params.Capacity,
			Note:          params.Note,
		})
		if err != nil {
			return fmt.Errorf("update session: %w", err)
		}

		// Persist changes
		err = s.sessionRepo.UpdateSession(tCtx, session)
		if err != nil {
			return fmt.Errorf("persist session updates: %w", err)
		}

		// Publish domain events
		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			session.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		session.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to update session", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session updated")
	log.Info(ctx, "session updated")

	return nil
}

// StartSession starts a session (changes status to started)
func (s *SessionService) StartSession(
	ctx context.Context,
	sessionID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole string,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.StartSession",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "StartSession",
		"session_id", sessionID,
		"requester_id", requesterID,
	)

	span.SetAttributes(
		attribute.String("session_id", sessionID.String()),
		attribute.String("requester_id", requesterID.String()),
	)

	// Parse requester role
	role, err := parseMemberRole(requesterRole)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid requester role", "error", err)
		return MapErrToAppError(err)
	}

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		session, err := s.sessionRepo.GetSessionByID(tCtx, sessionID)
		if err != nil {
			return fmt.Errorf("get session: %w", err)
		}

		err = session.Start(requesterID, role)
		if err != nil {
			return fmt.Errorf("start session: %w", err)
		}

		err = s.sessionRepo.UpdateSession(tCtx, session)
		if err != nil {
			return fmt.Errorf("persist session start: %w", err)
		}

		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			session.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		session.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to start session", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session started")
	log.Info(ctx, "session started")

	return nil
}

// CompleteSession completes a session (changes status to completed)
func (s *SessionService) CompleteSession(
	ctx context.Context,
	sessionID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole string,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.CompleteSession",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "CompleteSession",
		"session_id", sessionID,
		"requester_id", requesterID,
	)

	span.SetAttributes(
		attribute.String("session_id", sessionID.String()),
		attribute.String("requester_id", requesterID.String()),
	)

	// Parse requester role
	role, err := parseMemberRole(requesterRole)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid requester role", "error", err)
		return MapErrToAppError(err)
	}

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		session, err := s.sessionRepo.GetSessionByID(tCtx, sessionID)
		if err != nil {
			return fmt.Errorf("get session: %w", err)
		}

		err = session.Complete(requesterID, role)
		if err != nil {
			return fmt.Errorf("complete session: %w", err)
		}

		err = s.sessionRepo.UpdateSession(tCtx, session)
		if err != nil {
			return fmt.Errorf("persist session completion: %w", err)
		}

		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			session.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		session.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to complete session", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session completed")
	log.Info(ctx, "session completed")

	return nil
}

// CancelSession cancels a session (soft delete with reason)
func (s *SessionService) CancelSession(
	ctx context.Context,
	sessionID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole string,
	reason string,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.CancelSession",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "CancelSession",
		"session_id", sessionID,
		"requester_id", requesterID,
	)

	span.SetAttributes(
		attribute.String("session_id", sessionID.String()),
		attribute.String("requester_id", requesterID.String()),
	)

	// Parse requester role
	role, err := parseMemberRole(requesterRole)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid requester role", "error", err)
		return MapErrToAppError(err)
	}

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		session, err := s.sessionRepo.GetSessionByID(tCtx, sessionID)
		if err != nil {
			return fmt.Errorf("get session: %w", err)
		}

		err = session.Cancel(requesterID, role, reason)
		if err != nil {
			return fmt.Errorf("cancel session: %w", err)
		}

		err = s.sessionRepo.UpdateSession(tCtx, session)
		if err != nil {
			return fmt.Errorf("persist session cancellation: %w", err)
		}

		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			session.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		session.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to cancel session", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session cancelled")
	log.Info(ctx, "session cancelled")

	return nil
}

// ListSessions retrieves sessions with optional filters
func (s *SessionService) ListSessions(
	ctx context.Context,
	params ListSessionsParams,
) ([]*domain.Session, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.ListSessions",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "ListSessions",
	)

	if params.ActivityGroupID != nil {
		span.SetAttributes(attribute.String("activity_group_id", params.ActivityGroupID.String()))
	}

	// Convert params to repository filter
	var status domain.SessionStatus
	if params.Status != nil {
		status = domain.SessionStatus(*params.Status)
	}

	filter := persistence.SessionFilter{
		ActivityGroupID:   params.ActivityGroupID,
		SessionTemplateID: params.SessionTemplateID,
		Status:            nil,
		IsRecurring:       params.IsRecurring,
		StartTimeFrom:     nil,
		StartTimeTo:       nil,
		Limit:             params.Limit,
		Offset:            params.Offset,
	}

	if params.Status != nil {
		filter.Status = &status
	}

	if params.StartTimeFrom != nil {
		filter.StartTimeFrom = &sql.NullTime{Time: *params.StartTimeFrom, Valid: true}
	}

	if params.StartTimeTo != nil {
		filter.StartTimeTo = &sql.NullTime{Time: *params.StartTimeTo, Valid: true}
	}

	sessions, err := s.sessionRepo.ListSessions(ctx, filter)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to list sessions", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "sessions listed")
	log.Debug(ctx, "sessions listed", "count", len(sessions))

	return sessions, nil
}
