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

func remove(users []uuid.UUID, gone uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(users))
	for _, u := range users {
		if u != gone {
			out = append(out, u)
		}
	}
	return out
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

func TestTeamDraft_AttendeeLeft(t *testing.T) {
	t.Run("captain leaving pauses the draft (D8)", func(t *testing.T) {
		users := newUsers(6)
		session, draft := newTestDraft(t, users, PickOrderSnake, nil)
		draft.ClearEvents()
		going := remove(users, users[0])

		draft.AttendeeLeft(users[0], going)

		assert.Equal(t, DraftStatusPaused, draft.Status())
		assert.Equal(t, DraftPauseReasonCaptainLeft, draft.PausedReason())
		assert.Equal(t, uuid.Nil, draft.Captains()[DraftSideA], "Team A's captain is vacant")
		assert.Nil(t, draft.CurrentCaptainID(), "it was A's turn, and A has no captain")
		assert.True(t, draft.IsRunning())
		assert.Equal(t, []string{EventActivitySessionDraftPaused}, draftEventTypes(draft))
		assert.ErrorIs(t, draft.Pick(session, users[1], users[2], going), ErrDraftPaused)
	})

	t.Run("picked player leaving is dropped", func(t *testing.T) {
		users := newUsers(5)
		session, draft := newTestDraft(t, users, PickOrderSnake, nil)
		require.NoError(t, draft.Pick(session, users[0], users[2], users))
		draft.ClearEvents()

		draft.AttendeeLeft(users[2], remove(users, users[2]))

		assert.Equal(t, DraftStatusActive, draft.Status())
		assert.Equal(t, []uuid.UUID{users[0]}, draft.Roster(DraftSideA))
		assert.Equal(t, []string{EventActivitySessionDraftPlayerDropped}, draftEventTypes(draft))
	})

	t.Run("last available player leaving completes", func(t *testing.T) {
		users := newUsers(4)
		session, draft := newTestDraft(t, users, PickOrderSnake, nil)
		require.NoError(t, draft.Pick(session, users[0], users[2], users))

		draft.AttendeeLeft(users[3], users[:3])

		assert.True(t, draft.IsCompleted())
	})

	t.Run("falling below the minimum cancels (D7)", func(t *testing.T) {
		users := newUsers(6) // min 3 per team: needs 6
		_, draft := newTestDraft(t, users, PickOrderSnake, intPtr(3))
		draft.ClearEvents()

		draft.AttendeeLeft(users[5], users[:5])

		assert.Equal(t, DraftStatusCancelled, draft.Status())
		assert.Equal(t, DraftCancelReasonNotEnoughPlayers, draft.CancelReason())
		assert.Equal(t, []string{EventActivitySessionDraftCancelled}, draftEventTypes(draft))
	})

	t.Run("new attendee joins the pool", func(t *testing.T) {
		users := newUsers(3)
		session, draft := newTestDraft(t, users, PickOrderSnake, nil)
		joined := append(users, uuid.New())

		require.NoError(t, draft.Pick(session, users[0], joined[3], joined))
		assert.Equal(t, DraftStatusActive, draft.Status(), "users[2] is still available")
	})

	t.Run("ignored once the draft ended", func(t *testing.T) {
		users := newUsers(4)
		session, draft := newTestDraft(t, users, PickOrderSnake, nil)
		require.NoError(t, draft.Cancel(session, session.CreatedByID(), ""))
		draft.ClearEvents()

		draft.AttendeeLeft(users[0], users[1:])

		assert.Empty(t, draft.Events())
	})
}

