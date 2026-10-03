package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Draft/voting events that end or open something for everyone carry the
// people going, so notifications can fan out without querying activity.

func lastEvent[T domainevent.Event](t *testing.T, events []domainevent.Event) T {
	t.Helper()
	require.NotEmpty(t, events)
	e, ok := events[len(events)-1].(T)
	require.Truef(t, ok, "last event is %T", events[len(events)-1])
	return e
}

func TestDraftCancelledEvent_Participants(t *testing.T) {
	t.Run("organizer cancel carries the people going", func(t *testing.T) {
		users := newUsers(4)
		session, draft := newTestDraft(t, users, PickOrderSnake, nil)
		draft.ClearEvents()

		require.NoError(t, draft.Cancel(session, session.CreatedByID(), "", users))

		e := lastEvent[*DraftCancelledEvent](t, draft.Events())
		assert.Equal(t, users, e.ParticipantUserIDs)
		assert.Equal(t, session.CreatedByID(), e.CancelledBy)
	})

	t.Run("not enough players carries who is still going", func(t *testing.T) {
		users := newUsers(6)
		_, draft := newTestDraft(t, users, PickOrderSnake, intPtr(3))
		draft.ClearEvents()

		draft.AttendeeLeft(users[5], users[:5])

		e := lastEvent[*DraftCancelledEvent](t, draft.Events())
		assert.Equal(t, users[:5], e.ParticipantUserIDs)
		assert.Equal(t, uuid.Nil, e.CancelledBy)
	})

	t.Run("session ended carries nobody", func(t *testing.T) {
		users := newUsers(4)
		session, draft := newTestDraft(t, users, PickOrderSnake, nil)
		draft.ClearEvents()

		draft.SessionEnded(session.CreatedByID())

		e := lastEvent[*DraftCancelledEvent](t, draft.Events())
		assert.Empty(t, e.ParticipantUserIDs)
	})

	t.Run("the event keeps its own copy", func(t *testing.T) {
		users := newUsers(4)
		session, draft := newTestDraft(t, users, PickOrderSnake, nil)
		confirmed := append([]uuid.UUID(nil), users...)

		require.NoError(t, draft.Cancel(session, session.CreatedByID(), "", confirmed))
		confirmed[0] = uuid.New()

		e := lastEvent[*DraftCancelledEvent](t, draft.Events())
		assert.Equal(t, users[0], e.ParticipantUserIDs[0])
	})
}

func TestVotingEvents_Participants(t *testing.T) {
	t.Run("opened carries the eligible voters", func(t *testing.T) {
		users := newUsers(4)
		round, _ := openVoting(t, newTestSessionOfType(t, ActivityTypeBasketball), users)

		e := lastEvent[*VotingOpenedEvent](t, round.Events())
		assert.Equal(t, users, e.ParticipantUserIDs)
		assert.Equal(t, users[0], e.AuthorID)
	})

	t.Run("closed carries the people going", func(t *testing.T) {
		users := newUsers(4)
		session := newTestSessionOfType(t, ActivityTypeBasketball)
		round, proposal := openVoting(t, session, users)
		require.NoError(t, round.Vote(users[1], proposal.ID(), users))
		round.ClearEvents()

		_, err := round.Close(session, session.CreatedByID(), "", nil, users)
		require.NoError(t, err)

		e := lastEvent[*VotingClosedEvent](t, round.Events())
		assert.Equal(t, users, e.ParticipantUserIDs)
		assert.Equal(t, proposal.ID(), *e.WinnerProposalID)
	})

	t.Run("not enough players carries who is still going", func(t *testing.T) {
		users := newUsers(6)
		round, _ := openVoting(t, newTestTeamSession(t, newTestTeamConfig(t, intPtr(3))), users)
		round.ClearEvents()

		round.AttendeeLeft(users[5], users[:5])

		e := lastEvent[*VotingCancelledEvent](t, round.Events())
		assert.Equal(t, VotingCancelReasonNotEnoughPlayers, e.Reason)
		assert.Equal(t, users[:5], e.ParticipantUserIDs)
	})

	t.Run("teams replaced and session ended carry nobody", func(t *testing.T) {
		for name, end := range map[string]func(*TeamVotingRound){
			"teams_replaced": (*TeamVotingRound).TeamsReplaced,
			"session_ended":  (*TeamVotingRound).SessionEnded,
		} {
			t.Run(name, func(t *testing.T) {
				users := newUsers(4)
				round, _ := openVoting(t, newTestSessionOfType(t, ActivityTypeBasketball), users)
				round.ClearEvents()

				end(round)

				e := lastEvent[*VotingCancelledEvent](t, round.Events())
				assert.Empty(t, e.ParticipantUserIDs)
			})
		}
	})
}
