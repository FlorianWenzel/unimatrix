// Package web wires the HTTP layer for unimatrix: routing, session
// handling, template rendering. The Server holds the dependencies; all
// handlers are methods on it.
package web

import (
	"embed"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/FlorianWenzel/unimatrix/internal/store"
)

//go:embed templates/*.html
var templatesFS embed.FS

type Server struct {
	store        *store.Store
	tmpl         *template.Template
	sessionKey   []byte
	cookieSecure bool
	logger       *slog.Logger
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
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{
		store:        s,
		tmpl:         tmpl,
		sessionKey:   cfg.SessionKey,
		cookieSecure: cfg.CookieSecure,
		logger:       cfg.Logger,
	}, nil
}

// Handler returns the HTTP handler for the application.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /about", s.about)
	mux.HandleFunc("GET /register", s.registerForm)
	mux.HandleFunc("POST /register", s.registerSubmit)
	mux.HandleFunc("GET /login", s.loginForm)
	mux.HandleFunc("POST /login", s.loginSubmit)
	mux.HandleFunc("POST /logout", s.logoutSubmit)
	mux.HandleFunc("POST /transmission", s.postTransmission)
	mux.HandleFunc("GET /transmission/{id}/edit", s.editTransmission)
	mux.HandleFunc("POST /transmission/{id}/edit", s.updateTransmission)
	return mux
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

type pageData struct {
	Title             string
	Drone             *store.Drone // nil if anonymous
	Transmissions     []store.Transmission
	Flash             string
	FilterDrone       string
	AllDrones         []string
}

func (s *Server) render(w http.ResponseWriter, name string, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
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
	
	s.render(w, "home.html", pageData{
		Title:         "The Collective",
		Drone:         s.currentDrone(r),
		Transmissions: txs,
		FilterDrone:   filterDrone,
		AllDrones:     allDrones,
	})
}

func (s *Server) about(w http.ResponseWriter, r *http.Request) {
	s.render(w, "about.html", pageData{
		Title: "About the Unimatrix",
		Drone: s.currentDrone(r),
	})
}

func (s *Server) registerForm(w http.ResponseWriter, r *http.Request) {
	if s.currentDroneID(r) != 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, "register.html", pageData{Title: "Assimilate"})
}

func (s *Server) registerSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	designation := strings.TrimSpace(r.PostFormValue("designation"))
	accessCode := r.PostFormValue("access_code")
	if designation == "" || accessCode == "" {
		s.render(w, "register.html", pageData{
			Title: "Assimilate", Flash: "Both fields are required.",
		})
		return
	}
	d, err := s.store.RegisterDrone(designation, accessCode)
	if err != nil {
		flash := "Assimilation failed."
		if errors.Is(err, store.ErrDroneExists) {
			flash = "That designation is already in the collective."
		}
		s.render(w, "register.html", pageData{Title: "Assimilate", Flash: flash})
		return
	}
	s.setSession(w, d.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	if s.currentDroneID(r) != 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, "login.html", pageData{Title: "Connect"})
}

func (s *Server) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	designation := strings.TrimSpace(r.PostFormValue("designation"))
	accessCode := r.PostFormValue("access_code")
	d, err := s.store.Authenticate(designation, accessCode)
	if err != nil {
		s.render(w, "login.html", pageData{
			Title: "Connect", Flash: "The collective does not recognize that designation.",
		})
		return
	}
	s.setSession(w, d.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logoutSubmit(w http.ResponseWriter, r *http.Request) {
	s.clearSession(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) postTransmission(w http.ResponseWriter, r *http.Request) {
	d := s.currentDrone(r)
	if d == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	body := r.PostFormValue("body")
	if _, err := s.store.PostTransmission(d.ID, body); err != nil {
		s.logger.Warn("post transmission", "drone", d.Designation, "err", err)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
