package application

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
)

// teamFormation keeps a session's captain draft and team voting round
// consistent with who is going and with each other. Shared by the services
// that form teams (TeamDraftService, TeamVotingService) and those that change
// attendance or teams (AttendeeService, SessionService). Every method must run
// inside a transaction holding the session lock (lockSessionCapacity).
type teamFormation struct {
	sessionRepo  SessionRepository
	attendeeRepo AttendeeRepository
	draftRepo    TeamDraftRepository
	votingRepo   TeamVotingRepository
}

func newTeamFormation(
	sessionRepo SessionRepository,
	attendeeRepo AttendeeRepository,
	draftRepo TeamDraftRepository,
	votingRepo TeamVotingRepository,
) teamFormation {
	return teamFormation{
		sessionRepo:  sessionRepo,
		attendeeRepo: attendeeRepo,
		draftRepo:    draftRepo,
		votingRepo:   votingRepo,
	}
}

// requireNoActiveDraft rejects team changes (manual, or a vote) while a draft
// is running.
func (c teamFormation) requireNoActiveDraft(ctx context.Context, sessionID uuid.UUID) error {
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

// requireNoOpenVoting rejects starting a draft while a voting round is open.
func (c teamFormation) requireNoOpenVoting(ctx context.Context, sessionID uuid.UUID) error {
	_, err := c.votingRepo.GetOpenRound(ctx, sessionID)
	switch {
	case err == nil:
		return domain.ErrVotingOpen
	case errors.Is(err, domain.ErrVotingNotOpen):
		return nil
	default:
		return fmt.Errorf("get open voting round: %w", err)
	}
}

// activeDraft returns the running draft, mapping "none" to
// ErrDraftNotActive for commands that need one.
func (c teamFormation) activeDraft(ctx context.Context, sessionID uuid.UUID) (*domain.TeamDraft, error) {
	draft, err := c.draftRepo.GetActiveDraft(ctx, sessionID)
	if errors.Is(err, domain.ErrDraftNotFound) {
		return nil, domain.ErrDraftNotActive
	}
	if err != nil {
		return nil, fmt.Errorf("get active draft: %w", err)
	}
	return draft, nil
}

// openVoting returns the open voting round, or nil when there is none.
func (c teamFormation) openVoting(ctx context.Context, sessionID uuid.UUID) (*domain.TeamVotingRound, error) {
	round, err := c.votingRepo.GetOpenRound(ctx, sessionID)
	if errors.Is(err, domain.ErrVotingNotOpen) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get open voting round: %w", err)
	}
	return round, nil
}

