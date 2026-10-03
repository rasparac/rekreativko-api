package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
)

type VotingStatus string

const (
	VotingStatusOpen      VotingStatus = "open"
	VotingStatusClosed    VotingStatus = "closed"
	VotingStatusCancelled VotingStatus = "cancelled"
)

type VotingCancelReason string

const (
	VotingCancelReasonNotEnoughPlayers VotingCancelReason = "not_enough_players" // going count fell below PlayersNeeded
	VotingCancelReasonTeamsReplaced    VotingCancelReason = "teams_replaced"     // teams were recreated while voting
	VotingCancelReasonSessionEnded     VotingCancelReason = "session_ended"      // session was cancelled or completed while voting
)

// KeepCurrentTeams is the vote choice "keep the current teams" - offered only
// when the session already had teams when the round started.
var KeepCurrentTeams = uuid.Nil

// DivisionProblem says why a proposed division is invalid.
type DivisionProblem string

const (
	DivisionWrongTeamCount   DivisionProblem = "wrong_team_count"
	DivisionPlayerNotGoing   DivisionProblem = "player_not_going"
	DivisionDuplicatePlayer  DivisionProblem = "duplicate_player"
	DivisionMissingPlayers   DivisionProblem = "missing_players"
	DivisionTeamBelowMinimum DivisionProblem = "team_below_minimum"
)

// InvalidDivisionError is ErrInvalidDivision with the reason and, where it
// applies, the people concerned.
type InvalidDivisionError struct {
	Problem DivisionProblem
	UserIDs []uuid.UUID
}

func (e *InvalidDivisionError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidDivision, e.Problem)
}

func (e *InvalidDivisionError) Unwrap() error {
	return ErrInvalidDivision
}

// TieError is ErrTieRequiresWinner with the options that tied.
type TieError struct {
	ProposalIDs     []uuid.UUID
	KeepCurrentTied bool
}

func (e *TieError) Error() string {
	return ErrTieRequiresWinner.Error()
}

func (e *TieError) Unwrap() error {
	return ErrTieRequiresWinner
}

// Key identifies the tied option set independent of order, so the same tie
// seen on two Close attempts compares equal and a changed tie does not.
func (e *TieError) Key() string {
	parts := make([]string, 0, len(e.ProposalIDs)+1)
	for _, id := range e.ProposalIDs {
		parts = append(parts, id.String())
	}
	if e.KeepCurrentTied {
		parts = append(parts, "keep_current")
	}
	slices.Sort(parts)

	return strings.Join(parts, ",")
}

// TeamProposal is one suggested division: teams in order (first = Team A),
// each a list of user IDs.
type TeamProposal struct {
	id        uuid.UUID
	authorID  uuid.UUID
	createdAt time.Time
	teams     [][]uuid.UUID
}

func ReconstructTeamProposal(id, authorID uuid.UUID, createdAt time.Time, teams [][]uuid.UUID) *TeamProposal {
	return &TeamProposal{id: id, authorID: authorID, createdAt: createdAt, teams: teams}
}

func (p *TeamProposal) ID() uuid.UUID        { return p.id }
func (p *TeamProposal) AuthorID() uuid.UUID  { return p.authorID }
func (p *TeamProposal) CreatedAt() time.Time { return p.createdAt }
func (p *TeamProposal) Teams() [][]uuid.UUID { return p.teams }

func (p *TeamProposal) remove(userID uuid.UUID) bool {
	for i, team := range p.teams {
		if j := slices.Index(team, userID); j >= 0 {
			p.teams[i] = slices.Delete(team, j, j+1)
			return true
		}
	}
	return false
}

// TeamVotingRound is one round of team proposals and voting for a session
// (mobile decisions D10-D16). A round opens with its first proposal; everyone
// going can propose a full division and cast one changeable vote - for a
// proposal, or to keep the current teams when there are some. The organizer
// closes it: the proposal with the most votes replaces the teams (ties need
// the organizer to pick). Proposals and votes are deleted when the round
// ends; the round itself stays so clients can see how it ended.
//
// Methods that depend on who is going take confirmed - the user IDs currently
// going/promoted - read by the caller under the session lock.
type TeamVotingRound struct {
	id              uuid.UUID
	sessionID       uuid.UUID
	activityGroupID *uuid.UUID
	status          VotingStatus
	cancelReason    VotingCancelReason

	// Fixed when the round opens: the session's team setup when it has
	// teams, otherwise the first proposal's team count and no minimum.
	teamCount          int
	minPlayersPerTeam  *int
	keepCurrentAllowed bool

	proposals []*TeamProposal
	votes     map[uuid.UUID]uuid.UUID // voter -> proposal ID, or KeepCurrentTeams

	// Result, once closed
	winnerProposalID *uuid.UUID
	keptCurrent      bool

	version   int
	startedAt time.Time
	endedAt   *time.Time

	events []domainevent.Event
}

