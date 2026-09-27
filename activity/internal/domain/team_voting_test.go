package domain

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func votingEventTypes(r *TeamVotingRound) []string {
	types := make([]string, 0, len(r.Events()))
	for _, e := range r.Events() {
		types = append(types, e.GetEventType())
	}
	return types
}

// openVoting opens a round on session with a 2-team split of users (first
// half / second half) proposed by users[0].
func openVoting(t *testing.T, session *Session, users []uuid.UUID) (*TeamVotingRound, *TeamProposal) {
	t.Helper()

	half := len(users) / 2
	round, proposal, err := OpenTeamVoting(OpenVotingInput{
		Session:   session,
		AuthorID:  users[0],
		Teams:     [][]uuid.UUID{users[:half], users[half:]},
		Confirmed: users,
	})
	require.NoError(t, err)

	return round, proposal
}

func requireDivisionProblem(t *testing.T, err error, want DivisionProblem) {
	t.Helper()

	var divErr *InvalidDivisionError
	require.Truef(t, errors.As(err, &divErr), "want InvalidDivisionError, got %v", err)
	assert.ErrorIs(t, err, ErrInvalidDivision)
	assert.Equal(t, want, divErr.Problem)
}

func TestOpenTeamVoting(t *testing.T) {
	t.Run("session without teams: first proposal fixes the team count, no minimum", func(t *testing.T) {
		users := newUsers(6)
		session := newTestSessionOfType(t, ActivityTypeBasketball)

		round, proposal := openVoting(t, session, users)

		assert.True(t, round.IsOpen())
		assert.Equal(t, 2, round.TeamCount())
		assert.Nil(t, round.MinPlayersPerTeam())
		assert.False(t, round.KeepCurrentAllowed())
		assert.Equal(t, 2, round.PlayersNeeded(), "one per team")
		assert.Len(t, round.Proposals(), 1)
		assert.Equal(t, users[0], proposal.AuthorID())
		assert.Equal(t, []string{EventActivitySessionVotingOpened}, votingEventTypes(round))
	})

	t.Run("session with teams: its team count and minimum, keep-current offered", func(t *testing.T) {
		users := newUsers(6)
		session := newTestTeamSession(t, newTestTeamConfig(t, intPtr(3)))

		round, _ := openVoting(t, session, users)

		assert.Equal(t, 2, round.TeamCount())
		assert.Equal(t, intPtr(3), round.MinPlayersPerTeam())
		assert.True(t, round.KeepCurrentAllowed())
		assert.Equal(t, 6, round.PlayersNeeded())
	})

	t.Run("author must be going", func(t *testing.T) {
		users := newUsers(4)
		session := newTestSessionOfType(t, ActivityTypeBasketball)
		_, _, err := OpenTeamVoting(OpenVotingInput{
			Session:   session,
			AuthorID:  uuid.New(),
			Teams:     [][]uuid.UUID{users[:2], users[2:]},
			Confirmed: users,
		})
		assert.ErrorIs(t, err, ErrAttendeeNotGoing)
	})

	t.Run("not enough people for the session's minimum", func(t *testing.T) {
		users := newUsers(5)
		session := newTestTeamSession(t, newTestTeamConfig(t, intPtr(3))) // needs 6
		_, _, err := OpenTeamVoting(OpenVotingInput{
			Session:   session,
			AuthorID:  users[0],
			Teams:     [][]uuid.UUID{users[:3], users[3:]},
			Confirmed: users,
		})
		assert.ErrorIs(t, err, ErrNotEnoughPlayers)
	})

	t.Run("not a team sport", func(t *testing.T) {
		users := newUsers(4)
		session := newTestSessionOfType(t, ActivityTypeRunning)
		_, _, err := OpenTeamVoting(OpenVotingInput{
			Session:   session,
			AuthorID:  users[0],
			Teams:     [][]uuid.UUID{users[:2], users[2:]},
			Confirmed: users,
		})
		assert.ErrorIs(t, err, ErrTeamsNotSupported)
	})

	t.Run("one team is not a division", func(t *testing.T) {
		users := newUsers(4)
		session := newTestSessionOfType(t, ActivityTypeBasketball)
		_, _, err := OpenTeamVoting(OpenVotingInput{
			Session:   session,
			AuthorID:  users[0],
			Teams:     [][]uuid.UUID{users},
			Confirmed: users,
		})
		requireDivisionProblem(t, err, DivisionWrongTeamCount)
	})
}

