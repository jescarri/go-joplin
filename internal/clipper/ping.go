package clipper

import (
	"net/http"
)

// handleHealth handles GET /health. Returns 200. Not traced or logged.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// handlePing handles GET /ping.
func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("JoplinClipperServer"))
}
