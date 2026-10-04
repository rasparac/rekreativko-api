//go:build integration

package persistence_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (e *sessionTeamTestEnv) resetTeams(session *domain.Session, requester uuid.UUID, role string) error {
	return e.sessionSvc.ResetTeams(e.ctx, application.ResetTeamsParams{
		SessionID:     session.ID(),
		RequesterID:   requester,
		RequesterRole: role,
	})
}

// requireInitialFormation checks the session is back to "like there was never
// any draft": no teams or team setup, nobody assigned, no draft and no voting
// round to report - and so proposals are allowed again.
func (e *sessionTeamTestEnv) requireInitialFormation(t *testing.T, session *domain.Session, going []uuid.UUID) {
	t.Helper()

	loaded, err := e.sessionRepo.GetSessionByID(e.ctx, session.ID())
	require.NoError(t, err)
	assert.False(t, loaded.HasTeams())
	assert.Nil(t, loaded.TeamConfig())

	for _, userID := range going {
		assert.Nil(t, e.teamOf(t, session, userID), "everyone is unassigned")
	}

	snapshot, err := e.formation.Snapshot(e.ctx, session.ID())
	require.NoError(t, err)
	assert.Nil(t, snapshot.Draft, "GET /draft has nothing to report")
	assert.Nil(t, snapshot.Voting.Round, "no voting round either")
	assert.ElementsMatch(t, going, snapshot.GoingUserIDs)

	// proposals are allowed again
	a, b := divide(going)
	_, err = e.propose(session, going[0], a, b)
	require.NoError(t, err)
}

// divide splits everyone going into two teams, for a proposal.
func divide(going []uuid.UUID) ([]uuid.UUID, []uuid.UUID) {
	half := (len(going) + 1) / 2
	return going[:half], going[half:]
}

// finishDraft has the captains pick everyone else, whoever's turn it is.
func (e *sessionTeamTestEnv) finishDraft(t *testing.T, session *domain.Session, u []uuid.UUID) {
	t.Helper()

	state, err := e.startDraft(session, u[0], u[1], nil)
	require.NoError(t, err)

	for _, player := range u[2:] {
		require.False(t, state.Draft.IsCompleted())
		state, err = e.pick(session, *state.Draft.CurrentCaptainID(), player)
		require.NoError(t, err)
	}
	require.True(t, state.Draft.IsCompleted())
}

// formationScenarios build each state team formation can be in.
var formationScenarios = map[string]func(t *testing.T, env *sessionTeamTestEnv, session *domain.Session, u []uuid.UUID){
	"teams assigned by hand": func(t *testing.T, env *sessionTeamTestEnv, session *domain.Session, u []uuid.UUID) {
		withTeams, err := env.createTeams(session, nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, env.assign(session, u[0], withTeams.Teams()[0].ID()))
		require.NoError(t, env.assign(session, u[1], withTeams.Teams()[1].ID()))
	},
	"a running captain draft": func(t *testing.T, env *sessionTeamTestEnv, session *domain.Session, u []uuid.UUID) {
		_, err := env.startDraft(session, u[0], u[1], nil)
		require.NoError(t, err)
		_, err = env.pick(session, u[0], u[2])
		require.NoError(t, err)
	},
	"a finished captain draft": func(t *testing.T, env *sessionTeamTestEnv, session *domain.Session, u []uuid.UUID) {
		env.finishDraft(t, session, u)
	},
	"an open voting round": func(t *testing.T, env *sessionTeamTestEnv, session *domain.Session, u []uuid.UUID) {
		a, b := divide(u)
		_, err := env.propose(session, u[0], a, b)
		require.NoError(t, err)
	},
	"a finished vote": func(t *testing.T, env *sessionTeamTestEnv, session *domain.Session, u []uuid.UUID) {
		a, b := divide(u)
		state, err := env.propose(session, u[0], a, b)
		require.NoError(t, err)
		require.NoError(t, env.vote(session, u[0], state.Round.Proposals()[0].ID()))
		_, err = env.closeVoting(session, nil)
		require.NoError(t, err)
	},
}

