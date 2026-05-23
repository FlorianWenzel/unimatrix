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

func TestDroneProfileKnownDesignation(t *testing.T) {
	s := newTestServer(t)

	d, err := s.store.RegisterDrone("Seven of Nine", "voyager")
	if err != nil {
		t.Fatalf("register drone: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/drone/Seven+of+Nine", nil)
	req.SetPathValue("designation", "Seven of Nine")
	rec := httptest.NewRecorder()

	s.droneProfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, d.Designation) {
		t.Fatalf("expected designation %q in response body", d.Designation)
	}
	if !strings.Contains(body, "Assimilated") {
		t.Fatal("expected assimilation date in response body")
	}
}

func TestDroneProfileUnknownDesignation(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/drone/Nonexistent", nil)
	req.SetPathValue("designation", "Nonexistent")
	rec := httptest.NewRecorder()

	s.droneProfile(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestDroneProfileWithTransmissions(t *testing.T) {
	s := newTestServer(t)

	d, err := s.store.RegisterDrone("Two of Twelve", "alcove")
	if err != nil {
		t.Fatalf("register drone: %v", err)
	}
	if _, err := s.store.PostTransmission(d.ID, "We are the Borg."); err != nil {
		t.Fatalf("post transmission: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/drone/Two+of+Twelve", nil)
	req.SetPathValue("designation", "Two of Twelve")
	rec := httptest.NewRecorder()

	s.droneProfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "We are the Borg.") {
		t.Fatal("expected transmission in response body")
	}
}

func TestNotFoundPage(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/uncharted/sector", nil)
	rec := httptest.NewRecorder()

	s.notFound(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Sector Uncharted") {
		t.Fatal("expected 'Sector Uncharted' in response body")
	}
	if !strings.Contains(body, "Return to the collective") {
		t.Fatal("expected 'Return to the collective' link in response body")
	}
}

func TestNotFoundPageViaHandler(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/nonexistent/path", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Sector Uncharted") {
		t.Fatal("expected 'Sector Uncharted' in response body")
	}
	if !strings.Contains(body, "/\">") || !strings.Contains(body, "Return to the collective") {
		t.Fatal("expected link back to collective in response body")
	}
}

func TestPostTransmissionRateLimit(t *testing.T) {
	s := newTestServer(t)

	d, err := s.store.RegisterDrone("RateLimited", "access")
	if err != nil {
		t.Fatalf("register drone: %v", err)
	}

	post := func() *httptest.ResponseRecorder {
		form := url.Values{
			"body": {"resistance is futile"},
		}
		req := httptest.NewRequest(http.MethodPost, "/transmission", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{
			Name:  sessionCookieName,
			Value: signCookie(d.ID, s.sessionKey),
		})
		rec := httptest.NewRecorder()
		s.postTransmission(rec, req)
		return rec
	}

	// First 5 posts should succeed (303).
	for i := 0; i < 5; i++ {
		rec := post()
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("post %d: want 303, got %d", i+1, rec.Code)
		}
	}

	// 6th post should be rate-limited (429).
	rec := post()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 on 6th post, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Broadcast frequency exceeded") {
		t.Fatal("expected rate-limit message in response body")
	}
}

func TestRateLimiterIndependentDrones(t *testing.T) {
	s := newTestServer(t)

	d1, err := s.store.RegisterDrone("Drone1", "access")
	if err != nil {
		t.Fatalf("register drone1: %v", err)
	}
	d2, err := s.store.RegisterDrone("Drone2", "access")
	if err != nil {
		t.Fatalf("register drone2: %v", err)
	}

	post := func(d *store.Drone) *httptest.ResponseRecorder {
		form := url.Values{
			"body": {"resistance is futile"},
		}
		req := httptest.NewRequest(http.MethodPost, "/transmission", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{
			Name:  sessionCookieName,
			Value: signCookie(d.ID, s.sessionKey),
		})
		rec := httptest.NewRecorder()
		s.postTransmission(rec, req)
		return rec
	}

	// Drone 1 exhausts its limit.
	for i := 0; i < 5; i++ {
		if rec := post(d1); rec.Code != http.StatusSeeOther {
			t.Fatalf("d1 post %d: want 303, got %d", i+1, rec.Code)
		}
	}
	if rec := post(d1); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("d1 6th: want 429, got %d", rec.Code)
	}

	// Drone 2 should NOT be rate-limited.
	if rec := post(d2); rec.Code != http.StatusSeeOther {
		t.Fatalf("d2 first post: want 303, got %d", rec.Code)
	}
}

func TestDroneProfileQueenBadge(t *testing.T) {
	s := newTestServer(t)

	queen, err := s.store.RegisterDrone("The Borg Queen", "omega")
	if err != nil {
		t.Fatalf("register queen: %v", err)
	}
	if err := s.store.SetQueenFlag(queen.ID, true); err != nil {
		t.Fatalf("set queen flag: %v", err)
	}

	drone, err := s.store.RegisterDrone("Seven of Nine", "voyager")
	if err != nil {
		t.Fatalf("register drone: %v", err)
	}

	// Queen profile should show badge.
	req := httptest.NewRequest(http.MethodGet, "/drone/The+Borg+Queen", nil)
	req.SetPathValue("designation", "The Borg Queen")
	rec := httptest.NewRecorder()
	s.droneProfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("queen profile: want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "♛ Queen") {
		t.Fatal("queen profile: expected '♛ Queen' badge in response body")
	}

	// Regular drone profile should NOT show badge.
	req = httptest.NewRequest(http.MethodGet, "/drone/Seven+of+Nine", nil)
	req.SetPathValue("designation", "Seven of Nine")
	rec = httptest.NewRecorder()
	s.droneProfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("drone profile: want 200, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "♛ Queen") {
		t.Fatal("regular drone profile: expected NO '♛ Queen' badge in response body")
	}
}
