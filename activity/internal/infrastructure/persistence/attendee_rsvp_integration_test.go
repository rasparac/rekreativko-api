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

type sessionOptions struct {
	groupID          *uuid.UUID
	capacity         *int
	requiresApproval bool
	visibility       domain.SessionVisibility
	activityType     domain.ActivityType
	lat, lng         float64 // default: Belgrade
}

// createSessionWith creates a basketball session through the service. The
// creator is whoever is returned by session.CreatedByID().
func (e *sessionTeamTestEnv) createSessionWith(t *testing.T, creator uuid.UUID, opts sessionOptions) *domain.Session {
	t.Helper()

	visibility := opts.visibility
	if visibility == "" {
		visibility = domain.SessionVisibilityPublic
	}

	activityType := opts.activityType
	if activityType == "" {
		activityType = domain.ActivityTypeBasketball
	}
	lat, lng := opts.lat, opts.lng
	if lat == 0 && lng == 0 {
		lat, lng = 44.8, 20.4
	}

	session, err := e.sessionSvc.CreateSession(e.ctx, application.CreateSessionParams{
		ActivityGroupID:  opts.groupID,
		CreatedByID:      creator,
		Title:            "Pickup Game",
		ActivityType:     string(activityType),
		DifficultyLevel:  string(domain.DifficultyLevelBeginner),
		LocationCity:     "Belgrade",
		LocationCountry:  "RS",
		LocationLat:      lat,
		LocationLng:      lng,
		StartTime:        time.Now().Add(time.Hour),
		Capacity:         opts.capacity,
		Visibility:       string(visibility),
		RequiresApproval: opts.requiresApproval,
	})
	require.NoError(t, err)

	return session
}

func (e *sessionTeamTestEnv) rsvp(session *domain.Session, userID uuid.UUID, status domain.AttendeeStatus) (*domain.Attendee, error) {
	return e.svc.CreateRSVP(e.ctx, application.CreateRSVPParams{
		SessionID: session.ID(),
		UserID:    userID,
		Status:    string(status),
	})
}

func (e *sessionTeamTestEnv) statusOf(t *testing.T, session *domain.Session, userID uuid.UUID) domain.AttendeeStatus {
	t.Helper()

	attendee, err := e.svc.GetRSVP(e.ctx, session.ID(), userID)
	require.NoError(t, err)
	return attendee.Status()
}

func intPtr(n int) *int { return &n }

// A session with a capacity takes people in order; everyone after that waits
// on the waitlist, and a freed spot goes to whoever has waited longest. The
// creator attends their own session, so they take one of the spots.
func TestAttendeeService_CreateRSVP_CapacityAndWaitlist(t *testing.T) {
	env := setupSessionTeamTest(t)
	session := env.createSessionWith(t, uuid.New(), sessionOptions{capacity: intPtr(3)}) // the creator + two more
	first, second, third, fourth := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	for _, userID := range []uuid.UUID{first, second} {
		attendee, err := env.rsvp(session, userID, domain.AttendeeStatusGoing)
		require.NoError(t, err)
		assert.Equal(t, domain.AttendeeStatusGoing, attendee.Status(), "a free spot is taken at once")
	}

	attendee, err := env.rsvp(session, third, domain.AttendeeStatusGoing)
	require.NoError(t, err)
	assert.Equal(t, domain.AttendeeStatusPending, attendee.Status(), "the session is full: the waitlist")
	attendee, err = env.rsvp(session, fourth, domain.AttendeeStatusGoing)
	require.NoError(t, err)
	assert.Equal(t, domain.AttendeeStatusPending, attendee.Status())

	t.Run("maybe and not going never take a spot", func(t *testing.T) {
		for _, status := range []domain.AttendeeStatus{domain.AttendeeStatusMaybe, domain.AttendeeStatusNotGoing} {
			attendee, err := env.rsvp(session, uuid.New(), status)
			require.NoError(t, err)
			assert.Equal(t, status, attendee.Status())
		}
		assert.Equal(t, domain.AttendeeStatusGoing, env.statusOf(t, session, first), "nobody was pushed out")
	})

	t.Run("a freed spot goes to whoever waited longest", func(t *testing.T) {
		require.NoError(t, env.svc.CancelRSVP(env.ctx, session.ID(), first))

		assert.Equal(t, domain.AttendeeStatusPromoted, env.statusOf(t, session, third))
		assert.Equal(t, domain.AttendeeStatusPending, env.statusOf(t, session, fourth), "still waiting")
		assert.Equal(t, 1, env.outboxCount(t, "activity.session.attendee.promoted"))

		require.NoError(t, env.svc.CancelRSVP(env.ctx, session.ID(), second))

		assert.Equal(t, domain.AttendeeStatusPromoted, env.statusOf(t, session, fourth))
		assert.Equal(t, 2, env.outboxCount(t, "activity.session.attendee.promoted"))
	})

	t.Run("someone who gave up their spot is not on the list any more", func(t *testing.T) {
		_, err := env.svc.GetRSVP(env.ctx, session.ID(), first)
		assert.ErrorIs(t, err, domain.ErrAttendeeNotFound)
	})
}

