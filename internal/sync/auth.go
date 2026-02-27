package sync

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

// sessionResponse represents the Joplin Server session API response.
type sessionResponse struct {
	ID string `json:"id"`
}

// Authenticate creates a new session with the Joplin Server (SyncBackend interface).
func (c *Client) Authenticate() error {
	payload := map[string]string{
		"email":    c.username,
		"password": c.password,
	}

	data, err := c.post("/api/sessions", payload)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	var resp sessionResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("cannot parse session response: %w", err)
	}

	if resp.ID == "" {
		return fmt.Errorf("empty session ID returned")
	}

	c.sessionID = resp.ID
	slog.Info("authenticated with Joplin Server")
	return nil
}

// IsAuthenticated returns whether the client has a session.
func (c *Client) IsAuthenticated() bool {
	return c.sessionID != ""
}
