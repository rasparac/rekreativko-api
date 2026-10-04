//go:build integration

package persistence_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (e *sessionTeamTestEnv) propose(session *domain.Session, author uuid.UUID, teams ...[]uuid.UUID) (*application.TeamVotingState, error) {
	return e.votingSvc.Propose(e.ctx, application.ProposeTeamsParams{
		SessionID: session.ID(),
		AuthorID:  author,
		Teams:     teams,
	})
}

func (e *sessionTeamTestEnv) vote(session *domain.Session, voter, proposalID uuid.UUID) error {
	_, err := e.votingSvc.Vote(e.ctx, application.CastVoteParams{
		SessionID:  session.ID(),
		VoterID:    voter,
		ProposalID: &proposalID,
	})
	return err
}

func (e *sessionTeamTestEnv) voteKeepCurrent(session *domain.Session, voter uuid.UUID) error {
	_, err := e.votingSvc.Vote(e.ctx, application.CastVoteParams{
		SessionID:   session.ID(),
		VoterID:     voter,
		KeepCurrent: true,
	})
	return err
}

func (e *sessionTeamTestEnv) closeVoting(session *domain.Session, winner *uuid.UUID) (*application.TeamVotingState, error) {
	return e.votingSvc.Close(e.ctx, application.CloseVotingParams{
		SessionID:        session.ID(),
		RequesterID:      session.CreatedByID(),
		WinnerProposalID: winner,
	})
}

func (e *sessionTeamTestEnv) voting(t *testing.T, session *domain.Session) *application.TeamVotingState {
	t.Helper()

	state, err := e.votingSvc.GetVoting(e.ctx, session.ID())
	require.NoError(t, err)

	return state
}

func TestTeamVoting_ProposeVoteCloseAppliesTheWinner(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 4) // basketball, no teams yet

	state, err := env.propose(session, u[0], []uuid.UUID{u[0], u[1]}, []uuid.UUID{u[2], u[3]})
	require.NoError(t, err)
	first := state.Round.Proposals()[0].ID()

	state, err = env.propose(session, u[1], []uuid.UUID{u[0], u[3]}, []uuid.UUID{u[1], u[2]})
	require.NoError(t, err)
	second := state.Round.Proposals()[1].ID()

	require.NoError(t, env.vote(session, u[0], second))
	require.NoError(t, env.vote(session, u[1], second))
	require.NoError(t, env.vote(session, u[2], first))

	// read back from the database before closing
	open := env.voting(t, session)
	require.True(t, open.Round.IsOpen())
	assert.Len(t, open.Round.Proposals(), 2)
	assert.Equal(t, 2, open.Round.VoteCount(second))
	assert.Equal(t, []uuid.UUID{u[0], u[3]}, open.Round.Proposals()[1].Teams()[0], "member order kept")
	require.NotNil(t, open.PlayersNeeded)
	assert.Equal(t, 2, *open.PlayersNeeded)

	closed, err := env.closeVoting(session, nil)
	require.NoError(t, err)
	assert.Equal(t, domain.VotingStatusClosed, closed.Round.Status())

	teamA, teamB := env.teamIDAt(t, session, 0), env.teamIDAt(t, session, 1)
	assert.Equal(t, &teamA, env.teamOf(t, session, u[0]))
	assert.Equal(t, &teamA, env.teamOf(t, session, u[3]))
	assert.Equal(t, &teamB, env.teamOf(t, session, u[1]))
	assert.Equal(t, &teamB, env.teamOf(t, session, u[2]))

	// D16: proposals and votes are gone, the result stays
	after := env.voting(t, session)
	assert.Equal(t, domain.VotingStatusClosed, after.Round.Status())
	assert.Empty(t, after.Round.Proposals())
	assert.Empty(t, after.Round.Votes())
	assert.Equal(t, second, *after.Round.WinnerProposalID())
}

