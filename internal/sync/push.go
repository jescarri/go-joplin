package sync

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"

	"github.com/jescarri/go-joplin/internal/e2ee"
	"github.com/jescarri/go-joplin/internal/models"
	"github.com/jescarri/go-joplin/internal/store"
)

// PushChanges uploads local changes to the sync target (Joplin Server or S3).
// If e2eeSvc is non-nil and an active master key is set and loaded, unencrypted items are encrypted before push.
func PushChanges(backend SyncBackend, db *store.DB, e2eeSvc *e2ee.Service) error {
	syncTarget := backend.SyncTarget()
	activeMasterKeyID, _ := db.GetActiveMasterKeyID()
	encryptBeforePush := e2eeSvc != nil && activeMasterKeyID != "" && e2eeSvc.HasMasterKey(activeMasterKeyID)

	// Push changed notes
	if err := pushNotes(backend, db, syncTarget, encryptBeforePush, e2eeSvc, activeMasterKeyID); err != nil {
		return err
	}

	// Push changed folders
	if err := pushFolders(backend, db, syncTarget, encryptBeforePush, e2eeSvc, activeMasterKeyID); err != nil {
		return err
	}

	// Push changed tags
	if err := pushTags(backend, db, syncTarget, encryptBeforePush, e2eeSvc, activeMasterKeyID); err != nil {
		return err
	}

	// Push changed note-tags
	if err := pushNoteTags(backend, db, syncTarget, encryptBeforePush, e2eeSvc, activeMasterKeyID); err != nil {
		return err
	}

	// Push changed resources
	if err := pushResources(backend, db, syncTarget); err != nil {
		return err
	}

	// Push deletes
	if err := pushDeletes(backend, db, syncTarget); err != nil {
		return err
	}

	return nil
}

func pushNotes(backend SyncBackend, db *store.DB, syncTarget int, encryptBeforePush bool, e2eeSvc *e2ee.Service, activeMasterKeyID string) error {
	// Find notes that have been modified locally but not synced
	rows, err := db.Query(`SELECT n.id FROM notes n
		LEFT JOIN sync_items si ON si.item_id = n.id AND si.sync_target = ?
		WHERE si.id IS NULL OR n.updated_time > si.sync_time`, syncTarget)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}

	for _, id := range ids {
		note, err := db.GetNote(id)
		if err != nil {
			return err
		}
		if note == nil {
			continue
		}

		content := serializeNoteForSync(note, encryptBeforePush, e2eeSvc, activeMasterKeyID)
		if content == nil {
			continue
		}
		itemName := id + ".md"
		path := fmt.Sprintf("/api/items/root:/%s:/content", url.PathEscape(itemName))

		if err := backend.Put(path, content); err != nil {
			slog.Error("failed to push note", "id", id, "error", err)
			continue
		}

		if err := db.UpsertSyncItem(id, models.TypeNote, syncTarget); err != nil {
			return err
		}
		slog.Debug("pushed note", "id", id, "title", note.Title)
	}
	return nil
}

func serializeNoteForSync(note *models.Note, encrypt bool, e2eeSvc *e2ee.Service, masterKeyID string) []byte {
	if encrypt && note.EncryptionApplied == 0 && e2eeSvc != nil && masterKeyID != "" {
		plain := models.SerializeNote(note)
		cipherText, err := e2eeSvc.EncryptString(masterKeyID, plain)
		if err != nil {
			slog.Error("failed to encrypt note for sync", "id", note.ID, "error", err)
			return nil
		}
		meta := map[string]string{
			"id":                     note.ID,
			"parent_id":              note.ParentID,
			"updated_time":           models.FmtTimeForSync(note.UpdatedTime),
			"deleted_time":           models.FmtTimeForSync(note.DeletedTime),
			"type_":                  strconv.Itoa(models.TypeNote),
			"encryption_applied":     "1",
			"encryption_cipher_text": cipherText,
			"master_key_id":          masterKeyID,
		}
		return []byte(models.SerializeEncryptedEnvelope(meta))
	}
	return []byte(models.SerializeNote(note))
}

