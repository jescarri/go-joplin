package mcp

import (
	"encoding/json"
	"strings"

	"github.com/jescarri/go-joplin/internal/config"
	"github.com/jescarri/go-joplin/internal/store"
)

const wildcard = "*"

// Policy controls which mutations (create/update note, folder, tag) are allowed.
// By default all mutations are denied. Use allow-lists or wildcard "*" to permit.
type Policy struct {
	allowAllFolders      bool
	allowAllTags         bool
	writableFolderIDs    map[string]bool
	writableFolderTitles map[string]bool
	writableTagIDs       map[string]bool
	writableTagTitles    map[string]bool
	allowCreateTag       bool
	allowCreateFolder    bool
}

// NewPolicy builds a Policy from config. Empty or unset = read-only for all.
func NewPolicy(cfg *config.Config) *Policy {
	p := &Policy{
		writableFolderIDs:    make(map[string]bool),
		writableFolderTitles: make(map[string]bool),
		writableTagIDs:       make(map[string]bool),
		writableTagTitles:    make(map[string]bool),
	}
	if cfg == nil {
		return p
	}
	p.allowCreateTag = cfg.MCPAllowCreateTag
	p.allowCreateFolder = cfg.MCPAllowCreateFolder

	if strings.TrimSpace(cfg.MCPAllowFolders) == wildcard {
		p.allowAllFolders = true
	} else {
		for _, s := range parseList(cfg.MCPAllowFolders) {
			if s == "" {
				continue
			}
			// IDs are 32-char hex; titles are anything else
			if looksLikeID(s) {
				p.writableFolderIDs[s] = true
			} else {
				p.writableFolderTitles[strings.ToLower(s)] = true
			}
		}
	}

	if strings.TrimSpace(cfg.MCPAllowTags) == wildcard {
		p.allowAllTags = true
	} else {
		for _, s := range parseList(cfg.MCPAllowTags) {
			if s == "" {
				continue
			}
			if looksLikeID(s) {
				p.writableTagIDs[s] = true
			} else {
				p.writableTagTitles[strings.ToLower(s)] = true
			}
		}
	}
	return p
}

func parseList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// looksLikeID returns true for 32-char hex strings (Joplin IDs).
func looksLikeID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, c := range s {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}

// CanCreateNoteInFolder returns true if a note may be created in the given folder.
func (p *Policy) CanCreateNoteInFolder(folderID, folderTitle string) bool {
	if p == nil {
		return false
	}
	if p.allowAllFolders {
		return true
	}
	if p.writableFolderIDs[folderID] {
		return true
	}
	if folderTitle != "" && p.writableFolderTitles[strings.ToLower(folderTitle)] {
		return true
	}
	return false
}

// CanCreateNoteInFolderOrEmpty returns true if we may create a note.
// When parentID is empty (note goes to root), allow only if allow-all or explicit empty allowed.
// For root notes, we treat as allowed when allowAllFolders (no parent restriction).
func (p *Policy) CanCreateNoteInFolderOrEmpty(parentID, parentTitle string) bool {
	if p == nil {
		return false
	}
	if p.allowAllFolders {
		return true
	}
	if parentID == "" && parentTitle == "" {
		// Creating orphan/root note - allow only if wildcard
		return p.allowAllFolders
	}
	return p.CanCreateNoteInFolder(parentID, parentTitle)
}

// CanUpdateNoteInFolder returns true if a note in the given folder may be updated.
func (p *Policy) CanUpdateNoteInFolder(folderID, folderTitle string) bool {
	return p.CanCreateNoteInFolder(folderID, folderTitle)
}

// CanCreateFolder returns true if creating new folders is allowed.
func (p *Policy) CanCreateFolder() bool {
	if p == nil {
		return false
	}
	return p.allowCreateFolder
}

// CanAttachTag returns true if the given tag (by ID or title) may be attached to a note.
func (p *Policy) CanAttachTag(tagID, tagTitle string) bool {
	if p == nil {
		return false
	}
	if p.allowAllTags {
		return true
	}
	if p.writableTagIDs[tagID] {
		return true
	}
	if tagTitle != "" && p.writableTagTitles[strings.ToLower(tagTitle)] {
		return true
	}
	return false
}

