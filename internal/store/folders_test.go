package store

import (
	"testing"

	"github.com/jescarri/go-joplin/internal/models"
)

func TestGetFolderByTitle_Found(t *testing.T) {
	db := testDB(t)

	f := &models.Folder{Title: "Homelab"}
	if err := db.CreateFolder(f); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}

	got, err := db.GetFolderByTitle("Homelab")
	if err != nil {
		t.Fatalf("GetFolderByTitle: %v", err)
	}
	if got == nil {
		t.Fatal("expected folder, got nil")
	}
	if got.ID != f.ID {
		t.Errorf("ID = %q, want %q", got.ID, f.ID)
	}
}

func TestGetFolderByTitle_CaseInsensitive(t *testing.T) {
	db := testDB(t)

	f := &models.Folder{Title: "Homelab"}
	if err := db.CreateFolder(f); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}

	for _, name := range []string{"homelab", "HOMELAB", "HomeLab"} {
		got, err := db.GetFolderByTitle(name)
		if err != nil {
			t.Fatalf("GetFolderByTitle(%q): %v", name, err)
		}
		if got == nil {
			t.Errorf("GetFolderByTitle(%q) = nil, want folder", name)
		} else if got.ID != f.ID {
			t.Errorf("GetFolderByTitle(%q).ID = %q, want %q", name, got.ID, f.ID)
		}
	}
}

func TestGetFolderByTitle_NotFound(t *testing.T) {
	db := testDB(t)

	got, err := db.GetFolderByTitle("NonExistent")
	if err != nil {
		t.Fatalf("GetFolderByTitle: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got folder %+v", got)
	}
}
