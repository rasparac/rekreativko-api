package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestLimiter returns a limiter on a clock the test moves.
func newTestLimiter(t *testing.T, limit RateLimit) (*RateLimiter, *time.Time) {
	t.Helper()

	rl, err := NewRateLimiter(limit)
	require.NoError(t, err)
	t.Cleanup(rl.Stop)

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	rl.now = func() time.Time { return now }

	return rl, &now
}

func TestNewRateLimiter_RejectsAnEmptyLimit(t *testing.T) {
	for _, limit := range []RateLimit{{}, {Burst: 5}, {PerMinute: 5}, {Burst: -1, PerMinute: 5}} {
		_, err := NewRateLimiter(limit)
		assert.Error(t, err, "%+v", limit)
	}
}

func TestRateLimiter_BurstThenRefill(t *testing.T) {
	rl, now := newTestLimiter(t, RateLimit{Burst: 3, PerMinute: 60}) // one token a second

	for range 3 {
		ok, _ := rl.take("a")
		require.True(t, ok, "a fresh bucket allows its whole burst")
	}

	ok, wait := rl.take("a")
	assert.False(t, ok)
	assert.Equal(t, time.Second, wait, "the next token is a second away")

	*now = now.Add(time.Second)
	ok, _ = rl.take("a")
	assert.True(t, ok, "a token came back, no waiting for a window to end")

	ok, _ = rl.take("a")
	assert.False(t, ok, "only one came back")

	*now = now.Add(time.Hour)
	for range 3 {
		ok, _ = rl.take("a")
		require.True(t, ok)
	}
	ok, _ = rl.take("a")
	assert.False(t, ok, "idle time refills to the burst, never beyond")
}

func TestRateLimiter_KeysAreIndependent(t *testing.T) {
	rl, _ := newTestLimiter(t, RateLimit{Burst: 1, PerMinute: 1})

	ok, _ := rl.take("a")
	require.True(t, ok)
	ok, _ = rl.take("a")
	require.False(t, ok)

	ok, _ = rl.take("b")
	assert.True(t, ok)
}

func TestRateLimiter_RetryAfterIsWholeSecondsRoundedUp(t *testing.T) {
	rl, now := newTestLimiter(t, RateLimit{Burst: 1, PerMinute: 30}) // a token every 2s

	rl.take("a")
	*now = now.Add(500 * time.Millisecond)

	w := httptest.NewRecorder()
	rl.serve(w, httptest.NewRequest(http.MethodGet, "/", nil), http.NotFoundHandler(), "a")

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "2", w.Header().Get("Retry-After"), "1.5s left rounds up")
	assert.Contains(t, w.Body.String(), "too_many_requests")

	assert.Equal(t, 1, retryAfterSeconds(0), "never 0, that invites an immediate retry")
}

func TestRateLimiter_SweepDropsOnlyBucketsThatAreFullAgain(t *testing.T) {
	rl, now := newTestLimiter(t, RateLimit{Burst: 2, PerMinute: 60}) // full again after 2s
	rl.take("a")

	*now = now.Add(time.Second)
	rl.take("b")
	rl.sweep()
	assert.Len(t, rl.buckets, 2, "a second is not enough to refill")

	*now = now.Add(time.Second)
	rl.sweep()
	assert.Len(t, rl.buckets, 1, "a is full again and dropped, b has not had its two seconds")
	assert.Contains(t, rl.buckets, "b")
}

func TestLimitCaller_LoggedInCallersAreLimitedByAccount(t *testing.T) {
	account, _ := newTestLimiter(t, RateLimit{Burst: 2, PerMinute: 1})
	anonymous, _ := newTestLimiter(t, RateLimit{Burst: 1, PerMinute: 1})

	handler := LimitCaller(account, func(string) *RateLimiter { return anonymous })(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	)

	send := func(accountID uuid.UUID) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		// Same address for everyone: behind one NAT.
		ctx := api.WithIpAddress(req.Context(), "203.0.113.1")
		if accountID != uuid.Nil {
			ctx = authcontext.WithAccountID(ctx, accountID)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req.WithContext(ctx))
		return w.Code
	}

	alice, bob := uuid.New(), uuid.New()

	assert.Equal(t, http.StatusOK, send(alice))
	assert.Equal(t, http.StatusOK, send(alice))
	assert.Equal(t, http.StatusTooManyRequests, send(alice), "alice used her burst")
	assert.Equal(t, http.StatusOK, send(bob), "bob shares alice's address, not her bucket")

	assert.Equal(t, http.StatusOK, send(uuid.Nil), "an anonymous caller uses the anonymous limiter")
	assert.Equal(t, http.StatusTooManyRequests, send(uuid.Nil))
}

func TestLimitCaller_AnonymousLimiterIsChosenByPath(t *testing.T) {
	account, _ := newTestLimiter(t, RateLimit{Burst: 10, PerMinute: 10})
	login, _ := newTestLimiter(t, RateLimit{Burst: 1, PerMinute: 1})
	other, _ := newTestLimiter(t, RateLimit{Burst: 5, PerMinute: 5})

	handler := LimitCaller(account, func(path string) *RateLimiter {
		if path == "/login" {
			return login
		}
		return other
	})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	send := func(path string) int {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req = req.WithContext(api.WithIpAddress(req.Context(), "203.0.113.1"))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w.Code
	}

	assert.Equal(t, http.StatusOK, send("/login"))
	assert.Equal(t, http.StatusTooManyRequests, send("/login"), "login is the tight one")
	assert.Equal(t, http.StatusOK, send("/refresh"), "the same address still has the looser bucket")
}