type OpenVotingInput struct {
	Session   *Session
	AuthorID  uuid.UUID
	Teams     [][]uuid.UUID // the first proposal
	Confirmed []uuid.UUID
}

// OpenTeamVoting opens a round with its first proposal.
func OpenTeamVoting(input OpenVotingInput) (*TeamVotingRound, *TeamProposal, error) {
	session := input.Session

	if err := session.requireTeamsChangeable(); err != nil {
		return nil, nil, err
	}

	if !session.ActivityType().IsTeamSport() {
		return nil, nil, ErrTeamsNotSupported
	}

	if !slices.Contains(input.Confirmed, input.AuthorID) {
		return nil, nil, ErrAttendeeNotGoing
	}

	r := &TeamVotingRound{
		id:              uuid.New(),
		sessionID:       session.ID(),
		activityGroupID: session.ActivityGroupID(),
		status:          VotingStatusOpen,
		votes:           map[uuid.UUID]uuid.UUID{},
		version:         1,
		startedAt:       time.Now().UTC(),
	}

	if config := session.TeamConfig(); config != nil {
		r.teamCount = config.TeamCount()
		r.minPlayersPerTeam = config.MinPlayersPerTeam()
		r.keepCurrentAllowed = true
	} else {
		r.teamCount = len(input.Teams)
		if r.teamCount < 2 || r.teamCount > MaxTeamCount {
			return nil, nil, &InvalidDivisionError{Problem: DivisionWrongTeamCount}
		}
	}

	if len(input.Confirmed) < r.PlayersNeeded() {
		return nil, nil, ErrNotEnoughPlayers
	}

	proposal, err := r.newProposal(input.AuthorID, input.Teams, input.Confirmed)
	if err != nil {
		return nil, nil, err
	}

	r.proposals = append(r.proposals, proposal)
	r.addEvent(NewVotingOpenedEvent(r, proposal, input.Confirmed))

	return r, proposal, nil
}

// ReconstructTeamVotingRound rebuilds a round from persisted data.
func ReconstructTeamVotingRound(
	id uuid.UUID,
	sessionID uuid.UUID,
	activityGroupID *uuid.UUID,
	status VotingStatus,
	cancelReason VotingCancelReason,
	teamCount int,
	minPlayersPerTeam *int,
	keepCurrentAllowed bool,
	proposals []*TeamProposal,
	votes map[uuid.UUID]uuid.UUID,
	winnerProposalID *uuid.UUID,
	keptCurrent bool,
	version int,
	startedAt time.Time,
	endedAt *time.Time,
) *TeamVotingRound {
	if votes == nil {
		votes = map[uuid.UUID]uuid.UUID{}
	}
	return &TeamVotingRound{
		id:                 id,
		sessionID:          sessionID,
		activityGroupID:    activityGroupID,
		status:             status,
		cancelReason:       cancelReason,
		teamCount:          teamCount,
		minPlayersPerTeam:  minPlayersPerTeam,
		keepCurrentAllowed: keepCurrentAllowed,
		proposals:          proposals,
		votes:              votes,
		winnerProposalID:   winnerProposalID,
		keptCurrent:        keptCurrent,
		version:            version,
		startedAt:          startedAt,
		endedAt:            endedAt,
	}
}

// Propose adds another proposal to an open round.
func (r *TeamVotingRound) Propose(session *Session, authorID uuid.UUID, teams [][]uuid.UUID, confirmed []uuid.UUID) (*TeamProposal, error) {
	if r.status != VotingStatusOpen {
		return nil, ErrVotingNotOpen
	}

	if err := session.requireTeamsChangeable(); err != nil {
		return nil, err
	}

	if !slices.Contains(confirmed, authorID) {
		return nil, ErrAttendeeNotGoing
	}

	proposal, err := r.newProposal(authorID, teams, confirmed)
	if err != nil {
		return nil, err
	}

	r.proposals = append(r.proposals, proposal)
	r.version++
	r.addEvent(NewProposalCreatedEvent(r, proposal))

	return proposal, nil
}

func (r *TeamVotingRound) newProposal(authorID uuid.UUID, teams [][]uuid.UUID, confirmed []uuid.UUID) (*TeamProposal, error) {
	if err := r.validateDivision(teams, confirmed); err != nil {
		return nil, err
	}

	copied := make([][]uuid.UUID, len(teams))
	for i, team := range teams {
		copied[i] = slices.Clone(team)
	}

	return &TeamProposal{
		id:        uuid.New(),
		authorID:  authorID,
		createdAt: time.Now().UTC(),
		teams:     copied,
	}, nil
}

