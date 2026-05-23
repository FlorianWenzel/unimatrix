package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	if v := rec.Header().Get("Content-Security-Policy"); v == "" {
		t.Fatal("Content-Security-Policy header missing")
	} else if !strings.Contains(v, "style-src") {
		t.Fatalf("Content-Security-Policy missing style-src directive: %q", v)
	}
	if v := rec.Header().Get("X-Frame-Options"); v != "DENY" {
		t.Fatalf("X-Frame-Options: want DENY, got %q", v)
	}
	if v := rec.Header().Get("X-Content-Type-Options"); v != "nosniff" {
		t.Fatalf("X-Content-Type-Options: want nosniff, got %q", v)
	}
	if v := rec.Header().Get("Referrer-Policy"); v != "strict-origin-when-cross-origin" {
		t.Fatalf("Referrer-Policy: want strict-origin-when-cross-origin, got %q", v)
	}
}

func TestSecurityHeadersOnRoutes(t *testing.T) {
	s := newTestServer(t)

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/", ""},
		{http.MethodGet, "/about", ""},
		{http.MethodPost, "/register", "designation=&access_code="},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			rec := httptest.NewRecorder()

			s.Handler().ServeHTTP(rec, req)

			if v := rec.Header().Get("Content-Security-Policy"); v == "" {
				t.Errorf("Content-Security-Policy header missing")
			} else if !strings.Contains(v, "style-src") {
				t.Errorf("Content-Security-Policy missing style-src directive: %q", v)
			}
			if v := rec.Header().Get("X-Frame-Options"); v != "DENY" {
				t.Errorf("X-Frame-Options: want DENY, got %q", v)
			}
			if v := rec.Header().Get("X-Content-Type-Options"); v != "nosniff" {
				t.Errorf("X-Content-Type-Options: want nosniff, got %q", v)
			}
			if v := rec.Header().Get("Referrer-Policy"); v != "strict-origin-when-cross-origin" {
				t.Errorf("Referrer-Policy: want strict-origin-when-cross-origin, got %q", v)
			}
		})
	}
}
