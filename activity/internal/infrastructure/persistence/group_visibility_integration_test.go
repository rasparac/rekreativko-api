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

// rekreativko-api-16j: a private group doesn't exist for anyone but its
// creator and confirmed members - and neither do its templates and members.
func TestPrivateGroup_TemplatesAndMembersAreHiddenFromStrangers(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator, stranger := uuid.New(), uuid.New()
	group := env.createActiveGroup(t, creator, domain.ActivityGroupVisibilityPrivate, "Belgrade")
	member := env.addMember(t, group.ID())
	template := env.createTemplate(t, group.ID(), creator)

	listTemplates := func(requester uuid.UUID) ([]*domain.SessionTemplate, error) {
		return env.templateSvc.ListSessionTemplates(env.ctx, application.ListSessionTemplatesParams{ActivityGroupID: testutil.Ptr(group.ID()), RequesterID: requester})
	}
	listMembers := func(requester uuid.UUID) ([]*domain.Member, error) {
		members, _, err := env.memberSvc.ListMembers(env.ctx, application.ListMembersParams{ActivityGroupID: testutil.Ptr(group.ID()), RequesterID: requester})
		return members, err
	}

	t.Run("a stranger finds nothing", func(t *testing.T) {
		_, err := env.templateSvc.GetSessionTemplate(env.ctx, template.ID(), stranger)
		assert.ErrorIs(t, err, domain.ErrSessionTemplateNotFound)

		_, err = listTemplates(stranger)
		assert.ErrorIs(t, err, domain.ErrActivityGroupNotFound)

		_, err = listMembers(stranger)
		assert.ErrorIs(t, err, domain.ErrActivityGroupNotFound)

		_, err = env.memberSvc.GetMember(env.ctx, group.ID(), member, stranger)
		assert.ErrorIs(t, err, domain.ErrMemberNotFound)
	})

	t.Run("exactly as if the group didn't exist", func(t *testing.T) {
		_, hidden := listMembers(stranger)
		_, _, missing := env.memberSvc.ListMembers(env.ctx, application.ListMembersParams{ActivityGroupID: testutil.Ptr(uuid.New()), RequesterID: stranger})
		requireAppErrorCode(t, hidden, "activity_group_not_found")
		requireAppErrorCode(t, missing, "activity_group_not_found")

		_, hidden = env.templateSvc.GetSessionTemplate(env.ctx, template.ID(), stranger)
		_, missing = env.templateSvc.GetSessionTemplate(env.ctx, uuid.New(), stranger)
		requireAppErrorCode(t, hidden, "session_template_not_found")
		requireAppErrorCode(t, missing, "session_template_not_found")
	})

	t.Run("a member sees them", func(t *testing.T) {
		_, err := env.templateSvc.GetSessionTemplate(env.ctx, template.ID(), member)
		require.NoError(t, err)

		templates, err := listTemplates(member)
		require.NoError(t, err)
		assert.Len(t, templates, 1)

		members, err := listMembers(member)
		require.NoError(t, err)
		assert.Len(t, members, 2)

		_, err = env.memberSvc.GetMember(env.ctx, group.ID(), creator, member)
		assert.NoError(t, err)
	})

	t.Run("someone who was removed still sees their own row, nothing else", func(t *testing.T) {
		removed := env.addMember(t, group.ID())
		require.NoError(t, env.memberSvc.RemoveMember(env.ctx, application.RemoveMemberParams{
			ActivityGroupID: group.ID(), RequesterID: creator, RequesterRole: "creator", UserID: removed,
		}))

		own, err := env.memberSvc.GetMember(env.ctx, group.ID(), removed, removed)
		require.NoError(t, err)
		assert.Equal(t, domain.MemberStatusRemoved, own.Status())

		_, err = listMembers(removed)
		assert.ErrorIs(t, err, domain.ErrActivityGroupNotFound)
	})
}

// Join requests, rejections and removals are the managers' business, even in
// a public group everyone may see.
func TestPublicGroup_OnlyManagersSeeMembersWhoArentConfirmed(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createActiveGroup(t, creator, domain.ActivityGroupVisibilityPublic, "Belgrade")
	admin := env.addAdmin(t, group)
	member := env.addMember(t, group.ID())
	requester := uuid.New()
	require.NoError(t, env.memberRepo.CreateMember(env.ctx, domain.NewJoinRequest(group.ID(), requester, nil)))

	list := func(requesterID uuid.UUID, status *string) ([]uuid.UUID, error) {
		members, _, err := env.memberSvc.ListMembers(env.ctx, application.ListMembersParams{
			ActivityGroupID: testutil.Ptr(group.ID()), RequesterID: requesterID, Status: status,
		})
		ids := make([]uuid.UUID, len(members))
		for i, m := range members {
			ids[i] = m.UserID()
		}
		return ids, err
	}

	for name, viewer := range map[string]uuid.UUID{"a member": member, "a stranger": uuid.New()} {
		t.Run(name+" sees confirmed members only", func(t *testing.T) {
			got, err := list(viewer, nil)
			require.NoError(t, err)
			assert.ElementsMatch(t, []uuid.UUID{creator, admin, member}, got)

			_, err = list(viewer, testutil.Ptr("pending"))
			assert.ErrorIs(t, err, domain.ErrInsufficientRole)

			_, err = env.memberSvc.GetMember(env.ctx, group.ID(), requester, viewer)
			assert.ErrorIs(t, err, domain.ErrMemberNotFound)
		})
	}

	t.Run("an admin sees the requests", func(t *testing.T) {
		got, err := list(admin, testutil.Ptr("pending"))
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{requester}, got)

		_, err = env.memberSvc.GetMember(env.ctx, group.ID(), requester, admin)
		assert.NoError(t, err)
	})

	t.Run("the requester sees their own request", func(t *testing.T) {
		own, err := env.memberSvc.GetMember(env.ctx, group.ID(), requester, requester)
		require.NoError(t, err)
		assert.True(t, own.IsPending())
	})
}
