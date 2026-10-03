package application

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
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

// TeamDraftService runs captain drafts: the creator/admin names two captains,
// the captains pick confirmed attendees in turn, and the result becomes the
// session's teams.
type TeamDraftService struct {
	logger      *logger.Logger
	txManager   *postgres.TransactionManager
	sessionRepo SessionRepository
	draftRepo   TeamDraftRepository
	formation   teamFormation
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
	votingRepo TeamVotingRepository,
	eventWriter domainevent.EventWriter,
	metrics *metrics.Metrics,
) *TeamDraftService {
	return &TeamDraftService{
		logger:      logger.WithName("activity.team_draft_service"),
		txManager:   txManager,
		sessionRepo: sessionRepo,
		draftRepo:   draftRepo,
		formation:   newTeamFormation(sessionRepo, attendeeRepo, draftRepo, votingRepo),
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

		if err := s.formation.requireNoActiveDraft(tCtx, params.SessionID); err != nil {
			return err
		}

		// A draft and a voting round never run at the same time.
		if err := s.formation.requireNoOpenVoting(tCtx, params.SessionID); err != nil {
			return err
		}

		confirmed, confirmedIDs, err := s.formation.confirmedAttendees(tCtx, params.SessionID)
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

		draft, err := s.formation.activeDraft(tCtx, params.SessionID)
		if err != nil {
			return err
		}

		confirmed, confirmedIDs, err := s.formation.confirmedAttendees(tCtx, params.SessionID)
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

		draft, err := s.formation.activeDraft(tCtx, params.SessionID)
		if err != nil {
			return err
		}

		confirmed, confirmedIDs, err := s.formation.confirmedAttendees(tCtx, params.SessionID)
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

		draft, err := s.formation.activeDraft(tCtx, params.SessionID)
		if err != nil {
			return err
		}

		_, confirmedIDs, err := s.formation.confirmedAttendees(tCtx, params.SessionID)
		if err != nil {
			return err
		}

		if err := draft.Cancel(session, params.RequesterID, requesterRole, confirmedIDs); err != nil {
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
		_, confirmedIDs, err := s.formation.confirmedAttendees(ctx, sessionID)
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
		teamEvents, err := s.formation.applyCompletedDraft(ctx, session, draft, confirmed)
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
