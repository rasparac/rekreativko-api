package application

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/activity/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
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
	logger       *logger.Logger
	txManager    *postgres.TransactionManager
	sessionRepo  SessionRepository
	memberRepo   MemberRepository
	groupRepo    ActivityGroupRepository
	attendeeRepo AttendeeRepository
	drafts       draftCoordinator
	eventWriter  domainevent.EventWriter
	tracer       trace.Tracer
	metrics      *metrics.Metrics
}

// parseMemberRole parses a string into a MemberRole. An empty string is valid
// and means "no group role" - standalone sessions have no membership concept,
// so getUserRole deliberately returns "" for them, deferring authorization
// entirely to the domain layer's "are you the creator" fallback checks (e.g.
// Session.canManageSession).
func parseMemberRole(roleStr string) (domain.MemberRole, error) {
	if roleStr == "" {
		return "", nil
	}

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
	memberRepo MemberRepository,
	groupRepo ActivityGroupRepository,
	attendeeRepo AttendeeRepository,
	draftRepo TeamDraftRepository,
	eventWriter domainevent.EventWriter,
	metrics *metrics.Metrics,
) *SessionService {
	return &SessionService{
		logger:       logger.WithName("activity.session_service"),
		txManager:    txManager,
		sessionRepo:  sessionRepo,
		memberRepo:   memberRepo,
		groupRepo:    groupRepo,
		attendeeRepo: attendeeRepo,
		drafts:       draftCoordinator{sessionRepo: sessionRepo, attendeeRepo: attendeeRepo, draftRepo: draftRepo},
		eventWriter:  eventWriter,
		tracer:       telemetry.Tracer(telemetry.TracerActivityService),
		metrics:      metrics,
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

	activityGroupID := "standalone"
	if params.ActivityGroupID != nil {
		activityGroupID = params.ActivityGroupID.String()
	}

	log := s.logger.WithValues(
		"method", "CreateSession",
		"activity_group_id", activityGroupID,
		"created_by_id", params.CreatedByID,
	)

	span.SetAttributes(
		attribute.String("activity_group_id", activityGroupID),
		attribute.String("created_by_id", params.CreatedByID.String()),
	)

	// Only members who can manage the group may schedule sessions for it.
	// A standalone session (no group) has no membership to check.
	var activityType domain.ActivityType
	var difficultyLevel domain.DifficultyLevel
	if params.ActivityGroupID != nil {
		if err := requireCanManageGroup(ctx, s.memberRepo, *params.ActivityGroupID, params.CreatedByID); err != nil {
			span.SetStatus(codes.Error, err.Error())
			log.Error(ctx, "user cannot create sessions for this group", "error", err)
			return nil, mapToAppErr(err)
		}

		// A grouped session always inherits its activity type and difficulty
		// level from the group - they're categories, not a per-occurrence choice.
		group, err := s.groupRepo.GetActivityGroupByID(ctx, *params.ActivityGroupID)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			log.Error(ctx, "failed to get activity group", "error", err)
			return nil, mapToAppErr(err)
		}
		activityType = group.ActivityType()
		difficultyLevel = group.DifficultyLevel()
	} else {
		activityType = domain.ActivityType(params.ActivityType)
		if !activityType.IsValid() {
			err := fmt.Errorf("invalid activity type: %s", params.ActivityType)
			span.SetStatus(codes.Error, err.Error())
			log.Error(ctx, "invalid activity type for standalone session", "error", err)
			return nil, domainerror.ValidationError("invalid_activity_type", "A valid activity_type is required for a standalone session", err)
		}

		difficultyLevel = domain.DifficultyLevel(params.DifficultyLevel)
		if !difficultyLevel.IsValid() {
			err := fmt.Errorf("invalid difficulty level: %s", params.DifficultyLevel)
			span.SetStatus(codes.Error, err.Error())
			log.Error(ctx, "invalid difficulty level for standalone session", "error", err)
			return nil, domainerror.ValidationError("invalid_difficulty_level", "A valid difficulty_level is required for a standalone session", err)
		}
	}

	title, err := domain.NewTitle(params.Title)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid session title", "error", err)
		return nil, domainerror.ValidationError("invalid_title", err.Error(), err)
	}

	// Create session location
	location, err := domain.NewSessionLocation(
		params.LocationCity,
		params.LocationCountry,
		params.LocationStreet,
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
	visibility := domain.SessionVisibility(params.Visibility)

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		// Create the domain object
		input := domain.SessionInput{
			ActivityGroupID:  params.ActivityGroupID,
			CreatedByID:      params.CreatedByID,
			Title:            title,
			ActivityType:     activityType,
			DifficultyLevel:  difficultyLevel,
			Location:         location,
			Schedule:         schedule,
			Capacity:         params.Capacity,
			Note:             params.Note,
			IsRecurring:      params.IsRecurring,
			Visibility:       &visibility,
			RequiresApproval: params.RequiresApproval,
			AutoAttendeeIDs:  []uuid.UUID{params.CreatedByID}, // creator is auto-attending their own session
		}

		var attendees []*domain.Attendee
		session, attendees, err = domain.NewSession(input)
		if err != nil {
			return fmt.Errorf("create session domain object: %w", err)
		}

		// Persist to database
		err = s.sessionRepo.CreateSession(tCtx, session)
		if err != nil {
			return fmt.Errorf("persist session: %w", err)
		}

		for _, attendee := range attendees {
			if err := s.attendeeRepo.CreateAttendee(tCtx, attendee); err != nil {
				return fmt.Errorf("persist auto-attendee: %w", err)
			}
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
	log.Debug(ctx, "session created", "session_id", session.ID())

	return session, nil
}

// GetSession retrieves a session by ID
// GetSession returns a session by ID. It does not resolve the caller's own
// RSVP status - that used to be computed here (an extra DB lookup on every
// call) but had no actual consumer: the mobile session-details screen
// already fetches the full attendee list separately and never read the
// embedded status. ListSessions/DiscoverSessions still resolve it, batched
// per page, for potential feed-card use.
func (s *SessionService) GetSession(
	ctx context.Context,
	sessionID uuid.UUID,
	requesterID uuid.UUID,
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

	// A private session doesn't exist as far as an unrelated requester is
	// concerned - not just access-denied, actually not found, mirroring
	// GetActivityGroup's ErrActivityGroupNotFound pattern.
	if !session.IsVisibleTo(requesterID, s.isSessionRelated(ctx, session, requesterID)) {
		span.SetStatus(codes.Error, "not visible to requester")
		log.Debug(ctx, "private session not visible to requester", "requester_id", requesterID)
		return nil, MapErrToAppError(domain.ErrSessionNotFound)
	}

	span.SetStatus(codes.Ok, "session found")
	log.Debug(ctx, "session found")

	return session, nil
}

// CreateTeams splits a session into teams - normally once people have joined.
// If the session already has teams they are replaced: every assigned attendee
// is unassigned first (team_unassigned, reason teams_replaced), then the old
// teams are deleted and new, empty ones created.
func (s *SessionService) CreateTeams(
	ctx context.Context,
	params CreateTeamsParams,
) (*domain.Session, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.CreateTeams",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "CreateTeams",
		"session_id", params.SessionID,
		"requester_id", params.RequesterID,
	)

	span.SetAttributes(
		attribute.String("session_id", params.SessionID.String()),
		attribute.String("requester_id", params.RequesterID.String()),
	)

	requesterRole, err := parseMemberRole(params.RequesterRole)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid requester role", "error", err)
		return nil, MapErrToAppError(err)
	}

	teamConfig, err := domain.NewTeamConfig(params.TeamCount, params.MinPlayersPerTeam, params.Colors)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid team config", "error", err)
		return nil, MapErrToAppError(err)
	}

	var session *domain.Session

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		session, err = s.sessionRepo.GetSessionByID(tCtx, params.SessionID)
		if err != nil {
			return fmt.Errorf("get session: %w", err)
		}

		// Same lock as team assignment and capacity checks, so nobody can be
		// assigned to an old team while it is being replaced.
		if err := lockSessionCapacity(tCtx, s.txManager, params.SessionID); err != nil {
			return err
		}

		if err := s.drafts.requireNoActiveDraft(tCtx, params.SessionID); err != nil {
			return err
		}

		hadTeams := session.HasTeams()

		if err := session.CreateTeams(params.RequesterID, requesterRole, teamConfig); err != nil {
			return fmt.Errorf("create teams: %w", err)
		}

		var unassigned []*domain.Attendee
		if hadTeams {
			attendees, _, err := s.attendeeRepo.ListAttendees(tCtx, persistence.AttendeeFilter{
				SessionID: &params.SessionID,
			})
			if err != nil {
				return fmt.Errorf("list attendees: %w", err)
			}

			for _, a := range attendees {
				if a.TeamID() == nil {
					continue
				}
				a.UnassignForReplacedTeams(params.RequesterID)
				unassigned = append(unassigned, a)
			}
		}

		// Clears every attendee's team_id, deletes the old teams and inserts
		// the new ones along with the session's team config.
		if err := s.sessionRepo.ReplaceTeams(tCtx, session); err != nil {
			return fmt.Errorf("persist teams: %w", err)
		}

		// One insert for the teams_created event and every attendee's
		// team_unassigned event.
		events := slices.Clone(session.Events())
		for _, a := range unassigned {
			events = append(events, a.Events()...)
		}

		if err := s.eventWriter.InsertEvents(tCtx, activitySchema, events); err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		session.ClearEvents()
		for _, a := range unassigned {
			a.ClearEvents()
		}

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to create teams", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "teams created")
	log.Debug(ctx, "teams created", "team_count", teamConfig.TeamCount())

	return session, nil
}

