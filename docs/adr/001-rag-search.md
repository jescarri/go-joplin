# ADR-001: RAG-Based Semantic Search

**Status:** Proposed
**Date:** 2026-03-23
**Author:** jescarri

## Context

go-joplin currently provides keyword-based full-text search via SQLite FTS4. The MCP `search_notes` tool and the Clipper `GET /search` endpoint both rely on FTS4 `MATCH` queries against note title and body. This works well for exact keyword lookups but fails when users search by meaning — e.g., searching "container orchestration" won't find a note titled "Kubernetes deployment guide" unless those exact words appear.

We want to add RAG (Retrieval Augmented Generation) semantic search using vector embeddings. Notes are chunked, embedded via an OpenAI-compatible API, and stored in sqlite-vec for KNN similarity search. The existing FTS4 search remains as a fallback when RAG is disabled or unavailable.

## Decision

Adopt a RAG pipeline that:

1. **Chunks** each note into fixed-size overlapping text segments
2. **Embeds** each chunk via an OpenAI-compatible `/v1/embeddings` endpoint (supports OpenAI, ollama, local-ai, etc.)
3. **Stores** embeddings in sqlite-vec (`vec0` virtual table) in the same SQLite database — never synced to S3/Joplin Server
4. **Searches** by embedding the query, running KNN against the vector table, deduplicating by note, and returning full note objects
5. **Falls back** to FTS4 gracefully when RAG is disabled, not yet indexed, or the embedding API is unreachable

RAG is opt-in (`rag.enabled: false` by default). When disabled, all search paths use FTS4 exactly as today.

## Design

### 1. Package Structure

```
internal/rag/
  chunker.go          — Fixed-size overlap text chunker
  embedder.go         — OpenAI-compatible embedding HTTP client
  indexer.go          — Async pipeline: hash check → chunk → embed → store
  search.go           — Vector similarity search with FTS4 fallback
  metrics.go          — Prometheus metrics registration

internal/store/
  rag.go              — RAG-specific DB operations (new file)
```

### 2. Database Schema

RAG tables live in the same `gojoplin.sqlite` but use the `rag_` prefix and are never referenced by the sync engine. Migration is gated behind a separate `rag_schema_version` key in `key_values`.

#### Data Model

A note is split into one or more chunks. Each chunk has exactly one embedding vector. The relationship is:

```
notes (existing)          rag_note_hashes           rag_chunks              rag_vec
┌──────────────┐         ┌─────────────────┐       ┌──────────────────┐    ┌──────────────────┐
│ id (PK)      │◄────────│ note_id (PK/FK) │       │ id (PK, auto)    │───►│ chunk_id (PK/FK) │
│ title        │         │ body_hash       │       │ note_id (FK)     │    │ embedding        │
│ body         │         │ updated_at      │       │ chunk_index      │    │   float[N]       │
│ ...          │◄────────┼─────────────────┘       │ content          │    └──────────────────┘
└──────────────┘         │                         │ token_count      │
                         │                         └──────────────────┘
                         │                              ▲
                         └──────────────────────────────┘
                           notes.id = rag_chunks.note_id
                           rag_chunks.id = rag_vec.chunk_id
```

**Relationships:**
- `rag_note_hashes.note_id` → `notes.id` (1:1) — tracks whether a note's content has changed since last indexing
- `rag_chunks.note_id` → `notes.id` (N:1) — a note produces 1..N chunks depending on body length
- `rag_vec.chunk_id` → `rag_chunks.id` (1:1) — each chunk has exactly one embedding vector

**Search join path:** To find notes matching a query, we embed the query, KNN-search `rag_vec` for nearest vectors, join to `rag_chunks` to get `note_id`, deduplicate by note, then fetch full `Note` objects from `notes`:

```sql
-- Step 1: KNN search → chunk_ids with distances
SELECT cv.chunk_id, c.note_id, cv.distance
FROM rag_vec cv
JOIN rag_chunks c ON c.id = cv.chunk_id
WHERE cv.embedding MATCH ?    -- query vector
ORDER BY cv.distance
LIMIT ?                       -- over-fetch (limit * 3) for dedup

-- Step 2: application-level dedup by note_id, keep lowest distance per note
-- Step 3: SELECT * FROM notes WHERE id IN (?) for the top N note_ids
```

**Deletion cascade:** When a note is deleted, `DeleteNoteRAGData(noteID)` removes rows from all three RAG tables:
1. `DELETE FROM rag_vec WHERE chunk_id IN (SELECT id FROM rag_chunks WHERE note_id = ?)`
2. `DELETE FROM rag_chunks WHERE note_id = ?`
3. `DELETE FROM rag_note_hashes WHERE note_id = ?`

Note: SQLite foreign keys with `ON DELETE CASCADE` are not used because `rag_vec` is a virtual table (`vec0`) which does not support foreign key constraints. Cascade is handled in application code.

#### Schema DDL

```sql
-- Content hash for change detection
CREATE TABLE IF NOT EXISTS rag_note_hashes (
    note_id    TEXT PRIMARY KEY,   -- references notes.id
    body_hash  TEXT NOT NULL,      -- SHA-256 hex of (title + "\n" + body)
    updated_at INTEGER NOT NULL DEFAULT 0
);

-- Text chunks with metadata
CREATE TABLE IF NOT EXISTS rag_chunks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,  -- referenced by rag_vec.chunk_id
    note_id     TEXT NOT NULL,                      -- references notes.id
    chunk_index INTEGER NOT NULL,                   -- 0-based position within the note
    content     TEXT NOT NULL,                      -- chunk text
    token_count INTEGER NOT NULL DEFAULT 0,
    UNIQUE(note_id, chunk_index)
);

CREATE INDEX IF NOT EXISTS idx_rag_chunks_note_id ON rag_chunks(note_id);

-- Vector table (sqlite-vec virtual table); dimension set at runtime from config
-- chunk_id references rag_chunks.id (enforced in application code, not FK constraint)
CREATE VIRTUAL TABLE IF NOT EXISTS rag_vec USING vec0(
    chunk_id INTEGER PRIMARY KEY,
    embedding float[<dimensions>]
);
```

The `vec0` DDL includes the dimension literal, so it must be generated at runtime from `RAGConfig.Dimensions`.

**RAG metadata** is tracked in the existing `key_values` table:

| Key | Value | Purpose |
|-----|-------|---------|
| `rag_schema_version` | `1` | Gates RAG schema migration |
| `rag_model` | e.g. `text-embedding-3-small` | Detects model changes |
| `rag_dimensions` | e.g. `1536` | Detects dimension changes |

### 2a. InitRAG — Startup Initialization

`db.InitRAG(model string, dimensions int) error` is called from `cmd/serve.go` only when RAG is enabled. It handles first-run setup, model/dimension changes, and idempotent restarts.

**Logic:**

```
1. Create rag_note_hashes and rag_chunks tables (IF NOT EXISTS — safe to re-run)

2. Read stored rag_model and rag_dimensions from key_values

3. If no stored values (first run):
   a. CREATE VIRTUAL TABLE rag_vec USING vec0(embedding float[<dimensions>])
   b. Store rag_model and rag_dimensions in key_values
   c. Return (IndexAll will backfill everything)

4. If stored model != config model OR stored dimensions != config dimensions:
   a. Log warning: "embedding model/dimensions changed, full re-index required"
   b. DROP TABLE IF EXISTS rag_vec
   c. DELETE FROM rag_chunks
   d. DELETE FROM rag_note_hashes        ← clears all hashes, forcing full re-index
   e. CREATE VIRTUAL TABLE rag_vec USING vec0(embedding float[<new_dimensions>])
   f. Update rag_model and rag_dimensions in key_values
   g. Return (IndexAll will re-process all notes)

5. If stored values match config (normal restart):
   a. Verify rag_vec table exists (CREATE VIRTUAL TABLE IF NOT EXISTS)
   b. Return (IndexAll will only process changed notes via hash check)
```