// CanCreateTag returns true if creating new tags is allowed.
func (p *Policy) CanCreateTag() bool {
	if p == nil {
		return false
	}
	return p.allowCreateTag
}

// CapabilitiesDoc is the JSON document exposed to LLMs via MCP.
type CapabilitiesDoc struct {
	Folders struct {
		ReadWrite   []FolderRef `json:"read_write"`
		ReadOnly    string      `json:"read_only,omitempty"`
		AllowCreate bool        `json:"allow_create"`
	} `json:"folders"`
	Notes struct {
		CreateInWritableFoldersOnly  bool   `json:"create_in_writable_folders_only"`
		UpdateOnlyInWritableFolders  bool   `json:"update_only_in_writable_folders"`
		ReadOnlyHint                 string `json:"read_only_hint,omitempty"`
	} `json:"notes"`
	Tags struct {
		AllowCreate bool        `json:"allow_create"`
		Writable    []TagRef    `json:"writable"`
		ReadOnly    string      `json:"read_only,omitempty"`
	} `json:"tags"`
}

// FolderRef is a folder reference in the capabilities doc.
type FolderRef struct {
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
}

// TagRef is a tag reference in the capabilities doc.
type TagRef struct {
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
}

// Capabilities builds the capability document, optionally enriching with folder/tag list from DB.
func (p *Policy) Capabilities(db *store.DB) (*CapabilitiesDoc, error) {
	doc := &CapabilitiesDoc{}
	doc.Notes.CreateInWritableFoldersOnly = true
	doc.Notes.UpdateOnlyInWritableFolders = true
	doc.Folders.AllowCreate = p != nil && p.allowCreateFolder
	doc.Tags.AllowCreate = p != nil && p.allowCreateTag

	if p == nil {
		doc.Notes.ReadOnlyHint = "all notes are read-only"
		doc.Folders.ReadOnly = "all folders are read-only"
		doc.Tags.ReadOnly = "all tags are read-only"
		return doc, nil
	}

	if p.allowAllFolders {
		doc.Folders.ReadOnly = "none (wildcard * allows all)"
		if db != nil {
			folders, _, err := db.ListFolders("title", "ASC", 500, 0)
			if err == nil && len(folders) > 0 {
				for _, f := range folders {
					doc.Folders.ReadWrite = append(doc.Folders.ReadWrite, FolderRef{ID: f.ID, Title: f.Title})
				}
			}
		}
	} else {
		doc.Folders.ReadOnly = "all others"
		if db != nil {
			for id := range p.writableFolderIDs {
				if f, _ := db.GetFolder(id); f != nil {
					doc.Folders.ReadWrite = append(doc.Folders.ReadWrite, FolderRef{ID: f.ID, Title: f.Title})
				}
			}
			for title := range p.writableFolderTitles {
				if f, _ := db.GetFolderByTitle(title); f != nil {
					doc.Folders.ReadWrite = append(doc.Folders.ReadWrite, FolderRef{ID: f.ID, Title: f.Title})
				}
			}
		}
	}

	if p.allowAllTags {
		doc.Tags.ReadOnly = "none (wildcard * allows all)"
		if db != nil {
			tags, _, err := db.ListTags("title", "ASC", 500, 0)
			if err == nil && len(tags) > 0 {
				for _, t := range tags {
					doc.Tags.Writable = append(doc.Tags.Writable, TagRef{ID: t.ID, Title: t.Title})
				}
			}
		}
	} else {
		doc.Tags.ReadOnly = "all others"
		if db != nil {
			for id := range p.writableTagIDs {
				if t, _ := db.GetTag(id); t != nil {
					doc.Tags.Writable = append(doc.Tags.Writable, TagRef{ID: t.ID, Title: t.Title})
				}
			}
			for title := range p.writableTagTitles {
				if t, _ := db.GetTagByTitle(title); t != nil {
					doc.Tags.Writable = append(doc.Tags.Writable, TagRef{ID: t.ID, Title: t.Title})
				}
			}
		}
	}
	return doc, nil
}

// CapabilitiesJSON returns the capabilities document as JSON.
func (p *Policy) CapabilitiesJSON(db *store.DB) ([]byte, error) {
	doc, err := p.Capabilities(db)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(doc, "", "  ")
}
