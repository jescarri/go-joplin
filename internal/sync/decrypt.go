package sync

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jescarri/go-joplin/internal/e2ee"
	"github.com/jescarri/go-joplin/internal/models"
	"github.com/jescarri/go-joplin/internal/store"
)

// DecryptPulledItems loads master keys, then decrypts all items with encryption_applied = 1.
func DecryptPulledItems(db *store.DB, svc *e2ee.Service, masterPassword string) error {
	if masterPassword == "" {
		slog.Debug("no master password configured, skipping decryption")
		return nil
	}

	// Load master keys
	masterKeys, err := db.ListMasterKeys()
	if err != nil {
		return fmt.Errorf("cannot list master keys: %w", err)
	}

	if len(masterKeys) == 0 {
		slog.Warn("no master keys in database, skipping decryption")
		return nil
	}

	slog.Info("found master keys in database", "count", len(masterKeys))

	for _, mk := range masterKeys {
		if svc.HasMasterKey(mk.ID) {
			continue
		}
		if err := svc.LoadMasterKey(mk.ID, mk.Content, masterPassword, mk.EncryptionMethod); err != nil {
			slog.Error("cannot decrypt master key", "id", mk.ID, "error", err)
			continue
		}
		slog.Info("loaded master key", "id", mk.ID)
	}

	// Decrypt items with encryption_applied = 1
	items, err := db.GetEncryptedItems()
	if err != nil {
		return fmt.Errorf("cannot get encrypted items: %w", err)
	}

	slog.Info("decrypting items", "count", len(items))

	for _, item := range items {
		if err := decryptItem(db, svc, item); err != nil {
			slog.Error("failed to decrypt item", "id", item.ID, "type", item.ItemType, "error", err)
			continue
		}
	}

	// Decrypt resource blobs
	encResources, err := db.GetEncryptedResourceBlobs()
	if err != nil {
		return fmt.Errorf("cannot get encrypted resource blobs: %w", err)
	}

	for _, er := range encResources {
		if err := decryptResourceBlob(db, svc, er.ID); err != nil {
			slog.Error("failed to decrypt resource blob", "id", er.ID, "error", err)
			continue
		}
	}

	return nil
}

func decryptItem(db *store.DB, svc *e2ee.Service, item store.EncryptedItem) error {
	plaintext, err := svc.DecryptString(item.CipherText)
	if err != nil {
		return err
	}

	// The decrypted text is in Joplin's serialized format — re-deserialize and update
	switch item.ItemType {
	case models.TypeNote:
		note := models.DeserializeNote(plaintext)
		note.ID = item.ID
		note.EncryptionCipherText = ""
		note.EncryptionApplied = 0
		return db.UpsertNote(note)

	case models.TypeFolder:
		folder := models.DeserializeFolder(plaintext)
		folder.ID = item.ID
		folder.EncryptionCipherText = ""
		folder.EncryptionApplied = 0
		return db.UpsertFolder(folder)

	case models.TypeTag:
		tag := models.DeserializeTag(plaintext)
		tag.ID = item.ID
		tag.EncryptionCipherText = ""
		tag.EncryptionApplied = 0
		return db.UpsertTag(tag)

	default:
		return fmt.Errorf("unsupported item type %d for decryption", item.ItemType)
	}
}

func decryptResourceBlob(db *store.DB, svc *e2ee.Service, resourceID string) error {
	// Read the encrypted blob from disk
	blobPath := filepath.Join(db.ResourceDir(), resourceID)
	data, err := os.ReadFile(blobPath)
	if err != nil {
		return fmt.Errorf("cannot read resource blob: %w", err)
	}

	// Check if it looks like JED01
	if len(data) < 5 || string(data[:5]) != "JED01" {
		slog.Debug("resource blob not JED01 encrypted, skipping", "id", resourceID)
		return nil
	}

	decrypted, err := svc.DecryptFile(string(data))
	if err != nil {
		return err
	}

	// Write decrypted blob back
	if err := os.WriteFile(blobPath, decrypted, 0o644); err != nil {
		return fmt.Errorf("cannot write decrypted resource blob: %w", err)
	}

	// Update the resource record
	resource, err := db.GetResource(resourceID)
	if err != nil {
		return err
	}
	if resource != nil {
		resource.EncryptionBlobEncrypted = 0
		resource.Size = int64(len(decrypted))
		return db.UpsertResource(resource)
	}

	return nil
}
