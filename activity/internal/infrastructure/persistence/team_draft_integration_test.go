//go:build integration

package persistence_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// draftSession creates a basketball session with n confirmed attendees, the
// first two of which will be the captains.
func (e *sessionTeamTestEnv) draftSession(t *testing.T, n int) (*domain.Session, []uuid.UUID) {
	t.Helper()

	session := e.createSession(t, domain.ActivityTypeBasketball)
	users := make([]uuid.UUID, n)
	for i := range users {
		users[i] = e.addGoingAttendee(t, session)
	}

	return session, users
}

func (e *sessionTeamTestEnv) startDraft(session *domain.Session, captainA, captainB uuid.UUID, minPlayersPerTeam *int) (*application.TeamDraftState, error) {
	return e.draftSvc.StartDraft(e.ctx, application.StartDraftParams{
		SessionID:         session.ID(),
		RequesterID:       session.CreatedByID(),
		CaptainIDs:        [2]uuid.UUID{captainA, captainB},
		PickOrder:         "snake",
		MinPlayersPerTeam: minPlayersPerTeam,
	})
}

func (e *sessionTeamTestEnv) replaceCaptain(session *domain.Session, position int, userID uuid.UUID) (*application.TeamDraftState, error) {
	return e.draftSvc.ReplaceCaptain(e.ctx, application.ReplaceDraftCaptainParams{
		SessionID:    session.ID(),
		RequesterID:  session.CreatedByID(),
		TeamPosition: position,
		UserID:       userID,
	})
}

func (e *sessionTeamTestEnv) pick(session *domain.Session, captainID, userID uuid.UUID) (*application.TeamDraftState, error) {
	return e.draftSvc.Pick(e.ctx, application.DraftPickParams{
		SessionID: session.ID(),
		CaptainID: captainID,
		UserID:    userID,
	})
}

func (e *sessionTeamTestEnv) teamOf(t *testing.T, session *domain.Session, userID uuid.UUID) *uuid.UUID {
	t.Helper()

	attendee, err := e.attendeeRepo.GetAttendeeBySessionAndUser(e.ctx, session.ID(), userID)
	require.NoError(t, err)

	return attendee.TeamID()
}

func TestTeamDraftService_SnakeDraftBecomesTheTeams(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 6) // captains u[0], u[1]

	state, err := env.startDraft(session, u[0], u[1], nil)
	require.NoError(t, err)
	assert.Equal(t, u[0], *state.Draft.CurrentCaptainID())
	assert.ElementsMatch(t, u[2:], state.Available)

	// snake: A, B, B, A
	_, err = env.pick(session, u[0], u[2])
	require.NoError(t, err)
	_, err = env.pick(session, u[1], u[3])
	require.NoError(t, err)
	_, err = env.pick(session, u[1], u[4])
	require.NoError(t, err)
	state, err = env.pick(session, u[0], u[5])
	require.NoError(t, err)
	require.True(t, state.Draft.IsCompleted())

	loaded, err := env.sessionRepo.GetSessionByID(env.ctx, session.ID())
	require.NoError(t, err)
	require.Len(t, loaded.Teams(), 2)
	teamA, teamB := loaded.Teams()[0].ID(), loaded.Teams()[1].ID()

	members, err := env.attendeeRepo.ListTeamMembers(env.ctx, session.ID())
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{u[0], u[2], u[5]}, members[teamA])
	assert.ElementsMatch(t, []uuid.UUID{u[1], u[3], u[4]}, members[teamB])

	got, err := env.draftSvc.GetDraft(env.ctx, session.ID())
	require.NoError(t, err)
	assert.Equal(t, domain.DraftStatusCompleted, got.Draft.Status())
	assert.Len(t, got.Draft.Picks(), 4)
	assert.Empty(t, got.Available)
	assert.Equal(t, 5, got.Draft.Version(), "start + 4 picks, read back from the database")
}

func (e *sessionTeamTestEnv) teamIDAt(t *testing.T, session *domain.Session, position int) uuid.UUID {
	t.Helper()

	loaded, err := e.sessionRepo.GetSessionByID(e.ctx, session.ID())
	require.NoError(t, err)
	require.Greater(t, len(loaded.Teams()), position)

	return loaded.Teams()[position].ID()
}

func TestTeamDraftService_StartNeedsEnoughPeople(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 5)

	_, err := env.startDraft(session, u[0], u[1], testutil.Ptr(3)) // needs 6
	assert.ErrorIs(t, err, domain.ErrNotEnoughPlayers)
}

