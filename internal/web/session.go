package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const sessionCookieName = "unimatrix_session"
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
