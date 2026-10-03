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

// TeamVotingState is what the proposals screen reads: the session's latest
// round (nil if it never had one) plus the head count.
type TeamVotingState struct {
	Round        *domain.TeamVotingRound
	PlayersGoing int
	// PlayersNeeded is how many must be going for voting (nil when the
	// session has no teams and no round is open - the first proposal decides).
	PlayersNeeded *int
}

// TeamVotingService runs team proposals and voting: people going propose
// divisions and vote, the organizer closes the round and the winner replaces
// the teams.
type TeamVotingService struct {
	logger      *logger.Logger
	txManager   *postgres.TransactionManager
	sessionRepo SessionRepository
	votingRepo  TeamVotingRepository
	formation   teamFormation
	eventWriter domainevent.EventWriter
	tracer      trace.Tracer
	metrics     *metrics.Metrics
}

// NewTeamVotingService creates a new team voting service
func NewTeamVotingService(
	logger *logger.Logger,
	txManager *postgres.TransactionManager,
	sessionRepo SessionRepository,
	attendeeRepo AttendeeRepository,
	draftRepo TeamDraftRepository,
	votingRepo TeamVotingRepository,
	eventWriter domainevent.EventWriter,
	metrics *metrics.Metrics,
) *TeamVotingService {
	return &TeamVotingService{
		logger:      logger.WithName("activity.team_voting_service"),
		txManager:   txManager,
		sessionRepo: sessionRepo,
		votingRepo:  votingRepo,
		formation:   newTeamFormation(sessionRepo, attendeeRepo, draftRepo, votingRepo),
		eventWriter: eventWriter,
		tracer:      telemetry.Tracer(telemetry.TracerActivityService),
		metrics:     metrics,
	}
}

// Propose adds a proposal, opening a new round when none is open.
func (s *TeamVotingService) Propose(ctx context.Context, params ProposeTeamsParams) (*TeamVotingState, error) {
	ctx, span := s.tracer.Start(ctx, "activity.service.ProposeTeams")
	defer span.End()

	log := s.logger.WithValues("method", "Propose", "session_id", params.SessionID, "author_id", params.AuthorID)
	span.SetAttributes(
		attribute.String("session_id", params.SessionID.String()),
		attribute.String("author_id", params.AuthorID.String()),
	)

	var state *TeamVotingState

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		session, err := s.sessionRepo.GetSessionByID(tCtx, params.SessionID)
		if err != nil {
			return fmt.Errorf("get session: %w", err)
		}

		if err := lockSessionCapacity(tCtx, s.txManager, params.SessionID); err != nil {
			return err
		}

		// A draft and a voting round never run at the same time.
		if err := s.formation.requireNoActiveDraft(tCtx, params.SessionID); err != nil {
			return err
		}

		_, confirmedIDs, err := s.formation.confirmedAttendees(tCtx, params.SessionID)
		if err != nil {
			return err
		}

		round, err := s.formation.openVoting(tCtx, params.SessionID)
		if err != nil {
			return err
		}

		if round == nil {
			round, _, err = domain.OpenTeamVoting(domain.OpenVotingInput{
				Session:   session,
				AuthorID:  params.AuthorID,
				Teams:     params.Teams,
				Confirmed: confirmedIDs,
			})
			if err != nil {
				return fmt.Errorf("open voting: %w", err)
			}

			if err := s.votingRepo.CreateRound(tCtx, round); err != nil {
				return fmt.Errorf("persist voting round: %w", err)
			}
		} else {
			if _, err := round.Propose(session, params.AuthorID, params.Teams, confirmedIDs); err != nil {
				return fmt.Errorf("propose: %w", err)
			}

			if err := s.votingRepo.UpdateRound(tCtx, round); err != nil {
				return fmt.Errorf("persist voting round: %w", err)
			}
		}

		if err := s.insertEvents(tCtx, round.Events()); err != nil {
			return err
		}
		round.ClearEvents()

		state = votingState(round, len(confirmedIDs), session)

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to propose teams", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "teams proposed")

	return state, nil
}

// Vote records or changes the caller's vote.
func (s *TeamVotingService) Vote(ctx context.Context, params CastVoteParams) (*TeamVotingState, error) {
	ctx, span := s.tracer.Start(ctx, "activity.service.CastTeamVote")
	defer span.End()

	log := s.logger.WithValues("method", "Vote", "session_id", params.SessionID, "voter_id", params.VoterID)
	span.SetAttributes(
		attribute.String("session_id", params.SessionID.String()),
		attribute.String("voter_id", params.VoterID.String()),
	)

	choice := domain.KeepCurrentTeams
	if !params.KeepCurrent {
		if params.ProposalID == nil {
			return nil, MapErrToAppError(domain.ErrProposalNotFound)
		}
		choice = *params.ProposalID
	}

	var state *TeamVotingState

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		session, err := s.sessionRepo.GetSessionByID(tCtx, params.SessionID)
		if err != nil {
			return fmt.Errorf("get session: %w", err)
		}

		// Serializes votes, so a vote can't land in a round being closed.
		if err := lockSessionCapacity(tCtx, s.txManager, params.SessionID); err != nil {
			return err
		}

		round, err := s.formation.openVoting(tCtx, params.SessionID)
		if err != nil {
			return err
		}
		if round == nil {
			return domain.ErrVotingNotOpen
		}

		_, confirmedIDs, err := s.formation.confirmedAttendees(tCtx, params.SessionID)
		if err != nil {
			return err
		}

		if err := round.Vote(params.VoterID, choice, confirmedIDs); err != nil {
			return fmt.Errorf("vote: %w", err)
		}

		if err := s.votingRepo.UpdateRound(tCtx, round); err != nil {
			return fmt.Errorf("persist voting round: %w", err)
		}

		if err := s.insertEvents(tCtx, round.Events()); err != nil {
			return err
		}
		round.ClearEvents()

		state = votingState(round, len(confirmedIDs), session)

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to vote", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "vote cast")

	return state, nil
}

