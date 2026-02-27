package store

import (
	"database/sql"
	"time"

	"github.com/jescarri/go-joplin/internal/models"
)

// GetFolder returns a folder by ID.
func (db *DB) GetFolder(id string) (*models.Folder, error) {
	f := &models.Folder{}
	err := db.QueryRow(`SELECT id, title, created_time, updated_time,
		user_created_time, user_updated_time, encryption_cipher_text, encryption_applied,
		parent_id, is_shared, share_id, master_key_id, icon, user_data, deleted_time
		FROM folders WHERE id = ?`, id).Scan(
		&f.ID, &f.Title, &f.CreatedTime, &f.UpdatedTime,
		&f.UserCreatedTime, &f.UserUpdatedTime, &f.EncryptionCipherText, &f.EncryptionApplied,
		&f.ParentID, &f.IsShared, &f.ShareID, &f.MasterKeyID, &f.Icon, &f.UserData, &f.DeletedTime,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	f.Type_ = models.TypeFolder
	return f, nil
}

// ListFolders returns all folders.
func (db *DB) ListFolders(orderBy, orderDir string, limit, offset int) ([]*models.Folder, bool, error) {
	if orderBy == "" {
		orderBy = "title"
	}
	if orderDir == "" {
		orderDir = "ASC"
	}
	if limit <= 0 {
		limit = 100
	}

	rows, err := db.Query(`SELECT id, title, created_time, updated_time,
		user_created_time, user_updated_time, encryption_cipher_text, encryption_applied,
		parent_id, is_shared, share_id, master_key_id, icon, user_data, deleted_time
		FROM folders ORDER BY `+sanitizeColumn(orderBy)+` `+sanitizeDir(orderDir)+` LIMIT ? OFFSET ?`,
		limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var folders []*models.Folder
	for rows.Next() {
		f := &models.Folder{}
		if err := rows.Scan(
			&f.ID, &f.Title, &f.CreatedTime, &f.UpdatedTime,
			&f.UserCreatedTime, &f.UserUpdatedTime, &f.EncryptionCipherText, &f.EncryptionApplied,
			&f.ParentID, &f.IsShared, &f.ShareID, &f.MasterKeyID, &f.Icon, &f.UserData, &f.DeletedTime,
		); err != nil {
			return nil, false, err
		}
		f.Type_ = models.TypeFolder
		folders = append(folders, f)
	}

	hasMore := len(folders) > limit
	if hasMore {
		folders = folders[:limit]
	}
	return folders, hasMore, nil
}

// FolderTree returns folders organized as a tree.
func (db *DB) FolderTree() ([]*models.Folder, error) {
	folders, _, err := db.ListFolders("title", "ASC", 10000, 0)
	if err != nil {
		return nil, err
	}

	// Count notes per folder
	rows, err := db.Query("SELECT parent_id, COUNT(*) FROM notes GROUP BY parent_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var pid string
		var count int
		if err := rows.Scan(&pid, &count); err != nil {
			return nil, err
		}
		counts[pid] = count
	}

	byID := make(map[string]*models.Folder)
	for _, f := range folders {
		f.NoteCount = counts[f.ID]
		f.Children = []*models.Folder{}
		byID[f.ID] = f
	}

	var roots []*models.Folder
	for _, f := range folders {
		if f.ParentID == "" {
			roots = append(roots, f)
		} else if parent, ok := byID[f.ParentID]; ok {
			parent.Children = append(parent.Children, f)
		} else {
			roots = append(roots, f)
		}
	}

	return roots, nil
}

// CreateFolder inserts a new folder.
func (db *DB) CreateFolder(f *models.Folder) error {
	now := time.Now().UnixMilli()
	if f.ID == "" {
		f.ID = models.NewID()
	}
	if f.CreatedTime == 0 {
		f.CreatedTime = now
	}
	if f.UpdatedTime == 0 {
		f.UpdatedTime = now
	}
	if f.UserCreatedTime == 0 {
		f.UserCreatedTime = f.CreatedTime
	}
	if f.UserUpdatedTime == 0 {
		f.UserUpdatedTime = f.UpdatedTime
	}

	_, err := db.Exec(`INSERT INTO folders (id, title, created_time, updated_time,
		user_created_time, user_updated_time, encryption_cipher_text, encryption_applied,
		parent_id, is_shared, share_id, master_key_id, icon, user_data, deleted_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.Title, f.CreatedTime, f.UpdatedTime,
		f.UserCreatedTime, f.UserUpdatedTime, f.EncryptionCipherText, f.EncryptionApplied,
		f.ParentID, f.IsShared, f.ShareID, f.MasterKeyID, f.Icon, f.UserData, f.DeletedTime,
	)
	if err != nil {
		return err
	}
	return db.recordItemChange(models.TypeFolder, f.ID, 1)
}

// UpdateFolder updates an existing folder.
func (db *DB) UpdateFolder(f *models.Folder) error {
	f.UpdatedTime = time.Now().UnixMilli()
	f.UserUpdatedTime = f.UpdatedTime

	_, err := db.Exec(`UPDATE folders SET title=?, updated_time=?,
		user_updated_time=?, encryption_cipher_text=?, encryption_applied=?,
		parent_id=?, is_shared=?, share_id=?, master_key_id=?, icon=?, user_data=?, deleted_time=?
		WHERE id = ?`,
		f.Title, f.UpdatedTime,
		f.UserUpdatedTime, f.EncryptionCipherText, f.EncryptionApplied,
		f.ParentID, f.IsShared, f.ShareID, f.MasterKeyID, f.Icon, f.UserData, f.DeletedTime,
		f.ID,
	)
	if err != nil {
		return err
	}
	return db.recordItemChange(models.TypeFolder, f.ID, 2)
}

// UpsertFolder inserts or replaces a folder (used during sync).
func (db *DB) UpsertFolder(f *models.Folder) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO folders (id, title, created_time, updated_time,
		user_created_time, user_updated_time, encryption_cipher_text, encryption_applied,
		parent_id, is_shared, share_id, master_key_id, icon, user_data, deleted_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.Title, f.CreatedTime, f.UpdatedTime,
		f.UserCreatedTime, f.UserUpdatedTime, f.EncryptionCipherText, f.EncryptionApplied,
		f.ParentID, f.IsShared, f.ShareID, f.MasterKeyID, f.Icon, f.UserData, f.DeletedTime,
	)
	return err
}

// DeleteFolder removes a folder by ID.
func (db *DB) DeleteFolder(id string) error {
	if _, err := db.Exec("DELETE FROM folders WHERE id = ?", id); err != nil {
		return err
	}
	return db.recordItemChange(models.TypeFolder, id, 3)
}
