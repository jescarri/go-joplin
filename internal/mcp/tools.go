package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jescarri/go-joplin/internal/models"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterAll registers all Joplin MCP tools on the server. Add new tools here to expose them.
func RegisterAll(server *Server, d *Deps) {
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "list_notes",
		Description: "List notes with optional folder and limit",
	}, listNotesHandler(d))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "get_note",
		Description: "Get a note by ID",
	}, getNoteHandler(d))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "create_note",
		Description: "Create a note. Pass title (required), optional body, optional folder placement via parent_id (folder ID) or folder_name (case-insensitive name), and optional tags via tags (list of tag IDs) or tag_names (list of tag names, auto-created if missing). Use folder_name and tag_names when you have names instead of IDs.",
	}, createNoteHandler(d))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "update_note",
		Description: "Update note title or body",
	}, updateNoteHandler(d))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "search_notes",
		Description: "Full-text search in notes",
	}, searchNotesHandler(d))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "list_folders",
		Description: "List all folders (notebooks). Use folder IDs as parent_id when creating notes or subfolders.",
	}, listFoldersHandler(d))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "get_folder",
		Description: "Get folder (notebook) by ID",
	}, getFolderHandler(d))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "create_folder",
		Description: "Create a folder (notebook). Pass title (required) and optional parent_id to create a subfolder.",
	}, createFolderHandler(d))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "list_tags",
		Description: "List tags",
	}, listTagsHandler(d))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "get_note_tags",
		Description: "Get tags for a note",
	}, getNoteTagsHandler(d))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "list_resources",
		Description: "List resources, optionally filtered by note ID",
	}, listResourcesHandler(d))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "trigger_sync",
		Description: "Trigger a sync run (no wait)",
	}, triggerSyncHandler(d))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "get_capabilities",
		Description: "Get mutation capabilities (folders/tags read-write vs read-only, tag/folder creation allowed). Use this to know which folders you can create notes in and which tags you can use.",
	}, getCapabilitiesHandler(d))
}

// --- Input/Output structs (easy to modify per tool) ---

type ListNotesIn struct {
	FolderID string `json:"folder_id,omitempty" jsonschema:"folder ID to filter"`
	Limit    int    `json:"limit,omitempty" jsonschema:"max number of notes (default 10)"`
	Offset   int    `json:"offset,omitempty" jsonschema:"offset for pagination"`
}

type GetNoteIn struct {
	NoteID string `json:"note_id" jsonschema:"note ID to retrieve"`
}

type CreateNoteIn struct {
	Title      string   `json:"title" jsonschema:"title of the note"`
	Body       string   `json:"body,omitempty" jsonschema:"note body in Markdown"`
	ParentID   string   `json:"parent_id,omitempty" jsonschema:"parent folder ID (use this OR folder_name, not both)"`
	FolderName string   `json:"folder_name,omitempty" jsonschema:"folder name to resolve to an ID (case-insensitive; use this OR parent_id, not both)"`
	Tags       []string `json:"tags,omitempty" jsonschema:"list of tag IDs to attach to the note"`
	TagNames   []string `json:"tag_names,omitempty" jsonschema:"list of tag names to attach (resolved or auto-created; use this OR tags, not both)"`
}

type UpdateNoteIn struct {
	NoteID string `json:"note_id" jsonschema:"note ID to update"`
	Title  string `json:"title,omitempty" jsonschema:"new title"`
	Body   string `json:"body,omitempty" jsonschema:"new body"`
}

type SearchNotesIn struct {
	Query string `json:"query" jsonschema:"search query"`
	Limit int    `json:"limit,omitempty" jsonschema:"max results (default 20)"`
}

type GetFolderIn struct {
	FolderID string `json:"folder_id" jsonschema:"folder ID to retrieve"`
}

type CreateFolderIn struct {
	Title    string `json:"title" jsonschema:"folder title (notebook name)"`
	ParentID string `json:"parent_id,omitempty" jsonschema:"parent folder ID for subfolders"`
}

type GetNoteTagsIn struct {
	NoteID string `json:"note_id" jsonschema:"note ID"`
}

