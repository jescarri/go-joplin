# Implementation Plan: RAG-Based Semantic Search

**ADR:** [`docs/adr/001-rag-search.md`](../../docs/adr/001-rag-search.md)
**Date:** 2026-03-23

## Context

Current search uses SQLite FTS4 (keyword matching). We want to replace the note search path with RAG-based semantic search using vector embeddings, while keeping the same API response shapes. Embeddings are stored locally (never synced to S3/Joplin Server). The system must support OpenAI and compatible local-ai models.

**Design decisions:**
- Graceful fallback: FTS4 when RAG is disabled or embeddings not ready
- Fixed-size chunking with overlap (512 tokens, 50 token overlap)
- Index on both sync completion AND Clipper API note creates/updates
- Indexer skips notes with `encryption_applied == 1` (see ADR section 2b)
- Model or dimension change triggers full re-index (see ADR section 2a)

---

## Step 1: Add sqlite-vec dependency and init

**Files:**
- `go.mod` — add `github.com/asg017/sqlite-vec-go-bindings/cgo`
- `internal/store/db.go` — call `sqlite_vec.Auto()` before any `sql.Open`; RAG tables are created in `InitRAG()` (not in the main v49 schema migration) so they work on existing databases

**Data model — see ADR section 2 for full details including ER diagram.**

Relationship chain: `notes` ←(1:1)— `rag_note_hashes` | `notes` ←(N:1)— `rag_chunks` ←(1:1)— `rag_vec`

