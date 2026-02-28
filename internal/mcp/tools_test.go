package mcp

import (
	"context"
	"os"
	"testing"

	"github.com/jescarri/go-joplin/internal/models"
	"github.com/jescarri/go-joplin/internal/store"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func testDeps(t *testing.T) *Deps {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		os.RemoveAll(dir)
	})
	return &Deps{DB: db}
}

func TestResolveParentID_ByID(t *testing.T) {
	d := testDeps(t)
	f := &models.Folder{Title: "Homelab"}
	if err := d.DB.CreateFolder(f); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}

	got, err := resolveParentID(d, f.ID, "")
	if err != nil {
		t.Fatalf("resolveParentID: %v", err)
	}
	if got != f.ID {
		t.Errorf("got %q, want %q", got, f.ID)
	}
}

func TestResolveParentID_ByName(t *testing.T) {
	d := testDeps(t)
	f := &models.Folder{Title: "Homelab"}
	if err := d.DB.CreateFolder(f); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}

	got, err := resolveParentID(d, "", "Homelab")
	if err != nil {
		t.Fatalf("resolveParentID: %v", err)
	}
	if got != f.ID {
		t.Errorf("got %q, want %q", got, f.ID)
	}
}

func TestResolveParentID_ByNameCaseInsensitive(t *testing.T) {
	d := testDeps(t)
	f := &models.Folder{Title: "Homelab"}
	if err := d.DB.CreateFolder(f); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}

	got, err := resolveParentID(d, "", "homelab")
	if err != nil {
		t.Fatalf("resolveParentID: %v", err)
	}
	if got != f.ID {
		t.Errorf("got %q, want %q", got, f.ID)
	}
}

func TestResolveParentID_InvalidID(t *testing.T) {
	d := testDeps(t)
	_, err := resolveParentID(d, "nonexistent_id", "")
	if err == nil {
		t.Fatal("expected error for invalid parent_id")
	}
}

func TestResolveParentID_InvalidName(t *testing.T) {
	d := testDeps(t)
	_, err := resolveParentID(d, "", "NonExistentFolder")
	if err == nil {
		t.Fatal("expected error for invalid folder_name")
	}
}

func TestResolveParentID_BothProvided(t *testing.T) {
	d := testDeps(t)
	_, err := resolveParentID(d, "some_id", "some_name")
	if err == nil {
		t.Fatal("expected error when both parent_id and folder_name provided")
	}
}