// Whoever stops holding a spot restarts team formation - a team member or not,
// a captain or not - however they stop.
func TestTeamReset_AnyConfirmedAttendeeLeavingResetsEverything(t *testing.T) {
	leavers := map[string]func(env *sessionTeamTestEnv, session *domain.Session, userID uuid.UUID) error{
		"cancels their RSVP": func(env *sessionTeamTestEnv, session *domain.Session, userID uuid.UUID) error {
			return env.svc.CancelRSVP(env.ctx, session.ID(), userID)
		},
		"switches to maybe": func(env *sessionTeamTestEnv, session *domain.Session, userID uuid.UUID) error {
			_, err := env.svc.UpdateRSVP(env.ctx, application.UpdateRSVPParams{
				SessionID: session.ID(), UserID: userID, NewStatus: string(domain.AttendeeStatusMaybe),
			})
			return err
		},
		"switches to not going": func(env *sessionTeamTestEnv, session *domain.Session, userID uuid.UUID) error {
			_, err := env.svc.UpdateRSVP(env.ctx, application.UpdateRSVPParams{
				SessionID: session.ID(), UserID: userID, NewStatus: string(domain.AttendeeStatusNotGoing),
			})
			return err
		},
		"is removed by a manager": func(env *sessionTeamTestEnv, session *domain.Session, userID uuid.UUID) error {
			return env.svc.RemoveAttendee(env.ctx, application.RemoveAttendeeParams{
				SessionID: session.ID(), UserID: userID, RequesterID: session.CreatedByID(),
			})
		},
	}

	for scenario, setup := range formationScenarios {
		for leaving, leave := range leavers {
			t.Run(scenario+"/"+leaving, func(t *testing.T) {
				env := setupSessionTeamTest(t)
				session, u := env.draftSession(t, 5)
				setup(t, env, session, u)

				// u[4] is on no team and picked by nobody: any leaver resets.
				require.NoError(t, leave(env, session, u[4]))

				env.requireInitialFormation(t, session, u[:4])
				assert.Equal(t, 1, env.outboxCount(t, "activity.session.teams_reset"))
			})
		}
	}
}

func TestTeamReset_ACaptainLeavingCancelsTheDraftToo(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 5)
	_, err := env.startDraft(session, u[0], u[1], nil)
	require.NoError(t, err)

	require.NoError(t, env.svc.CancelRSVP(env.ctx, session.ID(), u[1]))

	env.requireInitialFormation(t, session, []uuid.UUID{u[0], u[2], u[3], u[4]})
	assert.Equal(t, 1, env.outboxCount(t, "activity.session.draft.cancelled"))
	assert.Equal(t, 1, env.outboxCount(t, "activity.session.teams_reset"))
}

func TestTeamReset_LeavingWithNothingToResetRaisesNothing(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 3)

	require.NoError(t, env.svc.CancelRSVP(env.ctx, session.ID(), u[2]))

	assert.Zero(t, env.outboxCount(t, "activity.session.teams_reset"))
}

func TestTeamReset_AJoinerDoesNotResetAnything(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 4)
	env.finishDraft(t, session, u)

	late := env.addGoingAttendee(t, session)

	loaded, err := env.sessionRepo.GetSessionByID(env.ctx, session.ID())
	require.NoError(t, err)
	assert.True(t, loaded.HasTeams())
	assert.Nil(t, env.teamOf(t, session, late), "they stay unassigned for now")
	assert.Zero(t, env.outboxCount(t, "activity.session.teams_reset"))
}

