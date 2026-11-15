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

func (e *sessionTeamTestEnv) sendInvite(group *domain.ActivityGroup, invitee uuid.UUID) (*domain.GroupInvite, error) {
	return e.inviteSvc.SendInvite(e.ctx, application.SendInviteParams{
		ActivityGroupID: group.ID(),
		RequesterID:     group.CreatorID(),
		RequesterRole:   "creator",
		InvitedUserID:   invitee,
	})
}

func (e *sessionTeamTestEnv) inviteStatus(t *testing.T, invite *domain.GroupInvite) domain.InviteStatus {
	t.Helper()

	stored, err := e.inviteRepo.GetInviteByID(e.ctx, invite.ID())
	require.NoError(t, err)

	return stored.Status()
}

// storeInvite writes an invite as it would be after createdAt, bypassing the
// service, so it can already be past its expiry.
func (e *sessionTeamTestEnv) storeInvite(t *testing.T, group *domain.ActivityGroup, invitee uuid.UUID, createdAt, expiresAt time.Time) *domain.GroupInvite {
	t.Helper()

	invite := domain.ReconstructGroupInvite(uuid.New(), group.ID(), invitee, group.CreatorID(), domain.InviteStatusPending, createdAt, expiresAt, nil)
	require.NoError(t, e.inviteRepo.CreateInvite(e.ctx, invite))

	return invite
}

func TestInviteService_SendInvite(t *testing.T) {
	env := setupSessionTeamTest(t)
	group := env.createGroup(t, uuid.New())

	t.Run("a plain member may not invite", func(t *testing.T) {
		_, err := env.inviteSvc.SendInvite(env.ctx, application.SendInviteParams{
			ActivityGroupID: group.ID(),
			RequesterID:     env.addMember(t, group.ID()),
			RequesterRole:   "member",
			InvitedUserID:   uuid.New(),
		})
		assert.ErrorIs(t, err, domain.ErrUnauthorized)
	})

	t.Run("an existing member can't be invited", func(t *testing.T) {
		_, err := env.sendInvite(group, env.addMember(t, group.ID()))
		assert.ErrorIs(t, err, domain.ErrUserAlreadyMember)
	})

	t.Run("the creator invites someone, once", func(t *testing.T) {
		invitee := uuid.New()

		invite, err := env.sendInvite(group, invitee)
		require.NoError(t, err)
		assert.Equal(t, domain.InviteStatusPending, env.inviteStatus(t, invite))
		assert.Equal(t, 1, env.outboxCount(t, domain.EventActivityInviteSent))

		_, err = env.sendInvite(group, invitee)
		assert.ErrorIs(t, err, domain.ErrAlreadyInvited)
	})
}

func TestInviteService_AcceptInvite(t *testing.T) {
	env := setupSessionTeamTest(t)
	group := env.createGroup(t, uuid.New())
	invitee := uuid.New()
	invite, err := env.sendInvite(group, invitee)
	require.NoError(t, err)

	t.Run("only the invitee may", func(t *testing.T) {
		_, err := env.inviteSvc.AcceptInvite(env.ctx, invite.ID(), uuid.New())
		assert.ErrorIs(t, err, domain.ErrUnauthorized)
	})

	t.Run("an unknown invite is not found", func(t *testing.T) {
		_, err := env.inviteSvc.AcceptInvite(env.ctx, uuid.New(), invitee)
		assert.ErrorIs(t, err, domain.ErrInviteNotFound)
	})

	t.Run("an expired invite can't be accepted", func(t *testing.T) {
		late := uuid.New()
		expired := env.storeInvite(t, group, late, time.Now().Add(-8*24*time.Hour), time.Now().Add(-time.Hour))

		_, err := env.inviteSvc.AcceptInvite(env.ctx, expired.ID(), late)
		assert.ErrorIs(t, err, domain.ErrInviteExpired)
		_, err = env.memberSvc.GetMember(env.ctx, group.ID(), late, late)
		assert.ErrorIs(t, err, domain.ErrMemberNotFound)
	})

	t.Run("the invitee becomes a confirmed member", func(t *testing.T) {
		member, err := env.inviteSvc.AcceptInvite(env.ctx, invite.ID(), invitee)
		require.NoError(t, err)
		assert.True(t, member.IsConfirmed())
		assert.Equal(t, domain.InviteStatusAccepted, env.inviteStatus(t, invite))
		assert.Equal(t, domain.MemberRoleMember, env.roleOf(t, group.ID(), invitee))
		assert.Equal(t, 1, env.outboxCount(t, domain.EventActivityInviteAccepted))
	})

	t.Run("an answered invite can't be accepted again", func(t *testing.T) {
		_, err := env.inviteSvc.AcceptInvite(env.ctx, invite.ID(), invitee)
		assert.ErrorIs(t, err, domain.ErrInviteAlreadyProcessed)
	})
}

