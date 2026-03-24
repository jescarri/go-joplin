package rag

import (
	"context"
	"testing"

	"github.com/jescarri/go-joplin/internal/models"
)

func TestSearch_ReturnsNotes(t *testing.T) {
	db := testDB(t)
	emb := newMockEmbedder(4)
	idx := NewIndexer(db, emb, 512, 50, 1, 10)

	// Create and index notes
	n1 := createTestNote(t, db, "Kubernetes Guide", "how to deploy pods")
	n2 := createTestNote(t, db, "Docker Tutorial", "building containers")
	n3 := createTestNote(t, db, "Go Programming", "goroutines and channels")

	for _, id := range []string{n1.ID, n2.ID, n3.ID} {
		if err := idx.indexNote(context.Background(), id); err != nil {
			t.Fatalf("indexNote(%s): %v", id, err)
		}
	}

	notes, _, err := idx.Search(context.Background(), "container orchestration", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(notes) == 0 {
		t.Fatal("expected at least 1 result")
	}

	// Verify notes have required fields
	for _, n := range notes {
		if n.ID == "" {
			t.Error("note missing ID")
		}
		if n.Title == "" {
			t.Error("note missing Title")
		}
		if n.Type_ != models.TypeNote {
			t.Errorf("note Type_: got %d, want %d", n.Type_, models.TypeNote)
		}
	}
}

func TestSearch_EmptyIndex(t *testing.T) {
	db := testDB(t)
	emb := newMockEmbedder(4)
	idx := NewIndexer(db, emb, 512, 50, 1, 10)

	notes, hasMore, err := idx.Search(context.Background(), "anything", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("expected 0 results, got %d", len(notes))
	}
	if hasMore {
		t.Error("hasMore should be false for empty results")
	}
}

func TestSearch_LimitRespected(t *testing.T) {
	db := testDB(t)
	emb := newMockEmbedder(4)
	idx := NewIndexer(db, emb, 512, 50, 1, 10)

	// Create 5 notes
	for i := range 5 {
		n := createTestNote(t, db, "Note"+string(rune('A'+i)), "content for note")
		if err := idx.indexNote(context.Background(), n.ID); err != nil {
			t.Fatalf("indexNote: %v", err)
		}
	}

	notes, _, err := idx.Search(context.Background(), "content", 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(notes) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(notes))
	}
}

func TestSearch_DeduplicatesChunks(t *testing.T) {
	db := testDB(t)
	emb := newMockEmbedder(4)
	// Small chunk size to force multiple chunks per note
	idx := NewIndexer(db, emb, 3, 1, 1, 10)

	// Create a note with enough words to produce multiple chunks
	n := createTestNote(t, db, "Long Note", "one two three four five six seven eight nine ten eleven twelve")
	if err := idx.indexNote(context.Background(), n.ID); err != nil {
		t.Fatalf("indexNote: %v", err)
	}

	notes, _, err := idx.Search(context.Background(), "test query", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// Should return the note at most once despite multiple chunks
	noteCount := 0
	for _, note := range notes {
		if note.ID == n.ID {
			noteCount++
		}
	}
	if noteCount > 1 {
		t.Errorf("note appeared %d times, should appear at most once", noteCount)
	}
}

func TestSearch_ResponseFormat(t *testing.T) {
	db := testDB(t)
	emb := newMockEmbedder(4)
	idx := NewIndexer(db, emb, 512, 50, 1, 10)

	n := createTestNote(t, db, "My Note", "some body content")
	if err := idx.indexNote(context.Background(), n.ID); err != nil {
		t.Fatalf("indexNote: %v", err)
	}

	notes, _, err := idx.Search(context.Background(), "query", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 result, got %d", len(notes))
	}

	got := notes[0]
	if got.ID != n.ID {
		t.Errorf("ID: got %q, want %q", got.ID, n.ID)
	}
	if got.Title != "My Note" {
		t.Errorf("Title: got %q, want %q", got.Title, "My Note")
	}
	if got.UpdatedTime == 0 {
		t.Error("UpdatedTime should be set")
	}
}