func TestTeamVotingRound_ProposalValidation(t *testing.T) {
	users := newUsers(6)
	session := newTestTeamSession(t, newTestTeamConfig(t, intPtr(2)))
	round, _ := openVoting(t, session, users)
	stranger := uuid.New()

	tests := []struct {
		name  string
		teams [][]uuid.UUID
		want  DivisionProblem
	}{
		{"wrong team count", [][]uuid.UUID{users[:2], users[2:4], users[4:]}, DivisionWrongTeamCount},
		{"someone not going", [][]uuid.UUID{users[:3], append(users[3:6:6], stranger)}, DivisionPlayerNotGoing},
		{"someone on two teams", [][]uuid.UUID{users[:4], users[3:]}, DivisionDuplicatePlayer},
		{"someone left out", [][]uuid.UUID{users[:3], users[3:5]}, DivisionMissingPlayers},
		{"team below the minimum", [][]uuid.UUID{users[:5], users[5:]}, DivisionTeamBelowMinimum},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := round.Propose(session, users[1], tt.teams, users)
			requireDivisionProblem(t, err, tt.want)
		})
	}

	t.Run("valid, uneven is fine", func(t *testing.T) {
		p, err := round.Propose(session, users[1], [][]uuid.UUID{users[:4], users[4:]}, users)
		require.NoError(t, err)
		assert.Len(t, round.Proposals(), 2)
		assert.Equal(t, users[1], p.AuthorID())
	})

	t.Run("author not going", func(t *testing.T) {
		_, err := round.Propose(session, stranger, [][]uuid.UUID{users[:3], users[3:]}, users)
		assert.ErrorIs(t, err, ErrAttendeeNotGoing)
	})
}

func TestTeamVotingRound_Vote(t *testing.T) {
	users := newUsers(4)
	session := newTestTeamSession(t, newTestTeamConfig(t, nil))
	round, first := openVoting(t, session, users)
	second, err := round.Propose(session, users[1], [][]uuid.UUID{{users[0], users[3]}, {users[1], users[2]}}, users)
	require.NoError(t, err)
	round.ClearEvents()

	require.NoError(t, round.Vote(users[0], first.ID(), users))
	require.NoError(t, round.Vote(users[1], KeepCurrentTeams, users))
	assert.Equal(t, 1, round.VoteCount(first.ID()))
	assert.Equal(t, 1, round.VoteCount(KeepCurrentTeams))

	// changing a vote
	require.NoError(t, round.Vote(users[0], second.ID(), users))
	assert.Equal(t, 0, round.VoteCount(first.ID()))
	assert.Equal(t, 1, round.VoteCount(second.ID()))
	choice, voted := round.VoteOf(users[0])
	assert.True(t, voted)
	assert.Equal(t, second.ID(), choice)

	// same vote again: nothing happens
	before := round.Version()
	require.NoError(t, round.Vote(users[0], second.ID(), users))
	assert.Equal(t, before, round.Version())

	assert.ErrorIs(t, round.Vote(uuid.New(), first.ID(), users), ErrAttendeeNotGoing)
	assert.ErrorIs(t, round.Vote(users[2], uuid.New(), users), ErrProposalNotFound)
	assert.Equal(t, []string{EventActivitySessionVotingVoteCast, EventActivitySessionVotingVoteCast, EventActivitySessionVotingVoteCast}, votingEventTypes(round))

	t.Run("keep current needs teams", func(t *testing.T) {
		noTeams := newTestSessionOfType(t, ActivityTypeBasketball)
		r, _ := openVoting(t, noTeams, users)
		assert.ErrorIs(t, r.Vote(users[0], KeepCurrentTeams, users), ErrKeepCurrentNotAvailable)
	})
}