**Why model changes require full re-index:** Different embedding models produce vectors in incompatible embedding spaces. A vector from `text-embedding-3-small` is meaningless next to a vector from `nomic-embed-text`, even if the dimensions happen to match. Mixing vectors from different models would produce garbage search results.

**Why dimension changes require full re-index:** sqlite-vec rejects inserts where the vector length doesn't match the table's declared dimension. The table must be dropped and recreated with the new dimension.

### 2b. Encryption Interaction

Notes arrive from the Joplin Server with E2EE applied (`encryption_applied=1`, body is empty, cipher text in `encryption_cipher_text`). The sync engine's `DecryptPulledItems()` step decrypts them in-place — after decryption, `notes.body` contains plaintext markdown and `encryption_applied=0`.

**RAG indexing runs after decryption.** The flow is:

```
Sync cycle:
  1. PullChanges()         → notes stored with encryption_applied=1 (still encrypted)
  2. DecryptPulledItems()  → notes updated to encryption_applied=0 (plaintext in body)
  3. [sync completes]
  4. ragIndexer.IndexAll() → triggered after sync, processes decrypted notes
```

**The indexer skips any note where `encryption_applied == 1`.** These are notes that:
- Haven't been decrypted yet (no `master_password` configured)
- Failed decryption (wrong key, corrupted cipher text)

**RAG chunks do not need encryption.** The local SQLite database already stores decrypted note bodies in plaintext (`notes.body`). The chunks are derived from the same plaintext — encrypting them would add no security since the source material is already readable to anyone with access to the database file.

### 3. Configuration

```go
type RAGConfig struct {
    Enabled      bool   // GOJOPLIN_RAG_ENABLED
    Endpoint     string // GOJOPLIN_RAG_ENDPOINT — OpenAI-compatible base URL
    APIKey       string // GOJOPLIN_RAG_API_KEY
    Model        string // GOJOPLIN_RAG_MODEL (e.g. "text-embedding-3-small")
    Dimensions   int    // GOJOPLIN_RAG_DIMENSIONS (default: 1536)
    ChunkSize    int    // GOJOPLIN_RAG_CHUNK_SIZE (default: 512 tokens)
    ChunkOverlap int    // GOJOPLIN_RAG_CHUNK_OVERLAP (default: 50 tokens)
    Workers      int    // GOJOPLIN_RAG_WORKERS (default: 2)
    QueueSize    int    // GOJOPLIN_RAG_QUEUE_SIZE (default: 1000)
}
```

YAML config section:

```yaml
rag:
  enabled: false
  endpoint: "${GOJOPLIN_RAG_ENDPOINT}"
  api_key: "${GOJOPLIN_RAG_API_KEY}"
  model: "text-embedding-3-small"
  dimensions: 1536
  chunk_size: 512
  chunk_overlap: 50
  workers: 2
  queue_size: 1000
```

All env vars use the `GOJOPLIN_RAG_` prefix. Secrets (`api_key`) use `${VAR}` expansion, consistent with existing config patterns.

### 4. Core Interfaces

```go
// Embedder calls an OpenAI-compatible /v1/embeddings endpoint.
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dimensions() int
}

// RAGStore abstracts RAG-specific database operations.
type RAGStore interface {
    GetNote(id string) (*models.Note, error)
    GetNoteHash(noteID string) (string, error)
    UpsertNoteHash(noteID, hash string) error
    DeleteChunksByNoteID(noteID string) error
    InsertChunk(noteID string, idx int, content string, tokenCount int) (int64, error)
    InsertChunkEmbedding(chunkID int64, embedding []float32) error
    SearchVectors(embedding []float32, limit int) ([]VectorResult, error)
    DeleteNoteRAGData(noteID string) error
    ListAllNoteIDs() ([]string, error)
}

// RAGSearcher performs semantic search across notes.
// When nil in MCP/Clipper deps, handlers fall back to FTS4.
type RAGSearcher interface {
    Search(ctx context.Context, query string, limit int) ([]*models.Note, bool, error)
}
```

