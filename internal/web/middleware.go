package web

import (
	"net/http"
	"time"
)

// SecurityHeaders sets standard HTTP security headers on every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// UpdateLastSeen is middleware that updates the last_seen_at timestamp for
// authenticated drones on every request. It also detects reconnections
// (when a drone was idle >5 min) and sets a session flag.
func (s *Server) UpdateLastSeen(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if droneID := s.currentDroneID(r); droneID != 0 {
			// Get current state before updating
			d, err := s.store.DroneByID(droneID)
			if err == nil && d != nil {
				// Check if this is a reconnection (was idle >5 min)
				wasIdle := d.LastSeenAt == nil || time.Since(*d.LastSeenAt) > 5*time.Minute

				// Update in the background; don't block the request.
				go func() {
					if err := s.store.UpdateLastSeenAt(droneID); err != nil {
						s.logger.Warn("update last_seen_at", "drone_id", droneID, "err", err)
					}
				}()

				// If this is a reconnection, set a session flag
				if wasIdle {
					s.setReconnectionFlag(w, r)
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
