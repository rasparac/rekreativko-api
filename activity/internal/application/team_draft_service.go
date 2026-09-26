package application

import (
	"context"
	"errors"
	"fmt"
	"slices"

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

// TeamDraftState is a draft plus who can still be picked - the full state a
// client needs to render (or resync) the draft screen.
type TeamDraftState struct {
	Draft *domain.TeamDraft
	// Available is empty once the draft has ended.
	Available []uuid.UUID
}

// draftCoordinator keeps a session's captain draft consistent with who is
// attending. Shared by TeamDraftService and the services that change
// attendance or teams (AttendeeService, SessionService). Every method must run
// inside a transaction holding the session lock (lockSessionCapacity).
type draftCoordinator struct {
	sessionRepo  SessionRepository
	attendeeRepo AttendeeRepository
	draftRepo    TeamDraftRepository
}

// requireNoActiveDraft rejects manual team changes while a draft is running.
func (c draftCoordinator) requireNoActiveDraft(ctx context.Context, sessionID uuid.UUID) error {
	_, err := c.draftRepo.GetActiveDraft(ctx, sessionID)
	switch {
	case err == nil:
		return domain.ErrDraftAlreadyActive
	case errors.Is(err, domain.ErrDraftNotFound):
		return nil
	default:
		return fmt.Errorf("get active draft: %w", err)
	}
}

// activeDraft returns the running draft, mapping "none" to
// ErrDraftNotActive for commands that need one.
func (c draftCoordinator) activeDraft(ctx context.Context, sessionID uuid.UUID) (*domain.TeamDraft, error) {
	draft, err := c.draftRepo.GetActiveDraft(ctx, sessionID)
	if errors.Is(err, domain.ErrDraftNotFound) {
		return nil, domain.ErrDraftNotActive
	}
	if err != nil {
		return nil, fmt.Errorf("get active draft: %w", err)
	}
	return draft, nil
}

// confirmedAttendees returns the session's going/promoted attendees and their
// user IDs, in join order.
func (c draftCoordinator) confirmedAttendees(ctx context.Context, sessionID uuid.UUID) ([]*domain.Attendee, []uuid.UUID, error) {
	attendees, _, err := c.attendeeRepo.ListAttendees(ctx, persistence.AttendeeFilter{SessionID: &sessionID})
	if err != nil {
		return nil, nil, fmt.Errorf("list attendees: %w", err)
	}

	var (
		confirmed []*domain.Attendee
		userIDs   []uuid.UUID
	)
	for _, a := range attendees {
		if a.Status().IsConfirmed() {
			confirmed = append(confirmed, a)
			userIDs = append(userIDs, a.UserID())
		}
	}

	return confirmed, userIDs, nil
}

// applyCompletedDraft turns a completed draft into the session's teams:
// everyone is taken off the old teams, the two draft teams replace them and
// each roster is assigned. It persists the session's teams and the attendees
// and returns the events raised, in order, for the caller to insert together
// with the draft's own events.
func (c draftCoordinator) applyCompletedDraft(
	ctx context.Context,
	session *domain.Session,
	draft *domain.TeamDraft,
	confirmed []*domain.Attendee,
) ([]domainevent.Event, error) {
	if session.HasTeams() {
		for _, a := range confirmed {
			if a.TeamID() != nil {
				a.UnassignForReplacedTeams(draft.StartedBy())
			}
		}
	}

	if err := session.ApplyDraftTeams(draft); err != nil {
		return nil, fmt.Errorf("apply draft teams: %w", err)
	}

	// Clears every team_id, deletes the old teams and inserts the new ones.
	if err := c.sessionRepo.ReplaceTeams(ctx, session); err != nil {
		return nil, fmt.Errorf("persist draft teams: %w", err)
	}

	byUser := make(map[uuid.UUID]*domain.Attendee, len(confirmed))
	for _, a := range confirmed {
		byUser[a.UserID()] = a
	}

	captains := draft.Captains()
	for _, side := range []domain.DraftSide{domain.DraftSideA, domain.DraftSideB} {
		teamID := session.Teams()[side].ID()
		for _, userID := range draft.Roster(side) {
			a, ok := byUser[userID]
			if !ok {
				// Rosters only ever hold confirmed attendees (leavers are
				// dropped under the same lock), so this is a bug.
				return nil, fmt.Errorf("drafted user %s is not a confirmed attendee", userID)
			}
			a.AssignFromDraft(teamID, captains[side])
			if err := c.attendeeRepo.UpdateAttendee(ctx, a); err != nil {
				return nil, fmt.Errorf("persist drafted attendee: %w", err)
			}
		}
	}

	events := slices.Clone(session.Events())
	session.ClearEvents()
	for _, a := range confirmed {
		events = append(events, a.Events()...)
		a.ClearEvents()
	}

	return events, nil
}

// attendeeLeft updates a running draft after userID stopped being a confirmed
// attendee. Call it after the attendee change (and any waitlist promotion) is
// persisted, so the confirmed list is final. Returns the events to insert.
func (c draftCoordinator) attendeeLeft(ctx context.Context, sessionID, userID uuid.UUID) ([]domainevent.Event, error) {
	draft, err := c.draftRepo.GetActiveDraft(ctx, sessionID)
	if errors.Is(err, domain.ErrDraftNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active draft: %w", err)
	}

	confirmed, confirmedIDs, err := c.confirmedAttendees(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	draft.AttendeeLeft(userID, confirmedIDs)

	events := slices.Clone(draft.Events())
	draft.ClearEvents()

	if draft.IsCompleted() {
		session, err := c.sessionRepo.GetSessionByID(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("get session: %w", err)
		}

		teamEvents, err := c.applyCompletedDraft(ctx, session, draft, confirmed)
		if err != nil {
			return nil, err
		}
		events = append(events, teamEvents...)
	}

	if err := c.draftRepo.UpdateDraft(ctx, draft); err != nil {
		return nil, fmt.Errorf("persist draft: %w", err)
	}

	return events, nil
}

// TeamDraftService runs captain drafts: the creator/admin names two captains,
// the captains pick confirmed attendees in turn, and the result becomes the
// session's teams.
type TeamDraftService struct {
	logger      *logger.Logger
	txManager   *postgres.TransactionManager
	sessionRepo SessionRepository
	draftRepo   TeamDraftRepository
	drafts      draftCoordinator
	eventWriter domainevent.EventWriter
	tracer      trace.Tracer
	metrics     *metrics.Metrics
}

// NewTeamDraftService creates a new captain draft service
func NewTeamDraftService(
	logger *logger.Logger,
	txManager *postgres.TransactionManager,
	sessionRepo SessionRepository,
	attendeeRepo AttendeeRepository,
	draftRepo TeamDraftRepository,
	eventWriter domainevent.EventWriter,
	metrics *metrics.Metrics,
) *TeamDraftService {
	return &TeamDraftService{
		logger:      logger.WithName("activity.team_draft_service"),
		txManager:   txManager,
		sessionRepo: sessionRepo,
		draftRepo:   draftRepo,
		drafts:      draftCoordinator{sessionRepo: sessionRepo, attendeeRepo: attendeeRepo, draftRepo: draftRepo},
		eventWriter: eventWriter,
		tracer:      telemetry.Tracer(telemetry.TracerActivityService),
		metrics:     metrics,
	}
}

// StartDraft starts a captain draft. Rejected while another draft runs.
func (s *TeamDraftService) StartDraft(ctx context.Context, params StartDraftParams) (*TeamDraftState, error) {
	ctx, span := s.tracer.Start(ctx, "activity.service.StartDraft")
	defer span.End()

	log := s.logger.WithValues(
		"method", "StartDraft",
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

	var state *TeamDraftState

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		session, err := s.sessionRepo.GetSessionByID(tCtx, params.SessionID)
		if err != nil {
			return fmt.Errorf("get session: %w", err)
		}

		if err := lockSessionCapacity(tCtx, s.txManager, params.SessionID); err != nil {
			return err
		}

		if err := s.drafts.requireNoActiveDraft(tCtx, params.SessionID); err != nil {
			return err
		}

		confirmed, confirmedIDs, err := s.drafts.confirmedAttendees(tCtx, params.SessionID)
		if err != nil {
			return err
		}

		draft, err := domain.NewTeamDraft(domain.TeamDraftInput{
			Session:           session,
			RequesterID:       params.RequesterID,
			RequesterRole:     requesterRole,
			CaptainIDs:        params.CaptainIDs,
			PickOrder:         domain.PickOrder(params.PickOrder),
			MinPlayersPerTeam: params.MinPlayersPerTeam,
			Colors:            params.Colors,
			Confirmed:         confirmedIDs,
		})
		if err != nil {
			return fmt.Errorf("start draft: %w", err)
		}

		if err := s.draftRepo.CreateDraft(tCtx, draft); err != nil {
			return fmt.Errorf("persist draft: %w", err)
		}

		if err := s.finishCommand(tCtx, session, draft, confirmed); err != nil {
			return err
		}

		state = &TeamDraftState{Draft: draft, Available: availableIfRunning(draft, confirmedIDs)}

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to start draft", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "draft started")
	log.Debug(ctx, "draft started", "draft_id", state.Draft.ID(), "status", state.Draft.Status())

	return state, nil
}

// Pick lets the captain whose turn it is pick an available player.
func (s *TeamDraftService) Pick(ctx context.Context, params DraftPickParams) (*TeamDraftState, error) {
	ctx, span := s.tracer.Start(ctx, "activity.service.DraftPick")
	defer span.End()

	log := s.logger.WithValues(
		"method", "Pick",
		"session_id", params.SessionID,
		"captain_id", params.CaptainID,
		"user_id", params.UserID,
	)

	span.SetAttributes(
		attribute.String("session_id", params.SessionID.String()),
		attribute.String("captain_id", params.CaptainID.String()),
		attribute.String("user_id", params.UserID.String()),
	)

	var state *TeamDraftState

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		session, err := s.sessionRepo.GetSessionByID(tCtx, params.SessionID)
		if err != nil {
			return fmt.Errorf("get session: %w", err)
		}

		// Serializes picks: two captains (or a double tap) can't both take the
		// same turn or the same player.
		if err := lockSessionCapacity(tCtx, s.txManager, params.SessionID); err != nil {
			return err
		}

		draft, err := s.drafts.activeDraft(tCtx, params.SessionID)
		if err != nil {
			return err
		}

		confirmed, confirmedIDs, err := s.drafts.confirmedAttendees(tCtx, params.SessionID)
		if err != nil {
			return err
		}

		if err := draft.Pick(session, params.CaptainID, params.UserID, confirmedIDs); err != nil {
			return fmt.Errorf("pick player: %w", err)
		}

		if err := s.draftRepo.UpdateDraft(tCtx, draft); err != nil {
			return fmt.Errorf("persist draft: %w", err)
		}

		if err := s.finishCommand(tCtx, session, draft, confirmed); err != nil {
			return err
		}

		state = &TeamDraftState{Draft: draft, Available: availableIfRunning(draft, confirmedIDs)}

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to pick player", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "player picked")
	log.Debug(ctx, "player picked", "draft_status", state.Draft.Status())

	return state, nil
}

// ReplaceCaptain names a new captain for a side whose captain left; the draft
// resumes once both sides have a captain (and may complete right away).
func (s *TeamDraftService) ReplaceCaptain(ctx context.Context, params ReplaceDraftCaptainParams) (*TeamDraftState, error) {
	ctx, span := s.tracer.Start(ctx, "activity.service.ReplaceDraftCaptain")
	defer span.End()

	log := s.logger.WithValues(
		"method", "ReplaceCaptain",
		"session_id", params.SessionID,
		"requester_id", params.RequesterID,
		"user_id", params.UserID,
	)

	span.SetAttributes(
		attribute.String("session_id", params.SessionID.String()),
		attribute.String("requester_id", params.RequesterID.String()),
		attribute.String("user_id", params.UserID.String()),
		attribute.Int("team_position", params.TeamPosition),
	)

	requesterRole, err := parseMemberRole(params.RequesterRole)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid requester role", "error", err)
		return nil, MapErrToAppError(err)
	}

	var state *TeamDraftState

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		session, err := s.sessionRepo.GetSessionByID(tCtx, params.SessionID)
		if err != nil {
			return fmt.Errorf("get session: %w", err)
		}

		if err := lockSessionCapacity(tCtx, s.txManager, params.SessionID); err != nil {
			return err
		}

		draft, err := s.drafts.activeDraft(tCtx, params.SessionID)
		if err != nil {
			return err
		}

		confirmed, confirmedIDs, err := s.drafts.confirmedAttendees(tCtx, params.SessionID)
		if err != nil {
			return err
		}

		err = draft.ReplaceCaptain(
			session,
			params.RequesterID,
			requesterRole,
			domain.DraftSide(params.TeamPosition),
			params.UserID,
			confirmedIDs,
		)
		if err != nil {
			return fmt.Errorf("replace captain: %w", err)
		}

		if err := s.draftRepo.UpdateDraft(tCtx, draft); err != nil {
			return fmt.Errorf("persist draft: %w", err)
		}

		if err := s.finishCommand(tCtx, session, draft, confirmed); err != nil {
			return err
		}

		state = &TeamDraftState{Draft: draft, Available: availableIfRunning(draft, confirmedIDs)}

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to replace captain", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "captain replaced")
	log.Debug(ctx, "captain replaced", "draft_status", state.Draft.Status())

	return state, nil
}

// CancelDraft stops the running draft; the session's teams are untouched.
func (s *TeamDraftService) CancelDraft(ctx context.Context, params CancelDraftParams) (*TeamDraftState, error) {
	ctx, span := s.tracer.Start(ctx, "activity.service.CancelDraft")
	defer span.End()

	log := s.logger.WithValues(
		"method", "CancelDraft",
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

	var state *TeamDraftState

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		session, err := s.sessionRepo.GetSessionByID(tCtx, params.SessionID)
		if err != nil {
			return fmt.Errorf("get session: %w", err)
		}

		if err := lockSessionCapacity(tCtx, s.txManager, params.SessionID); err != nil {
			return err
		}

		draft, err := s.drafts.activeDraft(tCtx, params.SessionID)
		if err != nil {
			return err
		}

		if err := draft.Cancel(session, params.RequesterID, requesterRole); err != nil {
			return fmt.Errorf("cancel draft: %w", err)
		}

		if err := s.draftRepo.UpdateDraft(tCtx, draft); err != nil {
			return fmt.Errorf("persist draft: %w", err)
		}

		if err := s.eventWriter.InsertEvents(tCtx, activitySchema, draft.Events()); err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}
		draft.ClearEvents()

		state = &TeamDraftState{Draft: draft}

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to cancel draft", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "draft cancelled")
	log.Debug(ctx, "draft cancelled")

	return state, nil
}

// GetDraft returns the session's latest draft (running or ended). It does no
// visibility check of its own - callers fetch the session through
// SessionService.GetSession first.
func (s *TeamDraftService) GetDraft(ctx context.Context, sessionID uuid.UUID) (*TeamDraftState, error) {
	ctx, span := s.tracer.Start(ctx, "activity.service.GetDraft")
	defer span.End()

	span.SetAttributes(attribute.String("session_id", sessionID.String()))

	draft, err := s.draftRepo.GetLatestDraft(ctx, sessionID)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		if !errors.Is(err, domain.ErrDraftNotFound) {
			s.logger.Error(ctx, "failed to get draft", "session_id", sessionID, "error", err)
		}
		return nil, mapToAppErr(err)
	}

	state := &TeamDraftState{Draft: draft}

	if draft.IsRunning() {
		_, confirmedIDs, err := s.drafts.confirmedAttendees(ctx, sessionID)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			s.logger.Error(ctx, "failed to list confirmed attendees", "session_id", sessionID, "error", err)
			return nil, mapToAppErr(err)
		}
		state.Available = draft.Available(confirmedIDs)
	}

	span.SetStatus(codes.Ok, "draft found")

	return state, nil
}

// finishCommand applies a draft that just completed and inserts the draft's
// events together with any team events, in one call.
func (s *TeamDraftService) finishCommand(
	ctx context.Context,
	session *domain.Session,
	draft *domain.TeamDraft,
	confirmed []*domain.Attendee,
) error {
	events := slices.Clone(draft.Events())
	draft.ClearEvents()

	if draft.IsCompleted() {
		teamEvents, err := s.drafts.applyCompletedDraft(ctx, session, draft, confirmed)
		if err != nil {
			return err
		}
		events = append(events, teamEvents...)
	}

	if err := s.eventWriter.InsertEvents(ctx, activitySchema, events); err != nil {
		return fmt.Errorf("insert domain events: %w", err)
	}

	return nil
}

func availableIfRunning(draft *domain.TeamDraft, confirmedIDs []uuid.UUID) []uuid.UUID {
	if !draft.IsRunning() {
		return nil
	}
	return draft.Available(confirmedIDs)
}