func TestInviteService_DeclineInvite(t *testing.T) {
	env := setupSessionTeamTest(t)
	group := env.createGroup(t, uuid.New())
	invitee := uuid.New()
	invite, err := env.sendInvite(group, invitee)
	require.NoError(t, err)

	t.Run("only the invitee may", func(t *testing.T) {
		assert.ErrorIs(t, env.inviteSvc.DeclineInvite(env.ctx, invite.ID(), uuid.New()), domain.ErrUnauthorized)
	})

	t.Run("the invitee declines, and does not join", func(t *testing.T) {
		require.NoError(t, env.inviteSvc.DeclineInvite(env.ctx, invite.ID(), invitee))
		assert.Equal(t, domain.InviteStatusDeclined, env.inviteStatus(t, invite))
		assert.Equal(t, 1, env.outboxCount(t, domain.EventActivityInviteDeclined))

		_, err := env.memberSvc.GetMember(env.ctx, group.ID(), invitee, invitee)
		assert.ErrorIs(t, err, domain.ErrMemberNotFound)
	})

	t.Run("a declined invite can't be declined again", func(t *testing.T) {
		assert.ErrorIs(t, env.inviteSvc.DeclineInvite(env.ctx, invite.ID(), invitee), domain.ErrInviteAlreadyProcessed)
	})
}

func TestInviteService_ExpireStaleInvites(t *testing.T) {
	env := setupSessionTeamTest(t)
	group := env.createGroup(t, uuid.New())
	now := time.Now().UTC()

	stale := env.storeInvite(t, group, uuid.New(), now.Add(-8*24*time.Hour), now.Add(-time.Hour))
	fresh, err := env.sendInvite(group, uuid.New())
	require.NoError(t, err)
	answered, err := env.sendInvite(group, uuid.New())
	require.NoError(t, err)
	require.NoError(t, env.inviteSvc.DeclineInvite(env.ctx, answered.ID(), answered.InvitedUserID()))

	t.Run("only pending invites past their expiry", func(t *testing.T) {
		expired, err := env.inviteSvc.ExpireStaleInvites(env.ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, expired)

		assert.Equal(t, domain.InviteStatusExpired, env.inviteStatus(t, stale))
		assert.Equal(t, domain.InviteStatusPending, env.inviteStatus(t, fresh))
		assert.Equal(t, domain.InviteStatusDeclined, env.inviteStatus(t, answered))
		assert.Equal(t, 1, env.outboxCount(t, domain.EventActivityInviteExpired))
	})

	t.Run("the next run finds nothing", func(t *testing.T) {
		expired, err := env.inviteSvc.ExpireStaleInvites(env.ctx)
		require.NoError(t, err)
		assert.Zero(t, expired)
	})

	t.Run("an expired invite no longer blocks a new one", func(t *testing.T) {
		_, err := env.sendInvite(group, stale.InvitedUserID())
		assert.NoError(t, err)
	})
}

func TestInviteService_ListMyInvites(t *testing.T) {
	env := setupSessionTeamTest(t)
	invitee := uuid.New()
	now := time.Now().UTC()

	older := env.storeInvite(t, env.createGroup(t, uuid.New()), invitee, now.Add(-2*time.Hour), now.Add(time.Hour))
	newer, err := env.sendInvite(env.createGroup(t, uuid.New()), invitee)
	require.NoError(t, err)
	declined, err := env.sendInvite(env.createGroup(t, uuid.New()), invitee)
	require.NoError(t, err)
	require.NoError(t, env.inviteSvc.DeclineInvite(env.ctx, declined.ID(), invitee))
	_, err = env.sendInvite(env.createGroup(t, uuid.New()), uuid.New()) // someone else's
	require.NoError(t, err)

	t.Run("my pending invites, newest first", func(t *testing.T) {
		listed, err := env.inviteSvc.ListMyInvites(env.ctx, invitee)
		require.NoError(t, err)

		ids := make([]uuid.UUID, len(listed))
		for i, invite := range listed {
			ids[i] = invite.ID()
		}
		assert.Equal(t, []uuid.UUID{newer.ID(), older.ID()}, ids)
	})

	t.Run("nothing for someone never invited", func(t *testing.T) {
		listed, err := env.inviteSvc.ListMyInvites(env.ctx, uuid.New())
		require.NoError(t, err)
		assert.Empty(t, listed)
	})
}
