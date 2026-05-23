package web

import (
	"net/http"
	"net/http/httptest"
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
	}
	if v := rec.Header().Get("X-Frame-Options"); v != "DENY" {
		t.Fatalf("X-Frame-Options: want DENY, got %q", v)
	}
	if v := rec.Header().Get("X-Content-Type-Options"); v != "nosniff" {
		t.Fatalf("X-Content-Type-Options: want nosniff, got %q", v)
	}
}