### 5. Chunking Strategy

Fixed-size overlapping chunks (configurable, default 512 tokens / 50 token overlap):

- Token counting via whitespace split (good enough for embedding models)
- Title prepended to first chunk for context: `title + "\n\n" + body`
- Short notes (below chunk size) produce a single chunk
- Each chunk carries its index position within the note

### 6. Content Hash — Change Detection

```go
func hashNoteContent(title, body string) string {
    h := sha256.New()
    h.Write([]byte(title))
    h.Write([]byte("\n"))
    h.Write([]byte(body))
    return hex.EncodeToString(h.Sum(nil))
}
```

Before processing a note, the indexer compares the computed hash against `rag_note_hashes`. If unchanged, the note is skipped entirely — no chunking, no embedding API call.

### 7. Indexing Pipeline

**Per-note pipeline (runs in worker goroutine):**

```
1. GetNote(id) — fetch note
2. Skip if encryption_applied == 1 (encrypted notes can't be chunked)
3. SHA-256(title + "\n" + body) → compare with GetNoteHash(noteID)
4. If hash matches → skip (note unchanged)
5. Chunk(title + "\n\n" + body) → chunks
6. Embed(ctx, chunk_contents) → vectors (batched)
7. DeleteChunksByNoteID(noteID) → clear old chunks + vectors
8. For each chunk: InsertChunk() → chunk_id, then InsertChunkEmbedding(chunk_id, vector)
9. UpsertNoteHash(noteID, newHash)
```

**Worker pool:**

- `Indexer` struct with buffered channel (`queue`) for note IDs
- Configurable worker count (default: 2 goroutines)
- `Enqueue(noteID)` — non-blocking send; drops if full, logs warning
- `IndexAll(ctx)` — full sweep: iterates all notes, enqueues each (skips unchanged via hash)
- `Start(ctx)` — starts workers, stops on ctx cancellation
- `Stop()` — drains queue, waits for in-flight work

### 8. Search Pipeline

```
1. Embed(ctx, []string{query}) → query vector
2. SearchVectors(queryVec, limit*3) — over-fetch (multiple chunks per note)
3. Deduplicate by note_id, keep best (lowest distance) per note
4. Fetch full Note objects via GetNote(id) for top `limit` results
5. Return []*models.Note, hasMore, error — same signature as store.SearchNotes()
```

**Fallback:** If `RAGSearcher` is nil or returns an error, handlers fall back to `db.SearchNotes()` (FTS4). This means:
- RAG disabled in config → FTS4 always
- Embedding API down → FTS4 with logged warning
- Initial indexing not complete → FTS4 until vectors are available

### 9. Integration Points — Every Note Mutation Path

There are exactly 7 code paths that create, update, or delete notes. Each must have a corresponding RAG action. Missing any path causes stale or orphaned RAG data.

#### 9a. Note Creation

| Path | Code Location | Current Flow | RAG Action |
|------|--------------|--------------|------------|
| Clipper API `POST /notes` | `clipper/notes.go:handleCreateNote` | `db.CreateNote(note)` → `triggerSync()` | Add: `ragIndexer.Enqueue(note.ID)` after `CreateNote` succeeds |
| MCP `create_note` tool | `mcp/tools.go:createNoteHandler` | `db.CreateNote(note)` → `Syncer.TriggerSync()` | Add: `ragIndexer.Enqueue(note.ID)` after `CreateNote` succeeds |
| Sync pull (new note) | `sync/delta.go:applyDeltaItem` | `db.UpsertNote(note)` | Covered by `IndexAll()` after sync completes (see 9d) |
| Sync decrypt (first decrypt) | `sync/decrypt.go:decryptItem` | `db.UpsertNote(note)` with `encryption_applied=0` | Covered by `IndexAll()` after sync completes — note was previously skipped (encrypted), now eligible |

#### 9b. Note Update

