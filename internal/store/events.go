package store

import (
	"database/sql"
	"time"

	"github.com/jescarri/go-joplin/internal/models"
)

// recordItemChange inserts an item_change record.
func (db *DB) recordItemChange(itemType int, itemID string, changeType int) error {
	now := time.Now().UnixMilli()
	_, err := db.Exec(`INSERT INTO item_changes (item_type, item_id, type, created_time)
		VALUES (?, ?, ?, ?)`, itemType, itemID, changeType, now)
	return err
}

// GetEvents returns item changes after a cursor (change ID).
func (db *DB) GetEvents(cursor int, limit int) ([]*models.ItemChange, bool, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := db.Query(`SELECT id, item_type, item_id, type, created_time
		FROM item_changes WHERE id > ? ORDER BY id ASC LIMIT ?`, cursor, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var events []*models.ItemChange
	for rows.Next() {
		e := &models.ItemChange{}
		if err := rows.Scan(&e.ID, &e.ItemType, &e.ItemID, &e.Type, &e.CreatedTime); err != nil {
			return nil, false, err
		}
		events = append(events, e)
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}
	return events, hasMore, nil
}

// GetEvent returns a single item change by ID.
func (db *DB) GetEvent(id int) (*models.ItemChange, error) {
	e := &models.ItemChange{}
	err := db.QueryRow(`SELECT id, item_type, item_id, type, created_time
		FROM item_changes WHERE id = ?`, id).Scan(&e.ID, &e.ItemType, &e.ItemID, &e.Type, &e.CreatedTime)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return e, nil
}
