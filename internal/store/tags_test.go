package store

import (
	"testing"

	"github.com/jescarri/go-joplin/internal/models"
)

func TestGetTagByTitle_Found(t *testing.T) {
	db := testDB(t)

	tag := &models.Tag{Title: "maintenance"}
	if err := db.CreateTag(tag); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	got, err := db.GetTagByTitle("maintenance")
	if err != nil {
		t.Fatalf("GetTagByTitle: %v", err)
	}
	if got == nil {
		t.Fatal("expected tag, got nil")
	}
	if got.ID != tag.ID {
		t.Errorf("ID = %q, want %q", got.ID, tag.ID)
	}
}

func TestGetTagByTitle_CaseInsensitive(t *testing.T) {
	db := testDB(t)

	tag := &models.Tag{Title: "Cleaning"}
	if err := db.CreateTag(tag); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	for _, name := range []string{"cleaning", "CLEANING", "Cleaning"} {
		got, err := db.GetTagByTitle(name)
		if err != nil {
			t.Fatalf("GetTagByTitle(%q): %v", name, err)
		}
		if got == nil {
			t.Errorf("GetTagByTitle(%q) = nil, want tag", name)
		} else if got.ID != tag.ID {
			t.Errorf("GetTagByTitle(%q).ID = %q, want %q", name, got.ID, tag.ID)
		}
	}
}

func TestGetTagByTitle_NotFound(t *testing.T) {
	db := testDB(t)

	got, err := db.GetTagByTitle("nonexistent")
	if err != nil {
		t.Fatalf("GetTagByTitle: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got tag %+v", got)
	}
}

func TestAddNoteTag_CreatesAssociation(t *testing.T) {
	db := testDB(t)

	note := &models.Note{Title: "Test Note"}
	if err := db.CreateNote(note); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	tag := &models.Tag{Title: "todo"}
	if err := db.CreateTag(tag); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	if err := db.AddNoteTag(note.ID, tag.ID); err != nil {
		t.Fatalf("AddNoteTag: %v", err)
	}

	tags, err := db.GetNoteTagsByNote(note.ID)
	if err != nil {
		t.Fatalf("GetNoteTagsByNote: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	if tags[0].ID != tag.ID {
		t.Errorf("tag ID = %q, want %q", tags[0].ID, tag.ID)
	}
}

func TestAddNoteTag_Idempotent(t *testing.T) {
	db := testDB(t)

	note := &models.Note{Title: "Test Note"}
	if err := db.CreateNote(note); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	tag := &models.Tag{Title: "todo"}
	if err := db.CreateTag(tag); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	if err := db.AddNoteTag(note.ID, tag.ID); err != nil {
		t.Fatalf("AddNoteTag (first): %v", err)
	}
	if err := db.AddNoteTag(note.ID, tag.ID); err != nil {
		t.Fatalf("AddNoteTag (second): %v", err)
	}

	tags, err := db.GetNoteTagsByNote(note.ID)
	if err != nil {
		t.Fatalf("GetNoteTagsByNote: %v", err)
	}
	if len(tags) != 1 {
		t.Errorf("expected 1 tag after idempotent add, got %d", len(tags))
	}
}
