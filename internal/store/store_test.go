package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "unimatrix.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestRegisterAndAuthenticate(t *testing.T) {
	s := newTestStore(t)

	d, err := s.RegisterDrone("7-of-9", "irrelevant1")
	if err != nil {
		t.Fatalf("RegisterDrone: %v", err)
	}
	if d.ID == 0 || d.Designation != "7-of-9" {
		t.Fatalf("unexpected drone: %+v", d)
	}

	got, err := s.Authenticate("7-of-9", "irrelevant1")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got.ID != d.ID {
		t.Fatalf("auth returned different drone: got %d want %d", got.ID, d.ID)
	}

	if _, err := s.Authenticate("7-of-9", "wrong"); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("wrong access code: want ErrAuthFailed, got %v", err)
	}
	if _, err := s.Authenticate("unknown", "anything"); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("unknown drone: want ErrAuthFailed, got %v", err)
	}
}

func TestRegisterDuplicateDesignationFails(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.RegisterDrone("locutus", "access"); err != nil {
		t.Fatalf("first RegisterDrone: %v", err)
	}
	if _, err := s.RegisterDrone("locutus", "access"); !errors.Is(err, ErrDroneExists) {
		t.Fatalf("duplicate registration: want ErrDroneExists, got %v", err)
	}
}

func TestPostAndListTransmissions(t *testing.T) {
	s := newTestStore(t)

	d, err := s.RegisterDrone("3-of-5", "alcove")
	if err != nil {
		t.Fatalf("RegisterDrone: %v", err)
	}

	if _, err := s.PostTransmission(d.ID, "we are the borg"); err != nil {
		t.Fatalf("PostTransmission #1: %v", err)
	}
	if _, err := s.PostTransmission(d.ID, "resistance is futile"); err != nil {
		t.Fatalf("PostTransmission #2: %v", err)
	}

	got, err := s.ListTransmissions(10)
	if err != nil {
		t.Fatalf("ListTransmissions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 transmissions, got %d", len(got))
	}
	// newest first
	if got[0].Body != "resistance is futile" {
		t.Fatalf("ordering: newest first expected, got %+v", got[0])
	}
	if got[0].Designation != "3-of-5" {
		t.Fatalf("designation join: want 3-of-5, got %q", got[0].Designation)
	}
}

func TestPostTransmissionRejectsEmptyAndOverlong(t *testing.T) {
	s := newTestStore(t)
	d, err := s.RegisterDrone("2-of-9", "access")
	if err != nil {
		t.Fatalf("RegisterDrone: %v", err)
	}

	if _, err := s.PostTransmission(d.ID, "   "); err == nil {
		t.Fatalf("empty body should error")
	}

	long := make([]byte, 600)
	for i := range long {
		long[i] = 'x'
	}
	if _, err := s.PostTransmission(d.ID, string(long)); err == nil {
		t.Fatalf("overlong body should error")
	}
}

func TestCountDrones(t *testing.T) {
	s := newTestStore(t)

	// Empty store.
	c, err := s.CountDrones()
	if err != nil {
		t.Fatalf("CountDrones (empty): %v", err)
	}
	if c != 0 {
		t.Fatalf("empty store: want 0, got %d", c)
	}

	// Register one drone.
	if _, err := s.RegisterDrone("One", "access"); err != nil {
		t.Fatalf("RegisterDrone: %v", err)
	}
	c, err = s.CountDrones()
	if err != nil {
		t.Fatalf("CountDrones (1): %v", err)
	}
	if c != 1 {
		t.Fatalf("after 1 registration: want 1, got %d", c)
	}

	// Register a second drone.
	if _, err := s.RegisterDrone("Two", "access"); err != nil {
		t.Fatalf("RegisterDrone 2: %v", err)
	}
	c, err = s.CountDrones()
	if err != nil {
		t.Fatalf("CountDrones (2): %v", err)
	}
	if c != 2 {
		t.Fatalf("after 2 registrations: want 2, got %d", c)
	}
}