// ListTeamMembers returns the user IDs on each of a session's teams, keyed by
// team ID. It does no visibility check of its own - callers fetch the session
// through GetSession first.
func (s *SessionService) ListTeamMembers(
	ctx context.Context,
	sessionID uuid.UUID,
) (map[uuid.UUID][]uuid.UUID, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.ListTeamMembers",
	)
	defer span.End()

	span.SetAttributes(attribute.String("session_id", sessionID.String()))

	members, err := s.attendeeRepo.ListTeamMembers(ctx, sessionID)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		s.logger.Error(ctx, "failed to list team members", "session_id", sessionID, "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "team members listed")

	return members, nil
}

// isSessionRelated reports whether userID is either an attendee of this
// session or a confirmed member of its owning group (creator/admin
// included, since a group-scoped session's creator is already a confirmed
// member by construction) - the "isRelated" input Session.IsVisibleTo needs
// for a private session. A standalone session has no group, so only the
// attendee check applies to it - the creator, added as an auto-attendee on
// creation, always satisfies it for their own private standalone session.
func (s *SessionService) isSessionRelated(ctx context.Context, session *domain.Session, userID uuid.UUID) bool {
	attendee, err := s.attendeeRepo.GetAttendeeBySessionAndUser(ctx, session.ID(), userID)
	if err == nil && attendee != nil {
		return true
	}

	if session.ActivityGroupID() == nil {
		return false
	}

	member, err := s.memberRepo.GetMemberByGroupAndUser(ctx, *session.ActivityGroupID(), userID)
	if err != nil || member == nil {
		return false
	}

	return member.Status() == domain.MemberStatusConfirmed
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
		params.LocationStreet,
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

		if err := session.SetVisibility(params.RequesterID, requesterRole, domain.SessionVisibility(params.Visibility)); err != nil {
			return fmt.Errorf("set session visibility: %w", err)
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
	log.Debug(ctx, "session updated")

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
	log.Debug(ctx, "session started")

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
	log.Debug(ctx, "session completed")

	return nil
}

// ExpireCompletedSessions finds every scheduled/started session past its end
// time and auto-completes it. Meant to be run periodically by a cron job -
// nothing else ever transitions a session's status once its end time passes,
// which otherwise leaves it stuck as "scheduled" forever (still shown by
// discover/near-you).
func (s *SessionService) ExpireCompletedSessions(ctx context.Context) (int, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.ExpireCompletedSessions",
	)
	defer span.End()

	log := s.logger.WithValues("method", "ExpireCompletedSessions")

	sessions, err := s.sessionRepo.FindSessionsPastEndTime(ctx)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to find sessions past end time", "error", err)
		return 0, mapToAppErr(err)
	}

	now := time.Now().UTC()

	var expiredCount int
	for _, session := range sessions {
		err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
			if err := session.ExpireSchedule(now); err != nil {
				return fmt.Errorf("expire session: %w", err)
			}

			if err := s.sessionRepo.UpdateSession(tCtx, session); err != nil {
				return fmt.Errorf("persist session expiry: %w", err)
			}

			if err := s.eventWriter.InsertEvents(tCtx, activitySchema, session.Events()); err != nil {
				return fmt.Errorf("insert domain events: %w", err)
			}
			session.ClearEvents()

			return nil
		})
		if err != nil {
			log.Error(ctx, "failed to expire session", "session_id", session.ID(), "error", err)
			continue
		}
		expiredCount++
	}

	span.SetAttributes(attribute.Int("expired_count", expiredCount))
	log.Info(ctx, "expired completed sessions", "count", expiredCount, "found", len(sessions))

	return expiredCount, nil
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

		attendeeUserIDs, err := s.resolveSessionAttendeeUserIDs(tCtx, sessionID, requesterID)
		if err != nil {
			return fmt.Errorf("resolve session attendees: %w", err)
		}

		err = session.Cancel(requesterID, role, reason, attendeeUserIDs)
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
	log.Debug(ctx, "session cancelled")

	return nil
}

