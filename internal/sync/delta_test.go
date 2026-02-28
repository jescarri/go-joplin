package sync

import (
	"fmt"
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
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		os.RemoveAll(dir)
	})
	return db
}

func TestApplyDeltaItem_SkipsEncryptedWhenLocalDecrypted(t *testing.T) {
	db := testDB(t)

	// Insert a decrypted note locally
	note := &models.Note{
		ID:                "abcdef01234567890abcdef012345678",
		Title:             "My Decrypted Note",
		Body:              "Visible body content",
		EncryptionApplied: 0,
		MarkupLanguage:    1,
	}
	if err := db.CreateNote(note); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	// Simulate server sending an encrypted version of the same note
	encryptedContent := "\n\n\n\n" +
		"id: abcdef01234567890abcdef012345678\n" +
		"parent_id: \n" +
		"created_time: 2026-02-27T00:00:00.000Z\n" +
		"updated_time: 2026-02-27T01:00:00.000Z\n" +
		"encryption_cipher_text: JED01fakeciphertext\n" +
		"encryption_applied: 1\n" +
		"markup_language: 1\n" +
		"type_: 1"

	item := DeltaItem{
		ID:       "delta1",
		ItemName: "abcdef01234567890abcdef012345678.md",
		Type:     1, // PUT
	}

	err := applyDeltaItem(nil, db, 9, item, []byte(encryptedContent))
	if err != nil {
		t.Fatalf("applyDeltaItem: %v", err)
	}

	// Local note should still be decrypted
	got, err := db.GetNote(note.ID)
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}
	if got == nil {
		t.Fatal("note disappeared after delta apply")
	}
	if got.Title != "My Decrypted Note" {
		t.Errorf("Title = %q, want %q (encrypted overwrote decrypted)", got.Title, "My Decrypted Note")
	}
	if got.EncryptionApplied != 0 {
		t.Errorf("EncryptionApplied = %d, want 0", got.EncryptionApplied)
	}
}

func TestApplyDeltaItem_AppliesEncryptedWhenNoLocal(t *testing.T) {
	db := testDB(t)

	encryptedContent := "\n\n\n\n" +
		"id: abcdef01234567890abcdef012345678\n" +
		"parent_id: \n" +
		"created_time: 2026-02-27T00:00:00.000Z\n" +
		"updated_time: 2026-02-27T01:00:00.000Z\n" +
		"encryption_cipher_text: JED01fakeciphertext\n" +
		"encryption_applied: 1\n" +
		"markup_language: 1\n" +
		"type_: 1"

	item := DeltaItem{
		ID:       "delta2",
		ItemName: "abcdef01234567890abcdef012345678.md",
		Type:     1,
	}

	err := applyDeltaItem(nil, db, 9, item, []byte(encryptedContent))
	if err != nil {
		t.Fatalf("applyDeltaItem: %v", err)
	}

	got, err := db.GetNote("abcdef01234567890abcdef012345678")
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}
	if got == nil {
		t.Fatal("expected note to be created")
	}
	if got.EncryptionApplied != 1 {
		t.Errorf("EncryptionApplied = %d, want 1", got.EncryptionApplied)
	}
}

func TestApplyDeltaItem_AppliesUnencryptedUpdate(t *testing.T) {
	db := testDB(t)

	original := &models.Note{
		ID:                "abcdef01234567890abcdef012345678",
		Title:             "Old Title",
		Body:              "Old body",
		EncryptionApplied: 0,
		MarkupLanguage:    1,
	}
	if err := db.CreateNote(original); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	// Plaintext update from server (not encrypted)
	unencryptedContent := "New Title\n\nNew body\n\n" +
		"id: abcdef01234567890abcdef012345678\n" +
		"parent_id: \n" +
		"created_time: 2026-02-27T00:00:00.000Z\n" +
		"updated_time: 2026-02-27T02:00:00.000Z\n" +
		"encryption_applied: 0\n" +
		"markup_language: 1\n" +
		"type_: 1"

	item := DeltaItem{
		ID:       "delta3",
		ItemName: "abcdef01234567890abcdef012345678.md",
		Type:     1,
	}

	err := applyDeltaItem(nil, db, 9, item, []byte(unencryptedContent))
	if err != nil {
		t.Fatalf("applyDeltaItem: %v", err)
	}

	got, err := db.GetNote(original.ID)
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}
	if got.Title != "New Title" {
		t.Errorf("Title = %q, want %q", got.Title, "New Title")
	}
	if got.Body != "New body" {
		t.Errorf("Body = %q, want %q", got.Body, "New body")
	}
}

