package store

import (
	"testing"
)

func TestInitRAG_FirstRun(t *testing.T) {
	db := testDB(t)
	if err := db.InitRAG("test-model", 4); err != nil {
		t.Fatalf("InitRAG: %v", err)
	}
	// Verify metadata stored
	if got := db.getKV("rag_model"); got != "test-model" {
		t.Errorf("rag_model: got %q, want %q", got, "test-model")
	}
	if got := db.getKV("rag_dimensions"); got != "4" {
		t.Errorf("rag_dimensions: got %q, want %q", got, "4")
	}
}

func TestInitRAG_Idempotent(t *testing.T) {
	db := testDB(t)
	if err := db.InitRAG("model", 4); err != nil {
		t.Fatalf("first InitRAG: %v", err)
	}
	if err := db.InitRAG("model", 4); err != nil {
		t.Fatalf("second InitRAG: %v", err)
	}
}

func TestInitRAG_DimensionChange(t *testing.T) {
	db := testDB(t)
	if err := db.InitRAG("model", 4); err != nil {
		t.Fatalf("InitRAG(4): %v", err)
	}

	// Insert some data
	chunkID, err := db.InsertChunk("note1", 0, "hello", 1)
	if err != nil {
		t.Fatalf("InsertChunk: %v", err)
	}
	if err := db.InsertChunkEmbedding(chunkID, []float32{0.1, 0.2, 0.3, 0.4}); err != nil {
		t.Fatalf("InsertChunkEmbedding: %v", err)
	}
	if err := db.UpsertNoteHash("note1", "abc123"); err != nil {
		t.Fatalf("UpsertNoteHash: %v", err)
	}

	// Change dimensions — should wipe everything
	if err := db.InitRAG("model", 8); err != nil {
		t.Fatalf("InitRAG(8): %v", err)
	}

	// Data should be wiped
	hash, _ := db.GetNoteHash("note1")
	if hash != "" {
		t.Error("hash should be wiped after dimension change")
	}
	if got := db.getKV("rag_dimensions"); got != "8" {
		t.Errorf("rag_dimensions: got %q, want %q", got, "8")
	}
}

func TestInitRAG_ModelChange(t *testing.T) {
	db := testDB(t)
	if err := db.InitRAG("model-a", 4); err != nil {
		t.Fatalf("InitRAG(model-a): %v", err)
	}
	if err := db.UpsertNoteHash("note1", "abc"); err != nil {
		t.Fatalf("UpsertNoteHash: %v", err)
	}

	// Change model — should wipe
	if err := db.InitRAG("model-b", 4); err != nil {
		t.Fatalf("InitRAG(model-b): %v", err)
	}
	hash, _ := db.GetNoteHash("note1")
	if hash != "" {
		t.Error("hash should be wiped after model change")
	}
	if got := db.getKV("rag_model"); got != "model-b" {
		t.Errorf("rag_model: got %q, want %q", got, "model-b")
	}
}

func TestNoteHash_CRUD(t *testing.T) {
	db := testDB(t)
	if err := db.InitRAG("model", 4); err != nil {
		t.Fatalf("InitRAG: %v", err)
	}

	// Get non-existent
	hash, err := db.GetNoteHash("nonexistent")
	if err != nil {
		t.Fatalf("GetNoteHash: %v", err)
	}
	if hash != "" {
		t.Errorf("expected empty, got %q", hash)
	}

	// Upsert
	if err := db.UpsertNoteHash("note1", "abc123"); err != nil {
		t.Fatalf("UpsertNoteHash: %v", err)
	}
	hash, err = db.GetNoteHash("note1")
	if err != nil {
		t.Fatalf("GetNoteHash: %v", err)
	}
	if hash != "abc123" {
		t.Errorf("got %q, want %q", hash, "abc123")
	}

	// Update
	if err := db.UpsertNoteHash("note1", "def456"); err != nil {
		t.Fatalf("UpsertNoteHash update: %v", err)
	}
	hash, _ = db.GetNoteHash("note1")
	if hash != "def456" {
		t.Errorf("got %q, want %q", hash, "def456")
	}
}