func TestTeamVoting_KeepCurrentWinsAndNothingChanges(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 4)
	withTeams, err := env.createTeams(session, nil, nil, nil)
	require.NoError(t, err)
	oldTeamA := withTeams.Teams()[0].ID()
	require.NoError(t, env.assign(session, u[0], oldTeamA))

	_, err = env.propose(session, u[1], []uuid.UUID{u[0], u[1]}, []uuid.UUID{u[2], u[3]})
	require.NoError(t, err)
	require.NoError(t, env.voteKeepCurrent(session, u[0]))
	require.NoError(t, env.voteKeepCurrent(session, u[1]))

	closed, err := env.closeVoting(session, nil)
	require.NoError(t, err)
	assert.True(t, closed.Round.KeptCurrent())

	assert.Equal(t, oldTeamA, env.teamIDAt(t, session, 0), "same teams")
	assert.Equal(t, &oldTeamA, env.teamOf(t, session, u[0]), "same assignment")
}

func TestTeamVoting_TieNeedsTheOrganizer(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 4)

	state, err := env.propose(session, u[0], []uuid.UUID{u[0], u[1]}, []uuid.UUID{u[2], u[3]})
	require.NoError(t, err)
	first := state.Round.Proposals()[0].ID()
	state, err = env.propose(session, u[1], []uuid.UUID{u[0], u[2]}, []uuid.UUID{u[1], u[3]})
	require.NoError(t, err)
	second := state.Round.Proposals()[1].ID()
	require.NoError(t, env.vote(session, u[0], first))
	require.NoError(t, env.vote(session, u[1], second))

	_, err = env.closeVoting(session, nil)
	var tie *domain.TieError
	require.True(t, errors.As(err, &tie), "got %v", err)
	assert.ElementsMatch(t, []uuid.UUID{first, second}, tie.ProposalIDs)
	assert.True(t, env.voting(t, session).Round.IsOpen(), "rejected close changed nothing")

	_, err = env.closeVoting(session, &second)
	require.NoError(t, err)
	teamA := env.teamIDAt(t, session, 0)
	assert.Equal(t, &teamA, env.teamOf(t, session, u[2]))
}

func TestTeamVoting_TieNotifiesOtherManagersOnce(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 4)

	state, err := env.propose(session, u[0], []uuid.UUID{u[0], u[1]}, []uuid.UUID{u[2], u[3]})
	require.NoError(t, err)
	first := state.Round.Proposals()[0].ID()
	state, err = env.propose(session, u[1], []uuid.UUID{u[0], u[2]}, []uuid.UUID{u[1], u[3]})
	require.NoError(t, err)
	second := state.Round.Proposals()[1].ID()
	require.NoError(t, env.vote(session, u[0], first))
	require.NoError(t, env.vote(session, u[1], second))

	closeAs := func(requester uuid.UUID, role string) error {
		_, err := env.votingSvc.Close(env.ctx, application.CloseVotingParams{
			SessionID:     session.ID(),
			RequesterID:   requester,
			RequesterRole: role,
		})
		return err
	}

	// the creator closing is the only manager, and they saw the 409
	var tie *domain.TieError
	require.ErrorAs(t, closeAs(session.CreatedByID(), ""), &tie)
	assert.Zero(t, env.outboxCount(t, "activity.session.voting.tied"))

	// an admin closing: the creator is told
	admin := uuid.New()
	require.ErrorAs(t, closeAs(admin, "admin"), &tie)
	assert.Equal(t, 1, env.outboxCount(t, "activity.session.voting.tied"))

	// the same tie again is not announced twice
	require.ErrorAs(t, closeAs(admin, "admin"), &tie)
	assert.Equal(t, 1, env.outboxCount(t, "activity.session.voting.tied"))

	var payload struct {
		ManagerUserIDs []uuid.UUID `json:"manager_user_ids"`
	}
	var raw []byte
	require.NoError(t, env.txManager.Querier(env.ctx).QueryRow(env.ctx,
		`SELECT payload FROM activity.event_outbox WHERE event_type = 'activity.session.voting.tied'`,
	).Scan(&raw))
	require.NoError(t, json.Unmarshal(raw, &payload))
	assert.Equal(t, []uuid.UUID{session.CreatedByID()}, payload.ManagerUserIDs)

	assert.True(t, env.voting(t, session).Round.IsOpen(), "a tie changes nothing")
}

