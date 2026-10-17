package middleware

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
)

type (
	// RateLimit is a token bucket: a caller can send Burst requests at once,
	// and gets PerMinute of them back each minute. A burst is smoothed out
	// instead of locking the caller out until a window ends.
	RateLimit struct {
		Burst     int
		PerMinute int
	}

	bucket struct {
		tokens float64
		last   time.Time
	}

	// RateLimiter holds one bucket per key (an account or a client address).
	// In memory and per process: a second instance counts on its own.
	RateLimiter struct {
		mu      sync.Mutex
		buckets map[string]*bucket
		burst   float64
		perSec  float64
		now     func() time.Time
		stop    chan struct{}
	}
)

var errInvalidRateLimit = errors.New("rate limit burst and per-minute rate must be positive")

func NewRateLimiter(limit RateLimit) (*RateLimiter, error) {
	if limit.Burst < 1 || limit.PerMinute < 1 {
		return nil, errInvalidRateLimit
	}

	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		burst:   float64(limit.Burst),
		perSec:  float64(limit.PerMinute) / 60,
		now:     time.Now,
		stop:    make(chan struct{}),
	}

	go rl.cleanup()

	return rl, nil
}

// ByClientIP limits every request by the client address ClientInfo resolved
// (the connection's own address if the context has none). It runs before
// authentication, so it also covers requests auth rejects.
func (rl *RateLimiter) ByClientIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rl.serve(w, r, next, clientAddress(r))
	})
}

// LimitCaller limits a request after authentication: by account for a
// logged-in caller, so users behind one address don't share a bucket, and
// otherwise by client address with the limiter anonymous picks for the path
// (login and the like get stricter ones).
func LimitCaller(account *RateLimiter, anonymous func(path string) *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if accountID := authcontext.GetAccountID(r.Context()); accountID != uuid.Nil {
				account.serve(w, r, next, accountID.String())
				return
			}

			anonymous(r.URL.Path).serve(w, r, next, clientAddress(r))
		})
	}
}

func (rl *RateLimiter) serve(w http.ResponseWriter, r *http.Request, next http.Handler, key string) {
	allowed, retryAfter := rl.take(key)
	if allowed {
		next.ServeHTTP(w, r)
		return
	}

	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(retryAfter)))
	// Nothing to do if the client has gone: the refusal is already decided.
	_ = api.WriteError(
		w,
		http.StatusTooManyRequests,
		"too_many_requests",
		http.StatusText(http.StatusTooManyRequests),
		nil,
	)
}

// take spends a token from key's bucket. When there is none it returns how
// long until there is one.
func (rl *RateLimiter) take(key string) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()

	b, exists := rl.buckets[key]
	if !exists {
		b = &bucket{tokens: rl.burst, last: now}
		rl.buckets[key] = b
	}

	b.tokens = math.Min(rl.burst, b.tokens+now.Sub(b.last).Seconds()*rl.perSec)
	b.last = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}

	wait := time.Duration((1 - b.tokens) / rl.perSec * float64(time.Second))
	return false, wait
}

// retryAfterSeconds rounds up, never to 0: "0" would invite an immediate retry
// that is refused again.
func retryAfterSeconds(wait time.Duration) int {
	return max(1, int(math.Ceil(wait.Seconds())))
}

// clientAddress is the address ClientInfo resolved; the connection's own
// address if it did not run.
func clientAddress(r *http.Request) string {
	if ip := api.IpAddressFromContext(r.Context()); ip != "" {
		return ip
	}
	return peerHost(r)
}

// cleanup sweeps once a minute until Stop.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.sweep()
		case <-rl.stop:
			return
		}
	}
}

// sweep drops buckets that have been idle long enough to be full again, as a
// missing bucket starts full.
func (rl *RateLimiter) sweep() {
	refill := time.Duration(rl.burst / rl.perSec * float64(time.Second))

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	for k, b := range rl.buckets {
		if now.Sub(b.last) >= refill {
			delete(rl.buckets, k)
		}
	}
}

// Stop gracefully shuts down the rate limiter's cleanup goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.stop)
}
