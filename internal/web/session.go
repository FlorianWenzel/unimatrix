package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const sessionCookieName = "unimatrix_session"
const csrfCookieName = "unimatrix_csrf"
const sessionMaxAge = 30 * 24 * time.Hour

// signCookie returns a tamper-evident "<droneID>.<base64(hmac)>" value.
// Cookies are not encrypted — drone IDs are not secrets — only signed
// so they can't be forged without the server's secret key.
func signCookie(droneID int64, secret []byte) string {
	body := strconv.FormatInt(droneID, 10)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(body))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return body + "." + sig
}

func parseCookie(raw string, secret []byte) (int64, error) {
	dot := strings.IndexByte(raw, '.')
	if dot < 1 {
		return 0, errors.New("malformed session cookie")
	}
	body, sig := raw[:dot], raw[dot+1:]
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(body))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	// constant-time compare
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return 0, errors.New("invalid session signature")
	}
	return strconv.ParseInt(body, 10, 64)
}

// setSession writes a signed session cookie on the response.
func (s *Server) setSession(w http.ResponseWriter, droneID int64) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    signCookie(droneID, s.sessionKey),
		Path:     "/",
		Expires:  time.Now().Add(sessionMaxAge),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure,
	})
}

// clearSession deletes the session cookie.
func (s *Server) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure,
	})
	w.Header().Set("Clear-Site-Data", `"cache", "cookies", "storage"`)
}

// currentDroneID returns the authenticated drone's ID, or 0 if anonymous.
func (s *Server) currentDroneID(r *http.Request) int64 {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return 0
	}
	id, err := parseCookie(c.Value, s.sessionKey)
	if err != nil {
		return 0
	}
	return id
}

// csrfToken generates a new random CSRF token as a base64 string.
func csrfToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// ensureCSRFToken returns a CSRF token for the request. If a valid token
// already exists in the cookie, it is returned; otherwise a new token is
// generated, set as a cookie, and returned. The cookie is HttpOnly=false
// so client-side JS can read it if needed, but SameSite=Lax prevents
// cross-site inclusion.
func (s *Server) ensureCSRFToken(w http.ResponseWriter, r *http.Request) (string, error) {
	c, err := r.Cookie(csrfCookieName)
	if err == nil && c.Value != "" {
		return c.Value, nil
	}
	tok, err := csrfToken()
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    tok,
		Path:     "/",
		Expires:  time.Now().Add(sessionMaxAge),
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure,
	})
	return tok, nil
}

// validateCSRF returns true if the _csrf form value matches the CSRF cookie.
func (s *Server) validateCSRF(r *http.Request) bool {
	c, err := r.Cookie(csrfCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	formValue := r.PostFormValue("_csrf")
	if formValue == "" {
		return false
	}
	return hmac.Equal([]byte(c.Value), []byte(formValue))
}

// csrfTokenFromRequest reads the CSRF token from the request cookie,
// returning an empty string if not present.
func (s *Server) csrfTokenFromRequest(r *http.Request) string {
	c, err := r.Cookie(csrfCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

const reconnectionCookieName = "unimatrix_reconnect"

// setReconnectionFlag sets a temporary cookie to indicate the drone has reconnected.
func (s *Server) setReconnectionFlag(w http.ResponseWriter, r *http.Request) {
	// Only set if not already set
	if _, err := r.Cookie(reconnectionCookieName); err == nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     reconnectionCookieName,
		Value:    "1",
		Path:     "/",
		MaxAge:   60, // expires in 60 seconds
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure,
	})
}

// checkAndClearReconnectionFlag checks if the reconnection flag is set,
// clears it if present, and returns true if it was set.
func (s *Server) checkAndClearReconnectionFlag(w http.ResponseWriter, r *http.Request) bool {
	c, err := r.Cookie(reconnectionCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	// Clear the cookie
	http.SetCookie(w, &http.Cookie{
		Name:     reconnectionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure,
	})
	return true
}