func TestApplyDeltaItem_SkipsEncryptedFolder(t *testing.T) {
	db := testDB(t)

	folder := &models.Folder{
		ID:                "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Title:             "Decrypted Folder",
		EncryptionApplied: 0,
	}
	if err := db.CreateFolder(folder); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}

	encryptedContent := "\n\n" +
		"id: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n" +
		"created_time: 2026-02-27T00:00:00.000Z\n" +
		"updated_time: 2026-02-27T01:00:00.000Z\n" +
		"encryption_cipher_text: JED01fakeciphertext\n" +
		"encryption_applied: 1\n" +
		"type_: 2"

	item := DeltaItem{
		ID:       "delta4",
		ItemName: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.md",
		Type:     1,
	}

	err := applyDeltaItem(nil, db, 9, item, []byte(encryptedContent))
	if err != nil {
		t.Fatalf("applyDeltaItem: %v", err)
	}

	got, err := db.GetFolder(folder.ID)
	if err != nil {
		t.Fatalf("GetFolder: %v", err)
	}
	if got.Title != "Decrypted Folder" {
		t.Errorf("Title = %q, want %q", got.Title, "Decrypted Folder")
	}
	if got.EncryptionApplied != 0 {
		t.Errorf("EncryptionApplied = %d, want 0", got.EncryptionApplied)
	}
}

func TestApplyDeltaItem_SkipsEncryptedTag(t *testing.T) {
	db := testDB(t)

	tag := &models.Tag{
		ID:                "cccccccccccccccccccccccccccccccc",
		Title:             "Decrypted Tag",
		EncryptionApplied: 0,
	}
	if err := db.CreateTag(tag); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	encryptedContent := "\n\n" +
		"id: cccccccccccccccccccccccccccccccc\n" +
		"created_time: 2026-02-27T00:00:00.000Z\n" +
		"updated_time: 2026-02-27T01:00:00.000Z\n" +
		"encryption_cipher_text: JED01fakeciphertext\n" +
		"encryption_applied: 1\n" +
		"type_: 5"

	item := DeltaItem{
		ID:       "delta5",
		ItemName: "cccccccccccccccccccccccccccccccc.md",
		Type:     1,
	}

	err := applyDeltaItem(nil, db, 9, item, []byte(encryptedContent))
	if err != nil {
		t.Fatalf("applyDeltaItem: %v", err)
	}

	got, err := db.GetTag(tag.ID)
	if err != nil {
		t.Fatalf("GetTag: %v", err)
	}
	if got.Title != "Decrypted Tag" {
		t.Errorf("Title = %q, want %q", got.Title, "Decrypted Tag")
	}
	if got.EncryptionApplied != 0 {
		t.Errorf("EncryptionApplied = %d, want 0", got.EncryptionApplied)
	}
}

func TestApplyDeltaItem_DeleteRemovesFolder(t *testing.T) {
	db := testDB(t)
	syncTarget := 9

	folder := &models.Folder{
		ID:    "dddddddddddddddddddddddddddddddd",
		Title: "To Be Deleted",
	}
	if err := db.CreateFolder(folder); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if err := db.UpsertSyncItem(folder.ID, models.TypeFolder, syncTarget); err != nil {
		t.Fatalf("UpsertSyncItem: %v", err)
	}

	item := DeltaItem{
		ID:       "delta-del-1",
		ItemName: folder.ID + ".md",
		Type:     3, // delete
	}

	if err := applyDeltaItem(nil, db, syncTarget, item, nil); err != nil {
		t.Fatalf("applyDeltaItem delete: %v", err)
	}

	got, err := db.GetFolder(folder.ID)
	if err != nil {
		t.Fatalf("GetFolder: %v", err)
	}
	if got != nil {
		t.Errorf("folder still exists after delete delta; want nil, got %+v", got)
	}

	si, err := db.GetSyncItem(folder.ID, syncTarget)
	if err != nil {
		t.Fatalf("GetSyncItem: %v", err)
	}
	if si != nil {
		t.Errorf("sync_item still exists after delete delta; want nil")
	}
}

