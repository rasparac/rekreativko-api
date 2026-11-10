//go:build integration

package persistence_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createCapacityOneGroup creates a public activity group with a default
// capacity of exactly one confirmed member.
func createCapacityOneGroup(t *testing.T, ctx context.Context, groupRepo application.ActivityGroupRepository) *domain.ActivityGroup {
	t.Helper()

	title, err := domain.NewTitle("Capacity Race Test Group")
	require.NoError(t, err)

	location, err := domain.NewActivityGroupLocation("Belgrade", "RS")
	require.NoError(t, err)

	capacity, err := domain.NewCapacity(1)
	require.NoError(t, err)

	group, err := domain.NewActivityGroup(
		uuid.New(),
		title,
		"Test description",
		domain.ActivityTypeRunning,
		location,
		domain.DifficultyLevelBeginner,
		domain.ActivityGroupVisibilityPublic,
		"UTC",
		&capacity,
	)
	require.NoError(t, err)

	require.NoError(t, groupRepo.CreateActivityGroup(ctx, group))

	return group
}

// Decided in rekreativko-api-6ev: someone who left or was rejected can ask to
// join again; someone a manager removed can't. Before, the old member row made
// every new request fail with already_participating.
func TestMemberService_RequestToJoinAgain(t *testing.T) {
	db := setupCapacityRaceTestDB(t)
	txManager := db.CreateTransactionManager()
	logger := testutil.CreateLogger()

	groupRepo := persistence.NewActivityGroupRepository(txManager, logger)
	memberRepo := persistence.NewMemberRepository(txManager, logger)
	eventWriter := domainevent.NewDomainEventManager(txManager, logger)
	svc := application.NewMemberService(logger, txManager, memberRepo, groupRepo, persistence.NewAccountStateManager(txManager), eventWriter, testMetrics())

	ctx := context.Background()
	title, err := domain.NewTitle("Rejoin Test Group")
	require.NoError(t, err)
	location, err := domain.NewActivityGroupLocation("Belgrade", "RS")
	require.NoError(t, err)
	group, err := domain.NewActivityGroup(uuid.New(), title, "", domain.ActivityTypeRunning, location,
		domain.DifficultyLevelBeginner, domain.ActivityGroupVisibilityPublic, "UTC", nil)
	require.NoError(t, err)
	require.NoError(t, group.Activate(group.CreatorID()))
	require.NoError(t, groupRepo.CreateActivityGroup(ctx, group))

	manage := func(userID uuid.UUID) (uuid.UUID, uuid.UUID, string, uuid.UUID) {
		return group.ID(), group.CreatorID(), "creator", userID
	}
	request := func(userID uuid.UUID) (*domain.Member, error) {
		return svc.RequestToJoinGroup(ctx, application.RequestToJoinGroupParams{ActivityGroupID: group.ID(), UserID: userID})
	}
	approve := func(userID uuid.UUID) error {
		g, by, role, u := manage(userID)
		return svc.ApproveMember(ctx, application.ApproveMemberParams{ActivityGroupID: g, RequesterID: by, RequesterRole: role, UserID: u})
	}
	joined := func(t *testing.T, userID uuid.UUID) *domain.Member {
		t.Helper()
		first, err := request(userID)
		require.NoError(t, err)
		require.NoError(t, approve(userID))
		return first
	}

	t.Run("after leaving", func(t *testing.T) {
		userID := uuid.New()
		first := joined(t, userID)
		require.NoError(t, svc.LeaveMember(ctx, group.ID(), userID))

		again, err := request(userID)

		require.NoError(t, err)
		assert.Equal(t, first.ID(), again.ID(), "the same row, re-opened")
		stored, err := memberRepo.GetMemberByGroupAndUser(ctx, group.ID(), userID)
		require.NoError(t, err)
		assert.True(t, stored.IsPending())
		assert.True(t, stored.JoinedAt().After(first.JoinedAt()), "queued as a new request")

		require.NoError(t, approve(userID), "and can be approved again")
	})

	t.Run("after being rejected", func(t *testing.T) {
		userID := uuid.New()
		_, err := request(userID)
		require.NoError(t, err)
		g, by, role, u := manage(userID)
		require.NoError(t, svc.RejectMember(ctx, application.RejectMemberParams{ActivityGroupID: g, RequesterID: by, RequesterRole: role, UserID: u}))

		_, err = request(userID)

		assert.NoError(t, err)
	})

	t.Run("not after a manager removed them", func(t *testing.T) {
		userID := uuid.New()
		joined(t, userID)
		g, by, role, u := manage(userID)
		require.NoError(t, svc.RemoveMember(ctx, application.RemoveMemberParams{ActivityGroupID: g, RequesterID: by, RequesterRole: role, UserID: u}))

		_, err := request(userID)

		assert.ErrorIs(t, err, domain.ErrRemovedFromGroup)
	})

	t.Run("not while pending or confirmed", func(t *testing.T) {
		userID := uuid.New()
		_, err := request(userID)
		require.NoError(t, err)

		_, err = request(userID)
		assert.ErrorIs(t, err, domain.ErrAlreadyParticipating, "pending")

		require.NoError(t, approve(userID))
		_, err = request(userID)
		assert.ErrorIs(t, err, domain.ErrAlreadyParticipating, "confirmed")
	})
}

