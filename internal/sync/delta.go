package sync

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/jescarri/go-joplin/internal/models"
	"github.com/jescarri/go-joplin/internal/store"
)

// DeltaResponse represents the Joplin Server delta API response.
type DeltaResponse struct {
	Items   []DeltaItem `json:"items"`
	Cursor  string      `json:"cursor"`
	HasMore bool        `json:"has_more"`
}

// DeltaItem represents a single item in a delta response.
type DeltaItem struct {
	ID        string `json:"id"`
	ItemName  string `json:"item_name"`
	Type      int    `json:"type"` // 1=put, 3=delete
	UpdatedTime int64 `json:"updated_time"`
}

// PullChanges fetches all changes from the server since the last sync cursor
// and applies them to the local database.
func PullChanges(backend SyncBackend, db *store.DB) error {
	cursor, err := db.GetSyncCursor()
	if err != nil {
		return fmt.Errorf("cannot get sync cursor: %w", err)
	}

	syncTarget := backend.SyncTarget()

	for {
		path := "/api/items/root:/:/delta"
		if cursor != "" {
			path += "?cursor=" + url.QueryEscape(cursor)
		}

		data, err := backend.Get(path)
		if err != nil {
			return fmt.Errorf("delta request failed: %w", err)
		}

		var delta DeltaResponse
		if err := json.Unmarshal(data, &delta); err != nil {
			return fmt.Errorf("cannot parse delta response: %w", err)
		}

		slog.Info("processing delta", "items", len(delta.Items), "has_more", delta.HasMore)

		typeCounts := make(map[string]int)
		for _, item := range delta.Items {
			if item.Type == 3 {
				typeCounts["delete"]++
			} else if isResourceBlob(item.ItemName) {
				typeCounts["resource_blob"]++
			}
			if err := applyDeltaItem(backend, db, syncTarget, item, nil); err != nil {
				slog.Error("failed to apply delta item", "item_name", item.ItemName, "error", err)
				continue
			}
		}
		slog.Info("delta summary", "counts", typeCounts)

		cursor = delta.Cursor
		if err := db.SetSyncCursor(cursor); err != nil {
			return fmt.Errorf("cannot save sync cursor: %w", err)
		}

		if !delta.HasMore {
			break
		}
	}

	return nil
}