func TestApplyDeltaItem_DeleteRemovesNote(t *testing.T) {
	db := testDB(t)
	syncTarget := 9

	note := &models.Note{
		ID:             "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		Title:          "Note To Delete",
		MarkupLanguage: 1,
	}
	if err := db.CreateNote(note); err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if err := db.UpsertSyncItem(note.ID, models.TypeNote, syncTarget); err != nil {
		t.Fatalf("UpsertSyncItem: %v", err)
	}

	item := DeltaItem{
		ID:       "delta-del-2",
		ItemName: note.ID + ".md",
		Type:     3,
	}

	if err := applyDeltaItem(nil, db, syncTarget, item, nil); err != nil {
		t.Fatalf("applyDeltaItem delete: %v", err)
	}

	got, err := db.GetNote(note.ID)
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}
	if got != nil {
		t.Errorf("note still exists after delete delta; want nil, got %+v", got)
	}
}

type mockBackend struct {
	responses map[string][]byte
}

func (m *mockBackend) Authenticate() error                    { return nil }
func (m *mockBackend) IsAuthenticated() bool                  { return true }
func (m *mockBackend) AcquireLock() (interface{}, error)      { return nil, nil }
func (m *mockBackend) ReleaseLock(lock interface{}) error     { return nil }
func (m *mockBackend) Put(path string, content []byte) error  { return nil }
func (m *mockBackend) Delete(path string) error               { return nil }
func (m *mockBackend) SyncTarget() int                        { return 9 }
func (m *mockBackend) Get(path string) ([]byte, error) {
	if data, ok := m.responses[path]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("not found: %s", path)
}

func TestPullMissingItems_PrunesStaleLocalItems(t *testing.T) {
	db := testDB(t)
	syncTarget := 9

	// Create two folders locally and mark them as synced
	alive := &models.Folder{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Title: "Alive"}
	stale := &models.Folder{ID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Title: "Stale"}
	for _, f := range []*models.Folder{alive, stale} {
		if err := db.CreateFolder(f); err != nil {
			t.Fatalf("CreateFolder(%s): %v", f.ID, err)
		}
		if err := db.UpsertSyncItem(f.ID, models.TypeFolder, syncTarget); err != nil {
			t.Fatalf("UpsertSyncItem(%s): %v", f.ID, err)
		}
	}

	// Server only has the "alive" folder
	childrenJSON := fmt.Sprintf(
		`{"items":[{"id":"srv1","name":"%s.md"}],"cursor":"c1","has_more":false}`,
		alive.ID,
	)

	backend := &mockBackend{responses: map[string][]byte{
		"/api/items/root:/:/children": []byte(childrenJSON),
	}}

	if err := PullMissingItems(backend, db); err != nil {
		t.Fatalf("PullMissingItems: %v", err)
	}

	// alive folder should still exist
	got, err := db.GetFolder(alive.ID)
	if err != nil {
		t.Fatalf("GetFolder(alive): %v", err)
	}
	if got == nil {
		t.Error("alive folder was incorrectly pruned")
	}

	// stale folder should be removed
	got, err = db.GetFolder(stale.ID)
	if err != nil {
		t.Fatalf("GetFolder(stale): %v", err)
	}
	if got != nil {
		t.Errorf("stale folder still exists after reconciliation; want nil, got %+v", got)
	}

	// sync_items for stale should also be gone
	si, err := db.GetSyncItem(stale.ID, syncTarget)
	if err != nil {
		t.Fatalf("GetSyncItem(stale): %v", err)
	}
	if si != nil {
		t.Error("sync_item for stale folder still exists after reconciliation")
	}
}

func TestParseItemName(t *testing.T) {
	tests := []struct {
		name   string
		wantID string
	}{
		{"abcdef01234567890abcdef012345678.md", "abcdef01234567890abcdef012345678"},
		{"folder/abcdef01234567890abcdef012345678.md", "abcdef01234567890abcdef012345678"},
		{"invalid.txt", ""},
		{"short.md", ""},
		{".resource/abcdef01234567890abcdef012345678", ""},
	}

	for _, tt := range tests {
		id, _ := parseItemName(tt.name)
		if id != tt.wantID {
			t.Errorf("parseItemName(%q) = %q, want %q", tt.name, id, tt.wantID)
		}
	}
}

func TestIsResourceBlob(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{".resource/abc123", true},
		{"abc123.md", false},
		{"ab", false},
	}

	for _, tt := range tests {
		got := isResourceBlob(tt.name)
		if got != tt.want {
			t.Errorf("isResourceBlob(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