// validateDivision checks D11: the round's team count, everyone going on
// exactly one team, nobody who isn't going, every team at or above the
// minimum (at least one player without a minimum).
func (r *TeamVotingRound) validateDivision(teams [][]uuid.UUID, confirmed []uuid.UUID) error {
	if len(teams) != r.teamCount {
		return &InvalidDivisionError{Problem: DivisionWrongTeamCount}
	}

	seen := make(map[uuid.UUID]struct{}, len(confirmed))
	for _, team := range teams {
		for _, userID := range team {
			if !slices.Contains(confirmed, userID) {
				return &InvalidDivisionError{Problem: DivisionPlayerNotGoing, UserIDs: []uuid.UUID{userID}}
			}
			if _, dup := seen[userID]; dup {
				return &InvalidDivisionError{Problem: DivisionDuplicatePlayer, UserIDs: []uuid.UUID{userID}}
			}
			seen[userID] = struct{}{}
		}
	}

	var missing []uuid.UUID
	for _, userID := range confirmed {
		if _, ok := seen[userID]; !ok {
			missing = append(missing, userID)
		}
	}
	if len(missing) > 0 {
		return &InvalidDivisionError{Problem: DivisionMissingPlayers, UserIDs: missing}
	}

	for _, team := range teams {
		if len(team) < r.minPerTeam() {
			return &InvalidDivisionError{Problem: DivisionTeamBelowMinimum}
		}
	}

	return nil
}

// Vote records (or changes) a vote: a proposal ID, or KeepCurrentTeams.
func (r *TeamVotingRound) Vote(voterID, choice uuid.UUID, confirmed []uuid.UUID) error {
	if r.status != VotingStatusOpen {
		return ErrVotingNotOpen
	}

	if !slices.Contains(confirmed, voterID) {
		return ErrAttendeeNotGoing
	}

	if choice == KeepCurrentTeams {
		if !r.keepCurrentAllowed {
			return ErrKeepCurrentNotAvailable
		}
	} else if r.proposal(choice) == nil {
		return ErrProposalNotFound
	}

	previous, voted := r.votes[voterID]
	if voted && previous == choice {
		return nil
	}

	r.votes[voterID] = choice
	r.version++
	r.addEvent(NewVoteCastEvent(r, voterID, choice, voted))

	return nil
}

// Close ends the round with the option that has the most votes. On a tie the
// organizer must pass winner (one of the tied options) - a nil winner then
// fails with a *TieError. It returns the winning proposal, or nil when
// "keep current teams" won (nothing changes). Proposals and votes are cleared.
// confirmed (the people going) is who gets told.
func (r *TeamVotingRound) Close(
	session *Session,
	requesterID uuid.UUID,
	requesterRole MemberRole,
	winner *uuid.UUID,
	confirmed []uuid.UUID,
) (*TeamProposal, error) {
	if !session.canManageSession(requesterID, requesterRole) {
		return nil, ErrUnauthorized
	}

	if r.status != VotingStatusOpen {
		return nil, ErrVotingNotOpen
	}

	if err := session.requireTeamsChangeable(); err != nil {
		return nil, err
	}

	mostVoted := r.mostVotedOptions()

	var choice uuid.UUID
	switch {
	case winner == nil && len(mostVoted) > 1:
		tie := &TieError{}
		for _, option := range mostVoted {
			if option == KeepCurrentTeams {
				tie.KeepCurrentTied = true
				continue
			}
			tie.ProposalIDs = append(tie.ProposalIDs, option)
		}
		return nil, tie
	case winner == nil:
		choice = mostVoted[0]
	case slices.Contains(mostVoted, *winner):
		choice = *winner
	default:
		return nil, ErrInvalidWinner
	}

	var winning *TeamProposal
	if choice == KeepCurrentTeams {
		r.keptCurrent = true
	} else {
		winning = r.proposal(choice)
		id := winning.id
		r.winnerProposalID = &id
	}

	now := time.Now().UTC()
	r.status = VotingStatusClosed
	r.endedAt = &now
	r.version++
	r.addEvent(NewVotingClosedEvent(r, requesterID, confirmed))

	r.clear()

	return winning, nil
}

// AttendeeLeft reacts to userID no longer going. Too few people left cancels
// the round; otherwise they drop out of every proposal (which may leave a team
// below the minimum - the organizer fixes that after) and lose their vote.
func (r *TeamVotingRound) AttendeeLeft(userID uuid.UUID, confirmed []uuid.UUID) {
	if r.status != VotingStatusOpen {
		return
	}

	r.version++

	if len(confirmed) < r.PlayersNeeded() {
		r.cancel(VotingCancelReasonNotEnoughPlayers, confirmed)
		return
	}

	for _, p := range r.proposals {
		p.remove(userID)
	}
	delete(r.votes, userID)

	r.addEvent(NewVotingPlayerRemovedEvent(r, userID))
}

