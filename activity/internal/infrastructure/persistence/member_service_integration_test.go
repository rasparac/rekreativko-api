//go:build integration

package persistence_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addMember makes a new account a confirmed, plain member of the group.
func (e *sessionTeamTestEnv) addMember(t *testing.T, groupID uuid.UUID) uuid.UUID {
	t.Helper()

	userID := uuid.New()
	require.NoError(t, e.memberRepo.CreateMember(e.ctx, domain.NewMemberFromInvite(groupID, userID)))

	return userID
}

// addAdmin makes a new account a confirmed member and has the creator promote
// it to admin.
func (e *sessionTeamTestEnv) addAdmin(t *testing.T, group *domain.ActivityGroup) uuid.UUID {
	t.Helper()

	userID := e.addMember(t, group.ID())
	require.NoError(t, e.changeRole(group, group.CreatorID(), "creator", userID, true))

	return userID
}

// changeRole promotes (toAdmin) or demotes userID, as the HTTP handler does
// with the requester's role read from their membership.
func (e *sessionTeamTestEnv) changeRole(group *domain.ActivityGroup, requester uuid.UUID, requesterRole string, userID uuid.UUID, toAdmin bool) error {
	params := application.UpdateMemberRoleParams{
		ActivityGroupID: group.ID(),
		RequesterID:     requester,
		RequesterRole:   requesterRole,
		UserID:          userID,
	}
	if toAdmin {
		return e.memberSvc.PromoteMember(e.ctx, params)
	}
	return e.memberSvc.DemoteMember(e.ctx, params)
}

func (e *sessionTeamTestEnv) roleOf(t *testing.T, groupID, userID uuid.UUID) domain.MemberRole {
	t.Helper()

	member, err := e.memberSvc.GetMember(e.ctx, groupID, userID)
	require.NoError(t, err)

	return member.Role()
}

func TestMemberService_PromoteMember(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createGroup(t, creator)
	promote := func(requester uuid.UUID, role string, userID uuid.UUID) error {
		return env.changeRole(group, requester, role, userID, true)
	}

	t.Run("only the creator may", func(t *testing.T) {
		admin := env.addAdmin(t, group)
		member := env.addMember(t, group.ID())

		assert.ErrorIs(t, promote(admin, "admin", member), domain.ErrUnauthorized)
		assert.Equal(t, domain.MemberRoleMember, env.roleOf(t, group.ID(), member))
	})

	t.Run("an invalid requester role is rejected", func(t *testing.T) {
		assert.Error(t, promote(creator, "owner", env.addMember(t, group.ID())))
	})

	t.Run("someone who isn't a member can't be promoted", func(t *testing.T) {
		assert.ErrorIs(t, promote(creator, "creator", uuid.New()), domain.ErrMemberNotFound)
	})

	t.Run("a pending join request can't be promoted", func(t *testing.T) {
		requester := uuid.New()
		require.NoError(t, env.memberRepo.CreateMember(env.ctx, domain.NewJoinRequest(group.ID(), requester, nil)))

		assert.ErrorIs(t, promote(creator, "creator", requester), domain.ErrNotConfirmed)
	})

	t.Run("the creator's own role can't change", func(t *testing.T) {
		assert.ErrorIs(t, promote(creator, "creator", creator), domain.ErrCannotDemoteCreator)
	})

	t.Run("the creator promotes a member, once", func(t *testing.T) {
		member := env.addMember(t, group.ID())
		roleChanges := env.outboxCount(t, domain.EventActivityGroupMemberRoleChanged)

		require.NoError(t, promote(creator, "creator", member))
		assert.Equal(t, domain.MemberRoleAdmin, env.roleOf(t, group.ID(), member))
		assert.Equal(t, roleChanges+1, env.outboxCount(t, domain.EventActivityGroupMemberRoleChanged))

		assert.ErrorIs(t, promote(creator, "creator", member), domain.ErrMemberAlreadyAdmin)
	})
}

