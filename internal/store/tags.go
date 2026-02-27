package store

import (
	"database/sql"
	"time"

	"github.com/jescarri/go-joplin/internal/models"
)

// GetTag returns a tag by ID.
func (db *DB) GetTag(id string) (*models.Tag, error) {
	t := &models.Tag{}
	err := db.QueryRow(`SELECT id, title, created_time, updated_time,
		user_created_time, user_updated_time, encryption_cipher_text, encryption_applied,
		is_shared, parent_id, user_data
		FROM tags WHERE id = ?`, id).Scan(
		&t.ID, &t.Title, &t.CreatedTime, &t.UpdatedTime,
		&t.UserCreatedTime, &t.UserUpdatedTime, &t.EncryptionCipherText, &t.EncryptionApplied,
		&t.IsShared, &t.ParentID, &t.UserData,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.Type_ = models.TypeTag
	return t, nil
}

// ListTags returns all tags with pagination.
func (db *DB) ListTags(orderBy, orderDir string, limit, offset int) ([]*models.Tag, bool, error) {
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
		is_shared, parent_id, user_data
		FROM tags ORDER BY `+sanitizeColumn(orderBy)+` `+sanitizeDir(orderDir)+` LIMIT ? OFFSET ?`,
		limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var tags []*models.Tag
	for rows.Next() {
		t := &models.Tag{}
		if err := rows.Scan(
			&t.ID, &t.Title, &t.CreatedTime, &t.UpdatedTime,
			&t.UserCreatedTime, &t.UserUpdatedTime, &t.EncryptionCipherText, &t.EncryptionApplied,
			&t.IsShared, &t.ParentID, &t.UserData,
		); err != nil {
			return nil, false, err
		}
		t.Type_ = models.TypeTag
		tags = append(tags, t)
	}

	hasMore := len(tags) > limit
	if hasMore {
		tags = tags[:limit]
	}
	return tags, hasMore, nil
}

// CreateTag inserts a new tag.
func (db *DB) CreateTag(t *models.Tag) error {
	now := time.Now().UnixMilli()
	if t.ID == "" {
		t.ID = models.NewID()
	}
	if t.CreatedTime == 0 {
		t.CreatedTime = now
	}
	if t.UpdatedTime == 0 {
		t.UpdatedTime = now
	}
	if t.UserCreatedTime == 0 {
		t.UserCreatedTime = t.CreatedTime
	}
	if t.UserUpdatedTime == 0 {
		t.UserUpdatedTime = t.UpdatedTime
	}

	_, err := db.Exec(`INSERT INTO tags (id, title, created_time, updated_time,
		user_created_time, user_updated_time, encryption_cipher_text, encryption_applied,
		is_shared, parent_id, user_data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Title, t.CreatedTime, t.UpdatedTime,
		t.UserCreatedTime, t.UserUpdatedTime, t.EncryptionCipherText, t.EncryptionApplied,
		t.IsShared, t.ParentID, t.UserData,
	)
	if err != nil {
		return err
	}
	return db.recordItemChange(models.TypeTag, t.ID, 1)
}

// UpdateTag updates an existing tag.
func (db *DB) UpdateTag(t *models.Tag) error {
	t.UpdatedTime = time.Now().UnixMilli()
	t.UserUpdatedTime = t.UpdatedTime

	_, err := db.Exec(`UPDATE tags SET title=?, updated_time=?,
		user_updated_time=?, encryption_cipher_text=?, encryption_applied=?,
		is_shared=?, parent_id=?, user_data=?
		WHERE id = ?`,
		t.Title, t.UpdatedTime,
		t.UserUpdatedTime, t.EncryptionCipherText, t.EncryptionApplied,
		t.IsShared, t.ParentID, t.UserData,
		t.ID,
	)
	if err != nil {
		return err
	}
	return db.recordItemChange(models.TypeTag, t.ID, 2)
}

// UpsertTag inserts or replaces a tag (used during sync).
func (db *DB) UpsertTag(t *models.Tag) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO tags (id, title, created_time, updated_time,
		user_created_time, user_updated_time, encryption_cipher_text, encryption_applied,
		is_shared, parent_id, user_data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Title, t.CreatedTime, t.UpdatedTime,
		t.UserCreatedTime, t.UserUpdatedTime, t.EncryptionCipherText, t.EncryptionApplied,
		t.IsShared, t.ParentID, t.UserData,
	)
	return err
}