func TestTeamDraft_ReplaceCaptain(t *testing.T) {
	// users: 0 = captain A, 1 = captain B, 2 picked by A, 3..5 available.
	// Captain A then leaves, pausing the draft.
	setup := func(t *testing.T) (*Session, *TeamDraft, []uuid.UUID, []uuid.UUID) {
		t.Helper()
		users := newUsers(6)
		session, draft := newTestDraft(t, users, PickOrderSnake, nil)
		require.NoError(t, draft.Pick(session, users[0], users[2], users))
		going := remove(users, users[0])
		draft.AttendeeLeft(users[0], going)
		require.Equal(t, DraftStatusPaused, draft.Status())
		draft.ClearEvents()
		return session, draft, users, going
	}

	t.Run("from the team resumes where it stopped", func(t *testing.T) {
		session, draft, users, going := setup(t)

		require.NoError(t, draft.ReplaceCaptain(session, session.CreatedByID(), "", DraftSideA, users[2], going))

		assert.Equal(t, DraftStatusActive, draft.Status())
		assert.Empty(t, draft.PausedReason())
		assert.Equal(t, []uuid.UUID{users[2]}, draft.Roster(DraftSideA), "promoted, not listed twice")
		assert.Empty(t, draft.Picks())
		assert.Equal(t, users[1], *draft.CurrentCaptainID(), "turn 1 is B's")
		assert.Equal(t, []string{EventActivitySessionDraftCaptainReplaced, EventActivitySessionDraftTurnChanged}, draftEventTypes(draft))
	})

	t.Run("from the pool", func(t *testing.T) {
		session, draft, users, going := setup(t)

		require.NoError(t, draft.ReplaceCaptain(session, session.CreatedByID(), "", DraftSideA, users[4], going))

		assert.Equal(t, []uuid.UUID{users[4], users[2]}, draft.Roster(DraftSideA))
		assert.NotContains(t, draft.Available(going), users[4])
	})

	rejections := []struct {
		name    string
		side    DraftSide
		user    func(users []uuid.UUID) uuid.UUID
		wantErr error
	}{
		{"team B still has its captain", DraftSideB, func(u []uuid.UUID) uuid.UUID { return u[3] }, ErrInvalidDraftCaptains},
		{"the other captain", DraftSideA, func(u []uuid.UUID) uuid.UUID { return u[1] }, ErrInvalidDraftCaptains},
		{"not going", DraftSideA, func(u []uuid.UUID) uuid.UUID { return u[0] }, ErrDraftCaptainNotConfirmed},
		{"invalid position", DraftSide(2), func(u []uuid.UUID) uuid.UUID { return u[3] }, ErrInvalidDraftCaptains},
	}
	for _, tt := range rejections {
		t.Run(tt.name, func(t *testing.T) {
			session, draft, users, going := setup(t)
			err := draft.ReplaceCaptain(session, session.CreatedByID(), "", tt.side, tt.user(users), going)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}

	t.Run("player from the other team", func(t *testing.T) {
		users := newUsers(6)
		session, draft := newTestDraft(t, users, PickOrderSnake, nil)
		require.NoError(t, draft.Pick(session, users[0], users[2], users))
		require.NoError(t, draft.Pick(session, users[1], users[3], users)) // on Team B
		going := remove(users, users[0])
		draft.AttendeeLeft(users[0], going)

		err := draft.ReplaceCaptain(session, session.CreatedByID(), "", DraftSideA, users[3], going)
		assert.ErrorIs(t, err, ErrInvalidDraftCaptains)
	})

	t.Run("regular member", func(t *testing.T) {
		session, draft, users, going := setup(t)
		err := draft.ReplaceCaptain(session, uuid.New(), MemberRoleMember, DraftSideA, users[2], going)
		assert.ErrorIs(t, err, ErrUnauthorized)
	})

	t.Run("not paused", func(t *testing.T) {
		users := newUsers(5)
		session, draft := newTestDraft(t, users, PickOrderSnake, nil)
		err := draft.ReplaceCaptain(session, session.CreatedByID(), "", DraftSideA, users[2], users)
		assert.ErrorIs(t, err, ErrDraftNotPaused)
	})

	t.Run("both captains left: resumes after the second replacement", func(t *testing.T) {
		users := newUsers(7)
		session, draft := newTestDraft(t, users, PickOrderSnake, nil)
		going := remove(remove(users, users[0]), users[1])
		draft.AttendeeLeft(users[0], remove(users, users[0]))
		draft.AttendeeLeft(users[1], going)

		require.NoError(t, draft.ReplaceCaptain(session, session.CreatedByID(), "", DraftSideA, users[2], going))
		assert.Equal(t, DraftStatusPaused, draft.Status(), "Team B still has no captain")

		require.NoError(t, draft.ReplaceCaptain(session, session.CreatedByID(), "", DraftSideB, users[3], going))
		assert.Equal(t, DraftStatusActive, draft.Status())
	})

	t.Run("resuming with nobody left to pick completes", func(t *testing.T) {
		users := newUsers(4)
		session, draft := newTestDraft(t, users, PickOrderSnake, nil)
		require.NoError(t, draft.Pick(session, users[0], users[2], users))
		going := remove(users, users[1])
		draft.AttendeeLeft(users[1], going) // Team B's captain leaves; users[3] is the only one available

		require.NoError(t, draft.ReplaceCaptain(session, session.CreatedByID(), "", DraftSideB, users[3], going))

		assert.True(t, draft.IsCompleted())
	})
}

func TestTeamDraft_Cancel(t *testing.T) {
	users := newUsers(4)
	session, draft := newTestDraft(t, users, PickOrderSnake, nil)

	assert.ErrorIs(t, draft.Cancel(session, uuid.New(), MemberRoleMember), ErrUnauthorized)

	require.NoError(t, draft.Cancel(session, session.CreatedByID(), ""))
	assert.Equal(t, DraftCancelReasonOrganizer, draft.CancelReason())
	assert.ErrorIs(t, draft.Cancel(session, session.CreatedByID(), ""), ErrDraftNotActive)
	assert.ErrorIs(t, draft.Pick(session, users[0], users[2], users), ErrDraftNotActive)

	t.Run("works while paused", func(t *testing.T) {
		session, draft := newTestDraft(t, users, PickOrderSnake, nil)
		draft.AttendeeLeft(users[0], users[1:])
		require.Equal(t, DraftStatusPaused, draft.Status())

		require.NoError(t, draft.Cancel(session, session.CreatedByID(), ""))
		assert.Equal(t, DraftStatusCancelled, draft.Status())
		assert.Empty(t, draft.PausedReason())
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

	require.NoError(t, draft.Cancel(session, session.CreatedByID(), ""))
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

	draft.AttendeeLeft(users[5], remove(users, users[5]))
	assert.Equal(t, 3, draft.Version())

	going := remove(remove(users, users[5]), users[0])
	draft.AttendeeLeft(users[0], going) // pause
	assert.Equal(t, 4, draft.Version())

	require.NoError(t, draft.ReplaceCaptain(session, session.CreatedByID(), "", DraftSideA, users[2], going))
	assert.Equal(t, 5, draft.Version())

	require.NoError(t, draft.Cancel(session, session.CreatedByID(), ""))
	assert.Equal(t, 6, draft.Version())
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
