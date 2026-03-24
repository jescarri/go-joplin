package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/jescarri/go-joplin/internal/models"
	"github.com/jescarri/go-joplin/internal/store"
)

// RAGStore abstracts the database operations needed by the RAG indexer and searcher.
type RAGStore interface {
	GetNote(id string) (*models.Note, error)
	GetNoteHash(noteID string) (string, error)
	UpsertNoteHash(noteID, hash string) error
	DeleteChunksByNoteID(noteID string) error
	InsertChunk(noteID string, idx int, content string, tokenCount int) (int64, error)
	InsertChunkEmbedding(chunkID int64, embedding []float32) error
	SearchVectors(embedding []float32, limit int) ([]store.VectorResult, error)
	DeleteNoteRAGData(noteID string) error
	ListAllNoteIDs() ([]string, error)
	ListIndexedNoteIDs() ([]string, error)
}

// Indexer manages the async pipeline: hash check → chunk → embed → store.
type Indexer struct {
	db        RAGStore
	embedder  Embedder
	chunkSize int
	overlap   int
	queue     chan string
	wg        sync.WaitGroup
	cancel    context.CancelFunc
}

// NewIndexer creates a new RAG indexer.
func NewIndexer(db RAGStore, embedder Embedder, chunkSize, overlap, workers, queueSize int) *Indexer {
	return &Indexer{
		db:        db,
		embedder:  embedder,
		chunkSize: chunkSize,
		overlap:   overlap,
		queue:     make(chan string, queueSize),
	}
}

// Start launches worker goroutines that process the queue.
func (idx *Indexer) Start(ctx context.Context) {
	ctx, idx.cancel = context.WithCancel(ctx)
	workers := 2
	for i := range workers {
		idx.wg.Add(1)
		go idx.worker(ctx, i)
	}
	slog.Info("RAG indexer started", "workers", workers)
}

// Stop cancels workers and waits for them to finish.
func (idx *Indexer) Stop() {
	if idx.cancel != nil {
		idx.cancel()
	}
	idx.wg.Wait()
	slog.Info("RAG indexer stopped")
}

// Enqueue adds a note ID to the processing queue. Non-blocking; drops if full.
func (idx *Indexer) Enqueue(noteID string) {
	select {
	case idx.queue <- noteID:
		IndexQueueDepth.Set(float64(len(idx.queue)))
	default:
		slog.Warn("RAG indexer queue full, dropping note", "note_id", noteID)
	}
}

func (idx *Indexer) worker(ctx context.Context, id int) {
	defer idx.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case noteID := <-idx.queue:
			IndexQueueDepth.Set(float64(len(idx.queue)))
			if _, err := idx.indexNote(ctx, noteID); err != nil {
				slog.Error("RAG index failed", "note_id", noteID, "worker", id, "error", err)
			}
		}
	}
}

// indexResult is returned by indexNote to report what happened.
type indexResult int

const (
	indexResultIndexed   indexResult = iota
	indexResultSkipped               // hash unchanged or encrypted
	indexResultError
	indexResultDeleted               // note was deleted, RAG data cleaned up
)

// IndexAll iterates all notes and indexes changed ones. Also cleans up orphans.
func (idx *Indexer) IndexAll(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "rag.index_all")
	defer span.End()

	slog.Info("RAG IndexAll starting")

	// Clean up orphaned RAG data
	indexed, err := idx.db.ListIndexedNoteIDs()
	if err != nil {
		return err
	}
	var orphans int
	for _, noteID := range indexed {
		note, err := idx.db.GetNote(noteID)
		if err != nil {
			continue
		}
		if note == nil {
			slog.Debug("cleaning up orphaned RAG data", "note_id", noteID)
			_ = idx.db.DeleteNoteRAGData(noteID)
			orphans++
		}
	}

	// Index all existing notes
	allIDs, err := idx.db.ListAllNoteIDs()
	if err != nil {
		return err
	}

	var indexedCount, skippedCount, errorCount int
	for _, noteID := range allIDs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		result, err := idx.indexNote(ctx, noteID)
		if err != nil {
			slog.Error("RAG index failed during IndexAll", "note_id", noteID, "error", err)
			errorCount++
			continue
		}
		switch result {
		case indexResultIndexed:
			indexedCount++
		case indexResultSkipped:
			skippedCount++
		}
	}

	span.SetAttributes(
		attribute.Int("total_notes", len(allIDs)),
		attribute.Int("indexed", indexedCount),
		attribute.Int("skipped", skippedCount),
		attribute.Int("errors", errorCount),
		attribute.Int("orphans_cleaned", orphans),
	)
	slog.Info("RAG IndexAll completed",
		"total", len(allIDs),
		"indexed", indexedCount,
		"skipped", skippedCount,
		"errors", errorCount,
		"orphans_cleaned", orphans,
	)
	return nil
}