func TestAttendeeService_CreateRSVP_Rejections(t *testing.T) {
	env := setupSessionTeamTest(t)
	session := env.createSessionWith(t, uuid.New(), sessionOptions{})
	user := uuid.New()
	_, err := env.rsvp(session, user, domain.AttendeeStatusGoing)
	require.NoError(t, err)

	t.Run("one RSVP per person", func(t *testing.T) {
		_, err := env.rsvp(session, user, domain.AttendeeStatusMaybe)
		assert.ErrorIs(t, err, domain.ErrAttendeeAlreadyAttending)
	})

	t.Run("an unknown status", func(t *testing.T) {
		_, err := env.rsvp(session, uuid.New(), domain.AttendeeStatus("definitely"))
		assert.Error(t, err)
	})

	t.Run("a session that is not scheduled any more", func(t *testing.T) {
		called := env.createSessionWith(t, uuid.New(), sessionOptions{})
		require.NoError(t, env.sessionSvc.CancelSession(env.ctx, called.ID(), called.CreatedByID(), "", "rained off"))

		_, err := env.rsvp(called, uuid.New(), domain.AttendeeStatusGoing)
		assert.ErrorIs(t, err, domain.ErrSessionNotScheduled)
	})

	t.Run("a session that does not exist", func(t *testing.T) {
		_, err := env.svc.CreateRSVP(env.ctx, application.CreateRSVPParams{
			SessionID: uuid.New(), UserID: uuid.New(), Status: string(domain.AttendeeStatusGoing),
		})
		assert.ErrorIs(t, err, domain.ErrSessionNotFound)
	})
}

// A private group's session is for the group; a public one is open to
// everybody, who then attend without joining the group.
func TestAttendeeService_CreateRSVP_GroupMembershipAndVisibility(t *testing.T) {
	env := setupSessionTeamTest(t)
	organizer := uuid.New()
	group := env.createGroup(t, organizer)
	groupID := group.ID()
	outsider := uuid.New()

	t.Run("a private group session turns away someone who is not in the group", func(t *testing.T) {
		session := env.createSessionWith(t, organizer, sessionOptions{groupID: &groupID, visibility: domain.SessionVisibilityPrivate})

		_, err := env.rsvp(session, outsider, domain.AttendeeStatusGoing)
		assert.ErrorIs(t, err, domain.ErrAttendeeNotGroupMember)

		assert.Equal(t, domain.AttendeeStatusGoing, env.statusOf(t, session, organizer), "the creator attends their own session")
	})

	t.Run("a public group session is open to outsiders", func(t *testing.T) {
		session := env.createSessionWith(t, organizer, sessionOptions{groupID: &groupID, visibility: domain.SessionVisibilityPublic})

		attendee, err := env.rsvp(session, outsider, domain.AttendeeStatusGoing)
		require.NoError(t, err)
		assert.Equal(t, domain.AttendeeStatusGoing, attendee.Status())
	})
}