// resolveSessionAttendeeUserIDs resolves who should be notified that a
// session was cancelled: every attendee still interested in it, other than
// whoever is cancelling it. Attendees who explicitly RSVP'd not going are
// excluded since they already opted out.
func (s *SessionService) resolveSessionAttendeeUserIDs(
	ctx context.Context,
	sessionID uuid.UUID,
	requesterID uuid.UUID,
) ([]uuid.UUID, error) {
	attendees, _, err := s.attendeeRepo.ListAttendees(ctx, persistence.AttendeeFilter{
		SessionID: &sessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("list attendees: %w", err)
	}

	var attendeeUserIDs []uuid.UUID
	for _, a := range attendees {
		if a.Status() == domain.AttendeeStatusNotGoing || a.UserID() == requesterID {
			continue
		}
		attendeeUserIDs = append(attendeeUserIDs, a.UserID())
	}

	return attendeeUserIDs, nil
}

// SetSessionVisibility makes a session public (anyone can RSVP as an attendee,
// without joining the group) or private (group members only)
func (s *SessionService) SetSessionVisibility(
	ctx context.Context,
	sessionID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole string,
	visibility string,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.SetSessionVisibility",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "SetSessionVisibility",
		"session_id", sessionID,
		"requester_id", requesterID,
		"visibility", visibility,
	)

	span.SetAttributes(
		attribute.String("session_id", sessionID.String()),
		attribute.String("requester_id", requesterID.String()),
		attribute.String("visibility", visibility),
	)

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

		err = session.SetVisibility(requesterID, role, domain.SessionVisibility(visibility))
		if err != nil {
			return fmt.Errorf("set session visibility: %w", err)
		}

		err = s.sessionRepo.UpdateSession(tCtx, session)
		if err != nil {
			return fmt.Errorf("persist session visibility: %w", err)
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
		log.Error(ctx, "failed to set session visibility", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session visibility changed")
	log.Debug(ctx, "session visibility changed")

	return nil
}

// ListSessions retrieves sessions with optional filters
func (s *SessionService) ListSessions(
	ctx context.Context,
	params ListSessionsParams,
	requesterID uuid.UUID,
) ([]*domain.Session, string, map[uuid.UUID]domain.AttendeeStatus, error) {
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

	var activityType domain.ActivityType
	if params.ActivityType != nil {
		activityType = domain.ActivityType(*params.ActivityType)
	}

	var difficultyLevel domain.DifficultyLevel
	if params.DifficultyLevel != nil {
		difficultyLevel = domain.DifficultyLevel(*params.DifficultyLevel)
	}

	filter := persistence.SessionFilter{
		ActivityGroupID:   params.ActivityGroupID,
		SessionTemplateID: params.SessionTemplateID,
		CreatedByID:       params.CreatedByID,
		Status:            nil,
		ActivityType:      nil,
		DifficultyLevel:   nil,
		IsRecurring:       params.IsRecurring,
		StartTimeFrom:     nil,
		StartTimeTo:       nil,
		Limit:             params.Limit,
		PageToken:         params.PageToken,
	}

	if params.Status != nil {
		filter.Status = &status
	}

	if params.ActivityType != nil {
		filter.ActivityType = &activityType
	}

	if params.DifficultyLevel != nil {
		filter.DifficultyLevel = &difficultyLevel
	}

	if params.StartTimeFrom != nil {
		filter.StartTimeFrom = &sql.NullTime{Time: *params.StartTimeFrom, Valid: true}
	}

	if params.StartTimeTo != nil {
		filter.StartTimeTo = &sql.NullTime{Time: *params.StartTimeTo, Valid: true}
	}

	if params.AttendeeID != nil {
		filter.AttendeeID = params.AttendeeID

		if len(params.AttendeeStatus) > 0 {
			filter.AttendeeStatuses = make([]domain.AttendeeStatus, len(params.AttendeeStatus))
			for i, st := range params.AttendeeStatus {
				filter.AttendeeStatuses[i] = domain.AttendeeStatus(st)
			}
		} else {
			// Default to "currently holds a spot" - the same going+promoted
			// pair AttendeesSection already treats as attending, rather than
			// requiring every caller to spell it out.
			filter.AttendeeStatuses = []domain.AttendeeStatus{
				domain.AttendeeStatusGoing,
				domain.AttendeeStatusPromoted,
			}
		}
	}

	// Scopes every listing shape (activity_group_id, created_by_id,
	// attendee_id, or none) to what requesterID may actually see: public
	// sessions, plus any private one they created, attend, or belong to a
	// group they're a confirmed member of. Same rule GetSession applies on
	// direct fetch, via Session.IsVisibleTo.
	filter.RequesterID = requesterID

	// Browsing someone else's profile (?attendee_id=<not you> or
	// ?created_by_id=<not you>) is a harder cap than the general rule above:
	// only their public sessions show up here, even if you'd otherwise be
	// able to see one of their private sessions by being a confirmed member
	// of its group yourself. That broader access still works via
	// ?activity_group_id=<group> or a direct GetSession - this only keeps
	// the profile screen from being a shortcut to a private session you'd
	// otherwise have to go find through the group.
	viewingSomeoneElsesAttendance := params.AttendeeID != nil && *params.AttendeeID != requesterID
	viewingSomeoneElsesCreations := params.CreatedByID != nil && *params.CreatedByID != requesterID
	if viewingSomeoneElsesAttendance || viewingSomeoneElsesCreations {
		publicVisibility := domain.SessionVisibilityPublic
		filter.Visibility = &publicVisibility
	}

	sessions, nextPageToken, err := s.sessionRepo.ListSessions(ctx, filter)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to list sessions", "error", err)
		return nil, "", nil, mapToAppErr(err)
	}

	sessionIDs := make([]uuid.UUID, len(sessions))
	for i, session := range sessions {
		sessionIDs[i] = session.ID()
	}

	attendeeStatuses, err := s.attendeeRepo.GetAttendeeStatusesForUser(ctx, requesterID, sessionIDs)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to get attendee statuses", "error", err)
		return nil, "", nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "sessions listed")
	log.Debug(ctx, "sessions listed", "count", len(sessions))

	return sessions, nextPageToken, attendeeStatuses, nil
}