- `rag_note_hashes.note_id` → `notes.id` — tracks content hash for change detection
- `rag_chunks.note_id` → `notes.id` — a note produces 1..N chunks
- `rag_vec.chunk_id` → `rag_chunks.id` — each chunk has exactly one embedding vector
- No SQL foreign keys on `rag_vec` (vec0 virtual tables don't support them) — cascade handled in application code

**RAG tables (created in `InitRAG()` via `ragSchemaStatements`, not in the main migration):**

```sql
CREATE TABLE IF NOT EXISTS rag_note_hashes (
    note_id    TEXT PRIMARY KEY,   -- references notes.id
    body_hash  TEXT NOT NULL,      -- SHA-256 hex of (title + "\n" + body)
    updated_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS rag_chunks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,  -- referenced by rag_vec.chunk_id
    note_id     TEXT NOT NULL,                      -- references notes.id
    chunk_index INTEGER NOT NULL,                   -- 0-based position within note
    content     TEXT NOT NULL,
    token_count INTEGER NOT NULL DEFAULT 0,
    UNIQUE(note_id, chunk_index)
);
CREATE INDEX IF NOT EXISTS idx_rag_chunks_note_id ON rag_chunks(note_id);

-- vec0 virtual table; dimension set at runtime, created in InitRAG()
CREATE VIRTUAL TABLE IF NOT EXISTS rag_vec USING vec0(
    chunk_id INTEGER PRIMARY KEY,  -- references rag_chunks.id (app-enforced)
    embedding float[<dimensions>]
);
```

**Deletion cascade (application code in `DeleteNoteRAGData`):**
1. `DELETE FROM rag_vec WHERE chunk_id IN (SELECT id FROM rag_chunks WHERE note_id = ?)`
2. `DELETE FROM rag_chunks WHERE note_id = ?`
3. `DELETE FROM rag_note_hashes WHERE note_id = ?`

**RAG metadata** tracked in `key_values`: `rag_schema_version`, `rag_model`, `rag_dimensions`.

**New function:** `db.InitRAG(model string, dimensions int) error` — called from `cmd/serve.go` only when RAG is enabled. Handles:
1. First run: creates all RAG tables + vec0 table, stores model/dimensions in key_values
2. Model or dimension change: drops rag_vec, deletes all chunks + hashes (forces full re-index), recreates vec0 with new dimensions, updates key_values
3. Normal restart: verifies tables exist, returns (IndexAll handles incremental updates via hash check)

See ADR section 2a for full `InitRAG` logic.

---

## Step 2: New package `internal/rag/`

### 2a: `internal/rag/embedder.go` — Embedding client

```go
// Embedder calls an OpenAI-compatible /v1/embeddings endpoint.
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dimensions() int
}

type openAIEmbedder struct {
    endpoint   string // e.g. "http://localhost:11434/v1" or "https://api.openai.com/v1"
    apiKey     string
    model      string
    dimensions int
    client     *http.Client
}
```

- Batch embedding support (sends multiple texts in one API call)
- Retries with exponential backoff on transient errors
- Full OTel tracing: span per `Embed()` call with attributes (model, batch_size, dimensions)
- Prometheus metrics: `rag_embedding_requests_total`, `rag_embedding_duration_seconds`, `rag_embedding_tokens_total`

### 2b: `internal/rag/chunker.go` — Text chunking

```go
// Chunk splits text into fixed-size overlapping chunks.
func Chunk(text string, chunkSize, overlap int) []ChunkResult

type ChunkResult struct {
    Index      int
    Content    string
    TokenCount int
}
```

- Fixed-size splitting: 512 tokens with 50 token overlap (configurable)
- Token counting via simple whitespace split (good enough for embedding models)
- Title prepended to first chunk for context
- OTel span per `Chunk()` call

### 2c: `internal/rag/indexer.go` — Orchestrator

```go
// Indexer manages the async pipeline: hash check → chunk → embed → store.
type Indexer struct {
    db        RAGStore
    embedder  Embedder
    chunkSize int
    overlap   int
    workers   int
    queue     chan string
    wg        sync.WaitGroup
    cancel    context.CancelFunc
}

// RAGStore is the interface the indexer needs from the store.
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
    ListIndexedNoteIDs() ([]string, error)  // for orphan cleanup in IndexAll
}
```

**Pipeline per note:**
1. `GetNote(id)` — fetch note body
2. Skip if `encryption_applied == 1` (encrypted notes can't be chunked)
3. `SHA-256(title + "\n" + body)` → compare with `GetNoteHash(noteID)`
4. If hash matches → skip (note unchanged)
5. `Chunk(title + "\n\n" + body, chunkSize, overlap)` → chunks
6. `Embed(ctx, chunk_contents)` → vectors (batched)
7. `DeleteChunksByNoteID(noteID)` → clear old chunks
8. For each chunk: `InsertChunk()` → get chunk_id, then `InsertChunkEmbedding(chunk_id, vector)`
9. `UpsertNoteHash(noteID, newHash)`

**Worker pool:**
- Buffered channel (`queue`) for note IDs, configurable worker count (default: 2)
- `Indexer.Enqueue(noteID string)` — non-blocking send to queue (drops if full, logs warning)
- `Indexer.IndexAll(ctx context.Context) error` — full reindex: iterate all notes, enqueue each
- `Indexer.Start(ctx context.Context)` — starts worker goroutines, stops on ctx cancellation
- `Indexer.Stop()` — drains queue, waits for in-flight work

**Tracing:** Parent span `rag.index_note` with child spans for hash_check, chunk, embed, store.
**Metrics:** `rag_index_notes_total` (counter), `rag_index_duration_seconds` (histogram), `rag_index_chunks_total` (counter), `rag_index_queue_depth` (gauge)

### 2d: `internal/rag/search.go` — Vector search

```go
// Search embeds the query and performs vector similarity search.
func (idx *Indexer) Search(ctx context.Context, query string, limit int) ([]*models.Note, bool, error)
```

**Pipeline:**
1. `Embed(ctx, []string{query})` → query vector
2. `SearchVectors(queryVec, limit*3)` — over-fetch chunks (multiple chunks per note)
3. Deduplicate by note_id, keep best (lowest distance) chunk per note
4. Fetch full `Note` objects via `GetNote(id)` for top `limit` results
5. Return `[]*models.Note, hasMore, error` — **same signature as `store.SearchNotes()`**

**Tracing:** Span `rag.search` with child spans for embed_query, vector_search, fetch_notes.
**Metrics:** `rag_search_requests_total`, `rag_search_duration_seconds`, `rag_search_results_total`

---

## Step 3: Store layer additions

**New file:** `internal/store/rag.go`

Implements all `RAGStore` interface methods:
- `GetNoteHash(noteID string) (string, error)`
- `UpsertNoteHash(noteID, hash string) error`
- `DeleteChunksByNoteID(noteID string) error` — deletes from both `rag_chunks` and `rag_vec`
- `InsertChunk(noteID string, idx int, content string, tokenCount int) (int64, error)` — returns autoincrement ID
- `InsertChunkEmbedding(chunkID int64, embedding []float32) error` — inserts into `rag_vec` table
- `SearchVectors(embedding []float32, limit int) ([]VectorResult, error)` — KNN query on vec0
- `DeleteNoteRAGData(noteID string) error` — cleanup on note deletion

```go
type VectorResult struct {
    ChunkID  int64
    NoteID   string
    Distance float64
}
```

**Vector search query:**
```sql
-- Note: sqlite-vec requires cv.k = ? instead of LIMIT for KNN queries
SELECT cv.chunk_id, c.note_id, cv.distance
FROM rag_vec cv
JOIN rag_chunks c ON c.id = cv.chunk_id
WHERE cv.embedding MATCH ? AND cv.k = ?
ORDER BY cv.distance
```

---

## Step 4: Configuration

**File:** `internal/config/config.go`

Add `RAGConfig` to `Config` struct:

```go
type RAGConfig struct {
    Enabled    bool   // GOJOPLIN_RAG_ENABLED
    Endpoint   string // GOJOPLIN_RAG_ENDPOINT (OpenAI-compatible base URL)
    APIKey     string // GOJOPLIN_RAG_API_KEY
    Model      string // GOJOPLIN_RAG_MODEL (e.g. "text-embedding-3-small")
    Dimensions int    // GOJOPLIN_RAG_DIMENSIONS (default: 1536)
    ChunkSize  int    // GOJOPLIN_RAG_CHUNK_SIZE (default: 512)
    ChunkOverlap int  // GOJOPLIN_RAG_CHUNK_OVERLAP (default: 50)
    Workers    int    // GOJOPLIN_RAG_WORKERS (default: 2)
    QueueSize  int    // GOJOPLIN_RAG_QUEUE_SIZE (default: 1000)
}
```

**File:** `internal/config/yaml.go` — add YAML mapping for `rag:` section

**File:** `config.yaml.example` — add RAG section:
```yaml
rag:
  enabled: false
  endpoint: "${GOJOPLIN_RAG_ENDPOINT}"  # e.g. "https://api.openai.com/v1" or "http://localhost:11434/v1"
  api_key: "${GOJOPLIN_RAG_API_KEY}"
  model: "text-embedding-3-small"
  dimensions: 1536
  chunk_size: 512
  chunk_overlap: 50
  workers: 2
  queue_size: 1000
```

---

## Step 5: Integration — Search path

**File:** `internal/mcp/tools.go` — modify `searchNotesHandler`

The MCP `Deps` struct gets new optional fields:
```go
type Deps struct {
    DB           *store.DB
    Syncer       SyncTrigger
    Policy       *Policy
    EnabledTools string
    RAGSearcher  RAGSearcher // nil when RAG disabled
    RAGIndexer   RAGIndexer  // nil when RAG disabled
}

type RAGSearcher interface {
    Search(ctx context.Context, query string, limit int) ([]*models.Note, bool, error)
}

type RAGIndexer interface {
    Enqueue(noteID string)
}
```

In `searchNotesHandler`: if `d.RAGSearcher != nil`, call `d.RAGSearcher.Search()`. If it returns error or RAG is not ready, fall back to `d.DB.SearchNotes()`. Response format stays identical (slim `{id, title, parent_id, updated_time}`).

**File:** `internal/clipper/search.go` — same pattern for the Clipper API `handleSearch`. The `clipper.Server` gets optional `RAGSearcher` and `RAGIndexer` fields. When searching notes and RAGSearcher is set, use it; fall back to FTS4 on error.

---

## Step 6: Integration — Every note mutation path

See ADR section 9 for the full analysis. There are 7 code paths that mutate notes — each needs a RAG action.

### 6a. Wiring in `cmd/serve.go`

```go
// After db.Open and before starting sync loop:
var ragIndexer *rag.Indexer
var ragSearcher rag.RAGSearcher
if cfg.RAG.Enabled {
    if err := db.InitRAG(cfg.RAG.Model, cfg.RAG.Dimensions); err != nil {
        return fmt.Errorf("rag init: %w", err)
    }
    embedder := rag.NewOpenAIEmbedder(cfg.RAG)
    ragIndexer = rag.NewIndexer(db, embedder, cfg.RAG)
    ragSearcher = ragIndexer
    ragIndexer.Start(ctx)
    defer ragIndexer.Stop()
    go ragIndexer.IndexAll(ctx)
}

// Pass both ragSearcher and ragIndexer (nil when RAG disabled)
mcpDeps := &mcp.Deps{DB: db, Syncer: engine, Policy: policy, RAGSearcher: ragSearcher, RAGIndexer: ragIndexer, ...}
srv := clipper.NewServer(db, ..., ragSearcher, ragIndexer)
```

### 6b. Create triggers

**File:** `internal/clipper/notes.go` — `handleCreateNote`, after `db.CreateNote(note)` succeeds:
```go
if s.ragIndexer != nil {
    s.ragIndexer.Enqueue(note.ID)
}
```

**File:** `internal/mcp/tools.go` — `createNoteHandler`, after `d.DB.CreateNote(note)` succeeds:
```go
if d.RAGIndexer != nil {
    d.RAGIndexer.Enqueue(note.ID)
}
```

### 6c. Update triggers

**File:** `internal/clipper/notes.go` — `handleUpdateNote`, after `db.UpdateNote(existing)` succeeds:
```go
if s.ragIndexer != nil {
    s.ragIndexer.Enqueue(existing.ID)
}
```

**File:** `internal/mcp/tools.go` — `updateNoteHandler`, after `d.DB.UpdateNote(note)` succeeds:
```go
if d.RAGIndexer != nil {
    d.RAGIndexer.Enqueue(note.ID)
}
```

The indexer's hash check handles the rest — if only metadata changed (body/title unchanged), hash matches and no re-embedding occurs.

### 6d. Delete triggers

**File:** `internal/clipper/notes.go` — `handleDeleteNote`, after `db.DeleteNote(id)` succeeds:
```go
if s.ragIndexer != nil {
    _ = s.db.DeleteNoteRAGData(id)
}
```

**File:** `internal/store/sync_state.go` — `DeleteLocalItem`, inside the `case models.TypeNote:` branch, after the `DELETE FROM notes` exec:
```go
case models.TypeNote:
    _, err = db.Exec("DELETE FROM notes WHERE id = ?", itemID)
    if err == nil {
        _ = db.DeleteNoteRAGData(itemID)  // clean up RAG data
    }
```

This is critical — `DeleteLocalItem` does raw SQL and does NOT call `DeleteNote()`. Without this fix, sync-deleted notes leave orphaned chunks and vectors in the RAG tables.

### 6e. Bulk reindex after sync

In the sync background loop, after each successful `engine.Sync(ctx)`:
```go
if ragIndexer != nil {
    go ragIndexer.IndexAll(ctx) // re-checks all notes, skips unchanged via hash
}
```

This is the safety net that catches all sync-originated changes (creates, updates, decrypts).

### 6f. Orphan cleanup on startup

`IndexAll` must also clean up orphaned RAG data for notes that were deleted while the server was down:
```
1. indexed_ids = SELECT note_id FROM rag_note_hashes
2. For each indexed_id: if GetNote(indexed_id) returns nil → DeleteNoteRAGData(indexed_id)
3. Then proceed with normal hash-check indexing of all existing notes
```

---

## Step 7: Observability

**File:** `internal/rag/metrics.go`

Register all RAG metrics with the existing `telemetry.Reg` prometheus registry:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `rag_embedding_requests_total` | Counter | model, status | Embedding API calls |
| `rag_embedding_duration_seconds` | Histogram | model | Embedding API latency |
| `rag_embedding_tokens_total` | Counter | model | Tokens sent to embedding API |
| `rag_index_notes_total` | Counter | status (ok/skip/error) | Notes processed by indexer |
| `rag_index_duration_seconds` | Histogram | — | Per-note indexing time |
| `rag_index_chunks_total` | Counter | — | Chunks created |
| `rag_index_queue_depth` | Gauge | — | Current indexer queue depth |
| `rag_search_requests_total` | Counter | status | Search requests |
| `rag_search_duration_seconds` | Histogram | — | End-to-end search latency |

**Tracing spans** (all under `rag.*` namespace):
- `rag.embed` — embedding API call (attributes: model, batch_size, dimensions)
- `rag.chunk` — chunking operation (attributes: note_id, chunks_count)
- `rag.index_note` — full indexing pipeline per note
- `rag.index_all` — full reindex sweep
- `rag.search` — search pipeline (attributes: query_length, results_count)
- `rag.search.vector_query` — the vec0 KNN query

---

## Step 8: ADR and CLAUDE.md updates

**File:** `docs/adr/001-rag-search.md` — Architecture Decision Record (already written).

**File:** `CLAUDE.md` — ADR rule added.

**File:** `implementation_plans/001-rag-search/implementation.md` — this file.

---

## Step 9: Tests

The critical path (hash check → chunk → store → search) must be testable without an external embedding service. See ADR section 13 for the full testing strategy, mock embedder implementation, and detailed test case tables.

### Mock Embedder

A `mockEmbedder` produces deterministic vectors from input text via SHA-256 hashing. This lets the full pipeline run in `go test ./...` with zero external dependencies. It records all `Embed()` calls for assertions (e.g., "embedder was NOT called when hash matched").

### Test Files

**`internal/rag/chunker_test.go`** — Pure unit tests:
- Empty text, short text, exact chunk size, multiple chunks with overlap
- Overlap content correctness (end of chunk N == start of chunk N+1)
- Token count accuracy, unicode safety, title prepend

**`internal/rag/embedder_test.go`** — Uses `httptest.Server` (no real API):
- Single/batch embedding, 429 retry, 500 error, malformed JSON
- Context cancellation, dimension mismatch detection

**`internal/store/rag_test.go`** — Real SQLite via `testDB(t)`, requires sqlite-vec:
- `InitRAG` first run, idempotent restart, dimension change, model change
- Hash CRUD, chunk insert/delete, vector insert + KNN search
- `DeleteNoteRAGData` cleans all 3 tables, no-op on non-existent note
- Search on empty vec table returns empty slice

**`internal/rag/indexer_test.go`** — Mock embedder + real SQLite:
- **New note:** chunks + vectors stored, hash saved, embed called
- **Unchanged note:** second index skipped, embed NOT called (hash match)
- **Updated note:** old chunks deleted, new chunks created, hash updated
- **Encrypted note:** `encryption_applied=1` → skipped, no chunks
- **Embed error:** no partial state left in DB, hash not updated
- **Orphan cleanup:** hash for deleted note → `IndexAll` removes it
- **Mixed batch:** 3 notes (new, unchanged, encrypted) → correct counts
- **Full queue:** `Enqueue` on full channel doesn't block

**`internal/rag/search_test.go`** — Mock embedder + real SQLite:
- Returns `[]*models.Note` with correct fields (ID, Title, ParentID, UpdatedTime, Type_)
- Deduplicates multiple chunks from same note → single result
- Empty index → empty slice, no error
- Limit and hasMore flag correctness

**`internal/mcp/tools_test.go`** — Add to existing file:
- `TestSearchNotes_WithRAGSearcher` — RAG used, response shape `{notes, count}` correct
- `TestSearchNotes_RAGFallbackToFTS4` — RAG error → FTS4 fallback, still returns results
- `TestSearchNotes_RAGNil` — nil RAGSearcher → FTS4, same behavior as before

### What is NOT unit tested

- **Search relevance** — mock vectors aren't semantic; relevance validated manually
- **Real embedding API** — embedder tests use `httptest.Server`, never OpenAI/ollama
- **Concurrency** — covered by `go test -race ./...`, no explicit concurrency tests
- **Performance** — no benchmarks in v1; Prometheus metrics cover production

---

## Verification

1. **Build:** `CGO_ENABLED=1 go build -ldflags="-s -w" -o go-joplin .`
2. **Tests:** `go test ./...` and `go test -race ./...`
3. **Lint:** `golangci-lint run ./...` and `go vet ./...`
4. **Manual test:**
   - Start server with RAG enabled pointing at a local ollama or OpenAI endpoint
   - Sync notes
   - Verify logs show indexing activity
   - Search via MCP tool and Clipper API — verify semantic results
   - Verify Prometheus metrics at `:9091/metrics` include `rag_*` metrics
   - Verify traces in OTLP collector show `rag.*` spans
5. **Fallback test:** Disable RAG in config → verify FTS4 search still works

---

## File Summary

| File | Action |
|------|--------|
| `go.mod` | Add sqlite-vec-go-bindings/cgo |
| `internal/store/db.go` | Add rag_note_hashes + rag_chunks tables, `InitRAG()` |
| `internal/store/rag.go` | **New** — RAGStore methods |
| `internal/store/rag_test.go` | **New** — Store-level RAG tests |
| `internal/rag/embedder.go` | **New** — OpenAI-compatible embedding client |
| `internal/rag/embedder_test.go` | **New** — Embedder tests |
| `internal/rag/chunker.go` | **New** — Fixed-size overlap chunker |
| `internal/rag/chunker_test.go` | **New** — Chunker tests |
| `internal/rag/indexer.go` | **New** — Async indexing orchestrator |
| `internal/rag/indexer_test.go` | **New** — Indexer tests |
| `internal/rag/search.go` | **New** — Vector search with fallback |
| `internal/rag/search_test.go` | **New** — Search integration tests |
| `internal/rag/metrics.go` | **New** — Prometheus metrics registration |
| `internal/config/config.go` | Add `RAGConfig` struct + env var loading |
| `internal/config/yaml.go` | Add YAML mapping for `rag:` section |
| `internal/config/config_test.go` | Add RAG config test cases |
| `config.yaml.example` | Add RAG config section |
| `internal/mcp/tools.go` | Add `RAGSearcher`+`RAGIndexer` to Deps, modify search/create/update handlers |
| `internal/mcp/server.go` | Wire RAGSearcher and RAGIndexer through Deps |
| `internal/clipper/search.go` | Add RAGSearcher fallback in note search |
| `internal/clipper/server.go` | Accept optional RAGSearcher and RAGIndexer |
| `internal/clipper/notes.go` | Enqueue on create/update, DeleteNoteRAGData on delete |
| `internal/store/sync_state.go` | Add `DeleteNoteRAGData` call in `DeleteLocalItem` for TypeNote |
| `cmd/serve.go` | Wire up RAG indexer, start/stop lifecycle, IndexAll after sync |
| `docs/adr/001-rag-search.md` | Architecture Decision Record |
| `implementation_plans/001-rag-search/implementation.md` | This file |
| `CLAUDE.md` | Add ADR + implementation plan rules |
