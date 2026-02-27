package sync

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/jescarri/go-joplin/internal/store"
)

// ChildrenResponse represents the Joplin Server children list API response.
type ChildrenResponse struct {
	Items   []ChildItem `json:"items"`
	Cursor  string      `json:"cursor"`
	HasMore bool        `json:"has_more"`
}

// ChildItem represents a single item in a children response.
type ChildItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PullMissingItems lists all items from the server root and fetches any
// that are not already tracked in sync_items. This catches items like
// master keys that some Joplin Server versions omit from the delta endpoint.
func PullMissingItems(backend SyncBackend, db *store.DB) error {
	syncTarget := backend.SyncTarget()

	// Collect all item names from the server
	var allItems []ChildItem
	cursor := ""

	for {
		path := "/api/items/root:/:/children"
		if cursor != "" {
			path += "?cursor=" + url.QueryEscape(cursor)
		}

		data, err := backend.Get(path)
		if err != nil {
			return fmt.Errorf("children request failed: %w", err)
		}

		var resp ChildrenResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return fmt.Errorf("cannot parse children response: %w", err)
		}

		allItems = append(allItems, resp.Items...)
		cursor = resp.Cursor

		if !resp.HasMore {
			break
		}
	}

	slog.Info("server root children", "count", len(allItems))

	// For each .md item, check if we already have it in sync_items
	fetched := 0
	for _, child := range allItems {
		if !strings.HasSuffix(child.Name, ".md") {
			continue
		}
		if isResourceBlob(child.Name) {
			continue
		}

		// Extract the item ID from the name
		itemID, _ := parseItemName(child.Name)
		if itemID == "" {
			continue
		}

		// Check if we already have this item synced
		si, err := db.GetSyncItem(itemID, syncTarget)
		if err != nil {
			slog.Error("cannot check sync item", "id", itemID, "error", err)
			continue
		}
		if si != nil {
			continue // already synced via delta
		}

		// Fetch and process this missing item
		slog.Info("fetching missing item from server", "name", child.Name)
		contentPath := fmt.Sprintf("/api/items/root:/%s:/content", url.PathEscape(child.Name))
		content, err := backend.Get(contentPath)
		if err != nil {
			slog.Error("cannot fetch missing item content", "name", child.Name, "error", err)
			continue
		}

		// Process it like a delta PUT item
		deltaItem := DeltaItem{
			ID:       child.ID,
			ItemName: child.Name,
			Type:     1, // PUT
		}
		if err := applyDeltaItem(backend, db, syncTarget, deltaItem, content); err != nil {
			slog.Error("failed to apply missing item", "name", child.Name, "error", err)
			continue
		}
		fetched++
	}

	if fetched > 0 {
		slog.Info("fetched missing items from server", "count", fetched)
	}

	return nil
}
