package mcp

import (
	"github.com/jescarri/go-joplin/internal/store"
)

// SyncTrigger triggers a sync run. Implemented by sync.Engine.
type SyncTrigger interface {
	TriggerSync()
}

// Deps holds dependencies for MCP tool handlers. Easy to extend for new tools.
type Deps struct {
	DB           *store.DB
	Syncer       SyncTrigger
	Policy       *Policy // nil = all mutations denied (read-only)
	EnabledTools string  // comma-separated tool names or "*" for all (default "*")
}
