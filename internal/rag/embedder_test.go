package rag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// embeddingTestServer creates an httptest server that returns the given embeddings.
// The handler fails the test on JSON encode/decode errors.
func embeddingTestServer(t *testing.T, handler func(t *testing.T, w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(t, w, r)
	}))
}

func writeEmbeddingResponse(t *testing.T, w http.ResponseWriter, resp embeddingResponse) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func decodeEmbeddingRequest(t *testing.T, r *http.Request) embeddingRequest {
	t.Helper()
	var req embeddingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	return req
}

func TestEmbed_SingleText(t *testing.T) {
	srv := embeddingTestServer(t, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
		req := decodeEmbeddingRequest(t, r)
		if len(req.Input) != 1 {
			t.Errorf("expected 1 input, got %d", len(req.Input))
		}
		writeEmbeddingResponse(t, w, embeddingResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: []float32{0.1, 0.2, 0.3}, Index: 0},
			},
		})
	})
	defer srv.Close()

	e := NewOpenAIEmbedder(srv.URL, "test-key", "test-model", 3)
	vecs, err := e.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 1 {
		t.Fatalf("expected 1 vector, got %d", len(vecs))
	}
	if len(vecs[0]) != 3 {
		t.Errorf("expected 3 dimensions, got %d", len(vecs[0]))
	}
}

func TestEmbed_Batch(t *testing.T) {
	srv := embeddingTestServer(t, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
		decodeEmbeddingRequest(t, r)
		writeEmbeddingResponse(t, w, embeddingResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: []float32{0.1, 0.2}, Index: 0},
				{Embedding: []float32{0.3, 0.4}, Index: 1},
				{Embedding: []float32{0.5, 0.6}, Index: 2},
			},
		})
	})
	defer srv.Close()

	e := NewOpenAIEmbedder(srv.URL, "", "test", 2)
	vecs, err := e.Embed(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 3 {
		t.Fatalf("expected 3 vectors, got %d", len(vecs))
	}
}

func TestEmbed_APIError500(t *testing.T) {
	srv := embeddingTestServer(t, func(t *testing.T, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		if _, err := w.Write([]byte(`{"error":"internal"}`)); err != nil {
			t.Fatalf("write error response: %v", err)
		}
	})
	defer srv.Close()

	e := NewOpenAIEmbedder(srv.URL, "", "test", 3)
	_, err := e.Embed(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestEmbed_DimensionMismatch(t *testing.T) {
	srv := embeddingTestServer(t, func(t *testing.T, w http.ResponseWriter, _ *http.Request) {
		writeEmbeddingResponse(t, w, embeddingResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: []float32{0.1, 0.2}, Index: 0}, // 2 dims, not 3
			},
		})
	})
	defer srv.Close()

	e := NewOpenAIEmbedder(srv.URL, "", "test", 3)
	_, err := e.Embed(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected error for dimension mismatch")
	}
}

func TestEmbed_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {} // Block forever
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	e := NewOpenAIEmbedder(srv.URL, "", "test", 3)
	_, err := e.Embed(ctx, []string{"hello"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestEmbed_AuthHeader(t *testing.T) {
	var gotAuth string
	srv := embeddingTestServer(t, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		writeEmbeddingResponse(t, w, embeddingResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: []float32{0.1}, Index: 0},
			},
		})
	})
	defer srv.Close()

	e := NewOpenAIEmbedder(srv.URL, "sk-test123", "test", 1)
	_, err := e.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if gotAuth != "Bearer sk-test123" {
		t.Errorf("auth header: got %q, want %q", gotAuth, "Bearer sk-test123")
	}
}
