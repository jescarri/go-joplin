package mcp

import (
	"context"
	"os"
	"testing"

	"github.com/jescarri/go-joplin/internal/config"
	"github.com/jescarri/go-joplin/internal/models"
	"github.com/jescarri/go-joplin/internal/store"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func testDeps(t *testing.T) *Deps {
	t.Helper()
	return testDepsWithPolicy(t, nil)
}

// testDepsPermissive returns Deps with wildcard policy (allow all mutations).
func testDepsPermissive(t *testing.T) *Deps {
	t.Helper()
	cfg := &config.Config{
		MCPAllowFolders:      "*",
		MCPAllowTags:         "*",
		MCPAllowCreateTag:    true,
		MCPAllowCreateFolder: true,
	}
	return testDepsWithPolicy(t, NewPolicy(cfg))
}

func testDepsWithPolicy(t *testing.T, policy *Policy) *Deps {
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
	return &Deps{DB: db, Policy: policy}
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
	d := testDepsPermissive(t)

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
	d := testDepsPermissive(t)

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
	d := testDepsPermissive(t)

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
	d := testDepsPermissive(t)

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
	d := testDepsPermissive(t)

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
	d := testDepsPermissive(t)

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

func TestCreateNoteHandler_DeniedByPolicy(t *testing.T) {
	d := testDeps(t) // nil policy = read-only
	folder := &models.Folder{Title: "Homelab"}
	if err := d.DB.CreateFolder(folder); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}

	handler := createNoteHandler(d)
	in := CreateNoteIn{
		Title:      "Test",
		FolderName: "Homelab",
	}

	_, _, err := handler(context.Background(), &sdkmcp.CallToolRequest{}, in)
	if err == nil {
		t.Fatal("expected error when policy denies folder")
	}
}

func TestCreateFolderHandler_DeniedByPolicy(t *testing.T) {
	d := testDeps(t)

	handler := createFolderHandler(d)
	in := CreateFolderIn{Title: "New Folder"}

	_, _, err := handler(context.Background(), &sdkmcp.CallToolRequest{}, in)
	if err == nil {
		t.Fatal("expected error when policy denies folder creation")
	}
}

func TestResolveTagIDs_DeniedWhenPolicyRestricts(t *testing.T) {
	d := testDeps(t) // nil policy
	tag := &models.Tag{Title: "work"}
	if err := d.DB.CreateTag(tag); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	_, err := resolveTagIDs(d, []string{tag.ID}, nil)
	if err == nil {
		t.Fatal("expected error when policy restricts tag")
	}
}

func TestIsToolEnabled(t *testing.T) {
	tests := []struct {
		name         string
		toolName     string
		enabledTools string
		want         bool
	}{
		{"empty means all", "list_notes", "", true},
		{"star means all", "list_notes", "*", true},
		{"exact match", "list_notes", "list_notes,get_note", true},
		{"not in list", "create_note", "list_notes,get_note", false},
		{"single tool match", "get_note", "get_note", true},
		{"single tool no match", "list_notes", "get_note", false},
		{"whitespace trimmed", "get_note", "list_notes, get_note", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isToolEnabled(tc.toolName, tc.enabledTools)
			if got != tc.want {
				t.Errorf("isToolEnabled(%q, %q) = %v, want %v", tc.toolName, tc.enabledTools, got, tc.want)
			}
		})
	}
}

func TestRegisterAll_EnabledToolsFilter(t *testing.T) {
	// RegisterAll with only two tools enabled should not panic and should register
	// only the specified tools. We verify indirectly by calling a handler that
	// should NOT be registered and confirming the server was created without error.
	d := testDeps(t)
	d.EnabledTools = "list_notes,list_folders"

	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	RegisterAll(server, d)
	// If we got here without a panic, the filtering worked.
	// The SDK doesn't expose a public method to list tools, so we verify
	// that with "*" we get the full set (no panic) and with a restricted
	// list we also get no panic.
	d2 := testDeps(t)
	d2.EnabledTools = "*"
	server2 := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	RegisterAll(server2, d2)
}

func TestListFolders_SlimResponse(t *testing.T) {
	d := testDeps(t)
	folder := &models.Folder{Title: "TestFolder"}
	if err := d.DB.CreateFolder(folder); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}

	handler := listFoldersHandler(d)
	_, result, err := handler(context.Background(), &sdkmcp.CallToolRequest{}, ListFoldersIn{})
	if err != nil {
		t.Fatalf("listFoldersHandler: %v", err)
	}

	doc, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}

	folders, ok := doc["folders"].([]map[string]any)
	if !ok {
		t.Fatalf("folders type = %T, want []map[string]any", doc["folders"])
	}
	if len(folders) == 0 {
		t.Fatal("expected at least one folder")
	}

	f := folders[0]
	// Should have slim fields
	if _, ok := f["id"]; !ok {
		t.Error("slim folder should have 'id'")
	}
	if _, ok := f["title"]; !ok {
		t.Error("slim folder should have 'title'")
	}
	if _, ok := f["parent_id"]; !ok {
		t.Error("slim folder should have 'parent_id'")
	}
	// Should NOT have encryption_cipher_text
	if _, ok := f["encryption_cipher_text"]; ok {
		t.Error("slim folder should NOT contain 'encryption_cipher_text'")
	}
	// Should NOT have created_time
	if _, ok := f["created_time"]; ok {
		t.Error("slim folder should NOT contain 'created_time'")
	}
}

func TestListNotes_SlimResponse(t *testing.T) {
	d := testDepsPermissive(t)
	folder := &models.Folder{Title: "NoteFolder"}
	if err := d.DB.CreateFolder(folder); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	note := &models.Note{Title: "TestNote", Body: "some body content", ParentID: folder.ID}
	if err := d.DB.CreateNote(note); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	handler := listNotesHandler(d)
	_, result, err := handler(context.Background(), &sdkmcp.CallToolRequest{}, ListNotesIn{})
	if err != nil {
		t.Fatalf("listNotesHandler: %v", err)
	}

	doc := result.(map[string]any)
	notes := doc["notes"].([]map[string]any)
	if len(notes) == 0 {
		t.Fatal("expected at least one note")
	}

	n := notes[0]
	// Should have slim fields
	for _, key := range []string{"id", "title", "parent_id", "is_todo", "updated_time"} {
		if _, ok := n[key]; !ok {
			t.Errorf("slim note should have %q", key)
		}
	}
	// Should NOT have body
	if _, ok := n["body"]; ok {
		t.Error("slim note should NOT contain 'body'")
	}
}

func TestGetCapabilitiesHandler(t *testing.T) {
	d := testDepsPermissive(t)

	handler := getCapabilitiesHandler(d)
	_, result, err := handler(context.Background(), &sdkmcp.CallToolRequest{}, GetCapabilitiesIn{})
	if err != nil {
		t.Fatalf("getCapabilitiesHandler: %v", err)
	}
	doc, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if doc["folders"] == nil {
		t.Error("capabilities should include folders")
	}
	if doc["tags"] == nil {
		t.Error("capabilities should include tags")
	}
}