func TestMemberService_DemoteMember(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createGroup(t, creator)
	demote := func(requester uuid.UUID, role string, userID uuid.UUID) error {
		return env.changeRole(group, requester, role, userID, false)
	}

	t.Run("an admin may not demote another admin", func(t *testing.T) {
		admin, other := env.addAdmin(t, group), env.addAdmin(t, group)

		assert.ErrorIs(t, demote(admin, "admin", other), domain.ErrUnauthorized)
		assert.Equal(t, domain.MemberRoleAdmin, env.roleOf(t, group.ID(), other))
	})

	t.Run("a plain member has nothing to be demoted from", func(t *testing.T) {
		assert.ErrorIs(t, demote(creator, "creator", env.addMember(t, group.ID())), domain.ErrMemberNotAdmin)
	})

	t.Run("the creator can't be demoted", func(t *testing.T) {
		assert.ErrorIs(t, demote(creator, "creator", creator), domain.ErrCannotDemoteCreator)
	})

	t.Run("the creator demotes an admin", func(t *testing.T) {
		admin := env.addAdmin(t, group)
		roleChanges := env.outboxCount(t, domain.EventActivityGroupMemberRoleChanged)

		require.NoError(t, demote(creator, "creator", admin))
		assert.Equal(t, domain.MemberRoleMember, env.roleOf(t, group.ID(), admin))
		assert.Equal(t, roleChanges+1, env.outboxCount(t, domain.EventActivityGroupMemberRoleChanged))
	})
}

func TestMemberService_GetMember(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createGroup(t, creator)

	t.Run("the creator is a confirmed member", func(t *testing.T) {
		member, err := env.memberSvc.GetMember(env.ctx, group.ID(), creator)
		require.NoError(t, err)
		assert.Equal(t, domain.MemberRoleCreator, member.Role())
		assert.True(t, member.IsConfirmed())
	})

	t.Run("anyone else is not found", func(t *testing.T) {
		_, err := env.memberSvc.GetMember(env.ctx, group.ID(), uuid.New())
		assert.ErrorIs(t, err, domain.ErrMemberNotFound)
	})
}

func TestMemberService_ListMembers(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createGroup(t, creator)
	admin := env.addAdmin(t, group)
	member := env.addMember(t, group.ID())
	requester := uuid.New()
	require.NoError(t, env.memberRepo.CreateMember(env.ctx, domain.NewJoinRequest(group.ID(), requester, nil)))
	env.addMember(t, env.createGroup(t, uuid.New()).ID()) // another group's member

	list := func(t *testing.T, params application.ListMembersParams) []uuid.UUID {
		t.Helper()
		params.ActivityGroupID = testutil.Ptr(group.ID())
		members, _, err := env.memberSvc.ListMembers(env.ctx, params)
		require.NoError(t, err)
		ids := make([]uuid.UUID, len(members))
		for i, m := range members {
			ids[i] = m.UserID()
		}
		return ids
	}

	t.Run("every member of the group, pending included", func(t *testing.T) {
		assert.ElementsMatch(t, []uuid.UUID{creator, admin, member, requester}, list(t, application.ListMembersParams{}))
	})

	t.Run("by status", func(t *testing.T) {
		assert.Equal(t, []uuid.UUID{requester}, list(t, application.ListMembersParams{Status: testutil.Ptr("pending")}))
	})

	t.Run("by role", func(t *testing.T) {
		assert.Equal(t, []uuid.UUID{admin}, list(t, application.ListMembersParams{Role: testutil.Ptr("admin")}))
	})

	t.Run("page by page", func(t *testing.T) {
		var seen []uuid.UUID
		params := application.ListMembersParams{ActivityGroupID: testutil.Ptr(group.ID()), Limit: 3}
		for range 3 {
			page, next, err := env.memberSvc.ListMembers(env.ctx, params)
			require.NoError(t, err)
			for _, m := range page {
				seen = append(seen, m.UserID())
			}
			if next == "" {
				break
			}
			params.PageToken = next
		}
		assert.ElementsMatch(t, []uuid.UUID{creator, admin, member, requester}, seen, "each member exactly once")
	})
}