type ListResourcesIn struct {
	NoteID string `json:"note_id,omitempty" jsonschema:"note ID to filter"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max results (default 20)"`
}

type ListFoldersIn struct{}      // no args
type ListTagsIn struct{}         // no args
type TriggerSyncIn struct{}      // no args
type GetCapabilitiesIn struct{}  // no args

// --- Handlers ---

func listNotesHandler(d *Deps) func(context.Context, *sdkmcp.CallToolRequest, ListNotesIn) (*sdkmcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest, in ListNotesIn) (*sdkmcp.CallToolResult, any, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}
		notes, _, err := d.DB.ListNotes("updated_time", "DESC", limit, in.Offset)
		if err != nil {
			return nil, nil, err
		}
		if in.FolderID != "" {
			var filtered []*models.Note
			for _, n := range notes {
				if n.ParentID == in.FolderID {
					filtered = append(filtered, n)
				}
			}
			notes = filtered
		}
		out := map[string]any{"notes": notes, "count": len(notes)}
		return nil, out, nil
	}
}

func getNoteHandler(d *Deps) func(context.Context, *sdkmcp.CallToolRequest, GetNoteIn) (*sdkmcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest, in GetNoteIn) (*sdkmcp.CallToolResult, any, error) {
		if in.NoteID == "" {
			return nil, nil, fmt.Errorf("note_id is required")
		}
		note, err := d.DB.GetNote(in.NoteID)
		if err != nil {
			return nil, nil, err
		}
		if note == nil {
			return nil, nil, fmt.Errorf("note not found: %s", in.NoteID)
		}
		return nil, note, nil
	}
}

func createNoteHandler(d *Deps) func(context.Context, *sdkmcp.CallToolRequest, CreateNoteIn) (*sdkmcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest, in CreateNoteIn) (*sdkmcp.CallToolResult, any, error) {
		if in.Title == "" {
			return nil, nil, fmt.Errorf("title is required")
		}

		parentID, folderTitle, err := resolveParentIDWithTitle(d, in.ParentID, in.FolderName)
		if err != nil {
			return nil, nil, err
		}

		if d.Policy == nil || !d.Policy.CanCreateNoteInFolderOrEmpty(parentID, folderTitle) {
			return nil, nil, fmt.Errorf("folder is read-only: create notes only in writable folders (use get_capabilities to see allowed folders, or set GOJOPLIN_MCP_ALLOW_FOLDERS=* to allow all)")
		}

		tagIDs, err := resolveTagIDs(d, in.Tags, in.TagNames)
		if err != nil {
			return nil, nil, err
		}

		note := &models.Note{Title: in.Title, Body: in.Body, ParentID: parentID}
		if err := d.DB.CreateNote(note); err != nil {
			return nil, nil, err
		}
		for _, tagID := range tagIDs {
			if err := d.DB.AddNoteTag(note.ID, tagID); err != nil {
				return nil, nil, fmt.Errorf("add tag %q: %w", tagID, err)
			}
		}
		if d.Syncer != nil {
			d.Syncer.TriggerSync()
		}
		return nil, note, nil
	}
}

// resolveParentID resolves the parent folder from either a direct ID or a name.
func resolveParentID(d *Deps, parentID, folderName string) (string, error) {
	id, _, err := resolveParentIDWithTitle(d, parentID, folderName)
	return id, err
}

// resolveParentIDWithTitle returns (folderID, folderTitle, error).
func resolveParentIDWithTitle(d *Deps, parentID, folderName string) (string, string, error) {
	if parentID != "" && folderName != "" {
		return "", "", fmt.Errorf("pass parent_id or folder_name, not both")
	}

	if folderName != "" {
		folder, err := d.DB.GetFolderByTitle(folderName)
		if err != nil {
			return "", "", fmt.Errorf("lookup folder by name %q: %w", folderName, err)
		}
		if folder == nil {
			return "", "", fmt.Errorf("folder not found: %q (use list_folders to see available folders)", folderName)
		}
		return folder.ID, folder.Title, nil
	}

	if parentID != "" {
		folder, err := d.DB.GetFolder(parentID)
		if err != nil {
			return "", "", fmt.Errorf("lookup folder %q: %w", parentID, err)
		}
		if folder == nil {
			return "", "", fmt.Errorf("folder not found: %q (use list_folders to see available folder IDs)", parentID)
		}
		return folder.ID, folder.Title, nil
	}

	return "", "", nil
}

