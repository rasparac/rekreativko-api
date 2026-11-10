package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newUsers(n int) []uuid.UUID {
	users := make([]uuid.UUID, n)
	for i := range users {
		users[i] = uuid.New()
	}
	return users
}

// newTestDraft starts a draft on a basketball session with the first two
// users as captains (Team A, Team B) and everyone in users going.
func newTestDraft(t *testing.T, users []uuid.UUID, order PickOrder, minPlayersPerTeam *int) (*Session, *TeamDraft) {
	t.Helper()

	session := newTestSessionOfType(t, ActivityTypeBasketball)
	draft, err := NewTeamDraft(TeamDraftInput{
		Session:           session,
		RequesterID:       session.CreatedByID(),
		CaptainIDs:        [2]uuid.UUID{users[0], users[1]},
		PickOrder:         order,
		MinPlayersPerTeam: minPlayersPerTeam,
		Confirmed:         users,
	})
	require.NoError(t, err)

	return session, draft
}

func draftEventTypes(d *TeamDraft) []string {
	types := make([]string, 0, len(d.Events()))
	for _, e := range d.Events() {
		types = append(types, e.GetEventType())
	}
	return types
}

func sidesOf(order []DraftSide) []int {
	out := make([]int, len(order))
	for i, s := range order {
		out[i] = int(s)
	}
	return out
}

func TestPickOrder_SideForTurn(t *testing.T) {
	sides := func(o PickOrder) []int {
		out := make([]int, 8)
		for turn := range out {
			out[turn] = int(o.sideForTurn(turn))
		}
		return out
	}

	assert.Equal(t, []int{0, 1, 1, 0, 0, 1, 1, 0}, sides(PickOrderSnake))
	assert.Equal(t, []int{0, 1, 0, 1, 0, 1, 0, 1}, sides(PickOrderAlternate))
}

