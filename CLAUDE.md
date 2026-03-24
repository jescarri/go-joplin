# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is this?

go-joplin is a headless Joplin Web Clipper server in Go. It syncs notes with Joplin Server (target 9) or S3-compatible storage (target 8), exposes the Joplin Clipper REST API, and serves an MCP (Model Context Protocol) endpoint over SSE at `/mcp`. No Joplin desktop app required.

## Build & Test

```bash
# Build (requires CGO for SQLite)
CGO_ENABLED=1 go build -ldflags="-s -w" -o go-joplin .

# Run all tests
go test ./...

# Run a single test
go test ./internal/store/ -run TestFolderCRUD

# Run with race detector
go test -race ./...

# Lint and vet (always run before submitting changes)
golangci-lint run ./...
go vet ./...
```

## Architecture

**Entrypoint:** `main.go` → `cmd/` (cobra CLI: `serve`, `sync`, `config`).

**`serve` command** is the main mode: starts the HTTP server with background sync every 10 minutes. Mutations trigger an immediate sync via `Engine.TriggerSync()`.

**Key packages:**

- `internal/config` — Loads config from Joplin `settings.json` (JSON) or native YAML (`.yaml`/`.yml` extension). Env vars override everything. Supports `${VAR}` expansion in YAML.
- `internal/store` — SQLite via `mattn/go-sqlite3` (CGO required). Joplin v49 schema with FTS4 for note search. All tables use Joplin's 32-char hex IDs.
- `internal/sync` — Sync engine: authenticate → lock → pull (delta) → reconcile → decrypt → push → unlock. Two backends behind `SyncBackend` interface: `Client` (Joplin Server HTTP) and `S3Backend`. `TracedBackend` wraps either with OpenTelemetry spans.
- `internal/clipper` — chi router implementing the Joplin Web Clipper REST API. Bearer auth middleware skips `/health`. Mounts MCP handler at `/mcp`.
- `internal/mcp` — MCP server using `modelcontextprotocol/go-sdk`. Tools and resources for notes/folders/tags/search/sync. `Policy` enforces mutation allow-lists (folders, tags, create permissions). `enabled_tools` config filters which tools are registered.
- `internal/e2ee` — Joplin E2EE decryption (AES-256-CBC with SJCL key derivation).
- `internal/telemetry` — OpenTelemetry tracing (OTLP HTTP/gRPC) and Prometheus metrics on a separate port.

**Data flow:** Sync engine pulls items as serialized Joplin `.md` format (not markdown — custom key:value + body format, see `internal/models/serialize.go`), stores in SQLite, optionally decrypts E2EE items. Clipper API and MCP tools read/write the same SQLite DB.

**Mutation policy:** By default all writes are denied. Controlled via `GOJOPLIN_MCP_ALLOW_*` env vars. Policy is shared between MCP tools and Clipper API handlers.

## Configuration

- Two config formats: Joplin `settings.json` (JSON) or native YAML (`.yaml`/`.yml`). Detected by file extension.
- **Secrets must never be hardcoded in config files.** Use `${VAR}` expansion in YAML values (e.g. `api.token: "${GOJOPLIN_API_TOKEN}"`) so secrets are rendered from environment variables at load time.
- **All env vars use the `GOJOPLIN_` prefix** (except `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and `OTEL_EXPORTER_OTLP_ENDPOINT`). When adding new config options, always use the `GOJOPLIN_` prefix for env var names.
- Precedence: env vars > CLI flags > config file.
- `ExpandEnv()` in `internal/config/config.go` handles `${VAR}` substitution via `os.Expand`.
- See `config.yaml.example` for the full YAML structure.

## Rules

- **Always run `golangci-lint run ./...` and `go vet ./...` after making changes.** These must pass before any code is considered done.
- **NEVER modify `internal/e2ee/`.** The cryptographic code must stay exactly as-is — the Joplin app depends on this exact implementation for E2EE interoperability. Do not refactor, "improve", or touch it in any way.
- **Prefer interfaces over code bloat.** Use small interfaces to decouple packages rather than duplicating code or adding concrete dependencies.
- **Follow Go best practices. Keep source files reasonably sized — never create files with thousands of lines.** Split by responsibility: one concern per file, break large files into focused units.
- **Never discard errors with `_`.** Always handle errors properly — return them, log them, or `t.Fatal` them in tests. `_ = someFunc()` is an anti-pattern that hides failures. In test `httptest` handlers, pass `t` into the closure and call `t.Fatalf` on error.
- **Follow ADRs in `docs/adr/`.** Architecture Decision Records document design choices. Read relevant ADRs before modifying the systems they cover.
- **Keep ADRs and implementation plans in sync.** Every feature that requires an ADR must also have a matching implementation plan at `implementation_plans/<feature-name>/implementation.md`, where `<feature-name>` matches the ADR filename (e.g. ADR `docs/adr/001-rag-search.md` → plan `implementation_plans/001-rag-search/implementation.md`). When the design changes, update both documents. Always write the ADR and implementation plan before writing code.

## Git Commits

- **Always use [Conventional Commits](https://www.conventionalcommits.org/)** for semantic-release. Format: `type(scope): description` (e.g. `feat(mcp): add search tool`, `fix(sync): handle nil lock response`).
- **Derive the commit message from the actual changes.** Read the diff, understand what changed and why, then write the message. Never use lazy messages like `fix`, `wip`, `w00t`, `stuff`, etc.
- **Before creating a PR or pushing, review all commits on the branch.** If any commit has a lazy/meaningless message, rewrite it with a proper conventional commit message derived from the actual changes in that commit.
- **Do NOT add a `Co-Authored-By` trailer.** No Claude co-author lines.

## Conventions

- Uses Go standard `log/slog` for structured logging.
- HTTP routing via `go-chi/chi/v5`.
- CLI via `spf13/cobra`.
- Config precedence: env vars > CLI flags > config file.
