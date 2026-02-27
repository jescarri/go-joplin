package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	"github.com/jescarri/go-joplin/internal/models"
)

// GetResource returns a resource by ID.
func (db *DB) GetResource(id string) (*models.Resource, error) {
	r := &models.Resource{}
	err := db.QueryRow(`SELECT id, title, mime, filename, created_time, updated_time,
		user_created_time, user_updated_time, file_extension, encryption_cipher_text,
		encryption_applied, encryption_blob_encrypted, size, is_shared, share_id,
		master_key_id, user_data, blob_updated_time
		FROM resources WHERE id = ?`, id).Scan(
		&r.ID, &r.Title, &r.Mime, &r.Filename, &r.CreatedTime, &r.UpdatedTime,
		&r.UserCreatedTime, &r.UserUpdatedTime, &r.FileExtension, &r.EncryptionCipherText,
		&r.EncryptionApplied, &r.EncryptionBlobEncrypted, &r.Size, &r.IsShared, &r.ShareID,
		&r.MasterKeyID, &r.UserData, &r.BlobUpdatedTime,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Type_ = models.TypeResource
	return r, nil
}

// ListResources returns resources with pagination.
func (db *DB) ListResources(orderBy, orderDir string, limit, offset int) ([]*models.Resource, bool, error) {
	if orderBy == "" {
		orderBy = "updated_time"
	}
	if orderDir == "" {
		orderDir = "DESC"
	}
	if limit <= 0 {
		limit = 10
	}

	rows, err := db.Query(`SELECT id, title, mime, filename, created_time, updated_time,
		user_created_time, user_updated_time, file_extension, encryption_cipher_text,
		encryption_applied, encryption_blob_encrypted, size, is_shared, share_id,
		master_key_id, user_data, blob_updated_time
		FROM resources ORDER BY `+sanitizeColumn(orderBy)+` `+sanitizeDir(orderDir)+` LIMIT ? OFFSET ?`,
		limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var resources []*models.Resource
	for rows.Next() {
		r := &models.Resource{}
		if err := rows.Scan(
			&r.ID, &r.Title, &r.Mime, &r.Filename, &r.CreatedTime, &r.UpdatedTime,
			&r.UserCreatedTime, &r.UserUpdatedTime, &r.FileExtension, &r.EncryptionCipherText,
			&r.EncryptionApplied, &r.EncryptionBlobEncrypted, &r.Size, &r.IsShared, &r.ShareID,
			&r.MasterKeyID, &r.UserData, &r.BlobUpdatedTime,
		); err != nil {
			return nil, false, err
		}
		r.Type_ = models.TypeResource
		resources = append(resources, r)
	}

	hasMore := len(resources) > limit
	if hasMore {
		resources = resources[:limit]
	}
	return resources, hasMore, nil
}

// GetResourcesByNote returns resources referenced by a note.
func (db *DB) GetResourcesByNote(noteID string) ([]*models.Resource, error) {
	// Get note body and find resource references
	var body string
	err := db.QueryRow("SELECT body FROM notes WHERE id = ?", noteID).Scan(&body)
	if err != nil {
		return nil, err
	}

	// Find all resource IDs referenced in the note (pattern: :/resource_id)
	var resources []*models.Resource
	// Simple approach: find all 32-char hex IDs after ":/"
	for i := 0; i < len(body)-33; i++ {
		if body[i] == ':' && body[i+1] == '/' {
			candidate := body[i+2 : i+34]
			if isHexID(candidate) {
				r, err := db.GetResource(candidate)
				if err != nil {
					return nil, err
				}
				if r != nil {
					resources = append(resources, r)
				}
			}
		}
	}
	return resources, nil
}

func isHexID(s string) bool {
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

// CreateResource inserts a new resource.
func (db *DB) CreateResource(r *models.Resource) error {
	now := time.Now().UnixMilli()
	if r.ID == "" {
		r.ID = models.NewID()
	}
	if r.CreatedTime == 0 {
		r.CreatedTime = now
	}
	if r.UpdatedTime == 0 {
		r.UpdatedTime = now
	}
	if r.UserCreatedTime == 0 {
		r.UserCreatedTime = r.CreatedTime
	}
	if r.UserUpdatedTime == 0 {
		r.UserUpdatedTime = r.UpdatedTime
	}

	_, err := db.Exec(`INSERT INTO resources (id, title, mime, filename, created_time, updated_time,
		user_created_time, user_updated_time, file_extension, encryption_cipher_text,
		encryption_applied, encryption_blob_encrypted, size, is_shared, share_id,
		master_key_id, user_data, blob_updated_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Title, r.Mime, r.Filename, r.CreatedTime, r.UpdatedTime,
		r.UserCreatedTime, r.UserUpdatedTime, r.FileExtension, r.EncryptionCipherText,
		r.EncryptionApplied, r.EncryptionBlobEncrypted, r.Size, r.IsShared, r.ShareID,
		r.MasterKeyID, r.UserData, r.BlobUpdatedTime,
	)
	if err != nil {
		return err
	}
	return db.recordItemChange(models.TypeResource, r.ID, 1)
}

// UpsertResource inserts or replaces a resource (used during sync).
func (db *DB) UpsertResource(r *models.Resource) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO resources (id, title, mime, filename, created_time, updated_time,
		user_created_time, user_updated_time, file_extension, encryption_cipher_text,
		encryption_applied, encryption_blob_encrypted, size, is_shared, share_id,
		master_key_id, user_data, blob_updated_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Title, r.Mime, r.Filename, r.CreatedTime, r.UpdatedTime,
		r.UserCreatedTime, r.UserUpdatedTime, r.FileExtension, r.EncryptionCipherText,
		r.EncryptionApplied, r.EncryptionBlobEncrypted, r.Size, r.IsShared, r.ShareID,
		r.MasterKeyID, r.UserData, r.BlobUpdatedTime,
	)
	return err
}

// DeleteResource removes a resource and its file.
func (db *DB) DeleteResource(id string) error {
	// Remove file
	filePath := filepath.Join(db.ResourceDir(), id)
	os.Remove(filePath)

	if _, err := db.Exec("DELETE FROM resources WHERE id = ?", id); err != nil {
		return err
	}
	return db.recordItemChange(models.TypeResource, id, 3)
}

// SaveResourceFile saves a resource's binary content to disk.
func (db *DB) SaveResourceFile(id string, data []byte) error {
	filePath := filepath.Join(db.ResourceDir(), id)
	return os.WriteFile(filePath, data, 0o644)
}

// GetResourceFile returns the path to a resource's binary file.
func (db *DB) GetResourceFile(id string) string {
	return filepath.Join(db.ResourceDir(), id)
}
