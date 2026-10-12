package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func inviteEventTypes(i *GroupInvite) []string {
	types := make([]string, 0, len(i.Events()))
	for _, e := range i.Events() {
		types = append(types, e.GetEventType())
	}
	return types
}

func newTestInvite(t *testing.T) (*GroupInvite, uuid.UUID) {
	t.Helper()

	invitee := uuid.New()
	invite, err := NewInvite(uuid.New(), invitee, uuid.New(), MemberRoleAdmin)
	require.NoError(t, err)
	invite.ClearEvents()

	return invite, invitee
}

// expiredInvite is a pending invite whose time ran out an hour ago.
func expiredInvite(status InviteStatus) (*GroupInvite, uuid.UUID) {
	invitee := uuid.New()
	return ReconstructGroupInvite(
		uuid.New(), uuid.New(), invitee, uuid.New(), status,
		time.Now().Add(-InviteExpiryDuration-time.Hour), time.Now().Add(-time.Hour), nil,
	), invitee
}

func TestInviteStatus_IsValid(t *testing.T) {
	for _, s := range []InviteStatus{InviteStatusPending, InviteStatusAccepted, InviteStatusDeclined, InviteStatusExpired} {
		assert.True(t, s.IsValid(), s)
	}
	assert.False(t, InviteStatus("").IsValid())
	assert.False(t, InviteStatus("revoked").IsValid())
}

func TestNewInvite(t *testing.T) {
	groupID, invitee, sender := uuid.New(), uuid.New(), uuid.New()

	t.Run("a manager sends a pending invite that expires later", func(t *testing.T) {
		for _, role := range []MemberRole{MemberRoleCreator, MemberRoleAdmin} {
			invite, err := NewInvite(groupID, invitee, sender, role)
			require.NoError(t, err, role)

			assert.Equal(t, InviteStatusPending, invite.Status())
			assert.Equal(t, groupID, invite.ActivityGroupID())
			assert.Equal(t, invitee, invite.InvitedUserID())
			assert.Equal(t, sender, invite.InvitedByID())
			assert.Nil(t, invite.RespondedAt())
			assert.False(t, invite.IsExpired())
			assert.WithinDuration(t, invite.CreatedAt().Add(InviteExpiryDuration), invite.ExpiresAt(), time.Second)
			assert.Equal(t, []string{EventActivityInviteSent}, inviteEventTypes(invite))
		}
	})

	t.Run("a plain member may not invite", func(t *testing.T) {
		_, err := NewInvite(groupID, invitee, sender, MemberRoleMember)
		assert.ErrorIs(t, err, ErrUnauthorized)
	})

	t.Run("nobody invites themselves", func(t *testing.T) {
		_, err := NewInvite(groupID, sender, sender, MemberRoleCreator)
		assert.ErrorIs(t, err, ErrCannotInviteCreator)
	})
}

func TestGroupInvite_AcceptAndDecline(t *testing.T) {
	for name, decide := range map[string]struct {
		do     func(*GroupInvite, uuid.UUID) error
		status InviteStatus
		event  string
	}{
		"accept":  {(*GroupInvite).Accept, InviteStatusAccepted, EventActivityInviteAccepted},
		"decline": {(*GroupInvite).Decline, InviteStatusDeclined, EventActivityInviteDeclined},
	} {
		t.Run(name+": the invitee answers a pending invite", func(t *testing.T) {
			invite, invitee := newTestInvite(t)

			require.NoError(t, decide.do(invite, invitee))

			assert.Equal(t, decide.status, invite.Status())
			assert.NotNil(t, invite.RespondedAt())
			assert.Equal(t, []string{decide.event}, inviteEventTypes(invite))
		})

		t.Run(name+": nobody else may answer for them", func(t *testing.T) {
			invite, _ := newTestInvite(t)

			assert.ErrorIs(t, decide.do(invite, uuid.New()), ErrUnauthorized)
			assert.Equal(t, InviteStatusPending, invite.Status())
		})

		t.Run(name+": not after it expired", func(t *testing.T) {
			invite, invitee := expiredInvite(InviteStatusPending)

			assert.ErrorIs(t, decide.do(invite, invitee), ErrInviteExpired)
			assert.Equal(t, InviteStatusPending, invite.Status())
		})

		t.Run(name+": not twice", func(t *testing.T) {
			invite, invitee := newTestInvite(t)
			require.NoError(t, invite.Accept(invitee))

			assert.ErrorIs(t, decide.do(invite, invitee), ErrInviteAlreadyProcessed)
		})
	}
}

func TestGroupInvite_Expire(t *testing.T) {
	t.Run("an overdue pending invite is marked expired", func(t *testing.T) {
		invite, _ := expiredInvite(InviteStatusPending)

		require.NoError(t, invite.Expire())

		assert.Equal(t, InviteStatusExpired, invite.Status())
		assert.Equal(t, []string{EventActivityInviteExpired}, inviteEventTypes(invite))
	})

	t.Run("one still within its time is left alone", func(t *testing.T) {
		invite, _ := newTestInvite(t)

		assert.ErrorIs(t, invite.Expire(), ErrInviteHasNotExpiredYet)
		assert.Equal(t, InviteStatusPending, invite.Status())
	})

	t.Run("an overdue invite that was already answered keeps its answer and raises nothing", func(t *testing.T) {
		invite, _ := expiredInvite(InviteStatusAccepted)

		require.NoError(t, invite.Expire())

		assert.Equal(t, InviteStatusAccepted, invite.Status())
		assert.Empty(t, invite.Events())
	})
}
