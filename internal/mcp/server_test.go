package mcp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// safeRecorder is an httptest.ResponseRecorder with a thread-safe body
// and Flush counter. The heartbeat goroutine and test goroutine can both
// safely access it.
type safeRecorder struct {
	header http.Header
	code   int

	mu     sync.Mutex
	buf    bytes.Buffer
	flushN int
}

func newSafeRecorder() *safeRecorder {
	return &safeRecorder{header: make(http.Header), code: http.StatusOK}
}

func (r *safeRecorder) Header() http.Header { return r.header }
func (r *safeRecorder) WriteHeader(code int) { r.code = code }

func (r *safeRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Write(p)
}

func (r *safeRecorder) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flushN++
}

func (r *safeRecorder) bodyString() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.String()
}

func (r *safeRecorder) flushCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.flushN
}

func TestHeartbeatMiddleware_GET_SendsHeartbeats(t *testing.T) {
	origInterval := sseHeartbeatInterval
	defer func() { setHeartbeatInterval(origInterval) }()
	setHeartbeatInterval(20 * time.Millisecond)

	inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(80 * time.Millisecond)
	})

	handler := sseHeartbeatMiddleware(inner)
	rec := newSafeRecorder()
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)

	handler.ServeHTTP(rec, req)

	body := rec.bodyString()
	count := strings.Count(body, ": heartbeat\n\n")
	if count < 2 {
		t.Errorf("expected at least 2 heartbeats, got %d; body=%q", count, body)
	}
	if rec.flushCount() < 2 {
		t.Errorf("expected at least 2 flushes, got %d", rec.flushCount())
	}
}

func TestHeartbeatMiddleware_POST_PassesThrough(t *testing.T) {
	var called bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := sseHeartbeatMiddleware(inner)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)

	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("inner handler was not called for POST")
	}
	if strings.Contains(rec.Body.String(), "heartbeat") {
		t.Error("POST should not produce heartbeats")
	}
}

func TestHeartbeatMiddleware_NoFlusher_PassesThrough(t *testing.T) {
	var called bool
	inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	})

	handler := sseHeartbeatMiddleware(inner)
	rec := &nonFlusherWriter{httptest.NewRecorder()}
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)

	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("inner handler was not called when Flusher unavailable")
	}
}

// nonFlusherWriter wraps a ResponseRecorder but hides the Flush method.
type nonFlusherWriter struct {
	*httptest.ResponseRecorder
}

func (n *nonFlusherWriter) Header() http.Header        { return n.ResponseRecorder.Header() }
func (n *nonFlusherWriter) Write(b []byte) (int, error) { return n.ResponseRecorder.Write(b) }
func (n *nonFlusherWriter) WriteHeader(code int)         { n.ResponseRecorder.WriteHeader(code) }

func TestHeartbeatMiddleware_StopsAfterHandlerReturns(t *testing.T) {
	origInterval := sseHeartbeatInterval
	defer func() { setHeartbeatInterval(origInterval) }()
	setHeartbeatInterval(10 * time.Millisecond)

	inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
	})

	handler := sseHeartbeatMiddleware(inner)
	rec := newSafeRecorder()
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)

	handler.ServeHTTP(rec, req)

	// After handler returns and done channel is closed, the goroutine exits.
	// Give it a moment to settle, then snapshot and verify stability.
	time.Sleep(30 * time.Millisecond)
	countAfter := strings.Count(rec.bodyString(), ": heartbeat\n\n")
	time.Sleep(40 * time.Millisecond)
	countLater := strings.Count(rec.bodyString(), ": heartbeat\n\n")
	if countLater != countAfter {
		t.Errorf("heartbeat count changed after handler returned: %d -> %d", countAfter, countLater)
	}
}

func TestSyncWriter_ConcurrentWrites(t *testing.T) {
	rec := newSafeRecorder()
	sw := &syncWriter{w: rec, f: rec}

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := sw.Write([]byte("x")); err != nil {
				t.Errorf("Write error: %v", err)
			}
			sw.Flush()
		}()
	}
	wg.Wait()

	body := rec.bodyString()
	if len(body) != 50 {
		t.Errorf("expected 50 bytes written, got %d", len(body))
	}
}