func TestTeamReset_OrganizerCanResetByHand(t *testing.T) {
	for scenario, setup := range formationScenarios {
		t.Run(scenario, func(t *testing.T) {
			env := setupSessionTeamTest(t)
			session, u := env.draftSession(t, 4)
			setup(t, env, session, u)

			require.NoError(t, env.resetTeams(session, session.CreatedByID(), ""))

			env.requireInitialFormation(t, session, u)
			assert.Equal(t, 1, env.outboxCount(t, "activity.session.teams_reset"))
		})
	}

	t.Run("a draft and a vote it cancels say so", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session, u := env.draftSession(t, 4)
		formationScenarios["a running captain draft"](t, env, session, u)

		require.NoError(t, env.resetTeams(session, session.CreatedByID(), ""))

		assert.Equal(t, 1, env.outboxCount(t, "activity.session.draft.cancelled"))
	})

	t.Run("an admin may too", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session, u := env.draftSession(t, 4)
		formationScenarios["teams assigned by hand"](t, env, session, u)

		require.NoError(t, env.resetTeams(session, uuid.New(), "admin"))

		env.requireInitialFormation(t, session, u)
	})

	t.Run("someone else may not", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session, u := env.draftSession(t, 4)
		formationScenarios["teams assigned by hand"](t, env, session, u)

		err := env.resetTeams(session, u[0], "member")

		assert.ErrorIs(t, err, domain.ErrUnauthorized)
		loaded, err := env.sessionRepo.GetSessionByID(env.ctx, session.ID())
		require.NoError(t, err)
		assert.True(t, loaded.HasTeams(), "nothing changed")
	})

	t.Run("resetting nothing is fine and silent", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session, _ := env.draftSession(t, 4)

		require.NoError(t, env.resetTeams(session, session.CreatedByID(), ""))
		require.NoError(t, env.resetTeams(session, session.CreatedByID(), ""))

		assert.Zero(t, env.outboxCount(t, "activity.session.teams_reset"))
	})

	t.Run("not once the session is over", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session, u := env.draftSession(t, 4)
		formationScenarios["teams assigned by hand"](t, env, session, u)
		require.NoError(t, env.sessionSvc.StartSession(env.ctx, session.ID(), session.CreatedByID(), ""))
		require.NoError(t, env.sessionSvc.CompleteSession(env.ctx, session.ID(), session.CreatedByID(), ""))

		err := env.resetTeams(session, session.CreatedByID(), "")

		assert.ErrorIs(t, err, domain.ErrSessionCompleted)
	})
}

func (e *sessionTeamTestEnv) teamsSource(t *testing.T, session *domain.Session) string {
	t.Helper()

	snapshot, err := e.formation.Snapshot(e.ctx, session.ID())
	require.NoError(t, err)
	return string(snapshot.Session.TeamsSource())
}

// Picked teams are not put to a vote, and which teams are current is recorded,
// not guessed from the latest draft.
func TestTeamsSource_ProposalsAreRefusedForDraftedTeams(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 4)
	assert.Empty(t, env.teamsSource(t, session), "no teams yet")

	env.finishDraft(t, session, u)
	assert.Equal(t, "draft", env.teamsSource(t, session), "read back from the database")

	a, b := divide(u)
	_, err := env.propose(session, u[0], a, b)
	assert.ErrorIs(t, err, domain.ErrTeamsDrafted)

	t.Run("still refused after a re-draft is cancelled", func(t *testing.T) {
		_, err := env.startDraft(session, u[2], u[3], nil)
		require.NoError(t, err)
		_, err = env.draftSvc.CancelDraft(env.ctx, application.CancelDraftParams{
			SessionID: session.ID(), RequesterID: session.CreatedByID(),
		})
		require.NoError(t, err)

		assert.Equal(t, "draft", env.teamsSource(t, session), "the drafted teams are still the current ones")
		_, err = env.propose(session, u[0], a, b)
		assert.ErrorIs(t, err, domain.ErrTeamsDrafted)
	})

	t.Run("allowed once the organizer replaces the teams by hand", func(t *testing.T) {
		_, err := env.createTeams(session, nil, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, "manual", env.teamsSource(t, session))

		_, err = env.propose(session, u[0], a, b)
		require.NoError(t, err)
	})

	t.Run("a vote winner makes the source vote, and proposals stay allowed", func(t *testing.T) {
		state := env.voting(t, session)
		require.True(t, state.Round.IsOpen())
		require.NoError(t, env.vote(session, u[0], state.Round.Proposals()[0].ID()))
		_, err := env.closeVoting(session, nil)
		require.NoError(t, err)
		assert.Equal(t, "vote", env.teamsSource(t, session))

		_, err = env.propose(session, u[1], a, b)
		require.NoError(t, err)
	})
}

func TestTeamsSource_AResetClearsIt(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 4)
	env.finishDraft(t, session, u)
	require.Equal(t, "draft", env.teamsSource(t, session))

	require.NoError(t, env.resetTeams(session, session.CreatedByID(), ""))

	assert.Empty(t, env.teamsSource(t, session))
	env.requireInitialFormation(t, session, u)
}