func (idx *Indexer) indexNote(ctx context.Context, noteID string) (indexResult, error) {
	start := time.Now()
	ctx, span := tracer.Start(ctx, "rag.index_note")
	defer span.End()
	span.SetAttributes(attribute.String("note_id", noteID))

	note, err := idx.db.GetNote(noteID)
	if err != nil {
		IndexNotesTotal.WithLabelValues("error").Inc()
		return indexResultError, err
	}
	if note == nil {
		// Note was deleted; clean up if there's leftover RAG data
		_ = idx.db.DeleteNoteRAGData(noteID)
		return indexResultDeleted, nil
	}

	// Skip encrypted notes
	if note.EncryptionApplied == 1 {
		IndexNotesTotal.WithLabelValues("skip").Inc()
		span.SetAttributes(attribute.String("status", "skip_encrypted"))
		return indexResultSkipped, nil
	}

	// Hash check
	newHash := hashNoteContent(note.Title, note.Body)
	existingHash, err := idx.db.GetNoteHash(noteID)
	if err != nil {
		IndexNotesTotal.WithLabelValues("error").Inc()
		return indexResultError, err
	}
	if existingHash == newHash {
		IndexNotesTotal.WithLabelValues("skip").Inc()
		span.SetAttributes(attribute.String("status", "skip_unchanged"))
		return indexResultSkipped, nil
	}

	// Chunk
	text := note.Title + "\n\n" + note.Body
	chunks := Chunk(text, idx.chunkSize, idx.overlap)
	if len(chunks) == 0 {
		// Empty note; clean up any existing data and store hash
		_ = idx.db.DeleteChunksByNoteID(noteID)
		_ = idx.db.UpsertNoteHash(noteID, newHash)
		IndexNotesTotal.WithLabelValues("ok").Inc()
		return indexResultIndexed, nil
	}

	// Embed
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	embeddings, err := idx.embedder.Embed(ctx, texts)
	if err != nil {
		IndexNotesTotal.WithLabelValues("error").Inc()
		return indexResultError, fmt.Errorf("embed: %w", err)
	}

	// Store: delete old, insert new
	if err := idx.db.DeleteChunksByNoteID(noteID); err != nil {
		IndexNotesTotal.WithLabelValues("error").Inc()
		return indexResultError, err
	}

	for i, chunk := range chunks {
		chunkID, err := idx.db.InsertChunk(noteID, chunk.Index, chunk.Content, chunk.TokenCount)
		if err != nil {
			IndexNotesTotal.WithLabelValues("error").Inc()
			return indexResultError, fmt.Errorf("insert chunk %d: %w", i, err)
		}
		if err := idx.db.InsertChunkEmbedding(chunkID, embeddings[i]); err != nil {
			IndexNotesTotal.WithLabelValues("error").Inc()
			return indexResultError, fmt.Errorf("insert embedding %d: %w", i, err)
		}
	}

	if err := idx.db.UpsertNoteHash(noteID, newHash); err != nil {
		IndexNotesTotal.WithLabelValues("error").Inc()
		return indexResultError, err
	}

	IndexNotesTotal.WithLabelValues("ok").Inc()
	IndexChunksTotal.Add(float64(len(chunks)))
	IndexDuration.Observe(time.Since(start).Seconds())
	span.SetAttributes(
		attribute.String("status", "indexed"),
		attribute.Int("chunks_count", len(chunks)),
	)
	return indexResultIndexed, nil
}

func hashNoteContent(title, body string) string {
	h := sha256.New()
	h.Write([]byte(title))
	h.Write([]byte("\n"))
	h.Write([]byte(body))
	return hex.EncodeToString(h.Sum(nil))
}
