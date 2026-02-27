package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/jescarri/go-joplin/internal/models"
)

// GetNote returns a note by ID.
func (db *DB) GetNote(id string) (*models.Note, error) {
	n := &models.Note{}
	err := db.QueryRow(`SELECT id, parent_id, title, body, created_time, updated_time,
		is_conflict, latitude, longitude, altitude, author, source_url,
		is_todo, todo_due, todo_completed, source, source_application, application_data,
		"order", user_created_time, user_updated_time, encryption_cipher_text,
		encryption_applied, markup_language, is_shared, share_id, conflict_original_id,
		master_key_id, user_data, deleted_time
		FROM notes WHERE id = ?`, id).Scan(
		&n.ID, &n.ParentID, &n.Title, &n.Body, &n.CreatedTime, &n.UpdatedTime,
		&n.IsConflict, &n.Latitude, &n.Longitude, &n.Altitude, &n.Author, &n.SourceURL,
		&n.IsTodo, &n.TodoDue, &n.TodoCompleted, &n.Source, &n.SourceApplication, &n.ApplicationData,
		&n.Order, &n.UserCreatedTime, &n.UserUpdatedTime, &n.EncryptionCipherText,
		&n.EncryptionApplied, &n.MarkupLanguage, &n.IsShared, &n.ShareID, &n.ConflictOriginalID,
		&n.MasterKeyID, &n.UserData, &n.DeletedTime,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	n.Type_ = models.TypeNote
	return n, nil
}

// ListNotes returns notes with pagination.
func (db *DB) ListNotes(orderBy string, orderDir string, limit, offset int) ([]*models.Note, bool, error) {
	if orderBy == "" {
		orderBy = "updated_time"
	}
	if orderDir == "" {
		orderDir = "DESC"
	}
	if limit <= 0 {
		limit = 10
	}

	query := fmt.Sprintf(`SELECT id, parent_id, title, body, created_time, updated_time,
		is_conflict, latitude, longitude, altitude, author, source_url,
		is_todo, todo_due, todo_completed, source, source_application, application_data,
		"order", user_created_time, user_updated_time, encryption_cipher_text,
		encryption_applied, markup_language, is_shared, share_id, conflict_original_id,
		master_key_id, user_data, deleted_time
		FROM notes ORDER BY %s %s LIMIT ? OFFSET ?`, sanitizeColumn(orderBy), sanitizeDir(orderDir))

	rows, err := db.Query(query, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var notes []*models.Note
	for rows.Next() {
		n := &models.Note{}
		if err := rows.Scan(
			&n.ID, &n.ParentID, &n.Title, &n.Body, &n.CreatedTime, &n.UpdatedTime,
			&n.IsConflict, &n.Latitude, &n.Longitude, &n.Altitude, &n.Author, &n.SourceURL,
			&n.IsTodo, &n.TodoDue, &n.TodoCompleted, &n.Source, &n.SourceApplication, &n.ApplicationData,
			&n.Order, &n.UserCreatedTime, &n.UserUpdatedTime, &n.EncryptionCipherText,
			&n.EncryptionApplied, &n.MarkupLanguage, &n.IsShared, &n.ShareID, &n.ConflictOriginalID,
			&n.MasterKeyID, &n.UserData, &n.DeletedTime,
		); err != nil {
			return nil, false, err
		}
		n.Type_ = models.TypeNote
		notes = append(notes, n)
	}

	hasMore := len(notes) > limit
	if hasMore {
		notes = notes[:limit]
	}
	return notes, hasMore, nil
}

// ListNotesByFolder returns notes in a specific folder.
func (db *DB) ListNotesByFolder(folderID string, orderBy string, orderDir string, limit, offset int) ([]*models.Note, bool, error) {
	if orderBy == "" {
		orderBy = "updated_time"
	}
	if orderDir == "" {
		orderDir = "DESC"
	}
	if limit <= 0 {
		limit = 10
	}

	query := fmt.Sprintf(`SELECT id, parent_id, title, body, created_time, updated_time,
		is_conflict, latitude, longitude, altitude, author, source_url,
		is_todo, todo_due, todo_completed, source, source_application, application_data,
		"order", user_created_time, user_updated_time, encryption_cipher_text,
		encryption_applied, markup_language, is_shared, share_id, conflict_original_id,
		master_key_id, user_data, deleted_time
		FROM notes WHERE parent_id = ? ORDER BY %s %s LIMIT ? OFFSET ?`, sanitizeColumn(orderBy), sanitizeDir(orderDir))

	rows, err := db.Query(query, folderID, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var notes []*models.Note
	for rows.Next() {
		n := &models.Note{}
		if err := rows.Scan(
			&n.ID, &n.ParentID, &n.Title, &n.Body, &n.CreatedTime, &n.UpdatedTime,
			&n.IsConflict, &n.Latitude, &n.Longitude, &n.Altitude, &n.Author, &n.SourceURL,
			&n.IsTodo, &n.TodoDue, &n.TodoCompleted, &n.Source, &n.SourceApplication, &n.ApplicationData,
			&n.Order, &n.UserCreatedTime, &n.UserUpdatedTime, &n.EncryptionCipherText,
			&n.EncryptionApplied, &n.MarkupLanguage, &n.IsShared, &n.ShareID, &n.ConflictOriginalID,
			&n.MasterKeyID, &n.UserData, &n.DeletedTime,
		); err != nil {
			return nil, false, err
		}
		n.Type_ = models.TypeNote
		notes = append(notes, n)
	}

	hasMore := len(notes) > limit
	if hasMore {
		notes = notes[:limit]
	}
	return notes, hasMore, nil
}

// CreateNote inserts a new note.
func (db *DB) CreateNote(n *models.Note) error {
	now := time.Now().UnixMilli()
	if n.ID == "" {
		n.ID = models.NewID()
	}
	if n.CreatedTime == 0 {
		n.CreatedTime = now
	}
	if n.UpdatedTime == 0 {
		n.UpdatedTime = now
	}
	if n.UserCreatedTime == 0 {
		n.UserCreatedTime = n.CreatedTime
	}
	if n.UserUpdatedTime == 0 {
		n.UserUpdatedTime = n.UpdatedTime
	}
	if n.MarkupLanguage == 0 {
		n.MarkupLanguage = 1 // Markdown
	}

	_, err := db.Exec(`INSERT INTO notes (id, parent_id, title, body, created_time, updated_time,
		is_conflict, latitude, longitude, altitude, author, source_url,
		is_todo, todo_due, todo_completed, source, source_application, application_data,
		"order", user_created_time, user_updated_time, encryption_cipher_text,
		encryption_applied, markup_language, is_shared, share_id, conflict_original_id,
		master_key_id, user_data, deleted_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.ParentID, n.Title, n.Body, n.CreatedTime, n.UpdatedTime,
		n.IsConflict, n.Latitude, n.Longitude, n.Altitude, n.Author, n.SourceURL,
		n.IsTodo, n.TodoDue, n.TodoCompleted, n.Source, n.SourceApplication, n.ApplicationData,
		n.Order, n.UserCreatedTime, n.UserUpdatedTime, n.EncryptionCipherText,
		n.EncryptionApplied, n.MarkupLanguage, n.IsShared, n.ShareID, n.ConflictOriginalID,
		n.MasterKeyID, n.UserData, n.DeletedTime,
	)
	if err != nil {
		return err
	}

	return db.recordItemChange(models.TypeNote, n.ID, 1)
}

// UpdateNote updates an existing note.
func (db *DB) UpdateNote(n *models.Note) error {
	n.UpdatedTime = time.Now().UnixMilli()
	n.UserUpdatedTime = n.UpdatedTime

	_, err := db.Exec(`UPDATE notes SET parent_id=?, title=?, body=?, updated_time=?,
		is_conflict=?, latitude=?, longitude=?, altitude=?, author=?, source_url=?,
		is_todo=?, todo_due=?, todo_completed=?, source=?, source_application=?, application_data=?,
		"order"=?, user_updated_time=?, encryption_cipher_text=?,
		encryption_applied=?, markup_language=?, is_shared=?, share_id=?, conflict_original_id=?,
		master_key_id=?, user_data=?, deleted_time=?
		WHERE id = ?`,
		n.ParentID, n.Title, n.Body, n.UpdatedTime,
		n.IsConflict, n.Latitude, n.Longitude, n.Altitude, n.Author, n.SourceURL,
		n.IsTodo, n.TodoDue, n.TodoCompleted, n.Source, n.SourceApplication, n.ApplicationData,
		n.Order, n.UserUpdatedTime, n.EncryptionCipherText,
		n.EncryptionApplied, n.MarkupLanguage, n.IsShared, n.ShareID, n.ConflictOriginalID,
		n.MasterKeyID, n.UserData, n.DeletedTime,
		n.ID,
	)
	if err != nil {
		return err
	}

	return db.recordItemChange(models.TypeNote, n.ID, 2)
}

// UpsertNote inserts or replaces a note (used during sync).
func (db *DB) UpsertNote(n *models.Note) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO notes (id, parent_id, title, body, created_time, updated_time,
		is_conflict, latitude, longitude, altitude, author, source_url,
		is_todo, todo_due, todo_completed, source, source_application, application_data,
		"order", user_created_time, user_updated_time, encryption_cipher_text,
		encryption_applied, markup_language, is_shared, share_id, conflict_original_id,
		master_key_id, user_data, deleted_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.ParentID, n.Title, n.Body, n.CreatedTime, n.UpdatedTime,
		n.IsConflict, n.Latitude, n.Longitude, n.Altitude, n.Author, n.SourceURL,
		n.IsTodo, n.TodoDue, n.TodoCompleted, n.Source, n.SourceApplication, n.ApplicationData,
		n.Order, n.UserCreatedTime, n.UserUpdatedTime, n.EncryptionCipherText,
		n.EncryptionApplied, n.MarkupLanguage, n.IsShared, n.ShareID, n.ConflictOriginalID,
		n.MasterKeyID, n.UserData, n.DeletedTime,
	)
	return err
}

// DeleteNote removes a note by ID.
func (db *DB) DeleteNote(id string) error {
	if _, err := db.Exec("DELETE FROM notes WHERE id = ?", id); err != nil {
		return err
	}
	// Also remove note-tag associations
	if _, err := db.Exec("DELETE FROM note_tags WHERE note_id = ?", id); err != nil {
		return err
	}
	return db.recordItemChange(models.TypeNote, id, 3)
}

func sanitizeColumn(col string) string {
	allowed := map[string]string{
		"updated_time":      "updated_time",
		"created_time":      "created_time",
		"title":             "title",
		"order":             `"order"`,
		"user_updated_time": "user_updated_time",
		"user_created_time": "user_created_time",
		"is_todo":           "is_todo",
		"todo_due":          "todo_due",
		"todo_completed":    "todo_completed",
	}
	if v, ok := allowed[col]; ok {
		return v
	}
	return "updated_time"
}

func sanitizeDir(dir string) string {
	if dir == "ASC" || dir == "asc" {
		return "ASC"
	}
	return "DESC"
}
