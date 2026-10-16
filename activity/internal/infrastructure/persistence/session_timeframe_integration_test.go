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

// timeframeFixture holds one public standalone session per case, by how they
// relate to now. Without an end time a session ends 3h after it starts.
type timeframeFixture struct {
	creator uuid.UUID

	upcoming         *domain.Session // starts in 1h, no end time
	running          *domain.Session // started 1h ago, no end time: ends in 2h
	runningWithEnd   *domain.Session // started 2h ago, ends in 1h
	overNoEnd        *domain.Session // started 4h ago, no end time: ended 1h ago
	overWithEnd      *domain.Session // started 5h ago, ended 4h ago
	justOverWithEnd  *domain.Session // started 30m ago, ended 10m ago
	allInStartOrder  []uuid.UUID
	upcomingIDs      []uuid.UUID
	pastMostRecentID []uuid.UUID
}

func (e *sessionTeamTestEnv) timeframeFixture(t *testing.T) *timeframeFixture {
	t.Helper()

	f := &timeframeFixture{creator: uuid.New()}

	place := func(startAgo time.Duration, endIn *time.Duration) *domain.Session {
		s := e.createSessionIn(t, nil, f.creator)

		end := "NULL"
		args := []any{s.ID(), startAgo.Seconds()}
		if endIn != nil {
			end = "now() + make_interval(secs => $3)"
			args = append(args, endIn.Seconds())
		}
		_, err := e.txManager.Querier(e.ctx).Exec(e.ctx,
			`UPDATE activity.session SET start_time = now() - make_interval(secs => $2), end_time = `+end+` WHERE id = $1`,
			args...,
		)
		require.NoError(t, err)

		return s
	}
	in := func(d time.Duration) *time.Duration { return &d }

	f.overWithEnd = place(5*time.Hour, in(-4*time.Hour))
	f.overNoEnd = place(4*time.Hour, nil)
	f.runningWithEnd = place(2*time.Hour, in(time.Hour))
	f.running = place(time.Hour, nil)
	f.justOverWithEnd = place(30*time.Minute, in(-10*time.Minute))
	f.upcoming = place(-time.Hour, nil)

	ids := func(ss ...*domain.Session) []uuid.UUID {
		out := make([]uuid.UUID, len(ss))
		for i, s := range ss {
			out[i] = s.ID()
		}
		return out
	}
	f.allInStartOrder = ids(f.overWithEnd, f.overNoEnd, f.runningWithEnd, f.running, f.justOverWithEnd, f.upcoming)
	f.upcomingIDs = ids(f.runningWithEnd, f.running, f.upcoming)
	f.pastMostRecentID = ids(f.justOverWithEnd, f.overNoEnd, f.overWithEnd)

	return f
}

func sessionIDs(sessions []*domain.Session) []uuid.UUID {
	out := make([]uuid.UUID, len(sessions))
	for i, s := range sessions {
		out[i] = s.ID()
	}
	return out
}

func TestSessionService_ListSessions_Timeframe(t *testing.T) {
	env := setupSessionTeamTest(t)
	f := env.timeframeFixture(t)

	list := func(when string, limit int, token string) ([]*domain.Session, string) {
		t.Helper()
		sessions, next, _, err := env.sessionSvc.ListSessions(env.ctx,
			application.ListSessionsParams{CreatedByID: &f.creator, When: when, Limit: limit, PageToken: token}, f.creator)
		require.NoError(t, err)
		return sessions, next
	}

	t.Run("upcoming is the default: not ended yet, soonest first", func(t *testing.T) {
		got, _ := list("", 50, "")
		assert.Equal(t, f.upcomingIDs, sessionIDs(got))

		got, _ = list("upcoming", 50, "")
		assert.Equal(t, f.upcomingIDs, sessionIDs(got))
	})

	t.Run("past: ended, most recent first", func(t *testing.T) {
		got, _ := list("past", 50, "")
		assert.Equal(t, f.pastMostRecentID, sessionIDs(got))
	})

	t.Run("past pages by start time, newest first", func(t *testing.T) {
		first, next := list("past", 2, "")
		require.NotEmpty(t, next)
		second, next := list("past", 2, next)
		assert.Empty(t, next)

		assert.Equal(t, f.pastMostRecentID, append(sessionIDs(first), sessionIDs(second)...))
	})

	t.Run("all: everything, soonest first", func(t *testing.T) {
		got, _ := list("all", 50, "")
		assert.Equal(t, f.allInStartOrder, sessionIDs(got))
	})

	t.Run("an unknown value is rejected", func(t *testing.T) {
		_, _, _, err := env.sessionSvc.ListSessions(env.ctx,
			application.ListSessionsParams{When: "yesterday", Limit: 10}, f.creator)
		requireAppErrorCode(t, err, "invalid_session_timeframe")
	})
}

func TestSessionService_DiscoverSessions_SkipsSessionsWithoutEndTimeOnceOver(t *testing.T) {
	env := setupSessionTeamTest(t)
	f := env.timeframeFixture(t)

	got, _, _, err := env.sessionSvc.DiscoverSessions(env.ctx,
		application.DiscoverSessionsParams{Lat: 44.8, Lng: 20.4, RadiusKM: 50, Limit: 50}, uuid.New())
	require.NoError(t, err)

	ids := make([]uuid.UUID, len(got))
	for i, r := range got {
		ids[i] = r.Session.ID()
	}
	assert.ElementsMatch(t, f.upcomingIDs, ids)
}

func TestSessionService_ExpireCompletedSessions_CompletesEndedSessionsWithoutEndTime(t *testing.T) {
	env := setupSessionTeamTest(t)
	f := env.timeframeFixture(t)

	expired, err := env.sessionSvc.ExpireCompletedSessions(env.ctx)
	require.NoError(t, err)
	assert.Equal(t, len(f.pastMostRecentID), expired)

	for _, id := range f.pastMostRecentID {
		s, err := env.sessionRepo.GetSessionByID(env.ctx, id)
		require.NoError(t, err)
		assert.Equal(t, domain.SessionStatusCompleted, s.Status())
	}
	for _, id := range f.upcomingIDs {
		s, err := env.sessionRepo.GetSessionByID(env.ctx, id)
		require.NoError(t, err)
		assert.Equal(t, domain.SessionStatusScheduled, s.Status())
	}

	// A second sweep (another instance, or the next tick) finds nothing.
	expired, err = env.sessionSvc.ExpireCompletedSessions(env.ctx)
	require.NoError(t, err)
	assert.Zero(t, expired)
}