| Path | Code Location | Current Flow | RAG Action |
|------|--------------|--------------|------------|
| Clipper API `PUT /notes/{id}` | `clipper/notes.go:handleUpdateNote` | `db.UpdateNote(existing)` → `triggerSync()` | Add: `ragIndexer.Enqueue(existing.ID)` after `UpdateNote` succeeds. The indexer's hash check detects the body changed → re-chunks → re-embeds. If only metadata changed (title unchanged, body unchanged), hash matches → skip. |
| MCP `update_note` tool | `mcp/tools.go:updateNoteHandler` | `db.UpdateNote(note)` → `Syncer.TriggerSync()` | Add: `ragIndexer.Enqueue(note.ID)` after `UpdateNote` succeeds. Same hash-check logic. |
| Sync pull (updated note) | `sync/delta.go:applyDeltaItem` | `db.UpsertNote(note)` | Covered by `IndexAll()` after sync completes. Hash check detects changed body → re-index. |
| Sync decrypt (re-decrypt) | `sync/decrypt.go:decryptItem` | `db.UpsertNote(note)` | Covered by `IndexAll()` after sync completes. |

**How updates work in detail:** The indexer does not diff chunks. When a note's hash changes, it deletes ALL old chunks + vectors for that note, re-chunks the entire note, re-embeds all chunks, and stores the new vectors. This is simpler and correct — partial chunk updates would be complex and error-prone since chunk boundaries shift when text changes.

#### 9c. Note Deletion

| Path | Code Location | Current Flow | RAG Action |
|------|--------------|--------------|------------|
| Clipper API `DELETE /notes/{id}` | `clipper/notes.go:handleDeleteNote` | `db.DeleteNote(id)` (deletes from `notes` + `note_tags`, records item_change) → `triggerSync()` | Add: `db.DeleteNoteRAGData(id)` after `DeleteNote` succeeds |
| Sync pull (delta type=3) | `sync/delta.go:applyDeltaItem` | `db.DeleteLocalItem(itemID, itemType)` → raw `DELETE FROM notes WHERE id = ?` | **Problem:** `DeleteLocalItem` does a raw SQL DELETE — it does NOT call `DeleteNote()` and does NOT know about RAG tables. **Fix:** Add `db.DeleteNoteRAGData(itemID)` call inside `DeleteLocalItem` when `itemType == TypeNote`. |
| Sync reconcile (orphan cleanup) | `sync/reconcile.go:141` | `db.DeleteLocalItem(si.ItemID, si.ItemType)` | Same fix as above — `DeleteLocalItem` handles it. |

**Important:** There is no MCP tool for deleting notes — deletion only happens via Clipper API or sync.

#### 9d. Bulk Reindex After Sync

After every successful `engine.Sync(ctx)` in the background loop, call `go ragIndexer.IndexAll(ctx)`. This is the safety net that catches:
- Notes created/updated/decrypted during sync (via `UpsertNote`)
- Any notes missed by individual `Enqueue` calls
- Notes that were encrypted and are now decryptable

`IndexAll` iterates all notes, computes hashes, and skips unchanged ones. Cost is O(N) hash comparisons but zero embedding API calls for unchanged notes.

#### 9e. Orphan Cleanup on Startup

On startup, `IndexAll` also handles orphaned RAG data — if a note was deleted while the server was down, `IndexAll` should detect that `rag_note_hashes` contains a `note_id` that no longer exists in `notes` and clean it up. Add to `IndexAll`:

```
1. indexed_ids = SELECT note_id FROM rag_note_hashes
2. For each indexed_id: if GetNote(indexed_id) returns nil → DeleteNoteRAGData(indexed_id)
3. Then proceed with normal hash-check indexing of all existing notes
```

#### 9f. Search Integration

**Where search is wired:**

- `internal/mcp/tools.go` — `Deps` gets `RAGSearcher` field (nil = FTS4); `searchNotesHandler` tries RAG first, falls back to FTS4 on error
- `internal/clipper/search.go` — `Server` gets `RAGSearcher` field; `handleSearch` for type "note" tries RAG first, falls back to FTS4 on error
- Response format is **unchanged**: MCP returns `{notes: [{id, title, parent_id, updated_time}], count}`, Clipper returns full paginated note objects