func TestTeamVotingRound_Close(t *testing.T) {
	setup := func(t *testing.T) (*Session, *TeamVotingRound, *TeamProposal, *TeamProposal, []uuid.UUID) {
		t.Helper()
		users := newUsers(4)
		session := newTestTeamSession(t, newTestTeamConfig(t, nil))
		round, first := openVoting(t, session, users)
		second, err := round.Propose(session, users[1], [][]uuid.UUID{{users[0], users[3]}, {users[1], users[2]}}, users)
		require.NoError(t, err)
		return session, round, first, second, users
	}

	t.Run("most votes wins, proposals and votes are cleared (D16)", func(t *testing.T) {
		session, round, first, second, users := setup(t)
		require.NoError(t, round.Vote(users[0], second.ID(), users))
		require.NoError(t, round.Vote(users[1], second.ID(), users))
		require.NoError(t, round.Vote(users[2], first.ID(), users))

		winning, err := round.Close(session, session.CreatedByID(), "", nil)

		require.NoError(t, err)
		assert.Equal(t, second.ID(), winning.ID())
		assert.Equal(t, VotingStatusClosed, round.Status())
		assert.Equal(t, second.ID(), *round.WinnerProposalID())
		assert.Empty(t, round.Proposals())
		assert.Empty(t, round.Votes())
	})

	t.Run("keep current wins: nothing to apply", func(t *testing.T) {
		session, round, _, _, users := setup(t)
		require.NoError(t, round.Vote(users[0], KeepCurrentTeams, users))

		winning, err := round.Close(session, session.CreatedByID(), "", nil)

		require.NoError(t, err)
		assert.Nil(t, winning)
		assert.True(t, round.KeptCurrent())
	})

	t.Run("tie needs the organizer to pick (D15)", func(t *testing.T) {
		session, round, first, second, users := setup(t)
		require.NoError(t, round.Vote(users[0], first.ID(), users))
		require.NoError(t, round.Vote(users[1], second.ID(), users))

		_, err := round.Close(session, session.CreatedByID(), "", nil)

		var tie *TieError
		require.True(t, errors.As(err, &tie))
		assert.ErrorIs(t, err, ErrTieRequiresWinner)
		assert.ElementsMatch(t, []uuid.UUID{first.ID(), second.ID()}, tie.ProposalIDs)
		assert.False(t, tie.KeepCurrentTied)
		assert.True(t, round.IsOpen(), "a rejected close changes nothing")

		winner := second.ID()
		winning, err := round.Close(session, session.CreatedByID(), "", &winner)
		require.NoError(t, err)
		assert.Equal(t, second.ID(), winning.ID())
	})

	t.Run("no votes at all ties every option, keep current included", func(t *testing.T) {
		session, round, _, _, _ := setup(t)

		_, err := round.Close(session, session.CreatedByID(), "", nil)

		var tie *TieError
		require.True(t, errors.As(err, &tie))
		assert.Len(t, tie.ProposalIDs, 2)
		assert.True(t, tie.KeepCurrentTied)
	})

	t.Run("picked winner must be among the most-voted options", func(t *testing.T) {
		session, round, first, second, users := setup(t)
		require.NoError(t, round.Vote(users[0], first.ID(), users))

		loser := second.ID()
		_, err := round.Close(session, session.CreatedByID(), "", &loser)
		assert.ErrorIs(t, err, ErrInvalidWinner)
	})

	t.Run("regular member can't close", func(t *testing.T) {
		session, round, _, _, _ := setup(t)
		_, err := round.Close(session, uuid.New(), MemberRoleMember, nil)
		assert.ErrorIs(t, err, ErrUnauthorized)
	})

	t.Run("closed round rejects everything", func(t *testing.T) {
		session, round, first, _, users := setup(t)
		require.NoError(t, round.Vote(users[0], first.ID(), users))
		_, err := round.Close(session, session.CreatedByID(), "", nil)
		require.NoError(t, err)

		assert.ErrorIs(t, round.Vote(users[1], first.ID(), users), ErrVotingNotOpen)
		_, err = round.Propose(session, users[1], [][]uuid.UUID{users[:2], users[2:]}, users)
		assert.ErrorIs(t, err, ErrVotingNotOpen)
		_, err = round.Close(session, session.CreatedByID(), "", nil)
		assert.ErrorIs(t, err, ErrVotingNotOpen)
	})
}