// Approval-gated sessions: asking creates a pending request for the managers;
// a spot is only taken when a manager approves.
func TestAttendeeService_ApprovalGatedSession(t *testing.T) {
	newSession := func(t *testing.T, capacity int) (*sessionTeamTestEnv, *domain.Session) {
		t.Helper()
		env := setupSessionTeamTest(t)
		// The creator attends their own session and takes one spot.
		return env, env.createSessionWith(t, uuid.New(), sessionOptions{capacity: intPtr(capacity + 1), requiresApproval: true})
	}
	approve := func(env *sessionTeamTestEnv, session *domain.Session, userID, requester uuid.UUID, role string) error {
		return env.svc.ApproveAttendee(env.ctx, application.ApproveAttendeeParams{
			SessionID: session.ID(), UserID: userID, RequesterID: requester, RequesterRole: role,
		})
	}
	reject := func(env *sessionTeamTestEnv, session *domain.Session, userID, requester uuid.UUID, role string) error {
		return env.svc.RejectAttendee(env.ctx, application.RejectAttendeeParams{
			SessionID: session.ID(), UserID: userID, RequesterID: requester, RequesterRole: role,
		})
	}

	t.Run("asking creates a pending request for the managers; approving confirms it", func(t *testing.T) {
		env, session := newSession(t, 5)
		user := uuid.New()

		attendee, err := env.rsvp(session, user, domain.AttendeeStatusGoing)
		require.NoError(t, err)
		assert.Equal(t, domain.AttendeeStatusPending, attendee.Status(), "not going until approved")
		assert.Equal(t, 1, env.outboxCount(t, "activity.session.attendee.join_requested"))

		snapshot, err := env.formation.Snapshot(env.ctx, session.ID())
		require.NoError(t, err)
		assert.NotContains(t, snapshot.GoingUserIDs, user, "a request is not a place")

		require.NoError(t, approve(env, session, user, session.CreatedByID(), ""))

		assert.Equal(t, domain.AttendeeStatusGoing, env.statusOf(t, session, user))
		assert.Equal(t, 1, env.outboxCount(t, "activity.session.attendee.join_approved"))
	})

	t.Run("rejecting turns the request down", func(t *testing.T) {
		env, session := newSession(t, 5)
		user := uuid.New()
		_, err := env.rsvp(session, user, domain.AttendeeStatusGoing)
		require.NoError(t, err)

		require.NoError(t, reject(env, session, user, session.CreatedByID(), ""))

		assert.Equal(t, domain.AttendeeStatusNotGoing, env.statusOf(t, session, user))
		assert.Equal(t, 1, env.outboxCount(t, "activity.session.attendee.join_rejected"))
	})

	t.Run("only a manager may decide", func(t *testing.T) {
		env, session := newSession(t, 5)
		user, stranger := uuid.New(), uuid.New()
		_, err := env.rsvp(session, user, domain.AttendeeStatusGoing)
		require.NoError(t, err)

		assert.ErrorIs(t, approve(env, session, user, stranger, "member"), domain.ErrUnauthorized)
		assert.ErrorIs(t, reject(env, session, user, stranger, ""), domain.ErrUnauthorized)
		assert.Equal(t, domain.AttendeeStatusPending, env.statusOf(t, session, user), "nothing changed")

		assert.NoError(t, approve(env, session, user, stranger, "admin"), "a group admin may")
	})

	t.Run("a full session does not take new requests", func(t *testing.T) {
		env, session := newSession(t, 1)
		first := uuid.New()
		_, err := env.rsvp(session, first, domain.AttendeeStatusGoing)
		require.NoError(t, err)
		require.NoError(t, approve(env, session, first, session.CreatedByID(), ""))

		_, err = env.rsvp(session, uuid.New(), domain.AttendeeStatusGoing)
		assert.ErrorIs(t, err, domain.ErrSessionFull, "no point waiting for a request that can never be approved")
	})

	t.Run("capacity is checked again when approving: the last spot goes once", func(t *testing.T) {
		env, session := newSession(t, 1)
		first, second := uuid.New(), uuid.New()
		_, err := env.rsvp(session, first, domain.AttendeeStatusGoing)
		require.NoError(t, err)
		_, err = env.rsvp(session, second, domain.AttendeeStatusGoing)
		require.NoError(t, err, "both asked while there was room")

		require.NoError(t, approve(env, session, first, session.CreatedByID(), ""))

		assert.ErrorIs(t, approve(env, session, second, session.CreatedByID(), ""), domain.ErrSessionFull)
		assert.Equal(t, domain.AttendeeStatusPending, env.statusOf(t, session, second))
	})

	t.Run("only a pending request can be decided", func(t *testing.T) {
		env, session := newSession(t, 5)
		user := uuid.New()
		_, err := env.rsvp(session, user, domain.AttendeeStatusGoing)
		require.NoError(t, err)
		require.NoError(t, approve(env, session, user, session.CreatedByID(), ""))

		assert.ErrorIs(t, approve(env, session, user, session.CreatedByID(), ""), domain.ErrAttendeeNotAwaitingApproval)
		assert.ErrorIs(t, reject(env, session, user, session.CreatedByID(), ""), domain.ErrAttendeeNotAwaitingApproval)
		assert.ErrorIs(t, approve(env, session, uuid.New(), session.CreatedByID(), ""), domain.ErrAttendeeNotFound)
	})

	t.Run("an approval-gated session still takes maybe and not going straight away", func(t *testing.T) {
		env, session := newSession(t, 5)

		attendee, err := env.rsvp(session, uuid.New(), domain.AttendeeStatusMaybe)
		require.NoError(t, err)
		assert.Equal(t, domain.AttendeeStatusMaybe, attendee.Status())
		assert.Zero(t, env.outboxCount(t, "activity.session.attendee.join_requested"))
	})
}