**Wiring in `cmd/serve.go`:**

```go
var ragIndexer *rag.Indexer
var ragSearcher rag.RAGSearcher
if cfg.RAG.Enabled {
    db.InitRAG(cfg.RAG.Model, cfg.RAG.Dimensions)
    embedder := rag.NewOpenAIEmbedder(cfg.RAG)
    ragIndexer = rag.NewIndexer(db, embedder, cfg.RAG)
    ragSearcher = ragIndexer // Indexer implements RAGSearcher
    ragIndexer.Start(ctx)
    defer ragIndexer.Stop()
    go ragIndexer.IndexAll(ctx)
}

// Pass ragIndexer and ragSearcher (nil when RAG disabled) to MCP and Clipper
mcpDeps := &mcp.Deps{DB: db, Syncer: engine, Policy: policy, RAGSearcher: ragSearcher, RAGIndexer: ragIndexer, ...}
srv := clipper.NewServer(db, ..., ragSearcher, ragIndexer)
```

### 10. Background Worker Lifecycle

```
cmd/serve.go
  ├── db.Open()
  ├── db.InitRAG(model, dimensions)    // creates/rebuilds vec0 table if RAG enabled
  ├── ragIndexer.Start(ctx)            // spawns worker goroutines
  ├── go ragIndexer.IndexAll(ctx)      // initial backfill (async)
  ├── sync loop:
  │     └── after Sync() → go ragIndexer.IndexAll(ctx)
  ├── HTTP server (MCP + Clipper use ragSearcher)
  └── SIGTERM → cancel(ctx)
        ├── ragIndexer.Stop()          // drains queue, waits for workers
        ├── httpSrv.Shutdown()
        └── db.Close()
```

The indexer follows the same context-cancellation pattern as the sync engine. In-flight embedding API calls respect the context. On shutdown, partial work is abandoned and resumes on next startup via hash-based change detection.

### 11. Observability

**Tracing spans** (all under `rag.*` namespace):

| Span | Attributes |
|------|-----------|
| `rag.embed` | model, batch_size, dimensions |
| `rag.chunk` | note_id, chunks_count |
| `rag.index_note` | note_id, status (indexed/skipped/error) |
| `rag.index_all` | total_notes, indexed, skipped |
| `rag.search` | query_length, results_count |
| `rag.search.vector_query` | limit, results_count, best_distance |

**Prometheus metrics** (registered with existing `telemetry.Reg`):

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

### 12. Dependencies

Single new Go module dependency:

- `github.com/asg017/sqlite-vec-go-bindings/cgo` — statically links sqlite-vec into the binary via CGO (already required for `mattn/go-sqlite3`)

The embedding HTTP client uses `net/http` from the standard library. No other new dependencies.

### 13. Testing Strategy

The critical path of the RAG pipeline (hash check → chunk → store → search) must be testable **without an external embedding service**. A mock embedder produces deterministic vectors so the entire pipeline can be exercised in `go test ./...`.

#### 13a. Mock Embedder

```go
// mockEmbedder returns deterministic vectors derived from input text hash.
// Each text gets a unique, reproducible float[N] vector based on SHA-256.
type mockEmbedder struct {
    dimensions int
    calls      [][]string // records all Embed() calls for assertions
}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
    m.calls = append(m.calls, texts)
    vecs := make([][]float32, len(texts))
    for i, t := range texts {
        vecs[i] = hashToVector(t, m.dimensions)
    }
    return vecs, nil
}

func (m *mockEmbedder) Dimensions() int { return m.dimensions }

// hashToVector produces a deterministic float32 vector from a string.
// Similar strings will NOT produce similar vectors — this is fine for
// testing pipeline correctness, not search relevance.
func hashToVector(s string, dims int) []float32 {
    h := sha256.Sum256([]byte(s))
    vec := make([]float32, dims)
    for i := range vec {
        vec[i] = float32(h[i%32]) / 255.0
    }
    return vec
}
```