// confirmedAttendees returns the session's going/promoted attendees and their
// user IDs, in join order.
func (c teamFormation) confirmedAttendees(ctx context.Context, sessionID uuid.UUID) ([]*domain.Attendee, []uuid.UUID, error) {
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

// formedTeams is the outcome of a draft or a vote: one roster per team, in
// team order, and who each team's players were assigned by.
type formedTeams struct {
	rosters    [][]uuid.UUID
	assignedBy []uuid.UUID // per team
	replacedBy uuid.UUID   // who replaced the old teams
}

// replaceTeams applies a completed draft or a winning proposal: everyone is
// taken off the old teams, applyTeams replaces them on the session
// (Session.ApplyDraftTeams / ApplyProposalTeams) and each roster is assigned.
// It persists the teams and the attendees and returns the events raised, in
// order, for the caller to insert with the draft's or round's own events.
// Rosters only ever hold confirmed attendees - leavers are dropped under the
// same lock - so anyone missing is skipped.
func (c teamFormation) replaceTeams(
	ctx context.Context,
	session *domain.Session,
	confirmed []*domain.Attendee,
	formed formedTeams,
	applyTeams func() error,
) ([]domainevent.Event, error) {
	if session.HasTeams() {
		for _, a := range confirmed {
			if a.TeamID() != nil {
				a.UnassignForReplacedTeams(formed.replacedBy)
			}
		}
	}

	if err := applyTeams(); err != nil {
		return nil, fmt.Errorf("apply teams: %w", err)
	}

	// Clears every team_id, deletes the old teams and inserts the new ones.
	if err := c.sessionRepo.ReplaceTeams(ctx, session); err != nil {
		return nil, fmt.Errorf("persist teams: %w", err)
	}

	byUser := make(map[uuid.UUID]*domain.Attendee, len(confirmed))
	for _, a := range confirmed {
		byUser[a.UserID()] = a
	}

	for position, roster := range formed.rosters {
		teamID := session.Teams()[position].ID()
		for _, userID := range roster {
			a, ok := byUser[userID]
			if !ok {
				continue
			}
			a.AssignToFormedTeam(teamID, formed.assignedBy[position])
			if err := c.attendeeRepo.UpdateAttendee(ctx, a); err != nil {
				return nil, fmt.Errorf("persist team member: %w", err)
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

// applyCompletedDraft turns a completed draft into the session's teams.
func (c teamFormation) applyCompletedDraft(
	ctx context.Context,
	session *domain.Session,
	draft *domain.TeamDraft,
	confirmed []*domain.Attendee,
) ([]domainevent.Event, error) {
	captains := draft.Captains()
	formed := formedTeams{
		rosters:    [][]uuid.UUID{draft.Roster(domain.DraftSideA), draft.Roster(domain.DraftSideB)},
		assignedBy: []uuid.UUID{captains[domain.DraftSideA], captains[domain.DraftSideB]},
		replacedBy: draft.StartedBy(),
	}

	return c.replaceTeams(ctx, session, confirmed, formed, func() error {
		return session.ApplyDraftTeams(draft)
	})
}

// applyWinningProposal turns a closed round's winning proposal into the
// session's teams. People who joined during the vote are in no proposal and
// stay unassigned for the organizer to place (D12).
func (c teamFormation) applyWinningProposal(
	ctx context.Context,
	session *domain.Session,
	round *domain.TeamVotingRound,
	winning *domain.TeamProposal,
	closedBy uuid.UUID,
	confirmed []*domain.Attendee,
) ([]domainevent.Event, error) {
	assignedBy := make([]uuid.UUID, len(winning.Teams()))
	for i := range assignedBy {
		assignedBy[i] = closedBy
	}

	formed := formedTeams{
		rosters:    winning.Teams(),
		assignedBy: assignedBy,
		replacedBy: closedBy,
	}

	return c.replaceTeams(ctx, session, confirmed, formed, func() error {
		return session.ApplyProposalTeams(round, closedBy)
	})
}

// teamsReplaced cancels an open voting round after the session's teams were
// recreated by hand (POST /teams). Returns the events to insert.
func (c teamFormation) teamsReplaced(ctx context.Context, sessionID uuid.UUID) ([]domainevent.Event, error) {
	round, err := c.openVoting(ctx, sessionID)
	if err != nil || round == nil {
		return nil, err
	}

	round.TeamsReplaced()

	if err := c.votingRepo.UpdateRound(ctx, round); err != nil {
		return nil, fmt.Errorf("persist voting round: %w", err)
	}

	events := slices.Clone(round.Events())
	round.ClearEvents()

	return events, nil
}

// sessionEnded cancels a running draft and an open voting round after the
// session was cancelled or completed, so neither keeps presenting itself as
// live. endedBy is uuid.Nil when the session was auto-completed. Returns the
// events to insert.
func (c teamFormation) sessionEnded(ctx context.Context, sessionID, endedBy uuid.UUID) ([]domainevent.Event, error) {
	var events []domainevent.Event

	draft, err := c.draftRepo.GetActiveDraft(ctx, sessionID)
	switch {
	case err == nil:
		draft.SessionEnded(endedBy)
		if err := c.draftRepo.UpdateDraft(ctx, draft); err != nil {
			return nil, fmt.Errorf("persist draft: %w", err)
		}
		events = append(events, draft.Events()...)
		draft.ClearEvents()
	case !errors.Is(err, domain.ErrDraftNotFound):
		return nil, fmt.Errorf("get active draft: %w", err)
	}

	round, err := c.openVoting(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if round != nil {
		round.SessionEnded()
		if err := c.votingRepo.UpdateRound(ctx, round); err != nil {
			return nil, fmt.Errorf("persist voting round: %w", err)
		}
		events = append(events, round.Events()...)
		round.ClearEvents()
	}

	return events, nil
}

// attendeeLeft updates a running draft and an open voting round after userID
// stopped going. Call it after the attendee change (and any waitlist
// promotion) is persisted, so the confirmed list is final. Returns the events
// to insert.
func (c teamFormation) attendeeLeft(ctx context.Context, sessionID, userID uuid.UUID) ([]domainevent.Event, error) {
	draftEvents, err := c.draftAttendeeLeft(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}

	votingEvents, err := c.votingAttendeeLeft(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}

	return append(draftEvents, votingEvents...), nil
}

func (c teamFormation) draftAttendeeLeft(ctx context.Context, sessionID, userID uuid.UUID) ([]domainevent.Event, error) {
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

func (c teamFormation) votingAttendeeLeft(ctx context.Context, sessionID, userID uuid.UUID) ([]domainevent.Event, error) {
	round, err := c.openVoting(ctx, sessionID)
	if err != nil || round == nil {
		return nil, err
	}

	_, confirmedIDs, err := c.confirmedAttendees(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	round.AttendeeLeft(userID, confirmedIDs)

	if err := c.votingRepo.UpdateRound(ctx, round); err != nil {
		return nil, fmt.Errorf("persist voting round: %w", err)
	}

	events := slices.Clone(round.Events())
	round.ClearEvents()

	return events, nil
}
