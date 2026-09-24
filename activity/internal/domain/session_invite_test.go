package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPendingSessionInvite(t *testing.T) (*SessionInvite, *Session) {
	t.Helper()

	session := newTestSessionWithCapacity(t, 5)
	invite, err := NewSessionInvite(session, uuid.New(), session.CreatedByID())
	require.NoError(t, err)

	return invite, session
}

func TestNewSessionInvite_CreatorInvitesToStandaloneSession_Pending(t *testing.T) {
	session := newTestSessionWithCapacity(t, 5)
	invitedUserID := uuid.New()

	invite, err := NewSessionInvite(session, invitedUserID, session.CreatedByID())
	require.NoError(t, err)

	assert.Equal(t, InviteStatusPending, invite.Status())
	assert.Equal(t, session.ID(), invite.SessionID())
	assert.Equal(t, invitedUserID, invite.InvitedUserID())
	assert.Equal(t, session.CreatedByID(), invite.InvitedByID())
	assert.WithinDuration(t, time.Now().Add(InviteExpiryDuration), invite.ExpiresAt(), time.Minute)

	require.Len(t, invite.Events(), 1)
	assert.IsType(t, &SessionInviteSentEvent{}, invite.Events()[0])
}

func TestNewSessionInvite_NonCreator_Unauthorized(t *testing.T) {
	session := newTestSessionWithCapacity(t, 5)

	_, err := NewSessionInvite(session, uuid.New(), uuid.New())

	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestNewSessionInvite_GroupSession_NotStandalone(t *testing.T) {
	session := newTestSessionWithCapacity(t, 5)
	groupID := uuid.New()
	session.activityGroupID = &groupID

	_, err := NewSessionInvite(session, uuid.New(), session.CreatedByID())

	assert.ErrorIs(t, err, ErrSessionNotStandalone)
}

func TestNewSessionInvite_InvitingSelf_Rejected(t *testing.T) {
	session := newTestSessionWithCapacity(t, 5)

	_, err := NewSessionInvite(session, session.CreatedByID(), session.CreatedByID())

	assert.ErrorIs(t, err, ErrCannotInviteCreator)
}

func TestNewSessionInvite_SessionNotScheduled_Rejected(t *testing.T) {
	session := newTestSessionWithCapacity(t, 5)
	session.status = SessionStatusCanceled

	_, err := NewSessionInvite(session, uuid.New(), session.CreatedByID())

	assert.ErrorIs(t, err, ErrSessionNotScheduled)
}

func TestSessionInvite_Accept(t *testing.T) {
	invite, _ := newPendingSessionInvite(t)
	invite.ClearEvents()

	require.NoError(t, invite.Accept(invite.InvitedUserID()))

	assert.Equal(t, InviteStatusAccepted, invite.Status())
	assert.NotNil(t, invite.RespondedAt())
	require.Len(t, invite.Events(), 1)
	assert.IsType(t, &SessionInviteAcceptedEvent{}, invite.Events()[0])
}

func TestSessionInvite_Decline(t *testing.T) {
	invite, _ := newPendingSessionInvite(t)
	invite.ClearEvents()

	require.NoError(t, invite.Decline(invite.InvitedUserID()))

	assert.Equal(t, InviteStatusDeclined, invite.Status())
	assert.NotNil(t, invite.RespondedAt())
	require.Len(t, invite.Events(), 1)
	assert.IsType(t, &SessionInviteDeclinedEvent{}, invite.Events()[0])
}

func TestSessionInvite_RespondByOtherUser_Unauthorized(t *testing.T) {
	invite, _ := newPendingSessionInvite(t)

	assert.ErrorIs(t, invite.Accept(uuid.New()), ErrUnauthorized)
	assert.ErrorIs(t, invite.Decline(uuid.New()), ErrUnauthorized)
	assert.Equal(t, InviteStatusPending, invite.Status())
}

func TestSessionInvite_RespondTwice_AlreadyProcessed(t *testing.T) {
	invite, _ := newPendingSessionInvite(t)
	require.NoError(t, invite.Accept(invite.InvitedUserID()))

	assert.ErrorIs(t, invite.Accept(invite.InvitedUserID()), ErrInviteAlreadyProcessed)
	assert.ErrorIs(t, invite.Decline(invite.InvitedUserID()), ErrInviteAlreadyProcessed)
}

func TestSessionInvite_RespondAfterExpiry_Expired(t *testing.T) {
	invite, _ := newPendingSessionInvite(t)
	invite.expiresAt = time.Now().Add(-time.Minute)

	assert.ErrorIs(t, invite.Accept(invite.InvitedUserID()), ErrInviteExpired)
	assert.ErrorIs(t, invite.Decline(invite.InvitedUserID()), ErrInviteExpired)
}

func TestSessionInvite_Expire(t *testing.T) {
	invite, _ := newPendingSessionInvite(t)
	invite.ClearEvents()

	assert.ErrorIs(t, invite.Expire(), ErrInviteHasNotExpiredYet)

	invite.expiresAt = time.Now().Add(-time.Minute)
	require.NoError(t, invite.Expire())

	assert.Equal(t, InviteStatusExpired, invite.Status())
	require.Len(t, invite.Events(), 1)
	assert.IsType(t, &SessionInviteExpiredEvent{}, invite.Events()[0])
}

func TestNewInvitedAttendee_CapacityAware(t *testing.T) {
	session := newTestSessionWithCapacity(t, 1)

	confirmed := NewInvitedAttendee(session, uuid.New(), 0)
	assert.Equal(t, AttendeeStatusGoing, confirmed.Status())
	assert.Equal(t, AttendeeSourceInvited, confirmed.Source())
	assert.Nil(t, confirmed.ActivityID())

	waitlisted := NewInvitedAttendee(session, uuid.New(), 1)
	assert.Equal(t, AttendeeStatusPending, waitlisted.Status())
	assert.Equal(t, AttendeeSourceInvited, waitlisted.Source())
}