This mock is used by indexer, search, and integration tests. It verifies the pipeline processes notes correctly without calling any external API. Search results won't be semantically meaningful, but we can verify:
- Correct notes are indexed (right note IDs in DB)
- Updated notes get re-indexed (old chunks replaced)
- Deleted notes get cleaned up
- Search returns results in the expected format
- Deduplication works (multiple chunks from same note → one result)

#### 13b. Test Files and Cases

**`internal/rag/chunker_test.go`** — Pure unit tests, no dependencies:

| Test | What it verifies |
|------|-----------------|
| `TestChunk_EmptyText` | Returns empty slice |
| `TestChunk_ShortText` | Text below chunk size → single chunk with full content |
| `TestChunk_ExactChunkSize` | Text exactly at chunk size → single chunk, no overlap created |
| `TestChunk_MultipleChunks` | Long text splits into N chunks with correct overlap |
| `TestChunk_OverlapContent` | Overlapping region matches end of chunk N and start of chunk N+1 |
| `TestChunk_TokenCount` | Each chunk reports correct token count |
| `TestChunk_Unicode` | Multi-byte characters don't split mid-rune |
| `TestChunk_TitlePrepend` | Title is prepended to body before chunking |

**`internal/rag/embedder_test.go`** — Uses `httptest.Server` to mock the OpenAI API:

| Test | What it verifies |
|------|-----------------|
| `TestEmbed_SingleText` | Single text returns single vector with correct dimensions |
| `TestEmbed_Batch` | Multiple texts return matching number of vectors |
| `TestEmbed_APIError429` | Rate limit response triggers retry |
| `TestEmbed_APIError500` | Server error returns error to caller |
| `TestEmbed_InvalidResponse` | Malformed JSON returns error |
| `TestEmbed_ContextCancelled` | Cancelled context aborts the request |
| `TestEmbed_DimensionMismatch` | API returns wrong dimension count → error |

**`internal/store/rag_test.go`** — Uses `testDB(t)` helper (real SQLite, temp dir), requires sqlite-vec:

| Test | What it verifies |
|------|-----------------|
| `TestInitRAG_FirstRun` | Creates all RAG tables, stores model/dimensions in key_values |
| `TestInitRAG_SameConfig` | Idempotent — no error, no data loss |
| `TestInitRAG_DimensionChange` | Drops rag_vec, clears chunks + hashes, recreates vec table |
| `TestInitRAG_ModelChange` | Same as dimension change — full wipe |
| `TestNoteHash_CRUD` | Upsert, get, delete hash; missing hash returns empty |
| `TestChunks_InsertAndDelete` | Insert chunks, verify content, delete by note_id, verify gone |
| `TestChunkEmbedding_InsertAndSearch` | Insert chunk + vector, KNN search returns it |
| `TestSearchVectors_MultipleNotes` | Insert vectors for 3 notes, search returns closest first |
| `TestSearchVectors_EmptyTable` | Search on empty vec table returns empty results, no error |
| `TestDeleteNoteRAGData` | Deletes from all 3 tables (rag_vec, rag_chunks, rag_note_hashes) |
| `TestDeleteNoteRAGData_NonExistent` | Deleting non-existent note is a no-op, no error |

**`internal/rag/indexer_test.go`** — Uses mock embedder + real SQLite (via `testDB`):

| Test | What it verifies |
|------|-----------------|
| `TestIndexNote_NewNote` | Note gets chunked, embedded (mock), chunks+vectors stored, hash saved |
| `TestIndexNote_UnchangedNote` | Same note indexed twice — second time skipped (hash match), no embed call |
| `TestIndexNote_UpdatedNote` | Change note body, re-index — old chunks deleted, new chunks created, hash updated |
| `TestIndexNote_EncryptedNote` | Note with `encryption_applied=1` is skipped entirely, no chunks created |
| `TestIndexNote_EmptyBody` | Note with empty body produces no chunks (or single empty chunk — define behavior) |
| `TestIndexAll_MixedNotes` | 3 notes (1 new, 1 unchanged, 1 encrypted) — verifies correct skip/index counts |
| `TestIndexAll_OrphanCleanup` | Hash exists for deleted note → `IndexAll` removes orphaned RAG data |
| `TestEnqueue_NonBlocking` | Enqueue on full queue doesn't block, logs warning |
| `TestIndexNote_EmbedError` | Embedder returns error → note not partially indexed (no chunks stored), hash not updated |

