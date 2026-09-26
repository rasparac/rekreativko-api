//go:build integration

package persistence_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sessionTeamTestEnv struct {
	ctx          context.Context
	svc          *application.AttendeeService
	sessionSvc   *application.SessionService
	sessionRepo  application.SessionRepository
	attendeeRepo application.AttendeeRepository
}

func setupSessionTeamTest(t *testing.T) *sessionTeamTestEnv {
	t.Helper()

	db := setupCapacityRaceTestDB(t)
	txManager := db.CreateTransactionManager()
	logger := testutil.CreateLogger()

	sessionRepo := persistence.NewSessionManager(txManager, logger)
	attendeeRepo := persistence.NewAttendeeRepository(txManager, logger)
	memberRepo := persistence.NewMemberRepository(txManager, logger)
	groupRepo := persistence.NewActivityGroupRepository(txManager, logger)
	eventWriter := domainevent.NewDomainEventManager(txManager)

	return &sessionTeamTestEnv{
		ctx:          context.Background(),
		svc:          application.NewAttendeeService(logger, txManager, attendeeRepo, memberRepo, sessionRepo, eventWriter, testMetrics()),
		sessionSvc:   application.NewSessionService(logger, txManager, sessionRepo, memberRepo, groupRepo, attendeeRepo, eventWriter, testMetrics()),
		sessionRepo:  sessionRepo,
		attendeeRepo: attendeeRepo,
	}
}

// createSession creates a standalone, public session with no teams yet.
func (e *sessionTeamTestEnv) createSession(t *testing.T, activityType domain.ActivityType) *domain.Session {
	t.Helper()

	title, err := domain.NewTitle("Pickup Game")
	require.NoError(t, err)

	location, err := domain.NewSessionLocation("Belgrade", "RS", "", 44.8, 20.4)
	require.NoError(t, err)

	schedule, err := domain.NewSessionSchedule(time.Now().Add(time.Hour), nil)
	require.NoError(t, err)

	visibility := domain.SessionVisibilityPublic
	session, _, err := domain.NewSession(domain.SessionInput{
		CreatedByID:     uuid.New(),
		Title:           title,
		ActivityType:    activityType,
		DifficultyLevel: domain.DifficultyLevelBeginner,
		Location:        location,
		Schedule:        schedule,
		Visibility:      &visibility,
	})
	require.NoError(t, err)

	require.NoError(t, e.sessionRepo.CreateSession(e.ctx, session))

	return session
}

func (e *sessionTeamTestEnv) createTeams(session *domain.Session, teamCount, playersPerTeam *int, colors []string) (*domain.Session, error) {
	return e.sessionSvc.CreateTeams(e.ctx, application.CreateTeamsParams{
		SessionID:      session.ID(),
		RequesterID:    session.CreatedByID(), // standalone session: creator manages it, no group role
		TeamCount:      teamCount,
		PlayersPerTeam: playersPerTeam,
		Colors:         colors,
	})
}

// createTeamSession creates a basketball session and splits it into two teams
// through the service, the way the creator does once people have joined.
func (e *sessionTeamTestEnv) createTeamSession(t *testing.T, playersPerTeam *int, colors []string) *domain.Session {
	t.Helper()

	session := e.createSession(t, domain.ActivityTypeBasketball)

	withTeams, err := e.createTeams(session, nil, playersPerTeam, colors)
	require.NoError(t, err)

	return withTeams
}

func (e *sessionTeamTestEnv) addGoingAttendee(t *testing.T, session *domain.Session) uuid.UUID {
	t.Helper()

	userID := uuid.New()
	attendee, err := domain.NewRSVPManualAttendee(session, session.ActivityGroupID(), userID, domain.AttendeeStatusGoing, 0)
	require.NoError(t, err)
	require.NoError(t, e.attendeeRepo.CreateAttendee(e.ctx, attendee))

	return userID
}

func (e *sessionTeamTestEnv) assign(session *domain.Session, userID, teamID uuid.UUID) error {
	_, err := e.svc.AssignAttendeeTeam(e.ctx, application.AssignAttendeeTeamParams{
		SessionID:   session.ID(),
		UserID:      userID,
		TeamID:      teamID,
		RequesterID: session.CreatedByID(), // standalone session: creator manages it, no group role
	})
	return err
}

