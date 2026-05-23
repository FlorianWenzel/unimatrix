package web

import (
	"sync"
	"time"
)

// RateLimiter tracks per-entity request timestamps for in-memory
// rate limiting. Safe for concurrent use.
type RateLimiter struct {
	mu   sync.Mutex
	bins map[int64][]time.Time
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{bins: make(map[int64][]time.Time)}
}

// Allow reports whether the entity identified by id may proceed,
// given a limit on the number of events within the sliding window.
// If allowed, the current timestamp is recorded.
func (rl *RateLimiter) Allow(id int64, limit int, window time.Duration) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-window)

	ts, ok := rl.bins[id]
	if !ok {
		rl.bins[id] = []time.Time{now}
		return true
	}

	// Evict stale timestamps.
	kept := ts[:0]
	for _, t := range ts {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}

	if len(kept) < limit {
		rl.bins[id] = append(kept, now)
		return true
	}

	rl.bins[id] = kept
	return false
}
