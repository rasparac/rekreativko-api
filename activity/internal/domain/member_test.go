package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newConfirmedMember() *Member {
	return NewMemberFromInvite(uuid.New(), uuid.New())
}

func memberEventTypes(m *Member) []string {
	types := make([]string, 0, len(m.Events()))
	for _, e := range m.Events() {
		types = append(types, e.GetEventType())
	}
	return types
}

func TestMemberRole(t *testing.T) {
	for role, manages := range map[MemberRole]bool{MemberRoleCreator: true, MemberRoleAdmin: true, MemberRoleMember: false} {
		assert.True(t, role.IsValid())
		assert.Equal(t, manages, role.CanManageMembers(), role)
	}
	assert.False(t, MemberRole("").IsValid())
	assert.False(t, MemberRole("overlord").IsValid())
	assert.False(t, MemberRole("overlord").CanManageMembers())
}

func TestMemberStatus(t *testing.T) {
	valid := map[MemberStatus]bool{
		MemberStatusPending: true, MemberStatusConfirmed: true, MemberStatusRejected: false,
		MemberStatusLeft: false, MemberStatusRemoved: false,
	}
	for status, active := range valid {
		assert.True(t, status.IsValid(), status)
		assert.Equal(t, active, status.IsActive(), "only pending and confirmed are active: %s", status)
	}
	assert.False(t, MemberStatus("").IsValid())
	assert.False(t, MemberStatus("banned").IsValid())
}

func TestNewMember_StartsInTheRightState(t *testing.T) {
	groupID, userID := uuid.New(), uuid.New()

	t.Run("the creator is a confirmed creator", func(t *testing.T) {
		m := NewCreatorMember(groupID, userID)
		assert.Equal(t, MemberRoleCreator, m.Role())
		assert.True(t, m.IsConfirmed())
		assert.Equal(t, groupID, m.ActivityGroupID())
		assert.Equal(t, userID, m.UserID())
	})

	t.Run("a join request is pending and tells the managers", func(t *testing.T) {
		m := NewJoinRequest(groupID, userID, []uuid.UUID{uuid.New()})
		assert.True(t, m.IsPending())
		assert.Equal(t, MemberRoleMember, m.Role())
		assert.Equal(t, []string{EventActivityGroupMemberJoinRequested}, memberEventTypes(m))
	})

	t.Run("an invite or an invite link makes a confirmed member straight away", func(t *testing.T) {
		for name, m := range map[string]*Member{
			"invite": NewMemberFromInvite(groupID, userID), "invite link": NewMemberFromInviteLink(groupID, userID),
		} {
			assert.True(t, m.IsConfirmed(), name)
			assert.Equal(t, MemberRoleMember, m.Role(), name)
			assert.False(t, m.IsPriority(), name)
			assert.Equal(t, []string{EventActivityGroupMemberJoined}, memberEventTypes(m), name)
		}
	})
}

func TestMember_ApproveAndReject(t *testing.T) {
	manager := uuid.New()

	for name, decide := range map[string]struct {
		do     func(*Member, MemberRole) error
		status MemberStatus
		event  string
	}{
		"approve": {func(m *Member, r MemberRole) error { return m.Approve(manager, r) }, MemberStatusConfirmed, EventActivityGroupMemberApproved},
		"reject":  {func(m *Member, r MemberRole) error { return m.Reject(manager, r) }, MemberStatusRejected, EventActivityGroupMemberRejected},
	} {
		t.Run(name+": a manager decides a pending request", func(t *testing.T) {
			m := NewJoinRequest(uuid.New(), uuid.New(), nil)
			m.ClearEvents()

			require.NoError(t, decide.do(m, MemberRoleAdmin))

			assert.Equal(t, decide.status, m.Status())
			assert.NotNil(t, m.DecidedAt())
			assert.Equal(t, []string{decide.event}, memberEventTypes(m))
		})

		t.Run(name+": a plain member may not", func(t *testing.T) {
			m := NewJoinRequest(uuid.New(), uuid.New(), nil)

			assert.ErrorIs(t, decide.do(m, MemberRoleMember), ErrUnauthorized)
			assert.True(t, m.IsPending())
		})

		t.Run(name+": only a pending request", func(t *testing.T) {
			assert.ErrorIs(t, decide.do(newConfirmedMember(), MemberRoleCreator), ErrNotPendingStatus)
		})
	}
}

func TestMember_Leave(t *testing.T) {
	t.Run("a member leaves", func(t *testing.T) {
		m := newConfirmedMember()
		m.ClearEvents()

		require.NoError(t, m.Leave())

		assert.Equal(t, MemberStatusLeft, m.Status())
		assert.Equal(t, []string{EventActivityGroupMemberLeft}, memberEventTypes(m))
	})

	t.Run("the creator cannot", func(t *testing.T) {
		assert.ErrorIs(t, NewCreatorMember(uuid.New(), uuid.New()).Leave(), ErrCreatorCannotLeave)
	})

	t.Run("only a confirmed member can leave", func(t *testing.T) {
		assert.ErrorIs(t, NewJoinRequest(uuid.New(), uuid.New(), nil).Leave(), ErrNotConfirmed)

		m := newConfirmedMember()
		require.NoError(t, m.Leave())
		assert.ErrorIs(t, m.Leave(), ErrNotConfirmed, "not twice")
	})
}

