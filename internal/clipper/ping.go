package clipper

import (
	"net/http"
)

// handlePing handles GET /ping.
func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("JoplinClipperServer"))
}