func TestFollowUnfollowCycle(t *testing.T) {
	s := newTestStore(t)

	a, err := s.RegisterDrone("DroneA", "access")
	if err != nil {
		t.Fatalf("register A: %v", err)
	}
	b, err := s.RegisterDrone("DroneB", "access")
	if err != nil {
		t.Fatalf("register B: %v", err)
	}

	// Should not be following initially.
	following, err := s.IsFollowing(a.ID, b.ID)
	if err != nil {
		t.Fatalf("IsFollowing (initial): %v", err)
	}
	if following {
		t.Fatal("expected IsFollowing to be false before follow")
	}

	// Follow.
	if err := s.FollowDrone(a.ID, b.ID); err != nil {
		t.Fatalf("FollowDrone: %v", err)
	}
	following, err = s.IsFollowing(a.ID, b.ID)
	if err != nil {
		t.Fatalf("IsFollowing (after follow): %v", err)
	}
	if !following {
		t.Fatal("expected IsFollowing to be true after follow")
	}

	// Duplicate follow should be idempotent (no error).
	if err := s.FollowDrone(a.ID, b.ID); err != nil {
		t.Fatalf("FollowDrone (duplicate): %v", err)
	}

	// Unfollow.
	if err := s.UnfollowDrone(a.ID, b.ID); err != nil {
		t.Fatalf("UnfollowDrone: %v", err)
	}
	following, err = s.IsFollowing(a.ID, b.ID)
	if err != nil {
		t.Fatalf("IsFollowing (after unfollow): %v", err)
	}
	if following {
		t.Fatal("expected IsFollowing to be false after unfollow")
	}

	// Unrelated drones should not be following each other.
	c, err := s.RegisterDrone("DroneC", "access")
	if err != nil {
		t.Fatalf("register C: %v", err)
	}
	following, err = s.IsFollowing(a.ID, c.ID)
	if err != nil {
		t.Fatalf("IsFollowing (unrelated): %v", err)
	}
	if following {
		t.Fatal("expected IsFollowing to be false for unrelated drones")
	}
}

func TestCountFollowers(t *testing.T) {
	s := newTestStore(t)

	target, err := s.RegisterDrone("Target", "access")
	if err != nil {
		t.Fatalf("register target: %v", err)
	}

	// Zero followers.
	c, err := s.CountFollowers(target.Designation)
	if err != nil {
		t.Fatalf("CountFollowers (0): %v", err)
	}
	if c != 0 {
		t.Fatalf("zero followers: want 0, got %d", c)
	}

	// Add first follower.
	f1, err := s.RegisterDrone("Follower1", "access")
	if err != nil {
		t.Fatalf("register f1: %v", err)
	}
	if err := s.FollowDrone(f1.ID, target.ID); err != nil {
		t.Fatalf("follow f1: %v", err)
	}
	c, err = s.CountFollowers(target.Designation)
	if err != nil {
		t.Fatalf("CountFollowers (1): %v", err)
	}
	if c != 1 {
		t.Fatalf("1 follower: want 1, got %d", c)
	}

	// Add second follower.
	f2, err := s.RegisterDrone("Follower2", "access")
	if err != nil {
		t.Fatalf("register f2: %v", err)
	}
	if err := s.FollowDrone(f2.ID, target.ID); err != nil {
		t.Fatalf("follow f2: %v", err)
	}
	c, err = s.CountFollowers(target.Designation)
	if err != nil {
		t.Fatalf("CountFollowers (2): %v", err)
	}
	if c != 2 {
		t.Fatalf("2 followers: want 2, got %d", c)
	}

	// Remove one follower.
	if err := s.UnfollowDrone(f1.ID, target.ID); err != nil {
		t.Fatalf("unfollow f1: %v", err)
	}
	c, err = s.CountFollowers(target.Designation)
	if err != nil {
		t.Fatalf("CountFollowers (after unfollow): %v", err)
	}
	if c != 1 {
		t.Fatalf("after unfollow: want 1, got %d", c)
	}
}