func TestChunks_InsertAndDelete(t *testing.T) {
	db := testDB(t)
	if err := db.InitRAG("model", 4); err != nil {
		t.Fatalf("InitRAG: %v", err)
	}

	id1, err := db.InsertChunk("note1", 0, "chunk zero", 2)
	if err != nil {
		t.Fatalf("InsertChunk: %v", err)
	}
	id2, err := db.InsertChunk("note1", 1, "chunk one", 2)
	if err != nil {
		t.Fatalf("InsertChunk: %v", err)
	}
	if id1 == 0 || id2 == 0 {
		t.Error("chunk IDs should be non-zero")
	}

	if err := db.DeleteChunksByNoteID("note1"); err != nil {
		t.Fatalf("DeleteChunksByNoteID: %v", err)
	}
}

func TestChunkEmbedding_InsertAndSearch(t *testing.T) {
	db := testDB(t)
	if err := db.InitRAG("model", 4); err != nil {
		t.Fatalf("InitRAG: %v", err)
	}

	chunkID, err := db.InsertChunk("note1", 0, "hello world", 2)
	if err != nil {
		t.Fatalf("InsertChunk: %v", err)
	}
	if err := db.InsertChunkEmbedding(chunkID, []float32{0.1, 0.2, 0.3, 0.4}); err != nil {
		t.Fatalf("InsertChunkEmbedding: %v", err)
	}

	results, err := db.SearchVectors([]float32{0.1, 0.2, 0.3, 0.4}, 10)
	if err != nil {
		t.Fatalf("SearchVectors: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].NoteID != "note1" {
		t.Errorf("NoteID: got %q, want %q", results[0].NoteID, "note1")
	}
}

func TestSearchVectors_EmptyTable(t *testing.T) {
	db := testDB(t)
	if err := db.InitRAG("model", 4); err != nil {
		t.Fatalf("InitRAG: %v", err)
	}

	results, err := db.SearchVectors([]float32{0.1, 0.2, 0.3, 0.4}, 10)
	if err != nil {
		t.Fatalf("SearchVectors: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestDeleteNoteRAGData(t *testing.T) {
	db := testDB(t)
	if err := db.InitRAG("model", 4); err != nil {
		t.Fatalf("InitRAG: %v", err)
	}

	// Insert all RAG data for a note
	if err := db.UpsertNoteHash("note1", "abc"); err != nil {
		t.Fatalf("UpsertNoteHash: %v", err)
	}
	chunkID, err := db.InsertChunk("note1", 0, "content", 1)
	if err != nil {
		t.Fatalf("InsertChunk: %v", err)
	}
	if err := db.InsertChunkEmbedding(chunkID, []float32{0.1, 0.2, 0.3, 0.4}); err != nil {
		t.Fatalf("InsertChunkEmbedding: %v", err)
	}

	// Delete all
	if err := db.DeleteNoteRAGData("note1"); err != nil {
		t.Fatalf("DeleteNoteRAGData: %v", err)
	}

	// Verify everything is gone
	hash, _ := db.GetNoteHash("note1")
	if hash != "" {
		t.Error("hash should be deleted")
	}
	results, _ := db.SearchVectors([]float32{0.1, 0.2, 0.3, 0.4}, 10)
	if len(results) != 0 {
		t.Error("vectors should be deleted")
	}
}

func TestDeleteNoteRAGData_NonExistent(t *testing.T) {
	db := testDB(t)
	if err := db.InitRAG("model", 4); err != nil {
		t.Fatalf("InitRAG: %v", err)
	}

	// Should be a no-op, no error
	if err := db.DeleteNoteRAGData("nonexistent"); err != nil {
		t.Fatalf("DeleteNoteRAGData: %v", err)
	}
}
