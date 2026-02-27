package store

import (
	"database/sql"

	"github.com/jescarri/go-joplin/internal/models"
)

// UpsertMasterKey inserts or updates a master key.
func (db *DB) UpsertMasterKey(mk *models.MasterKey) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO master_keys
		(id, created_time, updated_time, source_application, encryption_method, checksum, content)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		mk.ID, mk.CreatedTime, mk.UpdatedTime, mk.SourceApplication,
		mk.EncryptionMethod, mk.Checksum, mk.Content)
	return err
}

// GetMasterKey returns a master key by ID.
func (db *DB) GetMasterKey(id string) (*models.MasterKey, error) {
	mk := &models.MasterKey{}
	err := db.QueryRow(`SELECT id, created_time, updated_time, source_application,
		encryption_method, checksum, content FROM master_keys WHERE id = ?`, id).Scan(
		&mk.ID, &mk.CreatedTime, &mk.UpdatedTime, &mk.SourceApplication,
		&mk.EncryptionMethod, &mk.Checksum, &mk.Content)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return mk, nil
}

// ListMasterKeys returns all master keys.
func (db *DB) ListMasterKeys() ([]*models.MasterKey, error) {
	rows, err := db.Query(`SELECT id, created_time, updated_time, source_application,
		encryption_method, checksum, content FROM master_keys`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*models.MasterKey
	for rows.Next() {
		mk := &models.MasterKey{}
		if err := rows.Scan(&mk.ID, &mk.CreatedTime, &mk.UpdatedTime, &mk.SourceApplication,
			&mk.EncryptionMethod, &mk.Checksum, &mk.Content); err != nil {
			return nil, err
		}
		keys = append(keys, mk)
	}
	return keys, nil
}

// EncryptedItem holds the ID, type, and cipher text of an encrypted item.
type EncryptedItem struct {
	ID         string
	ItemType   int
	CipherText string
}

// GetEncryptedItems returns all items that still have encryption_applied = 1.
func (db *DB) GetEncryptedItems() ([]EncryptedItem, error) {
	var items []EncryptedItem

	// Notes
	rows, err := db.Query(`SELECT id, encryption_cipher_text FROM notes WHERE encryption_applied = 1 AND encryption_cipher_text != ''`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var ei EncryptedItem
		ei.ItemType = models.TypeNote
		if err := rows.Scan(&ei.ID, &ei.CipherText); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, ei)
	}
	rows.Close()

	// Folders
	rows, err = db.Query(`SELECT id, encryption_cipher_text FROM folders WHERE encryption_applied = 1 AND encryption_cipher_text != ''`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var ei EncryptedItem
		ei.ItemType = models.TypeFolder
		if err := rows.Scan(&ei.ID, &ei.CipherText); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, ei)
	}
	rows.Close()

	// Tags
	rows, err = db.Query(`SELECT id, encryption_cipher_text FROM tags WHERE encryption_applied = 1 AND encryption_cipher_text != ''`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var ei EncryptedItem
		ei.ItemType = models.TypeTag
		if err := rows.Scan(&ei.ID, &ei.CipherText); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, ei)
	}
	rows.Close()

	return items, nil
}

// GetEncryptedResources returns resources with encrypted blobs.
type EncryptedResource struct {
	ID string
}

// GetEncryptedResourceBlobs returns resource IDs that have encrypted blobs.
func (db *DB) GetEncryptedResourceBlobs() ([]EncryptedResource, error) {
	rows, err := db.Query(`SELECT id FROM resources WHERE encryption_blob_encrypted = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var resources []EncryptedResource
	for rows.Next() {
		var er EncryptedResource
		if err := rows.Scan(&er.ID); err != nil {
			return nil, err
		}
		resources = append(resources, er)
	}
	return resources, nil
}
