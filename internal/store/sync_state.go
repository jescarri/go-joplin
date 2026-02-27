package store

import (
	"database/sql"
	"time"

	"github.com/jescarri/go-joplin/internal/models"
)

// GetSyncCursor returns the stored delta sync cursor.
func (db *DB) GetSyncCursor() (string, error) {
	var cursor string
	err := db.QueryRow("SELECT value FROM key_values WHERE key = 'sync.cursor'").Scan(&cursor)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return cursor, err
}

// SetSyncCursor persists the delta sync cursor.
func (db *DB) SetSyncCursor(cursor string) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO key_values (key, value, type, updated_time) VALUES ('sync.cursor', ?, 1, strftime('%s','now') * 1000)`, cursor)
	return err
}

// GetActiveMasterKeyID returns the active E2EE master key ID from sync info (empty if not set).
func (db *DB) GetActiveMasterKeyID() (string, error) {
	var id string
	err := db.QueryRow("SELECT value FROM key_values WHERE key = 'sync.active_master_key_id'").Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

// SetActiveMasterKeyID stores the active E2EE master key ID from info.json.
func (db *DB) SetActiveMasterKeyID(id string) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO key_values (key, value, type, updated_time) VALUES ('sync.active_master_key_id', ?, 1, strftime('%s','now') * 1000)`, id)
	return err
}

// GetSyncItem returns the sync state for a specific item.
func (db *DB) GetSyncItem(itemID string, syncTarget int) (*models.SyncItem, error) {
	si := &models.SyncItem{}
	err := db.QueryRow(`SELECT id, sync_target, sync_time, item_type, item_id,
		sync_disabled, sync_disabled_reason, force_sync, item_location
		FROM sync_items WHERE item_id = ? AND sync_target = ?`, itemID, syncTarget).Scan(
		&si.ID, &si.SyncTarget, &si.SyncTime, &si.ItemType, &si.ItemID,
		&si.SyncDisabled, &si.SyncDisabledReason, &si.ForceSync, &si.ItemLocation,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return si, nil
}

// UpsertSyncItem records that an item has been synced.
func (db *DB) UpsertSyncItem(itemID string, itemType, syncTarget int) error {
	now := time.Now().UnixMilli()

	// Check if exists
	existing, err := db.GetSyncItem(itemID, syncTarget)
	if err != nil {
		return err
	}

	if existing != nil {
		_, err = db.Exec(`UPDATE sync_items SET sync_time = ? WHERE item_id = ? AND sync_target = ?`,
			now, itemID, syncTarget)
	} else {
		_, err = db.Exec(`INSERT INTO sync_items (sync_target, sync_time, item_type, item_id, sync_disabled, sync_disabled_reason, force_sync, item_location)
			VALUES (?, ?, ?, ?, 0, '', 0, 0)`,
			syncTarget, now, itemType, itemID)
	}
	return err
}

// AddDeletedItem records an item that needs to be deleted on the server.
func (db *DB) AddDeletedItem(itemID string, itemType, syncTarget int) error {
	now := time.Now().UnixMilli()
	_, err := db.Exec(`INSERT INTO deleted_items (item_type, item_id, deleted_time, sync_target)
		VALUES (?, ?, ?, ?)`, itemType, itemID, now, syncTarget)
	return err
}

// GetDeletedItems returns items that need to be deleted from the server.
func (db *DB) GetDeletedItems(syncTarget int) ([]*models.DeletedItem, error) {
	rows, err := db.Query(`SELECT id, item_type, item_id, deleted_time, sync_target
		FROM deleted_items WHERE sync_target = ?`, syncTarget)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*models.DeletedItem
	for rows.Next() {
		di := &models.DeletedItem{}
		if err := rows.Scan(&di.ID, &di.ItemType, &di.ItemID, &di.DeletedTime, &di.SyncTarget); err != nil {
			return nil, err
		}
		items = append(items, di)
	}
	return items, nil
}

// RemoveDeletedItem removes a deleted item record after it's been synced.
func (db *DB) RemoveDeletedItem(id int) error {
	_, err := db.Exec("DELETE FROM deleted_items WHERE id = ?", id)
	return err
}

// DeleteLocalItem removes an item from the local database (called during sync pull).
func (db *DB) DeleteLocalItem(itemID string, itemType int) error {
	switch itemType {
	case models.TypeNote:
		_, err := db.Exec("DELETE FROM notes WHERE id = ?", itemID)
		return err
	case models.TypeFolder:
		_, err := db.Exec("DELETE FROM folders WHERE id = ?", itemID)
		return err
	case models.TypeTag:
		_, err := db.Exec("DELETE FROM tags WHERE id = ?", itemID)
		return err
	case models.TypeNoteTag:
		_, err := db.Exec("DELETE FROM note_tags WHERE id = ?", itemID)
		return err
	case models.TypeResource:
		return db.DeleteResource(itemID)
	}
	return nil
}