func pushFolders(backend SyncBackend, db *store.DB, syncTarget int, encryptBeforePush bool, e2eeSvc *e2ee.Service, activeMasterKeyID string) error {
	rows, err := db.Query(`SELECT f.id FROM folders f
		LEFT JOIN sync_items si ON si.item_id = f.id AND si.sync_target = ?
		WHERE si.id IS NULL OR f.updated_time > si.sync_time`, syncTarget)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}

	for _, id := range ids {
		folder, err := db.GetFolder(id)
		if err != nil {
			return err
		}
		if folder == nil {
			continue
		}

		content := serializeFolderForSync(folder, encryptBeforePush, e2eeSvc, activeMasterKeyID)
		if content == nil {
			continue
		}
		itemName := id + ".md"
		path := fmt.Sprintf("/api/items/root:/%s:/content", url.PathEscape(itemName))

		if err := backend.Put(path, content); err != nil {
			slog.Error("failed to push folder", "id", id, "error", err)
			continue
		}

		if err := db.UpsertSyncItem(id, models.TypeFolder, syncTarget); err != nil {
			return err
		}
		slog.Debug("pushed folder", "id", id, "title", folder.Title)
	}
	return nil
}

func serializeFolderForSync(folder *models.Folder, encrypt bool, e2eeSvc *e2ee.Service, masterKeyID string) []byte {
	if encrypt && folder.EncryptionApplied == 0 && e2eeSvc != nil && masterKeyID != "" {
		plain := models.SerializeFolder(folder)
		cipherText, err := e2eeSvc.EncryptString(masterKeyID, plain)
		if err != nil {
			slog.Error("failed to encrypt folder for sync", "id", folder.ID, "error", err)
			return nil
		}
		meta := map[string]string{
			"id":                     folder.ID,
			"parent_id":              folder.ParentID,
			"updated_time":           models.FmtTimeForSync(folder.UpdatedTime),
			"deleted_time":           models.FmtTimeForSync(folder.DeletedTime),
			"type_":                  strconv.Itoa(models.TypeFolder),
			"encryption_applied":     "1",
			"encryption_cipher_text": cipherText,
			"master_key_id":          masterKeyID,
		}
		return []byte(models.SerializeEncryptedEnvelope(meta))
	}
	return []byte(models.SerializeFolder(folder))
}

func pushTags(backend SyncBackend, db *store.DB, syncTarget int, encryptBeforePush bool, e2eeSvc *e2ee.Service, activeMasterKeyID string) error {
	rows, err := db.Query(`SELECT t.id FROM tags t
		LEFT JOIN sync_items si ON si.item_id = t.id AND si.sync_target = ?
		WHERE si.id IS NULL OR t.updated_time > si.sync_time`, syncTarget)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}

	for _, id := range ids {
		tag, err := db.GetTag(id)
		if err != nil {
			return err
		}
		if tag == nil {
			continue
		}

		content := serializeTagForSync(tag, encryptBeforePush, e2eeSvc, activeMasterKeyID)
		if content == nil {
			continue
		}
		itemName := id + ".md"
		path := fmt.Sprintf("/api/items/root:/%s:/content", url.PathEscape(itemName))

		if err := backend.Put(path, content); err != nil {
			slog.Error("failed to push tag", "id", id, "error", err)
			continue
		}

		if err := db.UpsertSyncItem(id, models.TypeTag, syncTarget); err != nil {
			return err
		}
		slog.Debug("pushed tag", "id", id, "title", tag.Title)
	}
	return nil
}

func serializeTagForSync(tag *models.Tag, encrypt bool, e2eeSvc *e2ee.Service, masterKeyID string) []byte {
	if encrypt && tag.EncryptionApplied == 0 && e2eeSvc != nil && masterKeyID != "" {
		plain := models.SerializeTag(tag)
		cipherText, err := e2eeSvc.EncryptString(masterKeyID, plain)
		if err != nil {
			slog.Error("failed to encrypt tag for sync", "id", tag.ID, "error", err)
			return nil
		}
		meta := map[string]string{
			"id":                     tag.ID,
			"updated_time":           models.FmtTimeForSync(tag.UpdatedTime),
			"type_":                  strconv.Itoa(models.TypeTag),
			"encryption_applied":     "1",
			"encryption_cipher_text": cipherText,
			"master_key_id":          masterKeyID,
		}
		return []byte(models.SerializeEncryptedEnvelope(meta))
	}
	return []byte(models.SerializeTag(tag))
}