func TestTeamVotingRound_AttendeeLeft(t *testing.T) {
	t.Run("drops out of every proposal and loses their vote (D12)", func(t *testing.T) {
		users := newUsers(6)
		session := newTestSessionOfType(t, ActivityTypeBasketball)
		round, first := openVoting(t, session, users)
		require.NoError(t, round.Vote(users[5], first.ID(), users))
		round.ClearEvents()

		round.AttendeeLeft(users[5], users[:5])

		assert.True(t, round.IsOpen())
		assert.NotContains(t, first.Teams()[1], users[5])
		assert.Equal(t, 0, round.VoteCount(first.ID()))
		assert.Equal(t, []string{EventActivitySessionVotingPlayerRemoved}, votingEventTypes(round))
	})

	t.Run("a team may fall below the minimum and the proposal still counts", func(t *testing.T) {
		users := newUsers(7)
		session := newTestTeamSession(t, newTestTeamConfig(t, intPtr(3)))
		round, first, err := OpenTeamVoting(OpenVotingInput{
			Session:   session,
			AuthorID:  users[0],
			Teams:     [][]uuid.UUID{users[:4], users[4:]}, // 4 vs 3
			Confirmed: users,
		})
		require.NoError(t, err)

		round.AttendeeLeft(users[6], users[:6]) // 6 going, still >= 6 needed

		assert.True(t, round.IsOpen())
		assert.Len(t, first.Teams()[1], 2, "below the minimum of 3")
	})

	t.Run("falling below players needed cancels (D12)", func(t *testing.T) {
		users := newUsers(6)
		session := newTestTeamSession(t, newTestTeamConfig(t, intPtr(3))) // needs 6
		round, _ := openVoting(t, session, users)
		round.ClearEvents()

		round.AttendeeLeft(users[5], users[:5])

		assert.Equal(t, VotingStatusCancelled, round.Status())
		assert.Equal(t, VotingCancelReasonNotEnoughPlayers, round.CancelReason())
		assert.Empty(t, round.Proposals())
		assert.Equal(t, []string{EventActivitySessionVotingCancelled}, votingEventTypes(round))
	})
}

func TestTeamVotingRound_TeamsReplaced(t *testing.T) {
	users := newUsers(4)
	session := newTestSessionOfType(t, ActivityTypeBasketball)
	round, _ := openVoting(t, session, users)

	round.TeamsReplaced()

	assert.Equal(t, VotingStatusCancelled, round.Status())
	assert.Equal(t, VotingCancelReasonTeamsReplaced, round.CancelReason())
}

func TestSession_ApplyProposalTeams(t *testing.T) {
	users := newUsers(4)
	config, err := NewTeamConfig(nil, intPtr(2), []string{"#FFFFFF", "#000000"})
	require.NoError(t, err)
	session := newTestTeamSession(t, &config)
	oldTeam := session.Teams()[0].ID()

	round, proposal := openVoting(t, session, users)
	assert.ErrorIs(t, session.ApplyProposalTeams(round, session.CreatedByID()), ErrVotingNotOpen, "only a closed round with a winner")

	require.NoError(t, round.Vote(users[0], proposal.ID(), users))
	_, err = round.Close(session, session.CreatedByID(), "", nil)
	require.NoError(t, err)
	session.ClearEvents()

	require.NoError(t, session.ApplyProposalTeams(round, session.CreatedByID()))

	require.Len(t, session.Teams(), 2)
	_, stillThere := session.Team(oldTeam)
	assert.False(t, stillThere)
	assert.Equal(t, "#FFFFFF", session.Teams()[0].Color(), "colors kept")
	assert.Equal(t, intPtr(2), session.TeamConfig().MinPlayersPerTeam())
	assert.True(t, session.Events()[0].(*SessionTeamsCreatedEvent).Replaced)
}
