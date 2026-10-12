//go:build integration

package persistence_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Who can see a session: a public one anybody; a private one - which does not
// exist for an unrelated user - only its creator, its attendees and, for a
// grouped session, the group's confirmed members.
func TestSessionService_GetSession_Visibility(t *testing.T) {
	env := setupSessionTeamTest(t)
	organizer, stranger := uuid.New(), uuid.New()

	// canSee also checks that EnsureSessionVisible, the single-query twin of
	// GetSession's visibility rule, gives the same answer.
	canSee := func(session *domain.Session, userID uuid.UUID) bool {
		t.Helper()
		_, err := env.sessionSvc.GetSession(env.ctx, session.ID(), userID)
		if err != nil {
			require.ErrorIs(t, err, domain.ErrSessionNotFound, "an invisible session is simply not found")
		}

		ensureErr := env.sessionSvc.EnsureSessionVisible(env.ctx, session.ID(), userID)
		if ensureErr != nil {
			require.ErrorIs(t, ensureErr, domain.ErrSessionNotFound, "an invisible session is simply not found")
		}

		require.Equal(t, err == nil, ensureErr == nil, "GetSession and EnsureSessionVisible disagree on visibility")
		return err == nil
	}

	t.Run("a public session is visible to everyone", func(t *testing.T) {
		session := env.createSessionWith(t, organizer, sessionOptions{visibility: domain.SessionVisibilityPublic})

		assert.True(t, canSee(session, stranger))
	})

	t.Run("a private standalone session: the creator and attendees only", func(t *testing.T) {
		session := env.createSessionWith(t, organizer, sessionOptions{visibility: domain.SessionVisibilityPrivate})
		attendee := env.addGoingAttendee(t, session)

		assert.True(t, canSee(session, organizer), "the creator")
		assert.True(t, canSee(session, attendee), "someone attending")
		assert.False(t, canSee(session, stranger), "anyone else")
	})

	t.Run("a private group session: the group's confirmed members, not those still waiting", func(t *testing.T) {
		group := env.createGroup(t, organizer)
		groupID := group.ID()
		session := env.createSessionWith(t, organizer, sessionOptions{groupID: &groupID, visibility: domain.SessionVisibilityPrivate})

		member, applicant := uuid.New(), uuid.New()
		require.NoError(t, env.memberRepo.CreateMember(env.ctx, domain.NewMemberFromInvite(groupID, member)))
		require.NoError(t, env.memberRepo.CreateMember(env.ctx, domain.NewJoinRequest(groupID, applicant, nil)))

		assert.True(t, canSee(session, organizer), "the group's creator")
		assert.True(t, canSee(session, member), "a confirmed member")
		assert.False(t, canSee(session, applicant), "a pending join request is not a member yet")
		assert.False(t, canSee(session, stranger))
	})

	t.Run("a deleted session is visible to nobody, not even its creator", func(t *testing.T) {
		group := env.createGroup(t, organizer)
		groupID := group.ID()
		session := env.createSessionWith(t, organizer, sessionOptions{groupID: &groupID, visibility: domain.SessionVisibilityPublic})

		_, err := env.sessionRepo.SoftDeleteGroupSessions(env.ctx, groupID)
		require.NoError(t, err)

		assert.False(t, canSee(session, organizer))
		assert.False(t, canSee(session, stranger))
	})

	t.Run("a session that does not exist", func(t *testing.T) {
		_, err := env.sessionSvc.GetSession(env.ctx, uuid.New(), stranger)
		assert.ErrorIs(t, err, domain.ErrSessionNotFound)
		assert.ErrorIs(t, env.sessionSvc.EnsureSessionVisible(env.ctx, uuid.New(), stranger), domain.ErrSessionNotFound)
	})
}

