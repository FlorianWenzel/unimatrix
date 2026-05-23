// Package web wires the HTTP layer for unimatrix: routing, session
// handling, template rendering. The Server holds the dependencies; all
// handlers are methods on it.
package web

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/FlorianWenzel/unimatrix/internal/store"
)

//go:embed templates/*.html
var templatesFS embed.FS

type Server struct {
	store        *store.Store
	tmpl         map[string]*template.Template
	sessionKey   []byte
	cookieSecure bool
	logger       *slog.Logger
	rateLimiter  *RateLimiter
}

// Config bundles the optional knobs for NewServer.
type Config struct {
	SessionKey   []byte // HMAC secret for session cookies (required)
	CookieSecure bool   // set when serving over HTTPS
	Logger       *slog.Logger
}

func NewServer(s *store.Store, cfg Config) (*Server, error) {
	if len(cfg.SessionKey) < 16 {
		return nil, errors.New("session key must be at least 16 bytes")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	tmpl := make(map[string]*template.Template)
	// Pages that include base.html — parse each separately to avoid
	// {{define "content"}} redefinition (Go 1.24 overwrites silently).
	for _, page := range []string{"home", "about", "login", "register", "transmission"} {
		t, err := template.ParseFS(templatesFS, "templates/base.html", "templates/"+page+".html")
		if err != nil {
			return nil, err
		}
		tmpl[page+".html"] = t
	}
	// Standalone pages — no base.html dependency.
	for _, page := range []string{"drone", "404"} {
		t, err := template.ParseFS(templatesFS, "templates/"+page+".html")
		if err != nil {
			return nil, err
		}
		tmpl[page+".html"] = t
	}
	return &Server{
		store:        s,
		tmpl:         tmpl,
		sessionKey:   cfg.SessionKey,
		cookieSecure: cfg.CookieSecure,
		logger:       cfg.Logger,
		rateLimiter:  NewRateLimiter(),
	}, nil
}

// Handler returns the HTTP handler for the application.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /favicon.ico", s.favicon)
	mux.HandleFunc("GET /robots.txt", s.robotsTxt)
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /about", s.about)
	mux.HandleFunc("GET /register", s.registerForm)
	mux.HandleFunc("POST /register", s.registerSubmit)
	mux.HandleFunc("GET /login", s.loginForm)
	mux.HandleFunc("POST /login", s.loginSubmit)
	mux.HandleFunc("POST /logout", s.logoutSubmit)
	mux.HandleFunc("POST /transmission", s.postTransmission)
	mux.HandleFunc("GET /transmission/{id}", s.transmissionPage)
	mux.HandleFunc("POST /like", s.likeTransmission)
	mux.HandleFunc("GET /drone/{designation}", s.droneProfile)
	mux.HandleFunc("/", s.notFound)
	return SecurityHeaders(mux)
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

// faviconInlineSVG is a 16×16 Borg-green cube rendered as inline SVG.
const faviconInlineSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" width="16" height="16">
  <rect x="0" y="0" width="16" height="16" fill="#0a0e0a"/>
  <polygon points="8,1 15,5 15,11 8,15 1,11 1,5" fill="#1a4a2a" stroke="#5fffaf" stroke-width="0.5"/>
  <polygon points="8,1 15,5 8,9 1,5" fill="#5fffaf"/>
  <polygon points="8,9 15,5 15,11 8,15" fill="#1a4a2a"/>
</svg>`

func (s *Server) favicon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write([]byte(faviconInlineSVG))
}

func (s *Server) robotsTxt(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("User-agent: *\nAllow: /\nAllow: /transmission/*\nDisallow: /register\nDisallow: /login\nDisallow: /logout\nDisallow: /healthz\nDisallow: /about\n"))
}

type pageData struct {
	Title            string
	Drone            *store.Drone // nil if anonymous
	ProfileDrone     *store.Drone // drone being viewed on profile page
	Transmissions    []store.Transmission
	Flash            string
	FilterDrone      string
	AllDrones        []string
	TopTransmissions []store.Transmission
	Transmission     *store.Transmission // single transmission view
	CSRFToken        string
	DroneCount       int
}

func (s *Server) render(w http.ResponseWriter, name string, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t, ok := s.tmpl[name]
	if !ok {
		s.logger.Error("render template", "name", name, "err", "unknown template")
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		s.logger.Error("render template", "name", name, "err", err)
		http.Error(w, "render failed", http.StatusInternalServerError)
	}
}

func (s *Server) currentDrone(r *http.Request) *store.Drone {
	id := s.currentDroneID(r)
	if id == 0 {
		return nil
	}
	d, err := s.store.DroneByID(id)
	if err != nil {
		s.logger.Warn("lookup drone", "id", id, "err", err)
		return nil
	}
	return d
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	filterDrone := r.URL.Query().Get("filter")

	// Get all unique drone designations
	allDrones, err := s.store.ListAllDroneDesignations()
	if err != nil {
		s.logger.Error("list drone designations", "err", err)
		http.Error(w, "the hive falters", http.StatusInternalServerError)
		return
	}

	var txs []store.Transmission
	if filterDrone != "" {
		// Filter by specific drone designation
		txs, err = s.store.ListTransmissionsByDrone(filterDrone, 50)
	} else {
		// Show all transmissions
		txs, err = s.store.ListTransmissions(50)
	}
	if err != nil {
		s.logger.Error("list transmissions", "err", err)
		http.Error(w, "the hive falters", http.StatusInternalServerError)
		return
	}

	// Get top transmissions by likes
	topTxs, err := s.store.GetTopTransmissions()
	if err != nil {
		s.logger.Error("get top transmissions", "err", err)
		http.Error(w, "the hive falters", http.StatusInternalServerError)
		return
	}

	csrfTok, err := s.ensureCSRFToken(w, r)
	if err != nil {
		s.logger.Error("csrf token", "err", err)
		http.Error(w, "the hive falters", http.StatusInternalServerError)
		return
	}

	s.render(w, "home.html", pageData{
		Title:            "The Collective",
		Drone:            s.currentDrone(r),
		Transmissions:    txs,
		FilterDrone:      filterDrone,
		AllDrones:        allDrones,
		TopTransmissions: topTxs,
		CSRFToken:        csrfTok,
	})
}

func (s *Server) about(w http.ResponseWriter, r *http.Request) {
	csrfTok, _ := s.ensureCSRFToken(w, r)
	count, err := s.store.CountDrones()
	if err != nil {
		s.logger.Error("count drones", "err", err)
		count = 0
	}
	s.render(w, "about.html", pageData{
		Title:      "About the Unimatrix",
		Drone:      s.currentDrone(r),
		CSRFToken:  csrfTok,
		DroneCount: count,
	})
}

func (s *Server) registerForm(w http.ResponseWriter, r *http.Request) {
	if s.currentDroneID(r) != 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	csrfTok, err := s.ensureCSRFToken(w, r)
	if err != nil {
		s.logger.Error("csrf token", "err", err)
		http.Error(w, "the hive falters", http.StatusInternalServerError)
		return
	}
	s.render(w, "register.html", pageData{Title: "Assimilate", CSRFToken: csrfTok})
}

func (s *Server) registerSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !s.validateCSRF(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	designation := strings.TrimSpace(r.PostFormValue("designation"))
	accessCode := r.PostFormValue("access_code")
	csrfTok := s.csrfTokenFromRequest(r)
	if accessCode == "" {
		s.render(w, "register.html", pageData{
			Title: "Assimilate", Flash: "Access code is required.", CSRFToken: csrfTok,
		})
		return
	}
	autoGenerated := designation == ""
	if autoGenerated {
		designation = generateBorgDesignation()
	}
	d, err := s.store.RegisterDrone(designation, accessCode)
	if err != nil {
		if errors.Is(err, store.ErrDroneExists) {
			if autoGenerated {
				// Retry once with a new random designation.
				designation = generateBorgDesignation()
				d, err = s.store.RegisterDrone(designation, accessCode)
			}
			if err != nil {
				flash := "That designation is already in the collective."
				if autoGenerated {
					flash = "The collective could not assign a unique designation. Try again."
				}
				s.render(w, "register.html", pageData{Title: "Assimilate", Flash: flash, CSRFToken: csrfTok})
				return
			}
		} else {
			flash := "Assimilation failed."
			s.render(w, "register.html", pageData{Title: "Assimilate", Flash: flash, CSRFToken: csrfTok})
			return
		}
	}
	s.setSession(w, d.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

var (
	borgOrdinals = []string{"One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten", "Eleven", "Twelve"}
	borgRoles    = []string{"Primary", "Secondary", "Tertiary", "Quaternary", "Quinary", "Senary", "Septenary", "Octonary", "Nonary"}
)

func generateBorgDesignation() string {
	ordinal := borgOrdinals[rand.IntN(len(borgOrdinals))]
	number := borgOrdinals[rand.IntN(len(borgOrdinals))]
	role := borgRoles[rand.IntN(len(borgRoles))]
	unimat := rand.IntN(99) + 1
	return fmt.Sprintf("%s of %s, %s Adjunct of Unimatrix %02d", ordinal, number, role, unimat)
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	if s.currentDroneID(r) != 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	csrfTok, err := s.ensureCSRFToken(w, r)
	if err != nil {
		s.logger.Error("csrf token", "err", err)
		http.Error(w, "the hive falters", http.StatusInternalServerError)
		return
	}
	s.render(w, "login.html", pageData{Title: "Connect", CSRFToken: csrfTok})
}

func (s *Server) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !s.validateCSRF(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	designation := strings.TrimSpace(r.PostFormValue("designation"))
	accessCode := r.PostFormValue("access_code")
	d, err := s.store.Authenticate(designation, accessCode)
	if err != nil {
		s.render(w, "login.html", pageData{
			Title: "Connect", Flash: "The collective does not recognize that designation.",
			CSRFToken: s.csrfTokenFromRequest(r),
		})
		return
	}
	s.setSession(w, d.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logoutSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !s.validateCSRF(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.clearSession(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) postTransmission(w http.ResponseWriter, r *http.Request) {
	d := s.currentDrone(r)
	if d == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if !s.rateLimiter.Allow(d.ID, 5, time.Minute) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("Broadcast frequency exceeded. Regenerate and retry."))
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !s.validateCSRF(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	body := r.PostFormValue("body")
	if _, err := s.store.PostTransmission(d.ID, body); err != nil {
		s.logger.Warn("post transmission", "drone", d.Designation, "err", err)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) likeTransmission(w http.ResponseWriter, r *http.Request) {
	d := s.currentDrone(r)
	if d == nil {
		http.Error(w, "not authorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !s.validateCSRF(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	transmissionIDStr := r.PostFormValue("id")
	if transmissionIDStr == "" {
		http.Error(w, "missing transmission ID", http.StatusBadRequest)
		return
	}

	// Convert to int64
	transmissionID, err := strconv.ParseInt(transmissionIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid transmission ID", http.StatusBadRequest)
		return
	}

	// Check if transmission exists
	if tx, err := s.store.TransmissionByID(transmissionID); err != nil || tx == nil {
		http.Error(w, "transmission not found", http.StatusNotFound)
		return
	}

	if err := s.store.LikeTransmission(d.ID, transmissionID); err != nil {
		s.logger.Error("like transmission", "drone", d.Designation, "id", transmissionID, "err", err)
		http.Error(w, "failed to like transmission", http.StatusInternalServerError)
		return
	}

	// Redirect back to the page the drone came from. Only same-origin
	// paths are honored so the form can't be turned into an open
	// redirect by a crafted Referer header.
	dest := "/"
	if u, err := url.Parse(r.Header.Get("Referer")); err == nil && u.Host == "" && strings.HasPrefix(u.Path, "/") {
		dest = u.Path
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}
}

func (s *Server) droneProfile(w http.ResponseWriter, r *http.Request) {
	designation := r.PathValue("designation")

	d, err := s.store.DroneByDesignation(designation)
	if err != nil {
		s.logger.Error("lookup drone", "designation", designation, "err", err)
		http.Error(w, "the hive falters", http.StatusInternalServerError)
		return
	}
	if d == nil {
		http.Error(w, "drone not found", http.StatusNotFound)
		return
	}

	txs, err := s.store.ListTransmissionsByDrone(designation, 50)
	if err != nil {
		s.logger.Error("list transmissions", "err", err)
		http.Error(w, "the hive falters", http.StatusInternalServerError)
		return
	}

	csrfTok, _ := s.ensureCSRFToken(w, r)

	s.render(w, "drone.html", pageData{
		Title:         d.Designation,
		Drone:         s.currentDrone(r),
		ProfileDrone:  d,
		Transmissions: txs,
		CSRFToken:     csrfTok,
	})
}

func (s *Server) transmissionPage(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.notFound(w, r)
		return
	}
	tx, err := s.store.TransmissionByID(id)
	if err != nil {
		s.logger.Error("lookup transmission", "id", id, "err", err)
		http.Error(w, "the hive falters", http.StatusInternalServerError)
		return
	}
	if tx == nil {
		s.notFound(w, r)
		return
	}
	csrfTok, _ := s.ensureCSRFToken(w, r)
	s.render(w, "transmission.html", pageData{
		Title:        "Transmission " + idStr,
		Drone:        s.currentDrone(r),
		Transmission: tx,
		CSRFToken:    csrfTok,
	})
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	csrfTok, _ := s.ensureCSRFToken(w, r)
	t, ok := s.tmpl["404.html"]
	if !ok {
		s.logger.Error("render 404", "err", "unknown template")
		return
	}
	if err := t.ExecuteTemplate(w, "404.html", pageData{
		Title:     "Sector Uncharted",
		Drone:     s.currentDrone(r),
		CSRFToken: csrfTok,
	}); err != nil {
		s.logger.Error("render 404", "err", err)
	}
}
