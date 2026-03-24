package mcp

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server is the MCP server type from the official SDK. Exported so callers can type the SSE handler callback.
type Server = sdkmcp.Server

const (
	implementationName    = "gojoplin"
	implementationVersion = "v1.1.0"
)

// sseHeartbeatInterval is how often SSE comment keepalives are sent.
// Must be well under any reverse-proxy read timeout (typically 60s).
var sseHeartbeatInterval = 20 * time.Second //nolint:gochecknoglobals // test-overridable

// setHeartbeatInterval overrides the heartbeat interval (for tests).
func setHeartbeatInterval(d time.Duration) { sseHeartbeatInterval = d }

// NewServer creates an MCP server with all tools and resources registered.
func NewServer(d *Deps) *Server {
	server := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    implementationName,
		Version: implementationVersion,
	}, nil)
	RegisterAll(server, d)
	RegisterResources(server, d)
	return server
}

// NewSSEHandler returns an http.Handler that serves MCP over SSE. Accepts GET (new session) and POST (message to session).
// Mount at e.g. /mcp. Caller must apply Bearer auth before this handler if required.
func NewSSEHandler(getServer func(*http.Request) *Server) http.Handler {
	return sseHeartbeatMiddleware(sdkmcp.NewSSEHandler(getServer, nil))
}

// syncWriter wraps an http.ResponseWriter with a mutex so that concurrent
// writes from the SDK's event loop and our heartbeat goroutine are serialized.
// It also implements http.Flusher so SSE data is pushed immediately.
type syncWriter struct {
	mu sync.Mutex
	w  http.ResponseWriter
	f  http.Flusher
}

func (s *syncWriter) Header() http.Header         { return s.w.Header() }
func (s *syncWriter) WriteHeader(statusCode int)   { s.w.WriteHeader(statusCode) }
func (s *syncWriter) Write(p []byte) (int, error)  { s.mu.Lock(); defer s.mu.Unlock(); return s.w.Write(p) }
func (s *syncWriter) Flush()                       { s.mu.Lock(); defer s.mu.Unlock(); s.f.Flush() }

// sseHeartbeatMiddleware wraps an SSE handler to inject periodic comment
// keepalives (": heartbeat\n\n") on GET requests. This prevents reverse
// proxies (e.g. nginx) from closing idle SSE streams due to read timeouts.
// POST requests pass through unchanged.
func sseHeartbeatMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		sw := &syncWriter{w: w, f: flusher}

		ticker := time.NewTicker(sseHeartbeatInterval)
		done := make(chan struct{})
		go func() {
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					// SSE comment lines (starting with ':') are ignored by
					// clients but reset reverse-proxy read timers.
					if _, err := sw.Write([]byte(": heartbeat\n\n")); err != nil {
						slog.Debug("heartbeat write failed", "error", err)
						return
					}
					sw.Flush()
				case <-done:
					return
				case <-r.Context().Done():
					return
				}
			}
		}()

		next.ServeHTTP(sw, r)
		close(done)
	})
}
