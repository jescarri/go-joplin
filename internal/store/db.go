package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

func init() {
	sqlite_vec.Auto()
}

// DB wraps a sql.DB for Joplin data storage.
type DB struct {
	*sql.DB
	dataDir string
}

// Open opens (or creates) a Joplin-compatible SQLite database in the given directory.
func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create data directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, "gojoplin.sqlite")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("cannot open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("cannot connect to database: %w", err)
	}

	store := &DB{DB: db, dataDir: dataDir}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	slog.Info("database opened", "path", dbPath)
	return store, nil
}

// DataDir returns the data directory path.
func (db *DB) DataDir() string {
	return db.dataDir
}

// ResourceDir returns the path where resource files are stored.
func (db *DB) ResourceDir() string {
	dir := filepath.Join(db.dataDir, "resources")
	os.MkdirAll(dir, 0o755)
	return dir
}

func (db *DB) migrate() error {
	// Check current schema version
	var version int
	row := db.QueryRow("SELECT value FROM key_values WHERE key = 'schema_version'")
	if err := row.Scan(&version); err != nil {
		// Table doesn't exist yet, create everything
		version = 0
	}

	if version >= 49 {
		return nil
	}

	slog.Info("running migrations", "from_version", version, "to_version", 49)

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, stmt := range schemaStatements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("migration statement failed: %w\nSQL: %s", err, stmt)
		}
	}

	// Set schema version
	if _, err := tx.Exec(`INSERT OR REPLACE INTO key_values (key, value, type, updated_time) VALUES ('schema_version', '49', 1, strftime('%s','now') * 1000)`); err != nil {
		return err
	}

	return tx.Commit()
}

