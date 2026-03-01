# Schema Maintenance

This document explains how gojoplin's SQLite schema relates to upstream Joplin and how to keep it aligned.

## Upstream Schema Location

The canonical Joplin schema lives in the Joplin monorepo:

```
packages/lib/services/database/Database.ts
```

Look for the `createTableStatements` and migration functions. The current schema version is tracked in the `version` table (Joplin Desktop) or `key_values` table (gojoplin).

## Gojoplin's Schema

Gojoplin defines its schema in `internal/store/db.go` in the `schemaStatements` slice. All tables use `CREATE TABLE IF NOT EXISTS` for idempotent creation.

The current schema version is **49**, stored as `schema_version` in the `key_values` table.

## Comparing Schemas

1. Check the upstream Joplin repo for the latest schema version in `Database.ts`.
2. Diff the `CREATE TABLE` statements against gojoplin's `schemaStatements` in `internal/store/db.go`.
3. Pay attention to:
   - New columns added to existing tables
   - New tables (e.g., `master_keys` was added for E2EE support)
   - New indexes
   - Changes to FTS virtual tables or triggers

## Updating the Schema

Gojoplin uses a simple migration approach:

1. **Add new tables**: Append new `CREATE TABLE IF NOT EXISTS` statements to `schemaStatements`.
2. **Add new columns**: SQLite does not support `ADD COLUMN IF NOT EXISTS`, so use `ALTER TABLE ... ADD COLUMN` wrapped in a version check, or rebuild the table.
3. **Bump version**: Update the version check in `migrate()` (currently `version >= 49`) and the `INSERT OR REPLACE` that sets `schema_version`.
4. **Test**: Run `go build ./...` and verify a fresh database can be created and an existing database can be migrated.

### Step-by-step

```bash
# 1. Clone the Joplin repo and find the latest schema
git clone https://github.com/laurent22/joplin.git
grep -n "CREATE TABLE" joplin/packages/lib/services/database/Database.ts

# 2. Compare with gojoplin
# Look at internal/store/db.go schemaStatements

# 3. Make changes to schemaStatements in db.go

# 4. Update the version number in migrate()

# 5. Verify
go build ./...
```

## Notes

- Gojoplin only supports Joplin Server (sync target 9). Schema features specific to other sync targets can be ignored.
- The `master_keys` table is used for E2EE support and stores encrypted master key content pulled from the server.
- FTS4 is used for note search via the `notes_fts` virtual table with automatic sync triggers.
- WAL mode and foreign keys are enabled at connection time, not in the schema.