// TestMemberService_ApproveMember_Concurrent_DoesNotOverbook is a regression
// test for rekreativko-api-b8q: ApproveMember (and RequestToJoin) enforced
// ActivityGroup.DefaultCapacity() with the same check-then-act race that
// rekreativko-api-b48 fixed for session attendees - CountConfirmedMembers
// was read, then compared, then written, with no lock in between.
//
// It creates two pending join requests against a capacity-1 group, then
// fires two concurrent ApproveMember calls, asserting that exactly one
// succeeds while the other is rejected with ErrActivityGroupFull - never
// both confirmed.
func TestMemberService_ApproveMember_Concurrent_DoesNotOverbook(t *testing.T) {
	db := setupCapacityRaceTestDB(t)
	txManager := db.CreateTransactionManager()
	logger := testutil.CreateLogger()

	groupRepo := persistence.NewActivityGroupRepository(txManager, logger)
	memberRepo := persistence.NewMemberRepository(txManager, logger)
	eventWriter := domainevent.NewDomainEventManager(txManager, logger)

	svc := application.NewMemberService(logger, txManager, memberRepo, groupRepo, persistence.NewAccountStateManager(txManager), eventWriter, testMetrics())

	ctx := context.Background()
	group := createCapacityOneGroup(t, ctx, groupRepo)

	const attempts = 20
	for i := 0; i < attempts; i++ {
		userA := uuid.New()
		userB := uuid.New()

		for _, userID := range []uuid.UUID{userA, userB} {
			member := domain.NewJoinRequest(group.ID(), userID, nil)
			require.NoError(t, memberRepo.CreateMember(ctx, member))
		}

		var wg sync.WaitGroup
		errs := make([]error, 2)

		wg.Add(2)
		go func() {
			defer wg.Done()
			errs[0] = svc.ApproveMember(ctx, application.ApproveMemberParams{
				ActivityGroupID: group.ID(),
				RequesterID:     group.CreatorID(),
				RequesterRole:   "creator",
				UserID:          userA,
			})
		}()
		go func() {
			defer wg.Done()
			errs[1] = svc.ApproveMember(ctx, application.ApproveMemberParams{
				ActivityGroupID: group.ID(),
				RequesterID:     group.CreatorID(),
				RequesterRole:   "creator",
				UserID:          userB,
			})
		}()
		wg.Wait()

		successCount := 0
		for _, err := range errs {
			if err == nil {
				successCount++
				continue
			}
			require.Truef(t, errors.Is(err, domain.ErrActivityGroupFull), "unexpected error on attempt %d: %v", i, err)
		}
		require.Equalf(t, 1, successCount, "expected exactly one approval to succeed on attempt %d - group capacity is 1", i)

		// Clean up both members so the next attempt starts from a group with
		// zero confirmed members again, rather than accumulating confirmed
		// members across attempts (which would make every subsequent attempt
		// fail both approvals instead of exercising the race).
		for _, userID := range []uuid.UUID{userA, userB} {
			member, err := memberRepo.GetMemberByGroupAndUser(ctx, group.ID(), userID)
			require.NoError(t, err)
			require.NoError(t, memberRepo.DeleteMember(ctx, member.ID()))
		}
	}
}