// schemaStatements creates the Joplin v49 schema.
var schemaStatements = []string{
	// Key-value store (must be first for schema version tracking)
	`CREATE TABLE IF NOT EXISTS key_values (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		key TEXT NOT NULL UNIQUE,
		value TEXT NOT NULL,
		type INTEGER NOT NULL DEFAULT 1,
		updated_time INT NOT NULL DEFAULT 0
	)`,

	// Folders (notebooks)
	`CREATE TABLE IF NOT EXISTS folders (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL DEFAULT '',
		created_time INT NOT NULL DEFAULT 0,
		updated_time INT NOT NULL DEFAULT 0,
		user_created_time INT NOT NULL DEFAULT 0,
		user_updated_time INT NOT NULL DEFAULT 0,
		encryption_cipher_text TEXT NOT NULL DEFAULT '',
		encryption_applied INT NOT NULL DEFAULT 0,
		parent_id TEXT NOT NULL DEFAULT '',
		is_shared INT NOT NULL DEFAULT 0,
		share_id TEXT NOT NULL DEFAULT '',
		master_key_id TEXT NOT NULL DEFAULT '',
		icon TEXT NOT NULL DEFAULT '',
		user_data TEXT NOT NULL DEFAULT '',
		deleted_time INT NOT NULL DEFAULT 0
	)`,

	// Notes
	`CREATE TABLE IF NOT EXISTS notes (
		id TEXT PRIMARY KEY,
		parent_id TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '',
		body TEXT NOT NULL DEFAULT '',
		created_time INT NOT NULL DEFAULT 0,
		updated_time INT NOT NULL DEFAULT 0,
		is_conflict INT NOT NULL DEFAULT 0,
		latitude NUMERIC NOT NULL DEFAULT 0,
		longitude NUMERIC NOT NULL DEFAULT 0,
		altitude NUMERIC NOT NULL DEFAULT 0,
		author TEXT NOT NULL DEFAULT '',
		source_url TEXT NOT NULL DEFAULT '',
		is_todo INT NOT NULL DEFAULT 0,
		todo_due INT NOT NULL DEFAULT 0,
		todo_completed INT NOT NULL DEFAULT 0,
		source TEXT NOT NULL DEFAULT '',
		source_application TEXT NOT NULL DEFAULT '',
		application_data TEXT NOT NULL DEFAULT '',
		"order" INT NOT NULL DEFAULT 0,
		user_created_time INT NOT NULL DEFAULT 0,
		user_updated_time INT NOT NULL DEFAULT 0,
		encryption_cipher_text TEXT NOT NULL DEFAULT '',
		encryption_applied INT NOT NULL DEFAULT 0,
		markup_language INT NOT NULL DEFAULT 1,
		is_shared INT NOT NULL DEFAULT 0,
		share_id TEXT NOT NULL DEFAULT '',
		conflict_original_id TEXT NOT NULL DEFAULT '',
		master_key_id TEXT NOT NULL DEFAULT '',
		user_data TEXT NOT NULL DEFAULT '',
		deleted_time INT NOT NULL DEFAULT 0
	)`,

	// Tags
	`CREATE TABLE IF NOT EXISTS tags (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL DEFAULT '',
		created_time INT NOT NULL DEFAULT 0,
		updated_time INT NOT NULL DEFAULT 0,
		user_created_time INT NOT NULL DEFAULT 0,
		user_updated_time INT NOT NULL DEFAULT 0,
		encryption_cipher_text TEXT NOT NULL DEFAULT '',
		encryption_applied INT NOT NULL DEFAULT 0,
		is_shared INT NOT NULL DEFAULT 0,
		parent_id TEXT NOT NULL DEFAULT '',
		user_data TEXT NOT NULL DEFAULT ''
	)`,

	// Note-Tag junction
	`CREATE TABLE IF NOT EXISTS note_tags (
		id TEXT PRIMARY KEY,
		note_id TEXT NOT NULL DEFAULT '',
		tag_id TEXT NOT NULL DEFAULT '',
		created_time INT NOT NULL DEFAULT 0,
		updated_time INT NOT NULL DEFAULT 0,
		user_created_time INT NOT NULL DEFAULT 0,
		user_updated_time INT NOT NULL DEFAULT 0,
		encryption_cipher_text TEXT NOT NULL DEFAULT '',
		encryption_applied INT NOT NULL DEFAULT 0,
		is_shared INT NOT NULL DEFAULT 0
	)`,

	// Resources
	`CREATE TABLE IF NOT EXISTS resources (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL DEFAULT '',
		mime TEXT NOT NULL DEFAULT '',
		filename TEXT NOT NULL DEFAULT '',
		created_time INT NOT NULL DEFAULT 0,
		updated_time INT NOT NULL DEFAULT 0,
		user_created_time INT NOT NULL DEFAULT 0,
		user_updated_time INT NOT NULL DEFAULT 0,
		file_extension TEXT NOT NULL DEFAULT '',
		encryption_cipher_text TEXT NOT NULL DEFAULT '',
		encryption_applied INT NOT NULL DEFAULT 0,
		encryption_blob_encrypted INT NOT NULL DEFAULT 0,
		size INT NOT NULL DEFAULT 0,
		is_shared INT NOT NULL DEFAULT 0,
		share_id TEXT NOT NULL DEFAULT '',
		master_key_id TEXT NOT NULL DEFAULT '',
		user_data TEXT NOT NULL DEFAULT '',
		blob_updated_time INT NOT NULL DEFAULT 0
	)`,

	// Master keys (E2EE)
	`CREATE TABLE IF NOT EXISTS master_keys (
		id TEXT PRIMARY KEY,
		created_time INT NOT NULL DEFAULT 0,
		updated_time INT NOT NULL DEFAULT 0,
		source_application TEXT NOT NULL DEFAULT '',
		encryption_method INT NOT NULL DEFAULT 0,
		checksum TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL DEFAULT ''
	)`,

	// Sync items
	`CREATE TABLE IF NOT EXISTS sync_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		sync_target INT NOT NULL,
		sync_time INT NOT NULL DEFAULT 0,
		item_type INT NOT NULL DEFAULT 0,
		item_id TEXT NOT NULL DEFAULT '',
		sync_disabled INT NOT NULL DEFAULT 0,
		sync_disabled_reason TEXT NOT NULL DEFAULT '',
		force_sync INT NOT NULL DEFAULT 0,
		item_location INT NOT NULL DEFAULT 0
	)`,

	// Deleted items
	`CREATE TABLE IF NOT EXISTS deleted_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		item_type INT NOT NULL,
		item_id TEXT NOT NULL,
		deleted_time INT NOT NULL DEFAULT 0,
		sync_target INT NOT NULL DEFAULT 0
	)`,

	// Item changes (for events API)
	`CREATE TABLE IF NOT EXISTS item_changes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		item_type INT NOT NULL,
		item_id TEXT NOT NULL,
		type INT NOT NULL,
		created_time INT NOT NULL DEFAULT 0
	)`,

	// FTS for notes search
	`CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts4(
		content="notes",
		notindexed="id",
		id,
		title,
		body
	)`,

	// FTS triggers
	`CREATE TRIGGER IF NOT EXISTS notes_fts_before_update BEFORE UPDATE ON notes BEGIN
		DELETE FROM notes_fts WHERE docid=old.rowid;
	END`,

	`CREATE TRIGGER IF NOT EXISTS notes_fts_before_delete BEFORE DELETE ON notes BEGIN
		DELETE FROM notes_fts WHERE docid=old.rowid;
	END`,

	`CREATE TRIGGER IF NOT EXISTS notes_fts_after_update AFTER UPDATE ON notes BEGIN
		INSERT INTO notes_fts(docid, id, title, body) VALUES(new.rowid, new.id, new.title, new.body);
	END`,

	`CREATE TRIGGER IF NOT EXISTS notes_fts_after_insert AFTER INSERT ON notes BEGIN
		INSERT INTO notes_fts(docid, id, title, body) VALUES(new.rowid, new.id, new.title, new.body);
	END`,

	// Indexes
	`CREATE INDEX IF NOT EXISTS idx_notes_parent_id ON notes(parent_id)`,
	`CREATE INDEX IF NOT EXISTS idx_notes_is_todo ON notes(is_todo)`,
	`CREATE INDEX IF NOT EXISTS idx_note_tags_note_id ON note_tags(note_id)`,
	`CREATE INDEX IF NOT EXISTS idx_note_tags_tag_id ON note_tags(tag_id)`,
	`CREATE INDEX IF NOT EXISTS idx_sync_items_item_id ON sync_items(item_id)`,
	`CREATE INDEX IF NOT EXISTS idx_sync_items_sync_target ON sync_items(sync_target)`,
	`CREATE INDEX IF NOT EXISTS idx_deleted_items_sync_target ON deleted_items(sync_target)`,
	`CREATE INDEX IF NOT EXISTS idx_item_changes_item_id ON item_changes(item_id)`,
	`CREATE INDEX IF NOT EXISTS idx_item_changes_created_time ON item_changes(created_time)`,

	// RAG tables (never synced; rag_ prefix)
	`CREATE TABLE IF NOT EXISTS rag_note_hashes (
		note_id    TEXT PRIMARY KEY,
		body_hash  TEXT NOT NULL,
		updated_at INTEGER NOT NULL DEFAULT 0
	)`,

	`CREATE TABLE IF NOT EXISTS rag_chunks (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		note_id     TEXT NOT NULL,
		chunk_index INTEGER NOT NULL,
		content     TEXT NOT NULL,
		token_count INTEGER NOT NULL DEFAULT 0,
		UNIQUE(note_id, chunk_index)
	)`,

	`CREATE INDEX IF NOT EXISTS idx_rag_chunks_note_id ON rag_chunks(note_id)`,
}

