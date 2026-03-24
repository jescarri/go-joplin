package mcp

import (
	"context"

	"github.com/jescarri/go-joplin/internal/models"
	"github.com/jescarri/go-joplin/internal/store"
)

// SyncTrigger triggers a sync run. Implemented by sync.Engine.
type SyncTrigger interface {
	TriggerSync()
}

// RAGSearcher performs semantic search. Nil = FTS4 fallback.
type RAGSearcher interface {
	Search(ctx context.Context, query string, limit int) ([]*models.Note, bool, error)
}

// RAGIndexer enqueues notes for RAG indexing.
type RAGIndexer interface {
	Enqueue(noteID string)
}

// Deps holds dependencies for MCP tool handlers. Easy to extend for new tools.
type Deps struct {
	DB           *store.DB
	Syncer       SyncTrigger
	Policy       *Policy        // nil = all mutations denied (read-only)
	EnabledTools string         // comma-separated tool names or "*" for all (default "*")
	RAGSearcher  RAGSearcher    // nil = FTS4 fallback
	RAGIndexer   RAGIndexer     // nil = no RAG indexing
}