func TestSessionService_UpdateSession(t *testing.T) {
	update := func(env *sessionTeamTestEnv, session *domain.Session, mutate func(*application.UpdateSessionParams)) error {
		params := application.UpdateSessionParams{
			RequesterID:     session.CreatedByID(),
			LocationCity:    "Novi Sad",
			LocationCountry: "RS",
			LocationLat:     45.25,
			LocationLng:     19.84,
			StartTime:       time.Now().Add(48 * time.Hour),
			Capacity:        intPtr(12),
			Note:            "Bring a white and a dark shirt",
			Visibility:      string(domain.SessionVisibilityPrivate),
		}
		if mutate != nil {
			mutate(&params)
		}
		return env.sessionSvc.UpdateSession(env.ctx, session.ID(), params)
	}

	t.Run("the creator changes where, when, how many and who can see it", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session := env.createSessionWith(t, uuid.New(), sessionOptions{visibility: domain.SessionVisibilityPublic})

		require.NoError(t, update(env, session, nil))

		loaded, err := env.sessionRepo.GetSessionByID(env.ctx, session.ID())
		require.NoError(t, err)
		assert.Equal(t, "Novi Sad", loaded.Location().City())
		assert.InDelta(t, 45.25, loaded.Location().Latitude(), 0.0001)
		require.NotNil(t, loaded.Capacity())
		assert.Equal(t, 12, *loaded.Capacity())
		assert.Equal(t, "Bring a white and a dark shirt", loaded.Note())
		assert.Equal(t, domain.SessionVisibilityPrivate, loaded.Visibility())
		assert.Equal(t, 1, env.outboxCount(t, "activity.session.updated"))
		assert.Equal(t, 1, env.outboxCount(t, "activity.session.visibility_changed"))
	})

	t.Run("only a manager may", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session := env.createSessionWith(t, uuid.New(), sessionOptions{})

		err := update(env, session, func(p *application.UpdateSessionParams) {
			p.RequesterID = uuid.New()
			p.RequesterRole = "member"
		})
		assert.ErrorIs(t, err, domain.ErrUnauthorized)

		assert.NoError(t, update(env, session, func(p *application.UpdateSessionParams) {
			p.RequesterID = uuid.New()
			p.RequesterRole = "admin"
		}), "a group admin may")
	})

	t.Run("what is rejected", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session := env.createSessionWith(t, uuid.New(), sessionOptions{})

		assert.ErrorIs(t, update(env, session, func(p *application.UpdateSessionParams) { p.Capacity = intPtr(0) }), domain.ErrInvalidSessionCapacity)
		assert.ErrorIs(t, update(env, session, func(p *application.UpdateSessionParams) { p.Visibility = "secret" }), domain.ErrInvalidSessionVisibility)
		assert.Error(t, update(env, session, func(p *application.UpdateSessionParams) { p.LocationLat = 123 }), "a latitude that does not exist")
		assert.Error(t, update(env, session, func(p *application.UpdateSessionParams) { p.StartTime = time.Now().Add(-time.Hour) }), "a start in the past")
		assert.Error(t, update(env, session, func(p *application.UpdateSessionParams) { p.RequesterRole = "overlord" }), "a role that does not exist")

		loaded, err := env.sessionRepo.GetSessionByID(env.ctx, session.ID())
		require.NoError(t, err)
		assert.Equal(t, "Belgrade", loaded.Location().City(), "a rejected update changes nothing")
	})

	t.Run("not once the session was called off or has started", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		called := env.createSessionWith(t, uuid.New(), sessionOptions{})
		require.NoError(t, env.sessionSvc.CancelSession(env.ctx, called.ID(), called.CreatedByID(), "", "rain"))
		assert.ErrorIs(t, update(env, called, nil), domain.ErrSessionCanceled)

		started := env.createSessionWith(t, uuid.New(), sessionOptions{})
		require.NoError(t, env.sessionSvc.StartSession(env.ctx, started.ID(), started.CreatedByID(), ""))
		assert.ErrorIs(t, update(env, started, nil), domain.ErrSessionAlreadyStarted)
	})

	t.Run("a session that does not exist", func(t *testing.T) {
		env := setupSessionTeamTest(t)
		session := env.createSessionWith(t, uuid.New(), sessionOptions{})

		err := env.sessionSvc.UpdateSession(env.ctx, uuid.New(), application.UpdateSessionParams{
			RequesterID: session.CreatedByID(), LocationCity: "x", LocationCountry: "RS", LocationLat: 1, LocationLng: 1,
			StartTime: time.Now().Add(time.Hour), Visibility: string(domain.SessionVisibilityPublic),
		})
		assert.ErrorIs(t, err, domain.ErrSessionNotFound)
	})
}

