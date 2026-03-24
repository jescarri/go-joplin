package rag

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/jescarri/go-joplin/internal/models"
)

// Search embeds the query and performs vector similarity search, returning notes.
func (idx *Indexer) Search(ctx context.Context, query string, limit int) ([]*models.Note, bool, error) {
	start := time.Now()
	ctx, span := tracer.Start(ctx, "rag.search")
	defer span.End()
	span.SetAttributes(attribute.Int("limit", limit))

	if limit <= 0 {
		limit = 20
	}

	// Embed query
	vecs, err := idx.embedder.Embed(ctx, []string{query})
	if err != nil {
		SearchRequests.WithLabelValues("error").Inc()
		return nil, false, fmt.Errorf("embed query: %w", err)
	}

	// KNN search — over-fetch for dedup
	_, searchSpan := tracer.Start(ctx, "rag.search.vector_query")
	results, err := idx.db.SearchVectors(vecs[0], limit*3)
	searchSpan.End()
	if err != nil {
		SearchRequests.WithLabelValues("error").Inc()
		return nil, false, fmt.Errorf("vector search: %w", err)
	}

	// Deduplicate by note_id, keep best distance per note
	type bestHit struct {
		noteID   string
		distance float64
	}
	seen := make(map[string]*bestHit)
	var order []string
	for _, r := range results {
		if existing, ok := seen[r.NoteID]; ok {
			if r.Distance < existing.distance {
				existing.distance = r.Distance
			}
		} else {
			seen[r.NoteID] = &bestHit{noteID: r.NoteID, distance: r.Distance}
			order = append(order, r.NoteID)
		}
	}

	// Fetch notes in dedup order, up to limit+1 for hasMore
	var notes []*models.Note
	for _, noteID := range order {
		if len(notes) > limit {
			break
		}
		note, err := idx.db.GetNote(noteID)
		if err != nil {
			slog.Warn("failed to fetch note for search result", "note_id", noteID, "error", err)
			continue
		}
		if note == nil {
			continue
		}
		note.Type_ = models.TypeNote
		notes = append(notes, note)
	}

	hasMore := len(notes) > limit
	if hasMore {
		notes = notes[:limit]
	}

	SearchRequests.WithLabelValues("success").Inc()
	SearchDuration.Observe(time.Since(start).Seconds())
	span.SetAttributes(attribute.Int("results_count", len(notes)))
	return notes, hasMore, nil
}
