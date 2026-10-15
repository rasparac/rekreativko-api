//go:build integration

package persistence_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An expired invite stays 'pending' until the expiry job runs; the list must
// not show it in the meantime (it would inflate the badge and fail on accept).
func TestGroupInviteRepository_ListPendingInvitesForUser_SkipsExpired(t *testing.T) {
	env := setupSessionTeamTest(t)
	repo := persistence.NewGroupInviteRepository(env.txManager, testutil.CreateLogger())

	invitee := uuid.New()
	now := time.Now().UTC()

	createInvite := func(status domain.InviteStatus, createdAt, expiresAt time.Time) *domain.GroupInvite {
		t.Helper()
		group := env.createGroup(t, uuid.New())
		invite := domain.ReconstructGroupInvite(uuid.New(), group.ID(), invitee, group.CreatorID(), status, createdAt, expiresAt, nil)
		require.NoError(t, repo.CreateInvite(env.ctx, invite))
		return invite
	}

	older := createInvite(domain.InviteStatusPending, now.Add(-2*time.Hour), now.Add(time.Hour))
	newer := createInvite(domain.InviteStatusPending, now.Add(-time.Hour), now.Add(time.Hour))
	createInvite(domain.InviteStatusPending, now.Add(-8*24*time.Hour), now.Add(-time.Hour)) // expired, not yet swept

	listed, nextPageToken, err := repo.ListPendingInvitesForUser(env.ctx, persistence.ListPendingInvitesFilter{
		InvitedUserID: invitee,
		Limit:         application.MaxListedInvites,
	})
	require.NoError(t, err)
	assert.Empty(t, nextPageToken)

	ids := make([]uuid.UUID, len(listed))
	for i, invite := range listed {
		ids[i] = invite.ID()
	}
	assert.Equal(t, []uuid.UUID{newer.ID(), older.ID()}, ids)
}

func TestSessionInviteService_ListMyInvites_CapsTheList(t *testing.T) {
	env := setupSessionInviteTest(t)
	invitee := uuid.New()

	for range application.MaxListedInvites + 1 {
		env.invite(t, env.createPrivateStandaloneSession(t, 5), invitee)
	}

	listed, err := env.svc.ListMyInvites(env.ctx, invitee)
	require.NoError(t, err)
	assert.Len(t, listed, application.MaxListedInvites)
}