**`internal/rag/search_test.go`** — Uses mock embedder + real SQLite:

| Test | What it verifies |
|------|-----------------|
| `TestSearch_ReturnsNotes` | Index 3 notes, search returns `[]*models.Note` with correct fields |
| `TestSearch_ResponseFormat` | Returned notes have ID, Title, ParentID, UpdatedTime, Type_ set |
| `TestSearch_DeduplicatesChunks` | Long note produces multiple chunks; search returns the note once (best distance) |
| `TestSearch_EmptyIndex` | Search on empty index returns empty slice, no error |
| `TestSearch_LimitRespected` | Index 10 notes, search with limit=3 returns exactly 3 |
| `TestSearch_HasMore` | hasMore flag set correctly when more results available |

**`internal/mcp/tools_test.go`** — Add tests for RAG search integration (existing file):

| Test | What it verifies |
|------|-----------------|
| `TestSearchNotes_WithRAGSearcher` | When RAGSearcher is set in Deps, search uses it; verify response shape `{notes, count}` |
| `TestSearchNotes_RAGFallbackToFTS4` | RAGSearcher returns error → handler falls back to FTS4, still returns results |
| `TestSearchNotes_RAGNil` | RAGSearcher is nil → FTS4 used, same behavior as before |

#### 13c. What is NOT tested in unit tests

- **Search relevance** — mock embedder produces hash-based vectors, not semantic vectors. We verify the pipeline works, not that "kubernetes" matches "container orchestration". Relevance depends on the embedding model and is validated manually.
- **Real embedding API** — no tests call OpenAI/ollama. The `embedder_test.go` uses `httptest.Server` to verify HTTP client behavior (request format, response parsing, error handling).
- **Concurrent worker behavior** — race conditions are caught by `go test -race ./...` but we don't write explicit concurrency tests. The worker pool is simple (channel + goroutines) and the race detector covers it.
- **Performance/latency** — no benchmarks planned in v1. Prometheus metrics cover this in production.

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| sqlite-vec CGO compatibility | Both sqlite-vec and mattn/go-sqlite3 use CGO; the bindings are designed to work together. Validate in Step 1. |
| Embedding API latency blocks operations | Indexer runs in background goroutines, never blocks MCP/Clipper. Batch embedding reduces round-trips. |
| Large corpus causes slow initial backfill | Hash-based skip means only new/changed notes are processed. Workers are configurable. Progress survives restarts. |
| Model or dimension change requires full re-index | `InitRAG()` detects mismatch via `rag_model`/`rag_dimensions` in `key_values`. Drops vec table, clears all chunks and hashes, recreates vec table with new dimensions. Logs warning. Next `IndexAll()` re-processes all notes. |
| RAG tables accidentally synced | `rag_` prefix tables are never referenced in sync push logic. Sync only pushes items tracked in `sync_items`. |
| Embedding API unavailable | Graceful fallback to FTS4. Indexer retries on next trigger. Search logs warning and uses FTS4. |

## Consequences

- **Positive:** Users get semantic search — finding notes by meaning, not just keywords
- **Positive:** Fully opt-in; zero impact when disabled
- **Positive:** Works with any OpenAI-compatible endpoint (OpenAI, ollama, local-ai, vLLM, etc.)
- **Positive:** Full observability via existing tracing + metrics infrastructure
- **Negative:** Adds CGO dependency on sqlite-vec (but CGO is already required for mattn/go-sqlite3)
- **Negative:** Requires an embedding API endpoint to be available (but degrades gracefully)
- **Negative:** Initial backfill can be slow for large note collections (mitigated by background processing + hash skip)