// DiscoverSessions finds scheduled, public sessions near a location, ordered by distance
func (s *SessionService) DiscoverSessions(
	ctx context.Context,
	params DiscoverSessionsParams,
	requesterID uuid.UUID,
) ([]persistence.SessionWithDistance, string, map[uuid.UUID]domain.AttendeeStatus, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.DiscoverSessions",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "DiscoverSessions",
	)

	span.SetAttributes(
		attribute.Float64("lat", params.Lat),
		attribute.Float64("lng", params.Lng),
		attribute.Float64("radius_km", params.RadiusKM),
	)

	if params.Lat < -90 || params.Lat > 90 {
		err := domain.ErrSessionLocationLatitudeInvalid
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid latitude", "error", err)
		return nil, "", nil, MapErrToAppError(err)
	}

	if params.Lng < -180 || params.Lng > 180 {
		err := domain.ErrSessionLocationLongitudeInvalid
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid longitude", "error", err)
		return nil, "", nil, MapErrToAppError(err)
	}

	if params.RadiusKM <= 0 {
		err := domain.ErrInvalidDiscoveryRadius
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid radius", "error", err)
		return nil, "", nil, MapErrToAppError(err)
	}

	interests := make([]postgres.InterestPair, 0, len(params.Interests))
	for _, interest := range params.Interests {
		if interest.ActivityType != "" && !domain.ActivityType(interest.ActivityType).IsValid() {
			err := fmt.Errorf("invalid activity type: %s", interest.ActivityType)
			span.SetStatus(codes.Error, err.Error())
			log.Error(ctx, "invalid activity type", "error", err)
			return nil, "", nil, MapErrToAppError(err)
		}
		if interest.DifficultyLevel != "" && !domain.DifficultyLevel(interest.DifficultyLevel).IsValid() {
			err := fmt.Errorf("invalid difficulty level: %s", interest.DifficultyLevel)
			span.SetStatus(codes.Error, err.Error())
			log.Error(ctx, "invalid difficulty level", "error", err)
			return nil, "", nil, MapErrToAppError(err)
		}
		interests = append(interests, postgres.InterestPair{
			ActivityType:    interest.ActivityType,
			DifficultyLevel: interest.DifficultyLevel,
		})
	}
	span.SetAttributes(attribute.Int("interests_count", len(interests)))

	results, nextPageToken, err := s.sessionRepo.DiscoverSessions(ctx, persistence.DiscoverSessionsFilter{
		Lat:       params.Lat,
		Lng:       params.Lng,
		RadiusKM:  params.RadiusKM,
		Interests: interests,
		Limit:     params.Limit,
		PageToken: params.PageToken,
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to discover sessions", "error", err)
		return nil, "", nil, mapToAppErr(err)
	}

	sessionIDs := make([]uuid.UUID, len(results))
	for i, result := range results {
		sessionIDs[i] = result.Session.ID()
	}

	attendeeStatuses, err := s.attendeeRepo.GetAttendeeStatusesForUser(ctx, requesterID, sessionIDs)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to get attendee statuses", "error", err)
		return nil, "", nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "sessions discovered")
	log.Debug(ctx, "sessions discovered", "count", len(results))

	return results, nextPageToken, attendeeStatuses, nil
}
