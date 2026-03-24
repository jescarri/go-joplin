package rag

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jescarri/go-joplin/internal/telemetry"
)

// Prometheus metrics for RAG operations.
var (
	EmbeddingRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rag_embedding_requests_total",
			Help: "Total embedding API requests",
		},
		[]string{"model", "status"},
	)

	EmbeddingDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rag_embedding_duration_seconds",
			Help:    "Duration of embedding API calls",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"model"},
	)

	EmbeddingTokens = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rag_embedding_tokens_total",
			Help: "Total tokens sent to embedding API",
		},
		[]string{"model"},
	)

	IndexNotesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rag_index_notes_total",
			Help: "Notes processed by RAG indexer",
		},
		[]string{"status"},
	)

	IndexDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "rag_index_duration_seconds",
		Help:    "Per-note RAG indexing duration",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	})

	IndexChunksTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "rag_index_chunks_total",
		Help: "Total chunks indexed",
	})

	IndexQueueDepth = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rag_index_queue_depth",
		Help: "Current RAG indexer queue depth",
	})

	SearchRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rag_search_requests_total",
			Help: "Total RAG search requests",
		},
		[]string{"status"},
	)

	SearchDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "rag_search_duration_seconds",
		Help:    "End-to-end RAG search duration",
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	})
)

func init() {
	telemetry.Reg.MustRegister(
		EmbeddingRequests,
		EmbeddingDuration,
		EmbeddingTokens,
		IndexNotesTotal,
		IndexDuration,
		IndexChunksTotal,
		IndexQueueDepth,
		SearchRequests,
		SearchDuration,
	)
}