func TestSessionRepository_TeamsRoundTrip(t *testing.T) {
	env := setupSessionTeamTest(t)
	session := env.createTeamSession(t, testutil.Ptr(5), []string{"#FFFFFF", "#000000"})

	loaded, err := env.sessionRepo.GetSessionByID(env.ctx, session.ID())
	require.NoError(t, err)

	require.True(t, loaded.HasTeams())
	assert.Equal(t, 2, loaded.TeamConfig().TeamCount())
	assert.Equal(t, testutil.Ptr(5), loaded.TeamConfig().PlayersPerTeam())
	require.Len(t, loaded.Teams(), 2)
	assert.Equal(t, session.Teams()[0].ID(), loaded.Teams()[0].ID())
	assert.Equal(t, "Team A", loaded.Teams()[0].Name())
	assert.Equal(t, "#FFFFFF", loaded.Teams()[0].Color())
	assert.Equal(t, "Team B", loaded.Teams()[1].Name())
	assert.Equal(t, "#000000", loaded.Teams()[1].Color())

	// list queries carry the config but not the teams
	listed, _, err := env.sessionRepo.ListSessions(env.ctx, persistence.SessionFilter{
		CreatedByID: testutil.Ptr(session.CreatedByID()),
		RequesterID: session.CreatedByID(),
	})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.NotNil(t, listed[0].TeamConfig())
	assert.Equal(t, 2, listed[0].TeamConfig().TeamCount())
	assert.Empty(t, listed[0].Teams())
}

func TestAttendeeService_AssignTeam_PersistsAndListsMembers(t *testing.T) {
	env := setupSessionTeamTest(t)
	session := env.createTeamSession(t, nil, nil)
	teamA, teamB := session.Teams()[0].ID(), session.Teams()[1].ID()

	alice := env.addGoingAttendee(t, session)
	bob := env.addGoingAttendee(t, session)

	require.NoError(t, env.assign(session, alice, teamA))
	require.NoError(t, env.assign(session, bob, teamA))
	// move bob to team B
	require.NoError(t, env.assign(session, bob, teamB))

	members, err := env.attendeeRepo.ListTeamMembers(env.ctx, session.ID())
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{alice}, members[teamA])
	assert.Equal(t, []uuid.UUID{bob}, members[teamB])

	attendee, err := env.attendeeRepo.GetAttendeeBySessionAndUser(env.ctx, session.ID(), bob)
	require.NoError(t, err)
	require.NotNil(t, attendee.TeamID())
	assert.Equal(t, teamB, *attendee.TeamID())

	// unassign
	_, err = env.svc.UnassignAttendeeTeam(env.ctx, application.UnassignAttendeeTeamParams{
		SessionID:   session.ID(),
		UserID:      bob,
		RequesterID: session.CreatedByID(),
	})
	require.NoError(t, err)

	count, err := env.attendeeRepo.CountTeamMembers(env.ctx, teamB)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestAttendeeService_AssignTeam_TeamFull(t *testing.T) {
	env := setupSessionTeamTest(t)
	session := env.createTeamSession(t, testutil.Ptr(1), nil)
	teamA := session.Teams()[0].ID()

	require.NoError(t, env.assign(session, env.addGoingAttendee(t, session), teamA))

	err := env.assign(session, env.addGoingAttendee(t, session), teamA)
	assert.ErrorIs(t, err, domain.ErrTeamFull)
}

// TestAttendeeService_AssignTeam_Concurrent_DoesNotOverfillTeam mirrors
// TestAttendeeService_UpdateRSVP_ConcurrentGoing_DoesNotOverbook: two
// concurrent assignments into the last slot of a players_per_team=1 team must
// never both succeed. Without the session advisory lock both would read a
// team size of 0 and both commit.
func TestAttendeeService_AssignTeam_Concurrent_DoesNotOverfillTeam(t *testing.T) {
	env := setupSessionTeamTest(t)
	session := env.createTeamSession(t, testutil.Ptr(1), nil)
	teamA := session.Teams()[0].ID()

	userA := env.addGoingAttendee(t, session)
	userB := env.addGoingAttendee(t, session)

	const attempts = 20
	for i := range attempts {
		var wg sync.WaitGroup
		errs := make([]error, 2)

		for idx, userID := range []uuid.UUID{userA, userB} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs[idx] = env.assign(session, userID, teamA)
			}()
		}
		wg.Wait()

		succeeded := 0
		for _, err := range errs {
			if err == nil {
				succeeded++
				continue
			}
			require.Truef(t, errors.Is(err, domain.ErrTeamFull), "unexpected error on attempt %d: %v", i, err)
		}
		require.Equalf(t, 1, succeeded, "exactly one assignment should win on attempt %d", i)

		count, err := env.attendeeRepo.CountTeamMembers(env.ctx, teamA)
		require.NoError(t, err)
		require.Equalf(t, 1, count, "team with players_per_team=1 has %d members on attempt %d - overfilled", count, i)

		// Free the slot again for the next attempt.
		for _, userID := range []uuid.UUID{userA, userB} {
			_, err := env.svc.UnassignAttendeeTeam(env.ctx, application.UnassignAttendeeTeamParams{
				SessionID:   session.ID(),
				UserID:      userID,
				RequesterID: session.CreatedByID(),
			})
			require.NoError(t, err)
		}
	}
}

