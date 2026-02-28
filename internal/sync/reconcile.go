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
//
// It also removes local items whose IDs no longer appear on the server,
// handling cases where the delta delete event was missed (e.g. cursor
// already advanced past it).
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

	serverIDs := make(map[string]struct{})

	// For each .md item, check if we already have it in sync_items
	fetched := 0
	for _, child := range allItems {
		if !strings.HasSuffix(child.Name, ".md") {
			continue
		}
		if isResourceBlob(child.Name) {
			continue
		}

		itemID, _ := parseItemName(child.Name)
		if itemID == "" {
			continue
		}

		serverIDs[itemID] = struct{}{}

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

	// Also track resource blobs present on the server
	for _, child := range allItems {
		if isResourceBlob(child.Name) {
			resID := extractResourceID(child.Name)
			if resID != "" {
				serverIDs[resID] = struct{}{}
			}
		}
	}

	// Remove local items that no longer exist on the server
	localItems, err := db.ListSyncItemIDs(syncTarget)
	if err != nil {
		return fmt.Errorf("cannot list local sync items: %w", err)
	}

	pruned := 0
	for _, si := range localItems {
		if _, exists := serverIDs[si.ItemID]; exists {
			continue
		}
		slog.Info("removing stale local item not found on server", "item_id", si.ItemID, "item_type", si.ItemType)
		if err := db.DeleteLocalItem(si.ItemID, si.ItemType); err != nil {
			slog.Error("cannot delete stale local item", "item_id", si.ItemID, "error", err)
			continue
		}
		pruned++
	}

	if pruned > 0 {
		slog.Info("pruned stale local items", "count", pruned)
	}

	return nil
}
