package rag

import (
	"context"
	"crypto/sha256"
	"fmt"
)

// mockEmbedder returns deterministic vectors derived from input text hash.
type mockEmbedder struct {
	dims  int
	calls [][]string
	err   error // if set, Embed returns this error
}

func newMockEmbedder(dims int) *mockEmbedder {
	return &mockEmbedder{dims: dims}
}

func (m *mockEmbedder) Dimensions() int { return m.dims }

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	m.calls = append(m.calls, texts)
	if m.err != nil {
		return nil, m.err
	}
	vecs := make([][]float32, len(texts))
	for i, t := range texts {
		vecs[i] = hashToVector(t, m.dims)
	}
	return vecs, nil
}

func (m *mockEmbedder) callCount() int { return len(m.calls) }

func hashToVector(s string, dims int) []float32 {
	h := sha256.Sum256([]byte(s))
	vec := make([]float32, dims)
	for i := range vec {
		vec[i] = float32(h[i%32]) / 255.0
	}
	return vec
}

// failEmbedder always returns an error.
func newFailEmbedder(dims int) *mockEmbedder {
	return &mockEmbedder{dims: dims, err: fmt.Errorf("embedding API unavailable")}
}
