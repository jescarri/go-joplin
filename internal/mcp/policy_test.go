package mcp

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/jescarri/go-joplin/internal/config"
	"github.com/jescarri/go-joplin/internal/models"
	"github.com/jescarri/go-joplin/internal/store"
)

func TestNewPolicy_NilConfig(t *testing.T) {
	p := NewPolicy(nil)
	if p == nil {
		t.Fatal("NewPolicy(nil) returned nil")
	}
	if p.allowAllFolders {
		t.Error("expected allowAllFolders false for nil config")
	}
	if p.allowAllTags {
		t.Error("expected allowAllTags false for nil config")
	}
	if p.allowCreateTag {
		t.Error("expected allowCreateTag false for nil config")
	}
	if p.allowCreateFolder {
		t.Error("expected allowCreateFolder false for nil config")
	}
}

func TestNewPolicy_WildcardFolders(t *testing.T) {
	cfg := &config.Config{MCPAllowFolders: "*"}
	p := NewPolicy(cfg)
	if !p.allowAllFolders {
		t.Error("expected allowAllFolders true for wildcard")
	}
}

func TestNewPolicy_WildcardTags(t *testing.T) {
	cfg := &config.Config{MCPAllowTags: "*"}
	p := NewPolicy(cfg)
	if !p.allowAllTags {
		t.Error("expected allowAllTags true for wildcard")
	}
}

func TestNewPolicy_SpecificFolders(t *testing.T) {
	cfg := &config.Config{MCPAllowFolders: "Inbox,Folder1"}
	p := NewPolicy(cfg)
	if p.allowAllFolders {
		t.Error("expected allowAllFolders false for specific list")
	}
	if !p.CanCreateNoteInFolder("", "Inbox") {
		t.Error("Inbox should be allowed by title")
	}
	if !p.CanCreateNoteInFolder("", "folder1") {
		t.Error("folder1 (case-insensitive) should be allowed")
	}
	if p.CanCreateNoteInFolder("", "Other") {
		t.Error("Other should not be allowed")
	}
}

func TestNewPolicy_SpecificTags(t *testing.T) {
	cfg := &config.Config{MCPAllowTags: "work,personal"}
	p := NewPolicy(cfg)
	if !p.CanAttachTag("abc123def456789012345678901234ab", "work") {
		t.Error("work should be allowed")
	}
	if !p.CanAttachTag("", "personal") {
		t.Error("personal should be allowed by title")
	}
	if p.CanAttachTag("", "other") {
		t.Error("other should not be allowed")
	}
}

func TestNewPolicy_AllowCreateTag(t *testing.T) {
	cfg := &config.Config{MCPAllowCreateTag: true}
	p := NewPolicy(cfg)
	if !p.CanCreateTag() {
		t.Error("expected CanCreateTag true")
	}
}

func TestNewPolicy_AllowCreateFolder(t *testing.T) {
	cfg := &config.Config{MCPAllowCreateFolder: true}
	p := NewPolicy(cfg)
	if !p.CanCreateFolder() {
		t.Error("expected CanCreateFolder true")
	}
}

func TestPolicy_CapabilitiesJSON(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	defer os.RemoveAll(dir)

	// Add a folder and tag
	f := &models.Folder{Title: "Inbox"}
	if err := db.CreateFolder(f); err != nil {
		t.Fatal(err)
	}
	tag := &models.Tag{Title: "work"}
	if err := db.CreateTag(tag); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		MCPAllowFolders:      "Inbox",
		MCPAllowTags:         "work",
		MCPAllowCreateTag:    true,
		MCPAllowCreateFolder: false,
	}
	p := NewPolicy(cfg)
	raw, err := p.CapabilitiesJSON(db)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	folders, _ := doc["folders"].(map[string]any)
	if folders == nil {
		t.Fatal("missing folders in capabilities")
	}
	tags, _ := doc["tags"].(map[string]any)
	if tags == nil {
		t.Fatal("missing tags in capabilities")
	}
}

func TestPolicy_CanCreateNoteInFolderOrEmpty_AllowAll(t *testing.T) {
	cfg := &config.Config{MCPAllowFolders: "*"}
	p := NewPolicy(cfg)
	if !p.CanCreateNoteInFolderOrEmpty("", "") {
		t.Error("root note should be allowed when wildcard")
	}
	if !p.CanCreateNoteInFolderOrEmpty("fid123", "Folder") {
		t.Error("any folder should be allowed when wildcard")
	}
}

func TestPolicy_CanCreateNoteInFolderOrEmpty_EmptyPolicy(t *testing.T) {
	p := NewPolicy(nil)
	if p.CanCreateNoteInFolderOrEmpty("", "") {
		t.Error("root note should be denied with empty policy")
	}
	if p.CanCreateNoteInFolderOrEmpty("fid", "Inbox") {
		t.Error("folder should be denied with empty policy")
	}
}