// Close ends the open round: the winner replaces the teams (nothing changes
// when "keep current teams" wins). On a tie it fails with a *domain.TieError
// until the organizer passes a winner.
func (s *TeamVotingService) Close(ctx context.Context, params CloseVotingParams) (*TeamVotingState, error) {
	ctx, span := s.tracer.Start(ctx, "activity.service.CloseTeamVoting")
	defer span.End()

	log := s.logger.WithValues("method", "Close", "session_id", params.SessionID, "requester_id", params.RequesterID)
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

	var winner *uuid.UUID
	switch {
	case params.WinnerKeepCurrent:
		keep := domain.KeepCurrentTeams
		winner = &keep
	case params.WinnerProposalID != nil:
		winner = params.WinnerProposalID
	}

	var state *TeamVotingState

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		session, err := s.sessionRepo.GetSessionByID(tCtx, params.SessionID)
		if err != nil {
			return fmt.Errorf("get session: %w", err)
		}

		if err := lockSessionCapacity(tCtx, s.txManager, params.SessionID); err != nil {
			return err
		}

		round, err := s.formation.openVoting(tCtx, params.SessionID)
		if err != nil {
			return err
		}
		if round == nil {
			return domain.ErrVotingNotOpen
		}

		confirmed, confirmedIDs, err := s.formation.confirmedAttendees(tCtx, params.SessionID)
		if err != nil {
			return err
		}

		winning, err := round.Close(session, params.RequesterID, requesterRole, winner, confirmedIDs)
		if err != nil {
			return fmt.Errorf("close voting: %w", err)
		}

		events := slices.Clone(round.Events())
		round.ClearEvents()

		if winning != nil {
			teamEvents, err := s.formation.applyWinningProposal(tCtx, session, round, winning, params.RequesterID, confirmed)
			if err != nil {
				return err
			}
			events = append(events, teamEvents...)
		}

		// Also deletes the round's proposals and votes (D16).
		if err := s.votingRepo.UpdateRound(tCtx, round); err != nil {
			return fmt.Errorf("persist voting round: %w", err)
		}

		if err := s.insertEvents(tCtx, events); err != nil {
			return err
		}

		state = votingState(round, len(confirmedIDs), session)

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to close voting", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "voting closed")

	return state, nil
}

// GetVoting returns the session's latest round (any status, or none) with the
// head count. It does no visibility check of its own - callers fetch the
// session through SessionService.GetSession first.
func (s *TeamVotingService) GetVoting(ctx context.Context, sessionID uuid.UUID) (*TeamVotingState, error) {
	ctx, span := s.tracer.Start(ctx, "activity.service.GetTeamVoting")
	defer span.End()

	span.SetAttributes(attribute.String("session_id", sessionID.String()))

	session, err := s.sessionRepo.GetSessionByID(ctx, sessionID)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, mapToAppErr(err)
	}

	round, err := s.votingRepo.GetLatestRound(ctx, sessionID)
	if err != nil && !errors.Is(err, domain.ErrVotingNotOpen) {
		span.SetStatus(codes.Error, err.Error())
		s.logger.Error(ctx, "failed to get voting round", "session_id", sessionID, "error", err)
		return nil, mapToAppErr(err)
	}

	_, confirmedIDs, err := s.formation.confirmedAttendees(ctx, sessionID)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		s.logger.Error(ctx, "failed to list confirmed attendees", "session_id", sessionID, "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "voting found")

	return votingState(round, len(confirmedIDs), session), nil
}

func (s *TeamVotingService) insertEvents(ctx context.Context, events []domainevent.Event) error {
	if err := s.eventWriter.InsertEvents(ctx, activitySchema, events); err != nil {
		return fmt.Errorf("insert domain events: %w", err)
	}
	return nil
}

// votingState assembles the read model. players_needed comes from the open
// round, else from the session's teams; unknown before a first proposal on a
// session without teams.
func votingState(round *domain.TeamVotingRound, going int, session *domain.Session) *TeamVotingState {
	state := &TeamVotingState{Round: round, PlayersGoing: going}

	switch {
	case round != nil && round.IsOpen():
		needed := round.PlayersNeeded()
		state.PlayersNeeded = &needed
	case session.TeamConfig() != nil:
		needed := session.TeamConfig().PlayersNeeded()
		state.PlayersNeeded = &needed
	}

	return state
}