func TestTeamDraftService_CancelLeavesTeamsUntouched(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 4)

	withTeams, err := env.createTeams(session, nil, nil, nil)
	require.NoError(t, err)
	oldTeam := withTeams.Teams()[0].ID()
	require.NoError(t, env.assign(session, u[2], oldTeam))

	_, err = env.startDraft(session, u[0], u[1], nil)
	require.NoError(t, err)
	_, err = env.pick(session, u[0], u[3])
	require.NoError(t, err)

	state, err := env.draftSvc.CancelDraft(env.ctx, application.CancelDraftParams{
		SessionID:   session.ID(),
		RequesterID: session.CreatedByID(),
	})
	require.NoError(t, err)
	assert.Equal(t, domain.DraftCancelReasonOrganizer, state.Draft.CancelReason())

	assert.Equal(t, &oldTeam, env.teamOf(t, session, u[2]), "cancel keeps the old assignment")
	assert.Nil(t, env.teamOf(t, session, u[3]), "a draft pick never reached the teams")

	// with the draft over, manual assignment works again
	assert.NoError(t, env.assign(session, u[3], oldTeam))
}

func TestTeamDraftService_BlocksManualTeamChangesAndSecondDraft(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 4)

	withTeams, err := env.createTeams(session, nil, nil, nil)
	require.NoError(t, err)

	_, err = env.startDraft(session, u[0], u[1], nil)
	require.NoError(t, err)

	_, err = env.startDraft(session, u[2], u[3], nil)
	assert.ErrorIs(t, err, domain.ErrDraftAlreadyActive, "one draft at a time")

	_, err = env.createTeams(session, nil, nil, nil)
	assert.ErrorIs(t, err, domain.ErrDraftAlreadyActive)

	assert.ErrorIs(t, env.assign(session, u[2], withTeams.Teams()[0].ID()), domain.ErrDraftAlreadyActive)
}

func TestTeamDraftService_ReplacesExistingTeams(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 3)

	withTeams, err := env.createTeams(session, testutil.Ptr(4), nil, nil)
	require.NoError(t, err)
	oldTeam := withTeams.Teams()[3].ID()
	require.NoError(t, env.assign(session, u[2], oldTeam))

	_, err = env.startDraft(session, u[0], u[1], nil)
	require.NoError(t, err)
	state, err := env.pick(session, u[0], u[2])
	require.NoError(t, err)
	require.True(t, state.Draft.IsCompleted())

	loaded, err := env.sessionRepo.GetSessionByID(env.ctx, session.ID())
	require.NoError(t, err)
	require.Len(t, loaded.Teams(), 2, "4 old teams replaced by the 2 drafted ones")
	assert.Equal(t, loaded.Teams()[0].ID(), *env.teamOf(t, session, u[2]))
}

