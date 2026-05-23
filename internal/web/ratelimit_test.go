package web

import (
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToLimit(t *testing.T) {
	rl := NewRateLimiter()
	const droneID int64 = 1

	for i := 0; i < 5; i++ {
		if !rl.Allow(droneID, 5, time.Minute) {
			t.Fatalf("call %d: expected allow, got deny", i+1)
		}
	}
}

func TestRateLimiterBlocksBeyondLimit(t *testing.T) {
	rl := NewRateLimiter()
	const droneID int64 = 1

	for i := 0; i < 5; i++ {
		rl.Allow(droneID, 5, time.Minute)
	}

	if rl.Allow(droneID, 5, time.Minute) {
		t.Fatal("6th call: expected deny, got allow")
	}
}

func TestRateLimiterResetsAfterWindow(t *testing.T) {
	rl := NewRateLimiter()
	const droneID int64 = 1

	// Exhaust the limit.
	for i := 0; i < 5; i++ {
		rl.Allow(droneID, 5, time.Minute)
	}

	// Simulate the window passing by pushing timestamps into the past.
	rl.mu.Lock()
	past := time.Now().Add(-2 * time.Minute)
	rl.bins[droneID] = []time.Time{past, past, past, past, past}
	rl.mu.Unlock()

	// After the window, the drone should be allowed again.
	if !rl.Allow(droneID, 5, time.Minute) {
		t.Fatal("after window: expected allow, got deny")
	}
}

func TestRateLimiterIndependentDroneIDs(t *testing.T) {
	rl := NewRateLimiter()

	// Drone 1 exhausts its limit.
	for i := 0; i < 5; i++ {
		rl.Allow(1, 5, time.Minute)
	}
	if rl.Allow(1, 5, time.Minute) {
		t.Fatal("drone 1 6th call: expected deny, got allow")
	}

	// Drone 2 should NOT be rate-limited.
	if !rl.Allow(2, 5, time.Minute) {
		t.Fatal("drone 2: expected allow, got deny")
	}
}
