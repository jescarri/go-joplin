package sync

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jescarri/go-joplin/internal/e2ee"
	"github.com/jescarri/go-joplin/internal/store"
)

// Engine orchestrates the sync process: authenticate -> lock -> pull -> decrypt -> push -> unlock.
type Engine struct {
	backend        SyncBackend
	db             *store.DB
	e2ee           *e2ee.Service
	masterPassword string
	triggerCh      chan struct{}
}

// NewEngine creates a new sync engine with the given sync backend (HTTP client or S3).
func NewEngine(backend SyncBackend, db *store.DB, masterPassword string) *Engine {
	return &Engine{
		backend:        backend,
		db:             db,
		e2ee:           e2ee.NewService(),
		masterPassword: masterPassword,
		triggerCh:      make(chan struct{}, 1),
	}
}

// TriggerSync signals the background sync loop to run immediately.
func (e *Engine) TriggerSync() {
	select {
	case e.triggerCh <- struct{}{}:
	default:
		// Already a trigger pending
	}
}

// TriggerCh returns the channel that receives sync trigger signals.
func (e *Engine) TriggerCh() <-chan struct{} {
	return e.triggerCh
}

// Sync performs a full sync cycle.
func (e *Engine) Sync(ctx context.Context) error {
	// Authenticate if needed
	if !e.backend.IsAuthenticated() {
		if err := e.backend.Authenticate(); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	// Acquire lock
	lock, err := e.backend.AcquireLock()
	if err != nil {
		slog.Warn("cannot acquire sync lock, proceeding without lock", "error", err)
		// Continue without lock - some server versions don't support it
	}
	defer func() {
		if lock != nil {
			e.backend.ReleaseLock(lock)
		}
	}()

	// Check for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Pull changes from server
	slog.Info("pulling changes from server")
	if err := PullChanges(e.backend, e.db); err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}

	// Reconcile: fetch any items the delta missed (e.g. master keys)
	if err := PullMissingItems(e.backend, e.db); err != nil {
		slog.Warn("reconciliation failed", "error", err)
	}

	// Fetch master keys from info.json (Joplin stores them there, not as .md items)
	if err := PullSyncInfo(e.backend, e.db); err != nil {
		slog.Warn("cannot pull sync info", "error", err)
	}

	// Decrypt pulled items
	if e.masterPassword != "" {
		slog.Info("decrypting pulled items")
		if err := DecryptPulledItems(e.db, e.e2ee, e.masterPassword); err != nil {
			slog.Error("decryption pass failed", "error", err)
			// Continue with push even if decryption fails
		}
	}

	// Check for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Push local changes to server
	slog.Info("pushing local changes to server")
	if err := PushChanges(e.backend, e.db, e.e2ee); err != nil {
		return fmt.Errorf("push failed: %w", err)
	}

	slog.Info("sync completed")
	return nil
}