func TestSessionService_SetSessionVisibility(t *testing.T) {
	env := setupSessionTeamTest(t)
	session := env.createSessionWith(t, uuid.New(), sessionOptions{visibility: domain.SessionVisibilityPublic})
	set := func(requester uuid.UUID, role, visibility string) error {
		return env.sessionSvc.SetSessionVisibility(env.ctx, session.ID(), requester, role, visibility)
	}
	visibility := func() domain.SessionVisibility {
		loaded, err := env.sessionRepo.GetSessionByID(env.ctx, session.ID())
		require.NoError(t, err)
		return loaded.Visibility()
	}

	require.NoError(t, set(session.CreatedByID(), "", string(domain.SessionVisibilityPrivate)))
	assert.Equal(t, domain.SessionVisibilityPrivate, visibility())
	assert.Equal(t, 1, env.outboxCount(t, "activity.session.visibility_changed"))

	t.Run("setting what it already is changes nothing and raises nothing", func(t *testing.T) {
		require.NoError(t, set(session.CreatedByID(), "", string(domain.SessionVisibilityPrivate)))
		assert.Equal(t, 1, env.outboxCount(t, "activity.session.visibility_changed"))
	})

	t.Run("only a manager may, and only to a real value", func(t *testing.T) {
		assert.ErrorIs(t, set(uuid.New(), "member", string(domain.SessionVisibilityPublic)), domain.ErrUnauthorized)
		assert.ErrorIs(t, set(session.CreatedByID(), "", "secret"), domain.ErrInvalidSessionVisibility)
		assert.Equal(t, domain.SessionVisibilityPrivate, visibility(), "nothing changed")
	})

	t.Run("and it can go back", func(t *testing.T) {
		require.NoError(t, set(uuid.New(), "admin", string(domain.SessionVisibilityPublic)))
		assert.Equal(t, domain.SessionVisibilityPublic, visibility())
	})
}