// resolveTagIDs resolves tags from IDs and/or names, creating tags that don't exist by name if policy allows.
func resolveTagIDs(d *Deps, tagIDs, tagNames []string) ([]string, error) {
	seen := make(map[string]bool)
	var resolved []string

	for _, id := range tagIDs {
		if id == "" || seen[id] {
			continue
		}
		tag, err := d.DB.GetTag(id)
		if err != nil {
			return nil, fmt.Errorf("lookup tag %q: %w", id, err)
		}
		if tag == nil {
			return nil, fmt.Errorf("tag not found: %q (use list_tags to see available tag IDs)", id)
		}
		if d.Policy == nil || !d.Policy.CanAttachTag(tag.ID, tag.Title) {
			return nil, fmt.Errorf("tag %q not in writable list (use get_capabilities; or set GOJOPLIN_MCP_ALLOW_TAGS=* to allow all)", tag.Title)
		}
		seen[id] = true
		resolved = append(resolved, id)
	}

	for _, name := range tagNames {
		if name == "" {
			continue
		}
		tag, err := d.DB.GetTagByTitle(name)
		if err != nil {
			return nil, fmt.Errorf("lookup tag by name %q: %w", name, err)
		}
		if tag == nil {
			if d.Policy == nil || !d.Policy.CanCreateTag() {
				return nil, fmt.Errorf("tag creation disabled: cannot create tag %q (set GOJOPLIN_MCP_ALLOW_CREATE_TAG=true to allow)", name)
			}
			tag = &models.Tag{Title: name}
			if err := d.DB.CreateTag(tag); err != nil {
				return nil, fmt.Errorf("create tag %q: %w", name, err)
			}
		}
		if d.Policy == nil || !d.Policy.CanAttachTag(tag.ID, tag.Title) {
			return nil, fmt.Errorf("tag %q not in writable list (use get_capabilities; or set GOJOPLIN_MCP_ALLOW_TAGS=* to allow all)", tag.Title)
		}
		if !seen[tag.ID] {
			seen[tag.ID] = true
			resolved = append(resolved, tag.ID)
		}
	}

	return resolved, nil
}

func updateNoteHandler(d *Deps) func(context.Context, *sdkmcp.CallToolRequest, UpdateNoteIn) (*sdkmcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest, in UpdateNoteIn) (*sdkmcp.CallToolResult, any, error) {
		if in.NoteID == "" {
			return nil, nil, fmt.Errorf("note_id is required")
		}
		note, err := d.DB.GetNote(in.NoteID)
		if err != nil || note == nil {
			return nil, nil, fmt.Errorf("note not found: %s", in.NoteID)
		}
		var folderTitle string
		if note.ParentID != "" {
			if f, _ := d.DB.GetFolder(note.ParentID); f != nil {
				folderTitle = f.Title
			}
		}
		if d.Policy == nil || !d.Policy.CanUpdateNoteInFolder(note.ParentID, folderTitle) {
			return nil, nil, fmt.Errorf("note's folder is read-only (use get_capabilities to see writable folders)")
		}
		if in.Title != "" {
			note.Title = in.Title
		}
		if in.Body != "" {
			note.Body = in.Body
		}
		if err := d.DB.UpdateNote(note); err != nil {
			return nil, nil, err
		}
		if d.Syncer != nil {
			d.Syncer.TriggerSync()
		}
		return nil, note, nil
	}
}

func searchNotesHandler(d *Deps) func(context.Context, *sdkmcp.CallToolRequest, SearchNotesIn) (*sdkmcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest, in SearchNotesIn) (*sdkmcp.CallToolResult, any, error) {
		if in.Query == "" {
			return nil, nil, fmt.Errorf("query is required")
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		notes, _, err := d.DB.SearchNotes(in.Query, limit, 0)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"notes": notes, "count": len(notes)}, nil
	}
}

