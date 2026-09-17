package nominatim

import (
	"context"
	"sync"
	"time"
)

// rateLimiter is a process-wide gate that serializes calls with a minimum
// interval between them - enough to satisfy Nominatim's usage policy (no
// more than 1 request/second) without pulling in an external token-bucket
// dependency. Holding the lock for the whole wait is intentional: it queues
// concurrent callers one behind another instead of letting them race.
type rateLimiter struct {
	mu          sync.Mutex
	minInterval time.Duration
	last        time.Time
}

func newRateLimiter(minInterval time.Duration) *rateLimiter {
	return &rateLimiter{minInterval: minInterval}
}

func (r *rateLimiter) Wait(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.last.IsZero() {
		if wait := r.minInterval - time.Since(r.last); wait > 0 {
			timer := time.NewTimer(wait)
			defer timer.Stop()

			select {
			case <-timer.C:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	r.last = time.Now()
	return nil
}
