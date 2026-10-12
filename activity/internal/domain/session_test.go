package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSession_Complete(t *testing.T) {
	startedSession := func(t *testing.T) *Session {
		t.Helper()

		session := newTestSessionOfType(t, ActivityTypeRunning)
		require.NoError(t, session.Start(session.CreatedByID(), ""))
		session.ClearEvents()

		return session
	}

	tests := []struct {
		name        string
		requesterID func(s *Session) uuid.UUID
		role        MemberRole
		wantErr     error
	}{
		{
			name:        "standalone session creator (no group role)",
			requesterID: func(s *Session) uuid.UUID { return s.CreatedByID() },
			role:        "",
		},
		{
			name:        "group admin who did not create the session",
			requesterID: func(*Session) uuid.UUID { return uuid.New() },
			role:        MemberRoleAdmin,
		},
		{
			name:        "group creator who did not create the session",
			requesterID: func(*Session) uuid.UUID { return uuid.New() },
			role:        MemberRoleCreator,
		},
		{
			name:        "plain member who did not create the session",
			requesterID: func(*Session) uuid.UUID { return uuid.New() },
			role:        MemberRoleMember,
			wantErr:     ErrUnauthorized,
		},
		{
			name:        "stranger without a role",
			requesterID: func(*Session) uuid.UUID { return uuid.New() },
			role:        "",
			wantErr:     ErrUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := startedSession(t)

			err := session.Complete(tt.requesterID(session), tt.role)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, SessionStatusStarted, session.Status())
				assert.Empty(t, session.Events())
				return
			}

			require.NoError(t, err)
			assert.Equal(t, SessionStatusCompleted, session.Status())
			require.Len(t, session.Events(), 1)
			assert.Equal(t, EventActivitySessionCompleted, session.Events()[0].GetEventType())
		})
	}

	t.Run("not started yet", func(t *testing.T) {
		session := newTestSessionOfType(t, ActivityTypeRunning)

		assert.ErrorIs(t, session.Complete(session.CreatedByID(), ""), ErrSessionNotStarted)
	})
}

func TestSession_HasCapacity(t *testing.T) {
	limited := newTestSessionWithCapacity(t, 3)
	assert.True(t, limited.HasCapacity(0))
	assert.True(t, limited.HasCapacity(2))
	assert.False(t, limited.HasCapacity(3), "full")
	assert.False(t, limited.HasCapacity(4))

	unlimited := newTestSessionOfType(t, ActivityTypeBasketball)
	assert.True(t, unlimited.HasCapacity(0))
	assert.True(t, unlimited.HasCapacity(10_000), "no capacity means no limit")
}

func TestSession_CanRSVP(t *testing.T) {
	t.Run("a scheduled session takes RSVPs", func(t *testing.T) {
		session := newTestSessionOfType(t, ActivityTypeBasketball)

		assert.NoError(t, session.CanRSVP(false))
		assert.NoError(t, session.CanRSVP(true))
	})

	t.Run("not before it opens for regular members, but priority members may", func(t *testing.T) {
		session := newTestSessionOfType(t, ActivityTypeBasketball)
		opens := time.Now().Add(2 * time.Hour)
		session.openAt = &opens

		assert.ErrorIs(t, session.CanRSVP(false), ErrSessionNotOpen)
		assert.NoError(t, session.CanRSVP(true), "early access")

		past := time.Now().Add(-time.Minute)
		session.openAt = &past
		assert.NoError(t, session.CanRSVP(false), "once it has opened")
	})

	t.Run("not once started, called off or completed - not even for priority members", func(t *testing.T) {
		for name, mutate := range map[string]func(*Session){
			"started":   func(s *Session) { s.status = SessionStatusStarted },
			"cancelled": func(s *Session) { s.status = SessionStatusCanceled },
			"completed": func(s *Session) { s.status = SessionStatusCompleted },
		} {
			session := newTestSessionOfType(t, ActivityTypeBasketball)
			mutate(session)

			assert.ErrorIs(t, session.CanRSVP(true), ErrSessionNotScheduled, name)
		}
	})
}