// A join request is decided by a manager, never by a spot freeing up: only
// people waiting on the waitlist are promoted into it.
func TestAttendeeService_FreedSpotNeverApprovesAJoinRequest(t *testing.T) {
	env := setupSessionTeamTest(t)
	// the creator + one place
	session := env.createSessionWith(t, uuid.New(), sessionOptions{capacity: intPtr(2), requiresApproval: true})
	approved, waiting := uuid.New(), uuid.New()

	_, err := env.rsvp(session, approved, domain.AttendeeStatusGoing)
	require.NoError(t, err)
	_, err = env.rsvp(session, waiting, domain.AttendeeStatusGoing)
	require.NoError(t, err, "both asked while there was room")
	require.NoError(t, env.svc.ApproveAttendee(env.ctx, application.ApproveAttendeeParams{
		SessionID: session.ID(), UserID: approved, RequesterID: session.CreatedByID(),
	}))

	require.NoError(t, env.svc.CancelRSVP(env.ctx, session.ID(), approved))

	assert.Equal(t, domain.AttendeeStatusPending, env.statusOf(t, session, waiting),
		"still a request: a manager has not approved them")
	assert.Zero(t, env.outboxCount(t, "activity.session.attendee.promoted"))

	require.NoError(t, env.svc.ApproveAttendee(env.ctx, application.ApproveAttendeeParams{
		SessionID: session.ID(), UserID: waiting, RequesterID: session.CreatedByID(),
	}))
	assert.Equal(t, domain.AttendeeStatusGoing, env.statusOf(t, session, waiting), "approving is how they get in")
}
