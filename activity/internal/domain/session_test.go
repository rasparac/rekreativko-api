package domain

import (
	"testing"

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
