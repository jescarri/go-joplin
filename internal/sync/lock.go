package sync

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

// Lock represents a Joplin Server sync lock.
type Lock struct {
	Type       int    `json:"type"`
	ClientType int    `json:"clientType"`
	ClientID   string `json:"clientId"`
}

// AcquireLock acquires a sync lock on the Joplin Server (SyncBackend interface).
func (c *Client) AcquireLock() (interface{}, error) {
	lock := &Lock{
		Type:       1, // Sync lock
		ClientType: 3, // CLI client
		ClientID:   "joplingo",
	}

	data, err := c.post("/api/locks", lock)
	if err != nil {
		return nil, fmt.Errorf("cannot acquire lock: %w", err)
	}

	var resp Lock
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("cannot parse lock response: %w", err)
	}

	slog.Debug("acquired sync lock", "type", resp.Type, "clientType", resp.ClientType, "clientId", resp.ClientID)
	return &resp, nil
}

// ReleaseLock releases a sync lock (SyncBackend interface).
func (c *Client) ReleaseLock(lock interface{}) error {
	l, _ := lock.(*Lock)
	if l == nil {
		return nil
	}

	// Joplin Server expects: DELETE /api/locks/{type}_{clientType}_{clientId}
	path := fmt.Sprintf("/api/locks/%d_%d_%s", l.Type, l.ClientType, l.ClientID)
	if err := c.delete(path); err != nil {
		slog.Warn("failed to release lock", "error", err)
		return err
	}

	slog.Debug("released sync lock")
	return nil
}
