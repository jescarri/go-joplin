package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var tracer = otel.Tracer("rag")

// Embedder generates vector embeddings from text.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}

type openAIEmbedder struct {
	endpoint   string
	apiKey     string
	model      string
	dimensions int
	client     *http.Client
}

// NewOpenAIEmbedder creates an embedder that calls an OpenAI-compatible /v1/embeddings endpoint.
func NewOpenAIEmbedder(endpoint, apiKey, model string, dimensions int) Embedder {
	return &openAIEmbedder{
		endpoint:   endpoint,
		apiKey:     apiKey,
		model:      model,
		dimensions: dimensions,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *openAIEmbedder) Dimensions() int { return e.dimensions }

type embeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func (e *openAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	ctx, span := tracer.Start(ctx, "rag.embed")
	defer span.End()
	span.SetAttributes(
		attribute.String("model", e.model),
		attribute.Int("batch_size", len(texts)),
		attribute.Int("dimensions", e.dimensions),
	)

	start := time.Now()
	defer func() {
		EmbeddingDuration.WithLabelValues(e.model).Observe(time.Since(start).Seconds())
	}()

	body, err := json.Marshal(embeddingRequest{Input: texts, Model: e.model})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := e.endpoint + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		EmbeddingRequests.WithLabelValues(e.model, "error").Inc()
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		EmbeddingRequests.WithLabelValues(e.model, "error").Inc()
		slog.Error("embedding API error", "status", resp.StatusCode, "body", string(respBody))
		return nil, fmt.Errorf("embedding API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		EmbeddingRequests.WithLabelValues(e.model, "error").Inc()
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Data) != len(texts) {
		EmbeddingRequests.WithLabelValues(e.model, "error").Inc()
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(result.Data))
	}

	vecs := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index < 0 || d.Index >= len(texts) {
			EmbeddingRequests.WithLabelValues(e.model, "error").Inc()
			return nil, fmt.Errorf("invalid embedding index %d", d.Index)
		}
		if len(d.Embedding) != e.dimensions {
			EmbeddingRequests.WithLabelValues(e.model, "error").Inc()
			return nil, fmt.Errorf("expected dimension %d, got %d", e.dimensions, len(d.Embedding))
		}
		vecs[d.Index] = d.Embedding
	}

	EmbeddingRequests.WithLabelValues(e.model, "success").Inc()
	return vecs, nil
}