// InitRAG creates or rebuilds the rag_vec virtual table. If the model or dimensions
// changed since the last run, all RAG data is wiped to force a full re-index.
func (db *DB) InitRAG(model string, dimensions int) error {
	storedModel := db.getKV("rag_model")
	storedDims := db.getKV("rag_dimensions")

	needsRebuild := false
	if storedModel == "" && storedDims == "" {
		// First run
		slog.Info("initializing RAG tables", "model", model, "dimensions", dimensions)
	} else if storedModel != model || storedDims != fmt.Sprintf("%d", dimensions) {
		// Model or dimensions changed
		slog.Warn("RAG model/dimensions changed, full re-index required",
			"old_model", storedModel, "new_model", model,
			"old_dimensions", storedDims, "new_dimensions", dimensions)
		needsRebuild = true
	} else {
		// Normal restart, nothing to do
		return nil
	}

	if needsRebuild {
		if _, err := db.Exec("DROP TABLE IF EXISTS rag_vec"); err != nil {
			return fmt.Errorf("drop rag_vec: %w", err)
		}
		if _, err := db.Exec("DELETE FROM rag_chunks"); err != nil {
			return fmt.Errorf("clear rag_chunks: %w", err)
		}
		if _, err := db.Exec("DELETE FROM rag_note_hashes"); err != nil {
			return fmt.Errorf("clear rag_note_hashes: %w", err)
		}
	}

	ddl := fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS rag_vec USING vec0(chunk_id INTEGER PRIMARY KEY, embedding float[%d])`, dimensions)
	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("create rag_vec: %w", err)
	}

	if err := db.setKV("rag_model", model); err != nil {
		return err
	}
	return db.setKV("rag_dimensions", fmt.Sprintf("%d", dimensions))
}

func (db *DB) getKV(key string) string {
	var val string
	err := db.QueryRow("SELECT value FROM key_values WHERE key = ?", key).Scan(&val)
	if err != nil {
		return ""
	}
	return val
}

func (db *DB) setKV(key, value string) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO key_values (key, value, type, updated_time) VALUES (?, ?, 1, strftime('%s','now') * 1000)`, key, value)
	return err
}
