package clipper

import (
	"net/http"
	"strings"
)

// bearerAuth validates the API key from the Authorization: Bearer <key> header.
// Skips /health so probes do not require a token.
func (s *Server) bearerAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		if s.apiKey == "" {
			// No API key configured, reject all requests
			writeError(w, http.StatusUnauthorized, "API key not configured on server")
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "Authorization header required")
			return
		}

		if !strings.HasPrefix(auth, "Bearer ") {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "Authorization header must use Bearer scheme")
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if token != s.apiKey {
			writeError(w, http.StatusForbidden, "Invalid API key")
			return
		}

		next.ServeHTTP(w, r)
	})
}