func TestSession_Update_AndSetVisibility(t *testing.T) {
	update := func(session *Session, requester uuid.UUID, role MemberRole, capacity *int) error {
		location, err := NewSessionLocation("Novi Sad", "RS", "", 45.25, 19.84)
		require.NoError(t, err)
		schedule, err := NewSessionSchedule(time.Now().Add(48*time.Hour), nil)
		require.NoError(t, err)

		return session.Update(SessionUpdateInput{
			RequesterID: requester, RequesterRole: role,
			Location: location, Schedule: schedule, Capacity: capacity, Note: "bring water",
		})
	}

	t.Run("the creator updates where, when, how many", func(t *testing.T) {
		session := newTestSessionOfType(t, ActivityTypeBasketball)
		session.ClearEvents()

		require.NoError(t, update(session, session.CreatedByID(), "", intPtr(8)))

		assert.Equal(t, "Novi Sad", session.Location().City())
		assert.Equal(t, 8, *session.Capacity())
		assert.Equal(t, "bring water", session.Note())
		require.Len(t, session.Events(), 1)
		assert.Equal(t, EventActivitySessionUpdated, session.Events()[0].GetEventType())
	})

	t.Run("a group admin may, a plain member may not", func(t *testing.T) {
		session := newTestSessionOfType(t, ActivityTypeBasketball)

		assert.NoError(t, update(session, uuid.New(), MemberRoleAdmin, nil))
		assert.ErrorIs(t, update(session, uuid.New(), MemberRoleMember, nil), ErrUnauthorized)
	})

	t.Run("a capacity of 0 or less is rejected", func(t *testing.T) {
		session := newTestSessionOfType(t, ActivityTypeBasketball)

		assert.ErrorIs(t, update(session, session.CreatedByID(), "", intPtr(0)), ErrInvalidSessionCapacity)
		assert.ErrorIs(t, update(session, session.CreatedByID(), "", intPtr(-3)), ErrInvalidSessionCapacity)
	})

	t.Run("not once it has started or been called off", func(t *testing.T) {
		started := newTestSessionOfType(t, ActivityTypeBasketball)
		started.status = SessionStatusStarted
		assert.ErrorIs(t, update(started, started.CreatedByID(), "", nil), ErrSessionAlreadyStarted)

		called := newTestSessionOfType(t, ActivityTypeBasketball)
		called.status = SessionStatusCanceled
		assert.ErrorIs(t, update(called, called.CreatedByID(), "", nil), ErrSessionCanceled)
	})

	t.Run("visibility: a manager changes it, the same value is a no-op, junk is rejected", func(t *testing.T) {
		session := newTestSessionOfType(t, ActivityTypeBasketball)
		require.True(t, session.IsPublic())
		session.ClearEvents()

		require.NoError(t, session.SetVisibility(session.CreatedByID(), "", SessionVisibilityPrivate))
		assert.False(t, session.IsPublic())
		assert.Len(t, session.Events(), 1)

		session.ClearEvents()
		require.NoError(t, session.SetVisibility(session.CreatedByID(), "", SessionVisibilityPrivate))
		assert.Empty(t, session.Events(), "nothing changed, nothing raised")

		assert.ErrorIs(t, session.SetVisibility(uuid.New(), MemberRoleMember, SessionVisibilityPublic), ErrUnauthorized)
		assert.ErrorIs(t, session.SetVisibility(session.CreatedByID(), "", SessionVisibility("secret")), ErrInvalidSessionVisibility)
	})
}

func TestSessionStatus_IsValid(t *testing.T) {
	for _, s := range []SessionStatus{SessionStatusScheduled, SessionStatusStarted, SessionStatusCanceled, SessionStatusCompleted} {
		assert.True(t, s.IsValid(), s)
	}
	assert.False(t, SessionStatus("").IsValid())
	assert.False(t, SessionStatus("postponed").IsValid())
}

func TestSession_IsVisibleTo(t *testing.T) {
	stranger := uuid.New()
	related := func(isRelated bool) func() bool { return func() bool { return isRelated } }
	mustNotAsk := func() bool {
		t.Error("relation looked up although the answer was already known")
		return false
	}

	public := newTestSessionWithCapacity(t, 10)
	assert.True(t, public.IsVisibleTo(stranger, mustNotAsk), "everyone sees a public session")

	private := newTestSessionWithCapacity(t, 10)
	creator := private.CreatedByID()
	require.NoError(t, private.SetVisibility(creator, "", SessionVisibilityPrivate))
	assert.True(t, private.IsVisibleTo(creator, mustNotAsk), "its creator")
	assert.True(t, private.IsVisibleTo(stranger, related(true)), "an attendee, invitee or confirmed member")
	assert.False(t, private.IsVisibleTo(stranger, related(false)), "nobody else")
}