// TeamsReplaced cancels an open round when the session's teams were recreated
// underneath it: its team count and "keep current" option no longer match.
func (r *TeamVotingRound) TeamsReplaced() {
	if r.status != VotingStatusOpen {
		return
	}

	r.version++
	r.cancel(VotingCancelReasonTeamsReplaced, nil)
}

// SessionEnded cancels an open round because its session was cancelled or
// completed - its result could never be applied.
func (r *TeamVotingRound) SessionEnded() {
	if r.status != VotingStatusOpen {
		return
	}

	r.version++
	r.cancel(VotingCancelReasonSessionEnded, nil)
}

func (r *TeamVotingRound) cancel(reason VotingCancelReason, participants []uuid.UUID) {
	now := time.Now().UTC()
	r.status = VotingStatusCancelled
	r.cancelReason = reason
	r.endedAt = &now

	r.addEvent(NewVotingCancelledEvent(r, participants))

	r.clear()
}

// clear deletes proposals and votes once the round ended (D16).
func (r *TeamVotingRound) clear() {
	r.proposals = nil
	r.votes = map[uuid.UUID]uuid.UUID{}
}

// mostVotedOptions returns the options with the most votes - proposals in creation
// order, then keep-current. With no votes at all, every option ties.
func (r *TeamVotingRound) mostVotedOptions() []uuid.UUID {
	options := make([]uuid.UUID, 0, len(r.proposals)+1)
	for _, p := range r.proposals {
		options = append(options, p.id)
	}
	if r.keepCurrentAllowed {
		options = append(options, KeepCurrentTeams)
	}

	best := -1
	var mostVoted []uuid.UUID
	for _, option := range options {
		count := r.VoteCount(option)
		switch {
		case count > best:
			best = count
			mostVoted = []uuid.UUID{option}
		case count == best:
			mostVoted = append(mostVoted, option)
		}
	}

	return mostVoted
}

func (r *TeamVotingRound) proposal(id uuid.UUID) *TeamProposal {
	for _, p := range r.proposals {
		if p.id == id {
			return p
		}
	}
	return nil
}

func (r *TeamVotingRound) minPerTeam() int {
	if r.minPlayersPerTeam == nil {
		return 1
	}
	return *r.minPlayersPerTeam
}

// PlayersNeeded is how many must be going for voting to continue: team count
// x minimum when the session has teams, one per team otherwise.
func (r *TeamVotingRound) PlayersNeeded() int {
	return r.teamCount * r.minPerTeam()
}

// VoteCount counts the votes for a proposal ID or KeepCurrentTeams.
func (r *TeamVotingRound) VoteCount(choice uuid.UUID) int {
	count := 0
	for _, c := range r.votes {
		if c == choice {
			count++
		}
	}
	return count
}

// VoteOf returns what voterID voted for, if they voted.
func (r *TeamVotingRound) VoteOf(voterID uuid.UUID) (uuid.UUID, bool) {
	choice, ok := r.votes[voterID]
	return choice, ok
}

func (r *TeamVotingRound) IsOpen() bool                     { return r.status == VotingStatusOpen }
func (r *TeamVotingRound) ID() uuid.UUID                    { return r.id }
func (r *TeamVotingRound) SessionID() uuid.UUID             { return r.sessionID }
func (r *TeamVotingRound) ActivityGroupID() *uuid.UUID      { return r.activityGroupID }
func (r *TeamVotingRound) Status() VotingStatus             { return r.status }
func (r *TeamVotingRound) CancelReason() VotingCancelReason { return r.cancelReason }
func (r *TeamVotingRound) TeamCount() int                   { return r.teamCount }
func (r *TeamVotingRound) MinPlayersPerTeam() *int          { return r.minPlayersPerTeam }
func (r *TeamVotingRound) KeepCurrentAllowed() bool         { return r.keepCurrentAllowed }
func (r *TeamVotingRound) Proposals() []*TeamProposal       { return r.proposals }
func (r *TeamVotingRound) Votes() map[uuid.UUID]uuid.UUID   { return r.votes }
func (r *TeamVotingRound) WinnerProposalID() *uuid.UUID     { return r.winnerProposalID }
func (r *TeamVotingRound) KeptCurrent() bool                { return r.keptCurrent }
func (r *TeamVotingRound) Version() int                     { return r.version }
func (r *TeamVotingRound) StartedAt() time.Time             { return r.startedAt }
func (r *TeamVotingRound) EndedAt() *time.Time              { return r.endedAt }
func (r *TeamVotingRound) Events() []domainevent.Event      { return r.events }
func (r *TeamVotingRound) ClearEvents()                     { r.events = nil }
func (r *TeamVotingRound) addEvent(event domainevent.Event) { r.events = append(r.events, event) }