func TestMember_Remove(t *testing.T) {
	remover := uuid.New()

	t.Run("a manager removes a member", func(t *testing.T) {
		for _, role := range []MemberRole{MemberRoleCreator, MemberRoleAdmin} {
			m := newConfirmedMember()
			m.ClearEvents()

			require.NoError(t, m.Remove(remover, role), role)

			assert.Equal(t, MemberStatusRemoved, m.Status())
			assert.Equal(t, []string{EventActivityGroupMemberRemoved}, memberEventTypes(m))
		}
	})

	t.Run("a plain member may not remove anyone", func(t *testing.T) {
		assert.ErrorIs(t, newConfirmedMember().Remove(remover, MemberRoleMember), ErrUnauthorized)
	})

	t.Run("an admin cannot remove another admin, the creator can", func(t *testing.T) {
		admin := newConfirmedMember()
		require.NoError(t, admin.PromoteToAdmin(remover, MemberRoleCreator))

		assert.ErrorIs(t, admin.Remove(remover, MemberRoleAdmin), ErrUnauthorized)
		assert.NoError(t, admin.Remove(remover, MemberRoleCreator))
	})

	t.Run("the creator cannot be removed", func(t *testing.T) {
		assert.ErrorIs(t, NewCreatorMember(uuid.New(), uuid.New()).Remove(remover, MemberRoleCreator), ErrCreatorCannotLeave)
	})

	t.Run("only a confirmed member", func(t *testing.T) {
		assert.ErrorIs(t, NewJoinRequest(uuid.New(), uuid.New(), nil).Remove(remover, MemberRoleCreator), ErrNotConfirmed)
	})
}

func TestMember_RoleChanges(t *testing.T) {
	creator := uuid.New()

	t.Run("the creator promotes a member to admin and demotes them again", func(t *testing.T) {
		m := newConfirmedMember()
		m.ClearEvents()

		require.NoError(t, m.PromoteToAdmin(creator, MemberRoleCreator))
		assert.Equal(t, MemberRoleAdmin, m.Role())

		require.NoError(t, m.DemoteToMember(creator, MemberRoleCreator))
		assert.Equal(t, MemberRoleMember, m.Role())
		assert.Equal(t, []string{EventActivityGroupMemberRoleChanged, EventActivityGroupMemberRoleChanged}, memberEventTypes(m))
	})

	t.Run("only the creator may change roles", func(t *testing.T) {
		m := newConfirmedMember()

		assert.ErrorIs(t, m.PromoteToAdmin(creator, MemberRoleAdmin), ErrUnauthorized)
		assert.ErrorIs(t, m.PromoteToAdmin(creator, MemberRoleMember), ErrUnauthorized)
		assert.Equal(t, MemberRoleMember, m.Role())
	})

	t.Run("the creator's own role is fixed", func(t *testing.T) {
		m := NewCreatorMember(uuid.New(), creator)

		assert.ErrorIs(t, m.DemoteToMember(creator, MemberRoleCreator), ErrCannotDemoteCreator)
		assert.ErrorIs(t, m.PromoteToAdmin(creator, MemberRoleCreator), ErrCannotDemoteCreator)
	})

	t.Run("only a confirmed member", func(t *testing.T) {
		assert.ErrorIs(t, NewJoinRequest(uuid.New(), uuid.New(), nil).PromoteToAdmin(creator, MemberRoleCreator), ErrNotConfirmed)
	})

	t.Run("no no-op changes", func(t *testing.T) {
		member := newConfirmedMember()
		assert.ErrorIs(t, member.DemoteToMember(creator, MemberRoleCreator), ErrMemberNotAdmin)

		admin := newConfirmedMember()
		require.NoError(t, admin.PromoteToAdmin(creator, MemberRoleCreator))
		assert.ErrorIs(t, admin.PromoteToAdmin(creator, MemberRoleCreator), ErrMemberAlreadyAdmin)
	})
}

func TestMember_Priority(t *testing.T) {
	manager := uuid.New()

	t.Run("a manager sets and removes early access", func(t *testing.T) {
		m := newConfirmedMember()
		m.ClearEvents()

		require.NoError(t, m.SetPriority(manager, MemberRoleAdmin))
		assert.True(t, m.IsPriority())

		require.NoError(t, m.RemovePriority(manager, MemberRoleCreator))
		assert.False(t, m.IsPriority())
		assert.Equal(t, []string{EventActivityGroupMemberPrioritySet, EventActivityGroupMemberPriorityRemoved}, memberEventTypes(m))
	})

	t.Run("a plain member may not", func(t *testing.T) {
		m := newConfirmedMember()
		assert.ErrorIs(t, m.SetPriority(manager, MemberRoleMember), ErrUnauthorized)
		assert.ErrorIs(t, m.RemovePriority(manager, MemberRoleMember), ErrUnauthorized)
	})

	t.Run("not twice, and not to remove what is not there", func(t *testing.T) {
		m := newConfirmedMember()
		assert.ErrorIs(t, m.RemovePriority(manager, MemberRoleAdmin), ErrMemberNotPriority)

		require.NoError(t, m.SetPriority(manager, MemberRoleAdmin))
		assert.ErrorIs(t, m.SetPriority(manager, MemberRoleAdmin), ErrMemberAlreadyPriority)
	})

	t.Run("only a confirmed member", func(t *testing.T) {
		pending := NewJoinRequest(uuid.New(), uuid.New(), nil)
		assert.ErrorIs(t, pending.SetPriority(manager, MemberRoleAdmin), ErrNotConfirmed)
		assert.ErrorIs(t, pending.RemovePriority(manager, MemberRoleAdmin), ErrNotConfirmed)
	})
}