// Someone leaving resets team formation (see team_reset_integration_test.go),
// so mid-vote only a new joiner changes an open round.
func TestTeamVoting_AttendanceChanges(t *testing.T) {
	t.Run("joiner can vote and stays unassigned after the winner is applied", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session, u := env.draftSession(t, 4)
		state, err := env.propose(session, u[0], []uuid.UUID{u[0], u[1]}, []uuid.UUID{u[2], u[3]})
		require.NoError(t, err)
		proposal := state.Round.Proposals()[0].ID()

		late := env.addGoingAttendee(t, session)
		require.NoError(t, env.vote(session, late, proposal))

		_, err = env.closeVoting(session, nil)
		require.NoError(t, err)
		assert.Nil(t, env.teamOf(t, session, late), "the organizer places them")
		assert.NotNil(t, env.teamOf(t, session, u[0]))
	})
}

func TestTeamVoting_TeamsReplacedCancelsTheRound(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 4)
	_, err := env.propose(session, u[0], []uuid.UUID{u[0], u[1]}, []uuid.UUID{u[2], u[3]})
	require.NoError(t, err)

	_, err = env.createTeams(session, ptr(2), nil, nil)
	require.NoError(t, err)

	got := env.voting(t, session)
	assert.Equal(t, domain.VotingStatusCancelled, got.Round.Status())
	assert.Equal(t, domain.VotingCancelReasonTeamsReplaced, got.Round.CancelReason())
}

func TestTeamVoting_DraftAndVotingExcludeEachOther(t *testing.T) {
	t.Run("no proposals while a draft runs", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session, u := env.draftSession(t, 4)
		_, err := env.startDraft(session, u[0], u[1], nil)
		require.NoError(t, err)

		_, err = env.propose(session, u[0], []uuid.UUID{u[0], u[1]}, []uuid.UUID{u[2], u[3]})
		assert.ErrorIs(t, err, domain.ErrDraftAlreadyActive)
	})

	t.Run("no draft while voting is open", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session, u := env.draftSession(t, 4)
		_, err := env.propose(session, u[0], []uuid.UUID{u[0], u[1]}, []uuid.UUID{u[2], u[3]})
		require.NoError(t, err)

		_, err = env.startDraft(session, u[0], u[1], nil)
		assert.ErrorIs(t, err, domain.ErrVotingOpen)
	})
}

func TestTeamVoting_InvalidDivisionReportsTheReason(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 4)

	_, err := env.propose(session, u[0], []uuid.UUID{u[0], u[1]}, []uuid.UUID{u[2]})

	var division *domain.InvalidDivisionError
	require.True(t, errors.As(err, &division), "got %v", err)
	assert.Equal(t, domain.DivisionMissingPlayers, division.Problem)
	assert.Equal(t, []uuid.UUID{u[3]}, division.UserIDs)
}

// TestTeamVoting_ConcurrentVotes_NoneLost casts votes from everyone at once.
// Saving a round rewrites its votes, so without the session lock two votes
// could read the same round and the later save would drop the earlier vote.
func TestTeamVoting_ConcurrentVotes_NoneLost(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 10)
	state, err := env.propose(session, u[0], u[:5], u[5:])
	require.NoError(t, err)
	proposal := state.Round.Proposals()[0].ID()

	var wg sync.WaitGroup
	errs := make([]error, len(u))
	for i, voter := range u {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = env.vote(session, voter, proposal)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		require.NoErrorf(t, err, "voter %d", i)
	}

	got := env.voting(t, session)
	assert.Equal(t, len(u), got.Round.VoteCount(proposal), "every concurrent vote persisted")
	assert.Equal(t, 1+len(u), got.Round.Version(), "one version per vote")
}

func TestTeamVoting_SessionCancelledCancelsTheRound(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 4)
	_, err := env.propose(session, u[0], []uuid.UUID{u[0], u[1]}, []uuid.UUID{u[2], u[3]})
	require.NoError(t, err)

	require.NoError(t, env.sessionSvc.CancelSession(env.ctx, session.ID(), session.CreatedByID(), "", "rain"))

	got := env.voting(t, session)
	assert.Equal(t, domain.VotingStatusCancelled, got.Round.Status())
	assert.Equal(t, domain.VotingCancelReasonSessionEnded, got.Round.CancelReason())
}

func ptr(v int) *int {
	return &v
}