func TestResolveParentID_Empty(t *testing.T) {
	d := testDeps(t)
	got, err := resolveParentID(d, "", "")
	if err != nil {
		t.Fatalf("resolveParentID: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestResolveTagIDs_ByIDs(t *testing.T) {
	d := testDeps(t)

	tag := &models.Tag{Title: "maintenance"}
	if err := d.DB.CreateTag(tag); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	got, err := resolveTagIDs(d, []string{tag.ID}, nil)
	if err != nil {
		t.Fatalf("resolveTagIDs: %v", err)
	}
	if len(got) != 1 || got[0] != tag.ID {
		t.Errorf("got %v, want [%s]", got, tag.ID)
	}
}

func TestResolveTagIDs_ByNames_Existing(t *testing.T) {
	d := testDeps(t)

	tag := &models.Tag{Title: "cleaning"}
	if err := d.DB.CreateTag(tag); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	got, err := resolveTagIDs(d, nil, []string{"cleaning"})
	if err != nil {
		t.Fatalf("resolveTagIDs: %v", err)
	}
	if len(got) != 1 || got[0] != tag.ID {
		t.Errorf("got %v, want [%s]", got, tag.ID)
	}
}

func TestResolveTagIDs_ByNames_AutoCreates(t *testing.T) {
	d := testDeps(t)

	got, err := resolveTagIDs(d, nil, []string{"newtag"})
	if err != nil {
		t.Fatalf("resolveTagIDs: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 tag ID, got %d", len(got))
	}

	// Verify the tag was created in the DB
	tag, err := d.DB.GetTag(got[0])
	if err != nil {
		t.Fatalf("GetTag: %v", err)
	}
	if tag == nil {
		t.Fatal("auto-created tag not found in DB")
	}
	if tag.Title != "newtag" {
		t.Errorf("tag title = %q, want %q", tag.Title, "newtag")
	}
}

func TestResolveTagIDs_InvalidID(t *testing.T) {
	d := testDeps(t)
	_, err := resolveTagIDs(d, []string{"bad_id"}, nil)
	if err == nil {
		t.Fatal("expected error for invalid tag ID")
	}
}

func TestResolveTagIDs_Deduplicates(t *testing.T) {
	d := testDeps(t)

	tag := &models.Tag{Title: "todo"}
	if err := d.DB.CreateTag(tag); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	got, err := resolveTagIDs(d, []string{tag.ID}, []string{"todo"})
	if err != nil {
		t.Fatalf("resolveTagIDs: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 deduplicated tag, got %d: %v", len(got), got)
	}
}

func TestCreateNoteHandler_WithFolderNameAndTagNames(t *testing.T) {
	d := testDeps(t)

	folder := &models.Folder{Title: "Homelab"}
	if err := d.DB.CreateFolder(folder); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}

	handler := createNoteHandler(d)
	in := CreateNoteIn{
		Title:      "ToDo List",
		Body:       "* Update the kernel\n* Clean the servers",
		FolderName: "Homelab",
		TagNames:   []string{"cleaning", "maintenance", "updates", "todo"},
	}

	_, result, err := handler(context.Background(), &sdkmcp.CallToolRequest{}, in)
	if err != nil {
		t.Fatalf("createNoteHandler: %v", err)
	}

	note, ok := result.(*models.Note)
	if !ok {
		t.Fatalf("result type = %T, want *models.Note", result)
	}

	if note.ParentID != folder.ID {
		t.Errorf("note.ParentID = %q, want %q", note.ParentID, folder.ID)
	}
	if note.Title != "ToDo List" {
		t.Errorf("note.Title = %q, want %q", note.Title, "ToDo List")
	}

	// Verify tags were created and associated
	tags, err := d.DB.GetNoteTagsByNote(note.ID)
	if err != nil {
		t.Fatalf("GetNoteTagsByNote: %v", err)
	}
	if len(tags) != 4 {
		t.Errorf("expected 4 tags on note, got %d", len(tags))
	}

	tagNames := make(map[string]bool)
	for _, tg := range tags {
		tagNames[tg.Title] = true
	}
	for _, expected := range []string{"cleaning", "maintenance", "updates", "todo"} {
		if !tagNames[expected] {
			t.Errorf("missing tag %q on note", expected)
		}
	}
}

func TestCreateNoteHandler_InvalidFolderName(t *testing.T) {
	d := testDeps(t)

	handler := createNoteHandler(d)
	in := CreateNoteIn{
		Title:      "Test",
		FolderName: "NonExistent",
	}

	_, _, err := handler(context.Background(), &sdkmcp.CallToolRequest{}, in)
	if err == nil {
		t.Fatal("expected error for non-existent folder name")
	}
}

func TestCreateNoteHandler_InvalidParentID(t *testing.T) {
	d := testDeps(t)

	handler := createNoteHandler(d)
	in := CreateNoteIn{
		Title:    "Test",
		ParentID: "homelab_folder_id",
	}

	_, _, err := handler(context.Background(), &sdkmcp.CallToolRequest{}, in)
	if err == nil {
		t.Fatal("expected error for invalid parent_id")
	}
}

func TestCreateNoteHandler_NoFolder(t *testing.T) {
	d := testDeps(t)

	handler := createNoteHandler(d)
	in := CreateNoteIn{
		Title: "Orphan Note",
		Body:  "No folder",
	}

	_, result, err := handler(context.Background(), &sdkmcp.CallToolRequest{}, in)
	if err != nil {
		t.Fatalf("createNoteHandler: %v", err)
	}

	note := result.(*models.Note)
	if note.ParentID != "" {
		t.Errorf("note.ParentID = %q, want empty", note.ParentID)
	}
}
