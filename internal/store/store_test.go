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
