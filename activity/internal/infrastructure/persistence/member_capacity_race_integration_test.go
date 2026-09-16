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
	eventWriter := domainevent.NewDomainEventManager(txManager)

	svc := application.NewMemberService(logger, txManager, memberRepo, groupRepo, eventWriter, testMetrics())

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