func TestTeamDraftService_AttendanceChangesMidDraft(t *testing.T) {
	t.Run("captain going not_going pauses; organizer replaces them and it continues (D8)", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session, u := env.draftSession(t, 5)
		_, err := env.startDraft(session, u[0], u[1], nil)
		require.NoError(t, err)
		_, err = env.pick(session, u[0], u[2])
		require.NoError(t, err)

		_, err = env.svc.UpdateRSVP(env.ctx, application.UpdateRSVPParams{
			SessionID: session.ID(),
			UserID:    u[1],
			NewStatus: string(domain.AttendeeStatusNotGoing),
		})
		require.NoError(t, err)

		got, err := env.draftSvc.GetDraft(env.ctx, session.ID())
		require.NoError(t, err)
		require.Equal(t, domain.DraftStatusPaused, got.Draft.Status())
		assert.Equal(t, domain.DraftPauseReasonCaptainLeft, got.Draft.PausedReason())
		assert.Equal(t, uuid.Nil, got.Draft.Captains()[domain.DraftSideB], "vacant captain read back from the database")

		_, err = env.pick(session, u[1], u[3])
		assert.ErrorIs(t, err, domain.ErrDraftPaused)

		// a paused draft still blocks manual team changes
		_, err = env.createTeams(session, nil, nil, nil)
		assert.ErrorIs(t, err, domain.ErrDraftAlreadyActive)

		state, err := env.replaceCaptain(session, 1, u[3])
		require.NoError(t, err)
		assert.Equal(t, domain.DraftStatusActive, state.Draft.Status())
		assert.Equal(t, u[3], *state.Draft.CurrentCaptainID(), "B's turn, with the new captain")

		state, err = env.pick(session, u[3], u[4])
		require.NoError(t, err)
		require.True(t, state.Draft.IsCompleted())
		teamB := env.teamIDAt(t, session, 1)
		assert.Equal(t, &teamB, env.teamOf(t, session, u[3]), "the replacement captain ends up on Team B")
	})

	t.Run("falling below the minimum cancels the draft (D7)", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session, u := env.draftSession(t, 4) // min 2 per team: needs 4
		_, err := env.startDraft(session, u[0], u[1], testutil.Ptr(2))
		require.NoError(t, err)

		require.NoError(t, env.svc.CancelRSVP(env.ctx, session.ID(), u[3]))

		got, err := env.draftSvc.GetDraft(env.ctx, session.ID())
		require.NoError(t, err)
		assert.Equal(t, domain.DraftStatusCancelled, got.Draft.Status())
		assert.Equal(t, domain.DraftCancelReasonNotEnoughPlayers, got.Draft.CancelReason())
	})

	t.Run("picked player cancelling is dropped", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session, u := env.draftSession(t, 5)
		_, err := env.startDraft(session, u[0], u[1], nil)
		require.NoError(t, err)
		_, err = env.pick(session, u[0], u[2])
		require.NoError(t, err)

		require.NoError(t, env.svc.CancelRSVP(env.ctx, session.ID(), u[2]))

		got, err := env.draftSvc.GetDraft(env.ctx, session.ID())
		require.NoError(t, err)
		assert.True(t, got.Draft.IsRunning())
		assert.Equal(t, []uuid.UUID{u[0]}, got.Draft.Roster(domain.DraftSideA))
		assert.ElementsMatch(t, []uuid.UUID{u[3], u[4]}, got.Available)
	})

	t.Run("last available player removed completes the draft", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session, u := env.draftSession(t, 4)
		_, err := env.startDraft(session, u[0], u[1], nil)
		require.NoError(t, err)
		_, err = env.pick(session, u[0], u[2])
		require.NoError(t, err)

		require.NoError(t, env.svc.RemoveAttendee(env.ctx, application.RemoveAttendeeParams{
			SessionID:   session.ID(),
			UserID:      u[3],
			RequesterID: session.CreatedByID(),
		}))

		got, err := env.draftSvc.GetDraft(env.ctx, session.ID())
		require.NoError(t, err)
		require.True(t, got.Draft.IsCompleted())

		loaded, err := env.sessionRepo.GetSessionByID(env.ctx, session.ID())
		require.NoError(t, err)
		require.Len(t, loaded.Teams(), 2)
		assert.Equal(t, loaded.Teams()[0].ID(), *env.teamOf(t, session, u[2]))
		assert.Equal(t, loaded.Teams()[1].ID(), *env.teamOf(t, session, u[1]))
	})

	t.Run("newly confirmed attendee joins the pool", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session, u := env.draftSession(t, 3)
		_, err := env.startDraft(session, u[0], u[1], nil)
		require.NoError(t, err)

		late := env.addGoingAttendee(t, session)

		got, err := env.draftSvc.GetDraft(env.ctx, session.ID())
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{u[2], late}, got.Available)
	})
}

// TestTeamDraftService_ConcurrentPicks_OnlyOneWins fires two picks for the
// same turn at once. Without the session lock both could read the same turn
// and both commit, giving captain A two picks in a row.
func TestTeamDraftService_ConcurrentPicks_OnlyOneWins(t *testing.T) {
	env := setupSessionTeamTest(t)

	const attempts = 10
	for i := range attempts {
		session, u := env.draftSession(t, 6)
		_, err := env.startDraft(session, u[0], u[1], nil)
		require.NoError(t, err)

		var wg sync.WaitGroup
		errs := make([]error, 2)
		for idx, player := range []uuid.UUID{u[2], u[3]} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[idx] = env.pick(session, u[0], player)
			}()
		}
		wg.Wait()

		succeeded := 0
		for _, err := range errs {
			if err == nil {
				succeeded++
				continue
			}
			require.Truef(t, errors.Is(err, domain.ErrNotYourTurn), "unexpected error on attempt %d: %v", i, err)
		}
		require.Equalf(t, 1, succeeded, "exactly one pick should win on attempt %d", i)

		got, err := env.draftSvc.GetDraft(env.ctx, session.ID())
		require.NoError(t, err)
		require.Lenf(t, got.Draft.Picks(), 1, "attempt %d recorded %d picks", i, len(got.Draft.Picks()))
		require.Equal(t, u[1], *got.Draft.CurrentCaptainID())
	}
}
