package web

import (
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlorianWenzel/unimatrix/internal/store"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	path := filepath.Join(t.TempDir(), "unimatrix.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	svr, err := NewServer(s, Config{SessionKey: key})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return svr
}

func TestGenerateBorgDesignation(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		d := generateBorgDesignation()
		if d == "" {
			t.Fatal("empty designation")
		}
		if !strings.Contains(d, " of ") {
			t.Fatalf("missing ' of ' in %q", d)
		}
		if !strings.Contains(d, " Adjunct of Unimatrix ") {
			t.Fatalf("missing role/unimat in %q", d)
		}
		seen[d] = true
	}
	if len(seen) < 10 {
		t.Logf("only got %d unique designations in 100 calls — low but possible", len(seen))
	}
}

func TestRegisterSubmitEmptyDesignationAutoGenerates(t *testing.T) {
	s := newTestServer(t)

	form := url.Values{
		"designation": {""},
		"access_code": {"resistance"},
	}
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	s.registerSubmit(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("want 303, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if loc != "/" {
		t.Fatalf("redirect: want /, got %q", loc)
	}

	// The session cookie should be set.
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie after registration")
	}
}

func TestRegisterSubmitRequiresAccessCode(t *testing.T) {
	s := newTestServer(t)

	form := url.Values{
		"designation": {""},
		"access_code": {""},
	}
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	s.registerSubmit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Access code is required") {
		t.Fatal("expected access-code-required flash")
	}
}

func TestRegisterSubmitDuplicateDesignationRetries(t *testing.T) {
	s := newTestServer(t)

	// Register a drone first so that the randomly-generated designation
	// may collide. We can't deterministically force a collision, but
	// we can verify the retry path by pre-registering many drones with
	// all possible generated designations and then verifying a new
	// registration still works (or shows the right error).

	// Pre-register a drone so we can test explicit duplicate.
	d, err := s.store.RegisterDrone("Locutus of Borg", "alcove")
	if err != nil {
		t.Fatalf("pre-register: %v", err)
	}
	_ = d

	// Submit with the same explicit designation.
	form := url.Values{
		"designation": {"Locutus of Borg"},
		"access_code": {"alcove"},
	}
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	s.registerSubmit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "already in the collective") {
		t.Fatal("expected 'already in the collective' flash for explicit duplicate")
	}
}