// Discovery: scheduled, public sessions near a point, nearest first.
func TestSessionService_DiscoverSessions(t *testing.T) {
	env := setupSessionTeamTest(t)
	organizer := uuid.New()

	// Belgrade centre, Novi Sad (~80 km away), Tokyo.
	near := env.createSessionWith(t, organizer, sessionOptions{lat: 44.8, lng: 20.4, activityType: domain.ActivityTypeBasketball})
	novi := env.createSessionWith(t, organizer, sessionOptions{lat: 45.25, lng: 19.84, activityType: domain.ActivityTypeFootball})
	_ = env.createSessionWith(t, organizer, sessionOptions{lat: 35.68, lng: 139.69})
	// Not offered: private, and called off.
	_ = env.createSessionWith(t, organizer, sessionOptions{lat: 44.81, lng: 20.41, visibility: domain.SessionVisibilityPrivate})
	calledOff := env.createSessionWith(t, organizer, sessionOptions{lat: 44.82, lng: 20.42})
	require.NoError(t, env.sessionSvc.CancelSession(env.ctx, calledOff.ID(), organizer, "", "rain"))

	discover := func(radiusKM float64, interests ...application.ActivityInterestFilter) []uuid.UUID {
		t.Helper()
		results, _, _, err := env.sessionSvc.DiscoverSessions(env.ctx, application.DiscoverSessionsParams{
			Lat: 44.8, Lng: 20.4, RadiusKM: radiusKM, Interests: interests, Limit: 20,
		}, uuid.New())
		require.NoError(t, err)

		ids := make([]uuid.UUID, len(results))
		for i, r := range results {
			ids[i] = r.Session.ID()
		}
		return ids
	}

	t.Run("only what is within the radius", func(t *testing.T) {
		assert.Equal(t, []uuid.UUID{near.ID()}, discover(10))
	})

	t.Run("nearest first, and never private or called-off sessions", func(t *testing.T) {
		assert.Equal(t, []uuid.UUID{near.ID(), novi.ID()}, discover(200))
	})

	t.Run("narrowed to what the person is interested in", func(t *testing.T) {
		assert.Equal(t, []uuid.UUID{novi.ID()}, discover(200, application.ActivityInterestFilter{ActivityType: string(domain.ActivityTypeFootball)}))
	})

	t.Run("each result knows how far it is", func(t *testing.T) {
		results, _, _, err := env.sessionSvc.DiscoverSessions(env.ctx, application.DiscoverSessionsParams{
			Lat: 44.8, Lng: 20.4, RadiusKM: 200, Limit: 20,
		}, uuid.New())
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.InDelta(t, 0, results[0].DistanceKM, 0.5)
		assert.InDelta(t, 80, results[1].DistanceKM, 15)
	})

	t.Run("reports whether the asker is already attending each", func(t *testing.T) {
		asker := uuid.New()
		_, err := env.rsvp(near, asker, domain.AttendeeStatusMaybe)
		require.NoError(t, err)

		_, _, statuses, err := env.sessionSvc.DiscoverSessions(env.ctx, application.DiscoverSessionsParams{
			Lat: 44.8, Lng: 20.4, RadiusKM: 200, Limit: 20,
		}, asker)
		require.NoError(t, err)
		assert.Equal(t, domain.AttendeeStatusMaybe, statuses[near.ID()])
		assert.NotContains(t, statuses, novi.ID())
	})

	t.Run("pages", func(t *testing.T) {
		results, next, _, err := env.sessionSvc.DiscoverSessions(env.ctx, application.DiscoverSessionsParams{
			Lat: 44.8, Lng: 20.4, RadiusKM: 200, Limit: 1,
		}, uuid.New())
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.NotEmpty(t, next)

		more, _, _, err := env.sessionSvc.DiscoverSessions(env.ctx, application.DiscoverSessionsParams{
			Lat: 44.8, Lng: 20.4, RadiusKM: 200, Limit: 1, PageToken: next,
		}, uuid.New())
		require.NoError(t, err)
		require.Len(t, more, 1)
		assert.Equal(t, novi.ID(), more[0].Session.ID())
	})

	t.Run("bad input", func(t *testing.T) {
		bad := func(p application.DiscoverSessionsParams) error {
			_, _, _, err := env.sessionSvc.DiscoverSessions(env.ctx, p, uuid.New())
			return err
		}
		assert.ErrorIs(t, bad(application.DiscoverSessionsParams{Lat: 91, Lng: 0, RadiusKM: 5}), domain.ErrSessionLocationLatitudeInvalid)
		assert.ErrorIs(t, bad(application.DiscoverSessionsParams{Lat: 0, Lng: 181, RadiusKM: 5}), domain.ErrSessionLocationLongitudeInvalid)
		assert.ErrorIs(t, bad(application.DiscoverSessionsParams{Lat: 0, Lng: 0, RadiusKM: 0}), domain.ErrInvalidDiscoveryRadius)
		assert.Error(t, bad(application.DiscoverSessionsParams{Lat: 0, Lng: 0, RadiusKM: 5, Interests: []application.ActivityInterestFilter{{ActivityType: "chess-boxing"}}}))
		assert.Error(t, bad(application.DiscoverSessionsParams{Lat: 0, Lng: 0, RadiusKM: 5, Interests: []application.ActivityInterestFilter{{DifficultyLevel: "godlike"}}}))
	})
}