// DeleteTag removes a tag and its note associations.
func (db *DB) DeleteTag(id string) error {
	if _, err := db.Exec("DELETE FROM note_tags WHERE tag_id = ?", id); err != nil {
		return err
	}
	if _, err := db.Exec("DELETE FROM tags WHERE id = ?", id); err != nil {
		return err
	}
	return db.recordItemChange(models.TypeTag, id, 3)
}

// GetNoteTagsByNote returns tags for a given note.
func (db *DB) GetNoteTagsByNote(noteID string) ([]*models.Tag, error) {
	rows, err := db.Query(`SELECT t.id, t.title, t.created_time, t.updated_time,
		t.user_created_time, t.user_updated_time, t.encryption_cipher_text, t.encryption_applied,
		t.is_shared, t.parent_id, t.user_data
		FROM tags t JOIN note_tags nt ON t.id = nt.tag_id WHERE nt.note_id = ?`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []*models.Tag
	for rows.Next() {
		t := &models.Tag{}
		if err := rows.Scan(
			&t.ID, &t.Title, &t.CreatedTime, &t.UpdatedTime,
			&t.UserCreatedTime, &t.UserUpdatedTime, &t.EncryptionCipherText, &t.EncryptionApplied,
			&t.IsShared, &t.ParentID, &t.UserData,
		); err != nil {
			return nil, err
		}
		t.Type_ = models.TypeTag
		tags = append(tags, t)
	}
	return tags, nil
}

// GetNotesByTag returns notes for a given tag.
func (db *DB) GetNotesByTag(tagID string, limit, offset int) ([]*models.Note, bool, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := db.Query(`SELECT n.id, n.parent_id, n.title, n.body, n.created_time, n.updated_time,
		n.is_conflict, n.latitude, n.longitude, n.altitude, n.author, n.source_url,
		n.is_todo, n.todo_due, n.todo_completed, n.source, n.source_application, n.application_data,
		n."order", n.user_created_time, n.user_updated_time, n.encryption_cipher_text,
		n.encryption_applied, n.markup_language, n.is_shared, n.share_id, n.conflict_original_id,
		n.master_key_id, n.user_data, n.deleted_time
		FROM notes n JOIN note_tags nt ON n.id = nt.note_id WHERE nt.tag_id = ?
		ORDER BY n.updated_time DESC LIMIT ? OFFSET ?`, tagID, limit+1, offset)
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

// AddNoteTag creates a note-tag association.
func (db *DB) AddNoteTag(noteID, tagID string) error {
	// Check if already exists
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM note_tags WHERE note_id = ? AND tag_id = ?", noteID, tagID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	now := time.Now().UnixMilli()
	nt := &models.NoteTag{
		ID:              models.NewID(),
		NoteID:          noteID,
		TagID:           tagID,
		CreatedTime:     now,
		UpdatedTime:     now,
		UserCreatedTime: now,
		UserUpdatedTime: now,
	}

	_, err := db.Exec(`INSERT INTO note_tags (id, note_id, tag_id, created_time, updated_time,
		user_created_time, user_updated_time, encryption_cipher_text, encryption_applied, is_shared)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nt.ID, nt.NoteID, nt.TagID, nt.CreatedTime, nt.UpdatedTime,
		nt.UserCreatedTime, nt.UserUpdatedTime, nt.EncryptionCipherText, nt.EncryptionApplied, nt.IsShared,
	)
	if err != nil {
		return err
	}
	return db.recordItemChange(models.TypeNoteTag, nt.ID, 1)
}

// RemoveNoteTag removes a note-tag association.
func (db *DB) RemoveNoteTag(noteID, tagID string) error {
	var ntID string
	err := db.QueryRow("SELECT id FROM note_tags WHERE note_id = ? AND tag_id = ?", noteID, tagID).Scan(&ntID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}

	if _, err := db.Exec("DELETE FROM note_tags WHERE id = ?", ntID); err != nil {
		return err
	}
	return db.recordItemChange(models.TypeNoteTag, ntID, 3)
}

// UpsertNoteTag inserts or replaces a note-tag (used during sync).
func (db *DB) UpsertNoteTag(nt *models.NoteTag) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO note_tags (id, note_id, tag_id, created_time, updated_time,
		user_created_time, user_updated_time, encryption_cipher_text, encryption_applied, is_shared)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nt.ID, nt.NoteID, nt.TagID, nt.CreatedTime, nt.UpdatedTime,
		nt.UserCreatedTime, nt.UserUpdatedTime, nt.EncryptionCipherText, nt.EncryptionApplied, nt.IsShared,
	)
	return err
}
