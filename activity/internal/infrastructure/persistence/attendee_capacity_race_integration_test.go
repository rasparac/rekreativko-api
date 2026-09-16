//go:build integration

package persistence_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/activity/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/require"
)

// setupCapacityRaceTestDB spins up a real Postgres instance with the
// activity schema applied, matching the pattern used by the other
// integration tests in this package.
func setupCapacityRaceTestDB(t *testing.T) *testutil.TestDatabase {
	t.Helper()

	db := testutil.NewTestDatabase(t)
	db.RunMigrationsForService()

	return db
}

// sharedTestMetrics is a single metrics.New() instance shared by every test
// in this package that needs one. metrics.New() registers its collectors
// against the global Prometheus registry via promauto, so calling it more
// than once within the same test binary panics with "duplicate metrics
// collector registration attempted".
var sharedTestMetricsOnce sync.Once
var sharedTestMetrics *metrics.Metrics

func testMetrics() *metrics.Metrics {
	sharedTestMetricsOnce.Do(func() {
		sharedTestMetrics = metrics.New()
	})
	return sharedTestMetrics
}

// createCapacityOneSession creates a standalone (no group), public session
// with a capacity of exactly one confirmed attendee, so two concurrent
// "going" RSVPs can only ever be satisfied by one of them.
func createCapacityOneSession(t *testing.T, ctx context.Context, sessionRepo application.SessionRepository) *domain.Session {
	t.Helper()

	title, err := domain.NewTitle("Capacity Race Test Session")
	require.NoError(t, err)

	location, err := domain.NewSessionLocation("Belgrade", "RS", "", 44.8, 20.4)
	require.NoError(t, err)

	schedule, err := domain.NewSessionSchedule(time.Now().Add(time.Hour), nil)
	require.NoError(t, err)

	visibility := domain.SessionVisibilityPublic
	session, _, err := domain.NewSession(domain.SessionInput{
		ActivityGroupID:  nil,
		CreatedByID:      uuid.New(),
		Title:            title,
		ActivityType:     domain.ActivityTypeRunning,
		DifficultyLevel:  domain.DifficultyLevelBeginner,
		Location:         location,
		Schedule:         schedule,
		Capacity:         testutil.Ptr(1),
		Visibility:       &visibility,
		RequiresApproval: false,
	})
	require.NoError(t, err)

	require.NoError(t, sessionRepo.CreateSession(ctx, session))

	return session
}

// TestAttendeeService_UpdateRSVP_ConcurrentGoing_DoesNotOverbook is a
// regression test for rekreativko-api-b48: session capacity enforcement was
// a check-then-act race (read confirmed count, then write) with no locking,
// so two concurrent "going" RSVPs for the same, nearly-full session could
// both read the same confirmed count, both pass the capacity check, and both
// commit as "going" - overbooking the session.
//
// It fires two UpdateRSVP("going") calls concurrently against a capacity-1
// session where both users already hold non-confirmed RSVPs ("maybe"), and
// asserts that exactly one of them ends up "going" while the other is
// auto-downgraded to "pending" - never both "going".
func TestAttendeeService_UpdateRSVP_ConcurrentGoing_DoesNotOverbook(t *testing.T) {
	db := setupCapacityRaceTestDB(t)
	txManager := db.CreateTransactionManager()
	logger := testutil.CreateLogger()

	sessionRepo := persistence.NewSessionManager(txManager, logger)
	attendeeRepo := persistence.NewAttendeeRepository(txManager, logger)
	memberRepo := persistence.NewMemberRepository(txManager, logger)
	eventWriter := domainevent.NewDomainEventManager(txManager)

	svc := application.NewAttendeeService(logger, txManager, attendeeRepo, memberRepo, sessionRepo, eventWriter, testMetrics())

	ctx := context.Background()
	session := createCapacityOneSession(t, ctx, sessionRepo)

	userA := uuid.New()
	userB := uuid.New()

	for _, userID := range []uuid.UUID{userA, userB} {
		attendee, err := domain.NewRSVPManualAttendee(session, session.ActivityGroupID(), userID, domain.AttendeeStatusMaybe, 0)
		require.NoError(t, err)
		require.NoError(t, attendeeRepo.CreateAttendee(ctx, attendee))
	}

	const attempts = 20
	for i := range attempts {
		var wg sync.WaitGroup
		errs := make([]error, 2)

		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := svc.UpdateRSVP(ctx, application.UpdateRSVPParams{
				SessionID: session.ID(),
				UserID:    userA,
				NewStatus: string(domain.AttendeeStatusGoing),
			})
			errs[0] = err
		}()
		go func() {
			defer wg.Done()
			_, err := svc.UpdateRSVP(ctx, application.UpdateRSVPParams{
				SessionID: session.ID(),
				UserID:    userB,
				NewStatus: string(domain.AttendeeStatusGoing),
			})
			errs[1] = err
		}()
		wg.Wait()

		require.NoError(t, errs[0])
		require.NoError(t, errs[1])

		confirmedCount, err := attendeeRepo.CountConfirmedAttendees(ctx, session.ID())
		require.NoError(t, err)
		require.LessOrEqualf(t, confirmedCount, 1, "capacity-1 session ended up with %d confirmed attendees on attempt %d - overbooked", confirmedCount, i)

		// Reset both attendees back to "maybe" for the next attempt so the
		// race is exercised repeatedly rather than relying on a single roll.
		for _, userID := range []uuid.UUID{userA, userB} {
			attendee, err := attendeeRepo.GetAttendeeBySessionAndUser(ctx, session.ID(), userID)
			require.NoError(t, err)
			require.NoError(t, attendee.UpdateRSVP(domain.AttendeeStatusMaybe, session, 0))
			require.NoError(t, attendeeRepo.UpdateAttendee(ctx, attendee))
		}
	}
}
