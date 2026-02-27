package sync

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jescarri/go-joplin/internal/models"
	"github.com/jescarri/go-joplin/internal/store"
)

// SyncInfo represents Joplin's info.json file stored at the sync target root.
// Master keys are stored here, NOT as individual .md items.
type SyncInfo struct {
	Version           int                `json:"version"`
	E2EE              syncInfoValue[bool]   `json:"e2ee"`
	ActiveMasterKeyID syncInfoValue[string] `json:"activeMasterKeyId"`
	MasterKeys        []models.MasterKey `json:"masterKeys"`
	AppMinVersion     string             `json:"appMinVersion"`
}

type syncInfoValue[T any] struct {
	Value       T     `json:"value"`
	UpdatedTime int64 `json:"updatedTime"`
}

// PullSyncInfo fetches info.json from the sync target and stores any
// master keys found in the local database.
func PullSyncInfo(backend SyncBackend, db *store.DB) error {
	data, err := backend.Get("/api/items/root:/info.json:/content")
	if err != nil {
		return fmt.Errorf("cannot fetch info.json: %w", err)
	}
	if data == nil || len(data) == 0 {
		// info.json may not exist yet (e.g. fresh S3 bucket)
		return nil
	}

	var info SyncInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return fmt.Errorf("cannot parse info.json: %w", err)
	}

	slog.Info("loaded sync info", "version", info.Version, "e2ee", info.E2EE.Value,
		"master_keys", len(info.MasterKeys), "active_master_key", info.ActiveMasterKeyID.Value)

	for _, mk := range info.MasterKeys {
		if mk.ID == "" {
			continue
		}
		if err := db.UpsertMasterKey(&mk); err != nil {
			slog.Error("cannot upsert master key from info.json", "id", mk.ID, "error", err)
			continue
		}
		slog.Info("stored master key from info.json", "id", mk.ID)
	}

	if info.ActiveMasterKeyID.Value != "" {
		if err := db.SetActiveMasterKeyID(info.ActiveMasterKeyID.Value); err != nil {
			slog.Warn("cannot store active master key id", "error", err)
		}
	}

	return nil
}