func listFoldersHandler(d *Deps) func(context.Context, *sdkmcp.CallToolRequest, ListFoldersIn) (*sdkmcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest, in ListFoldersIn) (*sdkmcp.CallToolResult, any, error) {
		folders, _, err := d.DB.ListFolders("title", "ASC", 500, 0)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"folders": folders, "count": len(folders)}, nil
	}
}

func getFolderHandler(d *Deps) func(context.Context, *sdkmcp.CallToolRequest, GetFolderIn) (*sdkmcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest, in GetFolderIn) (*sdkmcp.CallToolResult, any, error) {
		if in.FolderID == "" {
			return nil, nil, fmt.Errorf("folder_id is required")
		}
		folder, err := d.DB.GetFolder(in.FolderID)
		if err != nil {
			return nil, nil, err
		}
		if folder == nil {
			return nil, nil, fmt.Errorf("folder not found: %s", in.FolderID)
		}
		return nil, folder, nil
	}
}

func createFolderHandler(d *Deps) func(context.Context, *sdkmcp.CallToolRequest, CreateFolderIn) (*sdkmcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest, in CreateFolderIn) (*sdkmcp.CallToolResult, any, error) {
		if in.Title == "" {
			return nil, nil, fmt.Errorf("title is required")
		}
		if d.Policy == nil || !d.Policy.CanCreateFolder() {
			return nil, nil, fmt.Errorf("folder creation disabled (set GOJOPLIN_MCP_ALLOW_CREATE_FOLDER=true to allow)")
		}
		folder := &models.Folder{Title: in.Title, ParentID: in.ParentID}
		if err := d.DB.CreateFolder(folder); err != nil {
			return nil, nil, err
		}
		if d.Syncer != nil {
			d.Syncer.TriggerSync()
		}
		return nil, folder, nil
	}
}

func listTagsHandler(d *Deps) func(context.Context, *sdkmcp.CallToolRequest, ListTagsIn) (*sdkmcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest, in ListTagsIn) (*sdkmcp.CallToolResult, any, error) {
		tags, _, err := d.DB.ListTags("title", "ASC", 500, 0)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"tags": tags, "count": len(tags)}, nil
	}
}

func getNoteTagsHandler(d *Deps) func(context.Context, *sdkmcp.CallToolRequest, GetNoteTagsIn) (*sdkmcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest, in GetNoteTagsIn) (*sdkmcp.CallToolResult, any, error) {
		if in.NoteID == "" {
			return nil, nil, fmt.Errorf("note_id is required")
		}
		tags, err := d.DB.GetNoteTagsByNote(in.NoteID)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"tags": tags, "count": len(tags)}, nil
	}
}

func listResourcesHandler(d *Deps) func(context.Context, *sdkmcp.CallToolRequest, ListResourcesIn) (*sdkmcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest, in ListResourcesIn) (*sdkmcp.CallToolResult, any, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		var resources []*models.Resource
		var err error
		if in.NoteID != "" {
			resources, err = d.DB.GetResourcesByNote(in.NoteID)
			if len(resources) > limit {
				resources = resources[:limit]
			}
		} else {
			var hasMore bool
			resources, hasMore, err = d.DB.ListResources("updated_time", "DESC", limit, 0)
			_ = hasMore
		}
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"resources": resources, "count": len(resources)}, nil
	}
}

func triggerSyncHandler(d *Deps) func(context.Context, *sdkmcp.CallToolRequest, TriggerSyncIn) (*sdkmcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest, in TriggerSyncIn) (*sdkmcp.CallToolResult, any, error) {
		if d.Syncer != nil {
			d.Syncer.TriggerSync()
		}
		return nil, map[string]any{"status": "triggered"}, nil
	}
}

func getCapabilitiesHandler(d *Deps) func(context.Context, *sdkmcp.CallToolRequest, GetCapabilitiesIn) (*sdkmcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest, in GetCapabilitiesIn) (*sdkmcp.CallToolResult, any, error) {
		policy := d.Policy
		if policy == nil {
			policy = NewPolicy(nil) // empty policy = all read-only
		}
		raw, err := policy.CapabilitiesJSON(d.DB)
		if err != nil {
			return nil, nil, err
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, nil, err
		}
		return nil, doc, nil
	}
}

