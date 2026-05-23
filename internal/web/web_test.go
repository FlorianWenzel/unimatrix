package web

import (
	"crypto/rand"
	"fmt"
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

// newCSRFToken returns a fresh CSRF token and sets it as a cookie on the request.
func newCSRFToken(t *testing.T, req *http.Request) string {
	t.Helper()
	tok, err := csrfToken()
	if err != nil {
		t.Fatalf("csrfToken: %v", err)
	}
	req.AddCookie(&http.Cookie{
		Name:  csrfCookieName,
		Value: tok,
	})
	return tok
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

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader("_csrf=x&designation=&access_code=resistance"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tok := newCSRFToken(t, req)
	req.PostForm = url.Values{
		"_csrf":       {tok},
		"designation": {""},
		"access_code": {"resistance"},
	}
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

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader("_csrf=x&designation=&access_code="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tok := newCSRFToken(t, req)
	req.PostForm = url.Values{
		"_csrf":       {tok},
		"designation": {""},
		"access_code": {""},
	}
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

	// Pre-register a drone so we can test explicit duplicate.
	d, err := s.store.RegisterDrone("Locutus of Borg", "alcove")
	if err != nil {
		t.Fatalf("pre-register: %v", err)
	}
	_ = d

	// Submit with the same explicit designation.
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader("_csrf=x&designation=Locutus+of+Borg&access_code=alcove"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tok := newCSRFToken(t, req)
	req.PostForm = url.Values{
		"_csrf":       {tok},
		"designation": {"Locutus of Borg"},
		"access_code": {"alcove"},
	}
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

	req := httptest.NewRequest(http.MethodGet, "/sector-7g/uncharted", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Sector Uncharted") {
		t.Fatal("expected 'Sector Uncharted' in response body")
	}
	if !strings.Contains(body, `href="/"`) {
		t.Fatal("expected link back to collective (/) in response body")
	}
	if !strings.Contains(body, "Return to the collective") {
		t.Fatal("expected 'Return to the collective' in response body")
	}
}

func TestPostTransmissionRateLimit(t *testing.T) {
	s := newTestServer(t)

	d, err := s.store.RegisterDrone("RateLimited", "access")
	if err != nil {
		t.Fatalf("register drone: %v", err)
	}

	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/transmission", strings.NewReader("_csrf=x&body=resistance+is+futile"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{
			Name:  sessionCookieName,
			Value: signCookie(d.ID, s.sessionKey),
		})
		tok := newCSRFToken(t, req)
		req.PostForm = url.Values{
			"_csrf": {tok},
			"body":  {"resistance is futile"},
		}
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
		req := httptest.NewRequest(http.MethodPost, "/transmission", strings.NewReader("_csrf=x&body=resistance+is+futile"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{
			Name:  sessionCookieName,
			Value: signCookie(d.ID, s.sessionKey),
		})
		tok := newCSRFToken(t, req)
		req.PostForm = url.Values{
			"_csrf": {tok},
			"body":  {"resistance is futile"},
		}
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

func TestPostTransmissionWithoutCSRF(t *testing.T) {
	s := newTestServer(t)

	d, err := s.store.RegisterDrone("CSRFTest", "access")
	if err != nil {
		t.Fatalf("register drone: %v", err)
	}

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

	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d (body: %s)", rec.Code, rec.Body.String())
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
	_ = drone

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

func TestFavicon(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	rec := httptest.NewRecorder()

	s.favicon(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "image/svg+xml") {
		t.Fatalf("want Content-Type image/svg+xml, got %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<svg") {
		t.Fatal("expected SVG content in response body")
	}
	if !strings.Contains(body, "#5fffaf") {
		t.Fatal("expected Borg-green (#5fffaf) in SVG")
	}
}

func TestFaviconViaHandler(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "image/svg+xml") {
		t.Fatalf("want Content-Type image/svg+xml, got %q", ct)
	}
}

func TestHomeFeedLinksToDroneProfile(t *testing.T) {
	s := newTestServer(t)

	d, err := s.store.RegisterDrone("Six of Ten", "alcove")
	if err != nil {
		t.Fatalf("register drone: %v", err)
	}
	if _, err := s.store.PostTransmission(d.ID, "We are the Borg."); err != nil {
		t.Fatalf("post transmission: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	s.home(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	escapedDesignation := url.PathEscape(d.Designation)
	expectedLink := `<a href="/drone/` + escapedDesignation + `" class="designation">` + d.Designation + `</a>`
	if !strings.Contains(body, expectedLink) {
		t.Fatalf("expected profile link %q in response body", expectedLink)
	}
}

func TestTopTransmissionsLinksToDroneProfile(t *testing.T) {
	s := newTestServer(t)

	d, err := s.store.RegisterDrone("Eight of Twelve", "alcove")
	if err != nil {
		t.Fatalf("register drone: %v", err)
	}
	if _, err := s.store.PostTransmission(d.ID, "Resistance."); err != nil {
		t.Fatalf("post transmission: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	s.home(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	escapedDesignation := url.PathEscape(d.Designation)
	expectedLink := `<a href="/drone/` + escapedDesignation + `" class="designation">` + d.Designation + `</a>`
	if !strings.Contains(body, expectedLink) {
		t.Fatalf("expected profile link %q in response body", expectedLink)
	}
}

func TestTransmissionPage(t *testing.T) {
	s := newTestServer(t)

	d, err := s.store.RegisterDrone("Seven of Nine", "alcove")
	if err != nil {
		t.Fatalf("register drone: %v", err)
	}
	tx, err := s.store.PostTransmission(d.ID, "We are the Borg.")
	if err != nil {
		t.Fatalf("post transmission: %v", err)
	}

	// Like the transmission to test acknowledgment count
	if err := s.store.LikeTransmission(d.ID, tx.ID); err != nil {
		t.Fatalf("like transmission: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/transmission/"+url.PathEscape(fmt.Sprint(tx.ID)), nil)
	req.SetPathValue("id", fmt.Sprint(tx.ID))
	rec := httptest.NewRecorder()

	s.transmissionPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Seven of Nine") {
		t.Fatal("expected drone designation in response body")
	}
	if !strings.Contains(body, "We are the Borg.") {
		t.Fatal("expected transmission body in response body")
	}
	if !strings.Contains(body, "1 acknowledgments") {
		t.Fatal("expected acknowledgment count in response body")
	}
	if !strings.Contains(body, "Return to the collective") {
		t.Fatal("expected link back to collective in response body")
	}
}

func TestTransmissionPageNotFound(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/transmission/99999", nil)
	req.SetPathValue("id", "99999")
	rec := httptest.NewRecorder()

	s.transmissionPage(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Sector Uncharted") {
		t.Fatal("expected 'Sector Uncharted' for unknown transmission")
	}
}

func TestRobotsTxt(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	rec := httptest.NewRecorder()

	s.robotsTxt(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("want Content-Type text/plain, got %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Disallow: /register") {
		t.Fatal("expected 'Disallow: /register' in response body")
	}
	if !strings.Contains(body, "Allow: /") {
		t.Fatal("expected 'Allow: /' in response body")
	}
}

func TestLoginSuccess(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()

	d, err := s.store.RegisterDrone("Three of Five", "alcove")
	if err != nil {
		t.Fatalf("register drone: %v", err)
	}

	// GET /login to obtain CSRF token cookie.
	getReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /login: want 200, got %d", getRec.Code)
	}

	// Extract CSRF token from the Set-Cookie header.
	var csrfTok string
	for _, c := range getRec.Result().Cookies() {
		if c.Name == csrfCookieName {
			csrfTok = c.Value
			break
		}
	}
	if csrfTok == "" {
		t.Fatal("no CSRF cookie set on GET /login")
	}

	// POST /login with valid credentials + CSRF token.
	form := url.Values{"designation": {d.Designation}, "access_code": {"alcove"}, "_csrf": {csrfTok}}
	postReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Carry the CSRF cookie into the POST.
	postReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfTok})
	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusSeeOther {
		t.Fatalf("POST /login: want 303, got %d (body: %s)", postRec.Code, postRec.Body.String())
	}

	// Assert redirect location is /.
	if loc := postRec.Header().Get("Location"); loc != "/" {
		t.Fatalf("POST /login: want redirect to /, got %q", loc)
	}

	// Assert session cookie set.
	var sessionSet bool
	for _, c := range postRec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			sessionSet = true
			break
		}
	}
	if !sessionSet {
		t.Fatal("no session cookie set after successful login")
	}
}

func TestLoginFailure(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()

	// GET /login to obtain CSRF token cookie.
	getReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, getReq)

	var csrfTok string
	for _, c := range getRec.Result().Cookies() {
		if c.Name == csrfCookieName {
			csrfTok = c.Value
			break
		}
	}
	if csrfTok == "" {
		t.Fatal("no CSRF cookie set on GET /login")
	}

	// POST /login with invalid credentials.
	form := url.Values{"designation": {"Nonexistent"}, "access_code": {"wrong"}, "_csrf": {csrfTok}}
	postReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfTok})
	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", postRec.Code)
	}
	body := postRec.Body.String()
	if !strings.Contains(body, "does not recognize") {
		t.Fatal("expected 'does not recognize' flash message on failed login")
	}
}

func TestLogoutClearsSession(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()

	d, err := s.store.RegisterDrone("Four of Seven", "alcove")
	if err != nil {
		t.Fatalf("register drone: %v", err)
	}

	// Step 1: GET /login to obtain CSRF token.
	getReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, getReq)

	var csrfTok string
	for _, c := range getRec.Result().Cookies() {
		if c.Name == csrfCookieName {
			csrfTok = c.Value
			break
		}
	}
	if csrfTok == "" {
		t.Fatal("no CSRF cookie set on GET /login")
	}

	// Step 2: POST /login to authenticate and obtain session cookie.
	form := url.Values{"designation": {d.Designation}, "access_code": {"alcove"}, "_csrf": {csrfTok}}
	loginReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfTok})
	loginRec := httptest.NewRecorder()
	h.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusSeeOther {
		t.Fatalf("login: want 303, got %d", loginRec.Code)
	}

	// Extract session cookie from login response.
	var sessionVal string
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == sessionCookieName {
			sessionVal = c.Value
			break
		}
	}
	if sessionVal == "" {
		t.Fatal("no session cookie after login")
	}

	// Step 3: GET any page to obtain a fresh CSRF token (since CSRF cookie is still valid).
	homeReq := httptest.NewRequest(http.MethodGet, "/", nil)
	homeReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionVal})
	homeRec := httptest.NewRecorder()
	h.ServeHTTP(homeRec, homeReq)

	// Re-extract CSRF token (may be the same or new).
	for _, c := range homeRec.Result().Cookies() {
		if c.Name == csrfCookieName {
			csrfTok = c.Value
			break
		}
	}

	// Step 4: POST /logout with session + CSRF cookies.
	logoutForm := url.Values{"_csrf": {csrfTok}}
	logoutReq := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(logoutForm.Encode()))
	logoutReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	logoutReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionVal})
	logoutReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfTok})
	logoutRec := httptest.NewRecorder()
	h.ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusSeeOther {
		t.Fatalf("POST /logout: want 303, got %d", logoutRec.Code)
	}

	// Assert session cookie is cleared (MaxAge=-1).
	var cleared bool
	for _, c := range logoutRec.Result().Cookies() {
		if c.Name == sessionCookieName && c.MaxAge < 0 {
			cleared = true
			break
		}
	}
	if !cleared {
		t.Fatal("session cookie not cleared after logout")
	}

	// Assert Clear-Site-Data header is present.
	csd := logoutRec.Header().Get("Clear-Site-Data")
	if csd == "" {
		t.Fatal("Clear-Site-Data header not set on logout")
	}
	if !strings.Contains(csd, "cache") || !strings.Contains(csd, "cookies") || !strings.Contains(csd, "storage") {
		t.Fatalf("Clear-Site-Data missing expected directives, got %q", csd)
	}
}
