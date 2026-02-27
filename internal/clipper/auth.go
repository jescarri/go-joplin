package clipper

import (
	"net/http"
)

// handleAuth handles POST /auth - auto-accept for headless mode.
func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "accepted",
		"token":  s.apiToken,
	})
}

// handleAuthCheck handles GET /auth/check - always return accepted.
func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "accepted",
		"token":  s.apiToken,
	})
}