func TestMemberService_RemoveMember(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createGroup(t, creator)
	remove := func(requester uuid.UUID, role string, userID uuid.UUID) error {
		return env.memberSvc.RemoveMember(env.ctx, application.RemoveMemberParams{
			ActivityGroupID: group.ID(), RequesterID: requester, RequesterRole: role, UserID: userID,
		})
	}
	admin := env.addAdmin(t, group)

	t.Run("a plain member may not", func(t *testing.T) {
		member := env.addMember(t, group.ID())
		assert.ErrorIs(t, remove(env.addMember(t, group.ID()), "member", member), domain.ErrUnauthorized)
	})

	t.Run("an admin may not remove another admin", func(t *testing.T) {
		assert.ErrorIs(t, remove(admin, "admin", env.addAdmin(t, group)), domain.ErrUnauthorized)
	})

	t.Run("nobody removes the creator", func(t *testing.T) {
		assert.ErrorIs(t, remove(admin, "admin", creator), domain.ErrCreatorCannotLeave)
	})

	t.Run("an admin removes a member", func(t *testing.T) {
		member := env.addMember(t, group.ID())

		require.NoError(t, remove(admin, "admin", member))
		removed, err := env.memberSvc.GetMember(env.ctx, group.ID(), member)
		require.NoError(t, err)
		assert.Equal(t, domain.MemberStatusRemoved, removed.Status())
		assert.Equal(t, 1, env.outboxCount(t, domain.EventActivityGroupMemberRemoved))
	})
}

func TestMemberService_RejectMember(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createGroup(t, creator)
	reject := func(requester uuid.UUID, role string, userID uuid.UUID) error {
		return env.memberSvc.RejectMember(env.ctx, application.RejectMemberParams{
			ActivityGroupID: group.ID(), RequesterID: requester, RequesterRole: role, UserID: userID,
		})
	}
	requestJoin := func(t *testing.T) uuid.UUID {
		t.Helper()
		userID := uuid.New()
		require.NoError(t, env.memberRepo.CreateMember(env.ctx, domain.NewJoinRequest(group.ID(), userID, nil)))
		return userID
	}

	t.Run("a plain member may not", func(t *testing.T) {
		assert.ErrorIs(t, reject(env.addMember(t, group.ID()), "member", requestJoin(t)), domain.ErrUnauthorized)
	})

	t.Run("only a pending request can be rejected", func(t *testing.T) {
		assert.ErrorIs(t, reject(creator, "creator", env.addMember(t, group.ID())), domain.ErrNotPendingStatus)
	})

	t.Run("an admin rejects a request", func(t *testing.T) {
		requester := requestJoin(t)

		require.NoError(t, reject(env.addAdmin(t, group), "admin", requester))
		rejected, err := env.memberSvc.GetMember(env.ctx, group.ID(), requester)
		require.NoError(t, err)
		assert.Equal(t, domain.MemberStatusRejected, rejected.Status())
		assert.Equal(t, 1, env.outboxCount(t, domain.EventActivityGroupMemberRejected))
	})
}

func TestMemberService_LeaveMember(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createGroup(t, creator)

	t.Run("the creator can't leave their group", func(t *testing.T) {
		assert.ErrorIs(t, env.memberSvc.LeaveMember(env.ctx, group.ID(), creator), domain.ErrCreatorCannotLeave)
	})

	t.Run("someone who isn't a member has nothing to leave", func(t *testing.T) {
		assert.ErrorIs(t, env.memberSvc.LeaveMember(env.ctx, group.ID(), uuid.New()), domain.ErrMemberNotFound)
	})

	t.Run("a member leaves", func(t *testing.T) {
		member := env.addMember(t, group.ID())

		require.NoError(t, env.memberSvc.LeaveMember(env.ctx, group.ID(), member))
		left, err := env.memberSvc.GetMember(env.ctx, group.ID(), member)
		require.NoError(t, err)
		assert.Equal(t, domain.MemberStatusLeft, left.Status())
		assert.Equal(t, 1, env.outboxCount(t, domain.EventActivityGroupMemberLeft))
	})
}
