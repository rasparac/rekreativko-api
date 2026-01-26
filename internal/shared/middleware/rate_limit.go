package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/rasparac/rekreativko-api/internal/shared/api"
)

type (
	clientRequests struct {
		count     int
		resetTime time.Time
	}

	RateLimiter struct {
		requests map[string]*clientRequests
		mu       sync.RWMutex
		limit    int
		window   time.Duration
	}
)

func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[string]*clientRequests),
		limit:    requestsPerMinute,
		window:   time.Minute,
	}

	go rl.cleanup()

	return rl
}

func (rl *RateLimiter) RateLimiter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := getClientIP(r)

		if rl.allow(clientIP) {
			next.ServeHTTP(w, r)
			return
		}

		api.WriteError(
			w,
			http.StatusTooManyRequests,
			"too_many_requests",
			http.StatusText(http.StatusTooManyRequests),
			nil,
		)

	})
}

func (rl *RateLimiter) allow(clientIP string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	client, exists := rl.requests[clientIP]
	if !exists || now.After(client.resetTime) {
		rl.requests[clientIP] = &clientRequests{
			count:     1,
			resetTime: now.Add(rl.window),
		}
		return true
	}

	if client.count >= rl.limit {
		return false
	}

	client.count++
	return true
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for k, v := range rl.requests {
			if now.After(v.resetTime) {
				delete(rl.requests, k)
			}
		}
		rl.mu.Unlock()
	}
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}

	if xri := r.Header.Get("X-Real-Ip"); xri != "" {
		return xri
	}

	return r.RemoteAddr
}
