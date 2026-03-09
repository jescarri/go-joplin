# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Native YAML config**
  - Optional config file format (`.yaml`/`.yml`) so the service can run without Joplin desktop settings. Use `--config config.yaml` or `GOJOPLIN_CONFIG_PATH=config.yaml`.
  - Secrets are not stored in the YAML file; use environment variables. API token and key support `${VAR}` expansion in the file (e.g. `api.token: "${GOJOPLIN_API_TOKEN}"`).
  - S3 credentials when using YAML are read from `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` (or `ACCESS_KEY_ID`, `SECRET_ACCESS_KEY`). Joplin Server credentials from `GOJOPLIN_USERNAME` and `GOJOPLIN_PASSWORD`.
  - Example file: `config.yaml.example`. Config path resolution: if the path has a `.yaml` or `.yml` extension, the YAML loader is used; otherwise Joplin `settings.json` (JSON) is expected.

- **Environment variables (documented)**
  - **Required (secrets):** `GOJOPLIN_API_TOKEN` (Web Clipper token), `GOJOPLIN_API_KEY` (Bearer for clipper/MCP), `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` (S3), `GOJOPLIN_USERNAME`/`GOJOPLIN_PASSWORD` (Joplin Server). Documented in README with a “Required (secrets)” and “Optional (overrides and runtime)” table.
  - README now lists all supported env vars and when each is required.

- **MCP / Clipper mutation allow-list**
  - Allow-list mechanism for restricting mutations (create/update notes, folders, tags).
  - Environment variables: `GOJOPLIN_MCP_ALLOW_FOLDERS`, `GOJOPLIN_MCP_ALLOW_TAGS`, `GOJOPLIN_MCP_ALLOW_CREATE_TAG`, `GOJOPLIN_MCP_ALLOW_CREATE_FOLDER`.
  - Use `*` to allow all mutations for folders or tags.
  - By default all mutations are denied (read-only). Configure allow-lists to permit writes.
  - MCP resource `joplingo://capabilities` and tool `get_capabilities` expose capabilities to LLMs (which folders/tags are read-write vs read-only).
  - Clipper REST API enforces the same policy for notes, folders, and tags.

- **MCP (Model Context Protocol)**  
  - SSE endpoint at `/mcp` with Bearer token authentication.  
  - Tools: `list_notes`, `get_note`, `create_note`, `update_note`, `search_notes`, `list_folders`, `get_folder`, `create_folder`, `list_tags`, `get_note_tags`, `list_resources`, `trigger_sync`, `get_capabilities`.  
  - Tool registration and handlers in `internal/mcp` (easy to extend).

- **Observability**  
  - OpenTelemetry tracing via OTLP HTTP; configurable endpoint, service name, and sample rate.  
  - Prometheus metrics on a dedicated port (default 9091); `http_request_duration_seconds` histogram by method and route.  
  - `/health` endpoint returning 200 without trace/log.  
  - Traced sync backends (S3 and Joplin Server) and E2EE operations.  
  - Example Prometheus recording rules in `docs/prometheus-recording-rules.yaml` for p99, p95, and p50 latency.

- **Configuration**  
  - New environment variables for observability:  
    - `GOJOPLIN_TRACING_ENABLED`, `GOJOPLIN_TRACING_PROTOCOL`, `GOJOPLIN_TRACING_SERVICE_NAME`, `GOJOPLIN_TRACING_SAMPLE_RATE`  
    - `GOJOPLIN_METRICS_ENABLED`, `GOJOPLIN_METRICS_PROMETHEUS_PORT`  
    - `OTEL_EXPORTER_OTLP_ENDPOINT` for the OTLP exporter.  
  - Config supports `${VAR}` expansion in tracing endpoint.

- **Store**  
  - `GetFolderByTitle` and `GetFolder` (by ID) in `internal/store/folders.go`.  
  - Tag lookup/helper functions in `internal/store/tags.go`.  
  - Extended sync state handling in `internal/store/sync_state.go`.  
  - Tests: `folders_test.go`, `store_test.go`, `tags_test.go`.

- **Sync**  
  - Traced backend wrapper (`internal/sync/traced.go`) for storage operations.  
  - Delta and reconcile logic updates; new tests in `delta_test.go`.

### Changed

- **Environment variables**  
  - Renamed `JOPLINGO_*` to `GOJOPLIN_*`:  
    - `JOPLINGO_CONFIG_PATH` → `GOJOPLIN_CONFIG_PATH`  
    - `JOPLINGO_DATA_DIR` → `GOJOPLIN_DATA_DIR`  
    - `JOPLINGO_PORT` → `GOJOPLIN_PORT`  
    - `JOPLINGO_USERNAME` → `GOJOPLIN_USERNAME`  
    - `JOPLINGO_PASSWORD` → `GOJOPLIN_PASSWORD`  
    - `JOPLINGO_API_KEY` → `GOJOPLIN_API_KEY`  
    - `JOPLINGO_MASTER_PASSWORD` → `GOJOPLIN_MASTER_PASSWORD`  
  - Update your environment or scripts if you used the old names.

- **Clipper**  
  - Server now accepts an optional MCP handler and uses metrics middleware.  
  - Ping and middleware updated for observability.

- **README**  
  - Documented MCP, observability, full env var table, and project layout.

### Dependencies

- New: `github.com/modelcontextprotocol/go-sdk`, OpenTelemetry OTLP and SDK, Prometheus client, and related packages (see `go.mod` / `go.sum`).