func pushNoteTags(backend SyncBackend, db *store.DB, syncTarget int, encryptBeforePush bool, e2eeSvc *e2ee.Service, activeMasterKeyID string) error {
	rows, err := db.Query(`SELECT nt.id, nt.note_id, nt.tag_id, nt.created_time, nt.updated_time,
		nt.user_created_time, nt.user_updated_time, nt.encryption_cipher_text,
		nt.encryption_applied, nt.is_shared
		FROM note_tags nt
		LEFT JOIN sync_items si ON si.item_id = nt.id AND si.sync_target = ?
		WHERE si.id IS NULL OR nt.updated_time > si.sync_time`, syncTarget)
	if err != nil {
		return err
	}
	defer rows.Close()

	var noteTags []*models.NoteTag
	for rows.Next() {
		nt := &models.NoteTag{}
		if err := rows.Scan(&nt.ID, &nt.NoteID, &nt.TagID, &nt.CreatedTime, &nt.UpdatedTime,
			&nt.UserCreatedTime, &nt.UserUpdatedTime, &nt.EncryptionCipherText,
			&nt.EncryptionApplied, &nt.IsShared); err != nil {
			return err
		}
		noteTags = append(noteTags, nt)
	}

	for _, nt := range noteTags {
		content := serializeNoteTagForSync(nt, encryptBeforePush, e2eeSvc, activeMasterKeyID)
		if content == nil {
			continue
		}
		itemName := nt.ID + ".md"
		path := fmt.Sprintf("/api/items/root:/%s:/content", url.PathEscape(itemName))

		if err := backend.Put(path, content); err != nil {
			slog.Error("failed to push note_tag", "id", nt.ID, "error", err)
			continue
		}

		if err := db.UpsertSyncItem(nt.ID, models.TypeNoteTag, syncTarget); err != nil {
			return err
		}
		slog.Debug("pushed note_tag", "id", nt.ID)
	}
	return nil
}

func serializeNoteTagForSync(nt *models.NoteTag, encrypt bool, e2eeSvc *e2ee.Service, masterKeyID string) []byte {
	if encrypt && nt.EncryptionApplied == 0 && e2eeSvc != nil && masterKeyID != "" {
		plain := models.SerializeNoteTag(nt)
		cipherText, err := e2eeSvc.EncryptString(masterKeyID, plain)
		if err != nil {
			slog.Error("failed to encrypt note_tag for sync", "id", nt.ID, "error", err)
			return nil
		}
		meta := map[string]string{
			"id":                     nt.ID,
			"note_id":                nt.NoteID,
			"tag_id":                 nt.TagID,
			"updated_time":           models.FmtTimeForSync(nt.UpdatedTime),
			"type_":                  strconv.Itoa(models.TypeNoteTag),
			"encryption_applied":     "1",
			"encryption_cipher_text": cipherText,
			"master_key_id":          masterKeyID,
		}
		return []byte(models.SerializeEncryptedEnvelope(meta))
	}
	return []byte(models.SerializeNoteTag(nt))
}

func pushResources(backend SyncBackend, db *store.DB, syncTarget int) error {
	rows, err := db.Query(`SELECT r.id FROM resources r
		LEFT JOIN sync_items si ON si.item_id = r.id AND si.sync_target = ?
		WHERE si.id IS NULL OR r.updated_time > si.sync_time`, syncTarget)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}

	for _, id := range ids {
		resource, err := db.GetResource(id)
		if err != nil {
			return err
		}
		if resource == nil {
			continue
		}

		// Push metadata
		content := models.SerializeResource(resource)
		itemName := id + ".md"
		path := fmt.Sprintf("/api/items/root:/%s:/content", url.PathEscape(itemName))

		if err := backend.Put(path, []byte(content)); err != nil {
			slog.Error("failed to push resource metadata", "id", id, "error", err)
			continue
		}

		// Push blob if exists
		blobPath := db.GetResourceFile(id)
		if blobData, err := os.ReadFile(blobPath); err == nil {
			blobItemName := ".resource/" + id
			blobAPIPath := fmt.Sprintf("/api/items/root:/%s:/content", url.PathEscape(blobItemName))
			if err := backend.Put(blobAPIPath, blobData); err != nil {
				slog.Error("failed to push resource blob", "id", id, "error", err)
				continue
			}
		}

		if err := db.UpsertSyncItem(id, models.TypeResource, syncTarget); err != nil {
			return err
		}
		slog.Debug("pushed resource", "id", id)
	}
	return nil
}

func pushDeletes(backend SyncBackend, db *store.DB, syncTarget int) error {
	deleted, err := db.GetDeletedItems(syncTarget)
	if err != nil {
		return err
	}

	for _, item := range deleted {
		itemName := item.ItemID + ".md"
		path := fmt.Sprintf("/api/items/root:/%s:/content", url.PathEscape(itemName))

		if err := backend.Delete(path); err != nil {
			slog.Error("failed to delete remote item", "id", item.ItemID, "error", err)
			continue
		}

		if err := db.RemoveDeletedItem(item.ID); err != nil {
			return err
		}
		slog.Debug("deleted remote item", "id", item.ItemID)
	}
	return nil
}
