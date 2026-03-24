package rag

import (
	"context"
	"os"
	"testing"

	"github.com/jescarri/go-joplin/internal/models"
	"github.com/jescarri/go-joplin/internal/store"
)

func testDB(t *testing.T) *store.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.InitRAG("test-model", 4); err != nil {
		t.Fatalf("InitRAG: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		os.RemoveAll(dir)
	})
	return db
}

func createTestNote(t *testing.T, db *store.DB, title, body string) *models.Note {
	t.Helper()
	n := &models.Note{Title: title, Body: body}
	if err := db.CreateNote(n); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	return n
}

func TestIndexNote_NewNote(t *testing.T) {
	db := testDB(t)
	emb := newMockEmbedder(4)
	idx := NewIndexer(db, emb, 512, 50, 1, 10)

	note := createTestNote(t, db, "Test", "hello world content here")
	if _, err := idx.indexNote(context.Background(), note.ID); err != nil {
		t.Fatalf("indexNote: %v", err)
	}

	// Verify hash was stored
	hash, _ := db.GetNoteHash(note.ID)
	if hash == "" {
		t.Error("expected hash to be stored")
	}

	// Verify embedder was called
	if emb.callCount() != 1 {
		t.Errorf("embedder calls: got %d, want 1", emb.callCount())
	}
}

func TestIndexNote_UnchangedNote(t *testing.T) {
	db := testDB(t)
	emb := newMockEmbedder(4)
	idx := NewIndexer(db, emb, 512, 50, 1, 10)

	note := createTestNote(t, db, "Test", "hello world")

	// Index once
	if _, err := idx.indexNote(context.Background(), note.ID); err != nil {
		t.Fatalf("first indexNote: %v", err)
	}
	if emb.callCount() != 1 {
		t.Fatalf("expected 1 embed call after first index, got %d", emb.callCount())
	}

	// Index again — should skip
	if _, err := idx.indexNote(context.Background(), note.ID); err != nil {
		t.Fatalf("second indexNote: %v", err)
	}
	if emb.callCount() != 1 {
		t.Errorf("embedder should not be called again; got %d calls", emb.callCount())
	}
}

func TestIndexNote_UpdatedNote(t *testing.T) {
	db := testDB(t)
	emb := newMockEmbedder(4)
	idx := NewIndexer(db, emb, 512, 50, 1, 10)

	note := createTestNote(t, db, "Test", "original body")

	// Index original
	if _, err := idx.indexNote(context.Background(), note.ID); err != nil {
		t.Fatalf("first indexNote: %v", err)
	}
	hash1, _ := db.GetNoteHash(note.ID)

	// Update note body
	note.Body = "updated body with new content"
	if err := db.UpdateNote(note); err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}

	// Re-index — should detect change
	if _, err := idx.indexNote(context.Background(), note.ID); err != nil {
		t.Fatalf("second indexNote: %v", err)
	}
	hash2, _ := db.GetNoteHash(note.ID)
	if hash1 == hash2 {
		t.Error("hash should have changed after update")
	}
	if emb.callCount() != 2 {
		t.Errorf("embedder calls: got %d, want 2", emb.callCount())
	}
}

func TestIndexNote_EncryptedNote(t *testing.T) {
	db := testDB(t)
	emb := newMockEmbedder(4)
	idx := NewIndexer(db, emb, 512, 50, 1, 10)

	note := &models.Note{Title: "Encrypted", Body: "", EncryptionApplied: 1}
	if err := db.CreateNote(note); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	if _, err := idx.indexNote(context.Background(), note.ID); err != nil {
		t.Fatalf("indexNote: %v", err)
	}

	// Embedder should NOT be called
	if emb.callCount() != 0 {
		t.Errorf("embedder should not be called for encrypted note; got %d calls", emb.callCount())
	}

	// No hash should be stored
	hash, _ := db.GetNoteHash(note.ID)
	if hash != "" {
		t.Error("no hash should be stored for encrypted note")
	}
}

func TestIndexNote_EmbedError(t *testing.T) {
	db := testDB(t)
	emb := newFailEmbedder(4)
	idx := NewIndexer(db, emb, 512, 50, 1, 10)

	note := createTestNote(t, db, "Test", "hello world")

	_, err := idx.indexNote(context.Background(), note.ID)
	if err == nil {
		t.Fatal("expected error from failing embedder")
	}

	// Hash should NOT be stored (no partial state)
	hash, _ := db.GetNoteHash(note.ID)
	if hash != "" {
		t.Error("hash should not be stored when embedding fails")
	}
}

func TestIndexAll_OrphanCleanup(t *testing.T) {
	db := testDB(t)
	emb := newMockEmbedder(4)
	idx := NewIndexer(db, emb, 512, 50, 1, 10)

	note := createTestNote(t, db, "Test", "hello")

	// Index the note
	if _, err := idx.indexNote(context.Background(), note.ID); err != nil {
		t.Fatalf("indexNote: %v", err)
	}

	// Delete the note directly (simulating sync delete)
	if err := db.DeleteNote(note.ID); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}

	// RAG data should still exist
	hash, _ := db.GetNoteHash(note.ID)
	if hash == "" {
		t.Fatal("expected hash to still exist before IndexAll")
	}

	// IndexAll should clean up orphans
	if err := idx.IndexAll(context.Background()); err != nil {
		t.Fatalf("IndexAll: %v", err)
	}

	hash, _ = db.GetNoteHash(note.ID)
	if hash != "" {
		t.Error("orphaned hash should have been cleaned up")
	}
}

func TestEnqueue_NonBlocking(t *testing.T) {
	db := testDB(t)
	emb := newMockEmbedder(4)
	idx := NewIndexer(db, emb, 512, 50, 1, 2) // queue size 2

	// Fill the queue
	idx.Enqueue("a")
	idx.Enqueue("b")

	// This should not block
	idx.Enqueue("c") // dropped
}

func TestIndexAll_MixedNotes(t *testing.T) {
	db := testDB(t)
	emb := newMockEmbedder(4)
	idx := NewIndexer(db, emb, 512, 50, 1, 10)

	// Create 3 notes: one normal, one already indexed, one encrypted
	note1 := createTestNote(t, db, "New", "new content")
	note2 := createTestNote(t, db, "Indexed", "already indexed")
	note3 := &models.Note{Title: "Encrypted", Body: "", EncryptionApplied: 1}
	if err := db.CreateNote(note3); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	// Pre-index note2
	if _, err := idx.indexNote(context.Background(), note2.ID); err != nil {
		t.Fatalf("pre-index: %v", err)
	}
	callsBefore := emb.callCount()

	// IndexAll should process note1, skip note2 and note3
	if err := idx.IndexAll(context.Background()); err != nil {
		t.Fatalf("IndexAll: %v", err)
	}

	// Only note1 should have triggered a new embed call
	newCalls := emb.callCount() - callsBefore
	if newCalls != 1 {
		t.Errorf("expected 1 new embed call (for note1), got %d", newCalls)
	}

	// All non-encrypted notes should have hashes
	h1, _ := db.GetNoteHash(note1.ID)
	h2, _ := db.GetNoteHash(note2.ID)
	h3, _ := db.GetNoteHash(note3.ID)
	if h1 == "" {
		t.Error("note1 should have hash")
	}
	if h2 == "" {
		t.Error("note2 should have hash")
	}
	if h3 != "" {
		t.Error("encrypted note3 should not have hash")
	}
}