// applyDeltaItem processes a single delta item. If prefetched is non-nil it is
// used as the item content; otherwise the content is fetched from the server.
func applyDeltaItem(backend SyncBackend, db *store.DB, syncTarget int, item DeltaItem, prefetched []byte) error {
	// Delete type
	if item.Type == 3 {
		itemID, itemType := parseItemName(item.ItemName)
		if itemID != "" {
			return db.DeleteLocalItem(itemID, itemType)
		}
		return nil
	}

	// Put type - use prefetched content or fetch from server
	content := prefetched
	if content == nil {
		path := fmt.Sprintf("/api/items/root:/%s:/content", url.PathEscape(item.ItemName))
		var err error
		content, err = backend.Get(path)
		if err != nil {
			return fmt.Errorf("cannot fetch item content: %w", err)
		}
	}

	// Check if this is a resource blob (binary file)
	if isResourceBlob(item.ItemName) {
		resourceID := extractResourceID(item.ItemName)
		if resourceID != "" {
			return db.SaveResourceFile(resourceID, content)
		}
		return nil
	}

	// Parse as Joplin text format
	itemType, _ := models.Deserialize(string(content))

	slog.Debug("applying delta item", "name", item.ItemName, "parsed_type", itemType)

	switch itemType {
	case models.TypeNote:
		note := models.DeserializeNote(string(content))
		if note.ID == "" {
			return nil
		}
		if note.EncryptionApplied == 1 {
			existing, err := db.GetNote(note.ID)
			if err == nil && existing != nil && existing.EncryptionApplied == 0 {
				slog.Debug("skipping encrypted server copy, local is already decrypted", "id", note.ID)
				return db.UpsertSyncItem(note.ID, models.TypeNote, syncTarget)
			}
		}
		if err := db.UpsertNote(note); err != nil {
			return fmt.Errorf("cannot upsert note %s: %w", note.ID, err)
		}
		return db.UpsertSyncItem(note.ID, models.TypeNote, syncTarget)

	case models.TypeFolder:
		folder := models.DeserializeFolder(string(content))
		if folder.ID == "" {
			return nil
		}
		if folder.EncryptionApplied == 1 {
			existing, err := db.GetFolder(folder.ID)
			if err == nil && existing != nil && existing.EncryptionApplied == 0 {
				slog.Debug("skipping encrypted server copy, local folder is already decrypted", "id", folder.ID)
				return db.UpsertSyncItem(folder.ID, models.TypeFolder, syncTarget)
			}
		}
		if err := db.UpsertFolder(folder); err != nil {
			return fmt.Errorf("cannot upsert folder %s: %w", folder.ID, err)
		}
		return db.UpsertSyncItem(folder.ID, models.TypeFolder, syncTarget)

	case models.TypeTag:
		tag := models.DeserializeTag(string(content))
		if tag.ID == "" {
			return nil
		}
		if tag.EncryptionApplied == 1 {
			existing, err := db.GetTag(tag.ID)
			if err == nil && existing != nil && existing.EncryptionApplied == 0 {
				slog.Debug("skipping encrypted server copy, local tag is already decrypted", "id", tag.ID)
				return db.UpsertSyncItem(tag.ID, models.TypeTag, syncTarget)
			}
		}
		if err := db.UpsertTag(tag); err != nil {
			return fmt.Errorf("cannot upsert tag %s: %w", tag.ID, err)
		}
		return db.UpsertSyncItem(tag.ID, models.TypeTag, syncTarget)

	case models.TypeNoteTag:
		nt := models.DeserializeNoteTag(string(content))
		if nt.ID == "" {
			return nil
		}
		if err := db.UpsertNoteTag(nt); err != nil {
			return fmt.Errorf("cannot upsert note_tag %s: %w", nt.ID, err)
		}
		return db.UpsertSyncItem(nt.ID, models.TypeNoteTag, syncTarget)

	case models.TypeResource:
		resource := models.DeserializeResource(string(content))
		if resource.ID == "" {
			return nil
		}
		if err := db.UpsertResource(resource); err != nil {
			return fmt.Errorf("cannot upsert resource %s: %w", resource.ID, err)
		}
		return db.UpsertSyncItem(resource.ID, models.TypeResource, syncTarget)

	case models.TypeMasterKey:
		mk := models.DeserializeMasterKey(string(content))
		if mk.ID == "" {
			return nil
		}
		if err := db.UpsertMasterKey(mk); err != nil {
			return fmt.Errorf("cannot upsert master_key %s: %w", mk.ID, err)
		}
		return db.UpsertSyncItem(mk.ID, models.TypeMasterKey, syncTarget)

	case models.TypeRevision:
		// Revisions (note history) are synced by Joplin but we don't persist their content.
		// Record as synced so we don't re-fetch them every cycle.
		itemID, _ := parseItemName(item.ItemName)
		if itemID != "" {
			return db.UpsertSyncItem(itemID, models.TypeRevision, syncTarget)
		}
		return nil

	default:
		slog.Warn("skipping unknown item type", "type", itemType, "name", item.ItemName)
	}

	return nil
}

// parseItemName extracts the item ID from a Joplin Server item name.
// Item names look like: "<32-hex-char-id>.md"
// Follows the same approach as Joplin's BaseItem.pathToId / isSystemPath:
// split on ".", verify the name part is a valid 32-char hex ID and the
// extension is "md".
func parseItemName(name string) (string, int) {
	// Take the last path component (handles "folder/id.md" style paths)
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 || parts[1] != "md" {
		return "", 0
	}
	id := parts[0]
	if !isValidID(id) {
		return "", 0
	}
	return id, 0 // Type is unknown from name alone
}

// isResourceBlob checks if an item name refers to a resource blob.
// Resource blobs don't end with ".md".
func isResourceBlob(name string) bool {
	return len(name) > 2 && !strings.HasSuffix(name, ".md")
}

// extractResourceID gets the resource ID from a blob item name.
func extractResourceID(name string) string {
	// Resource blobs are stored as ".resource/<id>"
	if len(name) >= 42 && name[:10] == ".resource/" {
		return name[10:]
	}
	return ""
}

func isValidID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