func TestAttendeeService_LeavingFreesTeamSlot(t *testing.T) {
	t.Run("rsvp not_going", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session := env.createTeamSession(t, testutil.Ptr(1), nil)
		teamA := session.Teams()[0].ID()
		leaver := env.addGoingAttendee(t, session)
		require.NoError(t, env.assign(session, leaver, teamA))

		attendee, err := env.svc.UpdateRSVP(env.ctx, application.UpdateRSVPParams{
			SessionID: session.ID(),
			UserID:    leaver,
			NewStatus: string(domain.AttendeeStatusNotGoing),
		})
		require.NoError(t, err)
		assert.Nil(t, attendee.TeamID())

		// the freed slot can be taken
		assert.NoError(t, env.assign(session, env.addGoingAttendee(t, session), teamA))
	})

	t.Run("rsvp cancelled", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session := env.createTeamSession(t, testutil.Ptr(1), nil)
		teamA := session.Teams()[0].ID()
		leaver := env.addGoingAttendee(t, session)
		require.NoError(t, env.assign(session, leaver, teamA))

		require.NoError(t, env.svc.CancelRSVP(env.ctx, session.ID(), leaver))

		assert.NoError(t, env.assign(session, env.addGoingAttendee(t, session), teamA))
	})

	t.Run("removed by manager", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session := env.createTeamSession(t, testutil.Ptr(1), nil)
		teamA := session.Teams()[0].ID()
		removed := env.addGoingAttendee(t, session)
		require.NoError(t, env.assign(session, removed, teamA))

		require.NoError(t, env.svc.RemoveAttendee(env.ctx, application.RemoveAttendeeParams{
			SessionID:   session.ID(),
			UserID:      removed,
			RequesterID: session.CreatedByID(),
		}))

		assert.NoError(t, env.assign(session, env.addGoingAttendee(t, session), teamA))
	})
}

func TestSessionService_CreateTeams_AfterPeopleJoined(t *testing.T) {
	env := setupSessionTeamTest(t)
	session := env.createSession(t, domain.ActivityTypeVolleyball)
	alice := env.addGoingAttendee(t, session)

	loaded, err := env.sessionRepo.GetSessionByID(env.ctx, session.ID())
	require.NoError(t, err)
	assert.False(t, loaded.HasTeams(), "a new session has no teams")

	withTeams, err := env.createTeams(session, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, withTeams.Teams(), 2)

	assert.NoError(t, env.assign(session, alice, withTeams.Teams()[0].ID()))
}

func TestSessionService_CreateTeams_ReplacesTeamsAndUnassignsEveryone(t *testing.T) {
	env := setupSessionTeamTest(t)
	session := env.createTeamSession(t, nil, nil)
	oldTeamA := session.Teams()[0].ID()

	alice := env.addGoingAttendee(t, session)
	bob := env.addGoingAttendee(t, session)
	require.NoError(t, env.assign(session, alice, oldTeamA))
	require.NoError(t, env.assign(session, bob, oldTeamA))

	// bob cancels - his soft-deleted row must not block deleting the old team
	require.NoError(t, env.svc.CancelRSVP(env.ctx, session.ID(), bob))

	replaced, err := env.createTeams(session, testutil.Ptr(3), testutil.Ptr(4), nil)
	require.NoError(t, err)
	require.Len(t, replaced.Teams(), 3)

	loaded, err := env.sessionRepo.GetSessionByID(env.ctx, session.ID())
	require.NoError(t, err)
	require.Len(t, loaded.Teams(), 3)
	assert.Equal(t, testutil.Ptr(4), loaded.TeamConfig().PlayersPerTeam())
	_, oldStillThere := loaded.Team(oldTeamA)
	assert.False(t, oldStillThere)

	attendee, err := env.attendeeRepo.GetAttendeeBySessionAndUser(env.ctx, session.ID(), alice)
	require.NoError(t, err)
	assert.Nil(t, attendee.TeamID(), "replacing teams unassigns everyone")

	// the old team is gone, the new ones work
	assert.ErrorIs(t, env.assign(session, alice, oldTeamA), domain.ErrTeamNotFound)
	assert.NoError(t, env.assign(session, alice, loaded.Teams()[2].ID()))
}

func TestSessionService_CreateTeams_NonTeamSportRejected(t *testing.T) {
	env := setupSessionTeamTest(t)
	session := env.createSession(t, domain.ActivityTypeRunning)

	_, err := env.createTeams(session, nil, nil, nil)

	assert.ErrorIs(t, err, domain.ErrTeamsNotSupported)
}