func TestNewTeamDraft(t *testing.T) {
	users := newUsers(6)

	t.Run("defaults to snake, active, captain A's turn", func(t *testing.T) {
		_, draft := newTestDraft(t, users, "", nil)

		assert.Equal(t, PickOrderSnake, draft.PickOrder())
		assert.Equal(t, DraftStatusActive, draft.Status())
		assert.Equal(t, users[0], *draft.CurrentCaptainID())
		assert.Equal(t, users[2:], draft.Available(users))
		assert.Equal(t, 1, draft.Version())
		assert.Equal(t, []string{EventActivitySessionDraftStarted, EventActivitySessionDraftTurnChanged}, draftEventTypes(draft))
	})

	t.Run("only the captains going completes immediately", func(t *testing.T) {
		_, draft := newTestDraft(t, users[:2], PickOrderSnake, nil)

		assert.True(t, draft.IsCompleted())
		assert.Nil(t, draft.CurrentCaptainID())
	})

	rejections := []struct {
		name    string
		modify  func(in *TeamDraftInput)
		wantErr error
	}{
		{"regular member", func(in *TeamDraftInput) { in.RequesterID = uuid.New(); in.RequesterRole = MemberRoleMember }, ErrUnauthorized},
		{"same captain twice", func(in *TeamDraftInput) { in.CaptainIDs = [2]uuid.UUID{users[0], users[0]} }, ErrInvalidDraftCaptains},
		{"captain not going", func(in *TeamDraftInput) { in.CaptainIDs = [2]uuid.UUID{users[0], uuid.New()} }, ErrDraftCaptainNotConfirmed},
		{"unknown pick order", func(in *TeamDraftInput) { in.PickOrder = "random" }, ErrInvalidPickOrder},
		{"bad colors", func(in *TeamDraftInput) { in.Colors = []string{"#FFFFFF"} }, ErrInvalidTeamColors},
		{"not enough people (D3: 2 x 4 > 6)", func(in *TeamDraftInput) { in.MinPlayersPerTeam = intPtr(4) }, ErrNotEnoughPlayers},
	}
	for _, tt := range rejections {
		t.Run(tt.name, func(t *testing.T) {
			session := newTestSessionOfType(t, ActivityTypeBasketball)
			in := TeamDraftInput{
				Session:     session,
				RequesterID: session.CreatedByID(),
				CaptainIDs:  [2]uuid.UUID{users[0], users[1]},
				Confirmed:   users,
			}
			tt.modify(&in)

			_, err := NewTeamDraft(in)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}

	t.Run("exactly enough people is fine (2 x 3 = 6)", func(t *testing.T) {
		_, draft := newTestDraft(t, users, PickOrderSnake, intPtr(3))
		assert.Equal(t, DraftStatusActive, draft.Status())
	})

	t.Run("not a team sport", func(t *testing.T) {
		session := newTestSessionOfType(t, ActivityTypeRunning)
		_, err := NewTeamDraft(TeamDraftInput{
			Session:     session,
			RequesterID: session.CreatedByID(),
			CaptainIDs:  [2]uuid.UUID{users[0], users[1]},
			Confirmed:   users,
		})
		assert.ErrorIs(t, err, ErrTeamsNotSupported)
	})
}

func TestTeamDraft_SnakeDraftRunsUntilEveryoneIsPicked(t *testing.T) {
	users := newUsers(6)
	session, draft := newTestDraft(t, users, PickOrderSnake, nil)
	a, b := users[0], users[1]

	require.NoError(t, draft.Pick(session, a, users[2], users))
	require.NoError(t, draft.Pick(session, b, users[3], users))
	require.Equal(t, b, *draft.CurrentCaptainID(), "snake: B picks twice in a row")
	require.NoError(t, draft.Pick(session, b, users[4], users))
	draft.ClearEvents()
	require.NoError(t, draft.Pick(session, a, users[5], users))

	assert.True(t, draft.IsCompleted())
	assert.Equal(t, []uuid.UUID{a, users[2], users[5]}, draft.Roster(DraftSideA))
	assert.Equal(t, []uuid.UUID{b, users[3], users[4]}, draft.Roster(DraftSideB))
	assert.Equal(t, []string{EventActivitySessionDraftPlayerPicked, EventActivitySessionDraftCompleted}, draftEventTypes(draft))
}

// D1/D2: the minimum is not a cap - the draft keeps going past it until
// everyone going is on a team.
func TestTeamDraft_MinimumDoesNotStopTheDraft(t *testing.T) {
	users := newUsers(7) // captains + 5 players, min 2 per team
	session, draft := newTestDraft(t, users, PickOrderAlternate, intPtr(2))

	for i, player := range users[2:] {
		captain := users[i%2]
		require.NoErrorf(t, draft.Pick(session, captain, player, users), "pick %d", i)
	}

	assert.True(t, draft.IsCompleted())
	assert.Len(t, draft.Roster(DraftSideA), 4, "uneven teams are fine")
	assert.Len(t, draft.Roster(DraftSideB), 3)
}

func TestTeamDraft_AlternateOrder(t *testing.T) {
	users := newUsers(5)
	session, draft := newTestDraft(t, users, PickOrderAlternate, nil)

	require.NoError(t, draft.Pick(session, users[0], users[2], users))
	require.NoError(t, draft.Pick(session, users[1], users[3], users))
	assert.Equal(t, users[0], *draft.CurrentCaptainID(), "alternate: back to A")
}

func TestTeamDraft_PickRejections(t *testing.T) {
	users := newUsers(5)
	session, draft := newTestDraft(t, users, PickOrderSnake, nil)
	a, b := users[0], users[1]

	assert.ErrorIs(t, draft.Pick(session, b, users[2], users), ErrNotYourTurn)
	assert.ErrorIs(t, draft.Pick(session, users[2], users[3], users), ErrNotDraftCaptain)
	assert.ErrorIs(t, draft.Pick(session, a, b, users), ErrPlayerNotAvailable, "captains can't be picked")
	assert.ErrorIs(t, draft.Pick(session, a, uuid.New(), users), ErrPlayerNotAvailable, "not going")

	require.NoError(t, draft.Pick(session, a, users[2], users))
	assert.ErrorIs(t, draft.Pick(session, b, users[2], users), ErrPlayerNotAvailable, "already picked")

	require.NoError(t, session.Cancel(session.CreatedByID(), "", "rain", nil))
	assert.ErrorIs(t, draft.Pick(session, b, users[3], users), ErrSessionCanceled)
}

func TestTeamDraft_ANewAttendeeJoinsThePool(t *testing.T) {
	users := newUsers(3)
	session, draft := newTestDraft(t, users, PickOrderSnake, nil)
	joined := append(users, uuid.New())

	require.NoError(t, draft.Pick(session, users[0], joined[3], joined))
	assert.Equal(t, DraftStatusActive, draft.Status(), "users[2] is still available")
}

func TestTeamDraft_Cancel(t *testing.T) {
	users := newUsers(4)
	session, draft := newTestDraft(t, users, PickOrderSnake, nil)

	assert.ErrorIs(t, draft.Cancel(session, uuid.New(), MemberRoleMember, nil), ErrUnauthorized)

	require.NoError(t, draft.Cancel(session, session.CreatedByID(), "", nil))
	assert.Equal(t, DraftCancelReasonOrganizer, draft.CancelReason())
	assert.ErrorIs(t, draft.Cancel(session, session.CreatedByID(), "", nil), ErrDraftNotActive)
	assert.ErrorIs(t, draft.Pick(session, users[0], users[2], users), ErrDraftNotActive)
}

func TestTeamDraft_SessionEnded(t *testing.T) {
	users := newUsers(4)

	t.Run("cancels an active draft", func(t *testing.T) {
		session, draft := newTestDraft(t, users, PickOrderSnake, nil)
		draft.ClearEvents()
		version := draft.Version()

		draft.SessionEnded(session.CreatedByID())

		assert.Equal(t, DraftStatusCancelled, draft.Status())
		assert.Equal(t, DraftCancelReasonSessionEnded, draft.CancelReason())
		assert.Nil(t, draft.CurrentCaptainID(), "an ended draft has nobody on the clock")
		assert.Greater(t, draft.Version(), version)
		assert.Equal(t, []string{EventActivitySessionDraftCancelled}, draftEventTypes(draft))
		assert.ErrorIs(t, draft.Pick(session, users[0], users[2], users), ErrDraftNotActive)
	})

	t.Run("leaves a finished draft alone", func(t *testing.T) {
		session, draft := newTestDraft(t, users, PickOrderSnake, nil)
		require.NoError(t, draft.Cancel(session, session.CreatedByID(), "", nil))
		draft.ClearEvents()

		draft.SessionEnded(session.CreatedByID())

		assert.Equal(t, DraftCancelReasonOrganizer, draft.CancelReason())
		assert.Empty(t, draft.Events())
	})
}

func TestTeamDraft_TurnOrder(t *testing.T) {
	users := newUsers(6) // 4 players to pick
	session, draft := newTestDraft(t, users, PickOrderSnake, nil)

	assert.Equal(t, []int{0, 1, 1, 0}, sidesOf(draft.TurnOrder(4)))
	assert.Equal(t, 4, draft.TotalPicks(4))

	require.NoError(t, draft.Pick(session, users[0], users[2], users))
	assert.Equal(t, []int{0, 1, 1, 0}, sidesOf(draft.TurnOrder(3)), "done turns stay in the order")
	assert.Equal(t, 1, draft.Turn())

	// someone joins: one more turn at the end
	assert.Equal(t, []int{0, 1, 1, 0, 0}, sidesOf(draft.TurnOrder(4)))

	require.NoError(t, draft.Cancel(session, session.CreatedByID(), "", nil))
	assert.Equal(t, []int{0}, sidesOf(draft.TurnOrder(3)), "ended: only the turns taken")
}

func TestTeamDraft_VersionBumpsOnEveryChange(t *testing.T) {
	users := newUsers(6)
	session, draft := newTestDraft(t, users, PickOrderSnake, nil)
	assert.Equal(t, 1, draft.Version())

	require.NoError(t, draft.Pick(session, users[0], users[2], users))
	assert.Equal(t, 2, draft.Version())

	assert.Error(t, draft.Pick(session, users[0], users[3], users), "rejected pick")
	assert.Equal(t, 2, draft.Version(), "a rejected command changes nothing")

	require.NoError(t, draft.Cancel(session, session.CreatedByID(), "", nil))
	assert.Equal(t, 3, draft.Version())
}

func TestSession_ApplyDraftTeams(t *testing.T) {
	users := newUsers(4)
	session, draft := newTestDraft(t, users, PickOrderSnake, nil)

	assert.ErrorIs(t, session.ApplyDraftTeams(draft), ErrDraftNotActive, "only a completed draft")

	require.NoError(t, draft.Pick(session, users[0], users[2], users))
	require.NoError(t, draft.Pick(session, users[1], users[3], users))
	require.True(t, draft.IsCompleted())
	session.ClearEvents()

	require.NoError(t, session.ApplyDraftTeams(draft))

	require.Len(t, session.Teams(), 2)
	assert.Equal(t, "Team A", session.Teams()[0].Name())
	created := session.Events()[0].(*SessionTeamsCreatedEvent)
	assert.Equal(t, draft.StartedBy(), created.CreatedBy)
}

func TestTeamDraft_Reset(t *testing.T) {
	users := newUsers(4)
	organizer := uuid.New()

	for _, reason := range []DraftCancelReason{DraftCancelReasonRosterChanged, DraftCancelReasonTeamsReset} {
		t.Run("cancels an active draft: "+string(reason), func(t *testing.T) {
			_, draft := newTestDraft(t, users, PickOrderSnake, nil)
			draft.ClearEvents()
			version := draft.Version()

			draft.Reset(reason, organizer)

			assert.Equal(t, DraftStatusCancelled, draft.Status())
			assert.Equal(t, reason, draft.CancelReason())
			assert.Greater(t, draft.Version(), version)
			assert.Equal(t, []string{EventActivitySessionDraftCancelled}, draftEventTypes(draft))
			cancelled, ok := draft.Events()[0].(*DraftCancelledEvent)
			require.True(t, ok)
			assert.Equal(t, organizer, cancelled.CancelledBy)
			assert.Empty(t, cancelled.ParticipantUserIDs, "the reset event tells everyone going")
		})
	}

	t.Run("leaves a finished draft alone", func(t *testing.T) {
		session, draft := newTestDraft(t, users, PickOrderSnake, nil)
		require.NoError(t, draft.Cancel(session, session.CreatedByID(), "", nil))
		draft.ClearEvents()

		draft.Reset(DraftCancelReasonRosterChanged, organizer)

		assert.Equal(t, DraftCancelReasonOrganizer, draft.CancelReason())
		assert.Empty(t, draft.Events())
	})
}
