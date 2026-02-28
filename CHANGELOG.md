# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **MCP (Model Context Protocol)**  
  - SSE endpoint at `/mcp` with Bearer token authentication.  
  - Tools: `list_notes`, `get_note`, `create_note`, `update_note`, `search_notes`, `list_folders`, `get_folder`, `create_folder`, `list_tags`, `get_note_tags`, `list_resources`, `trigger_sync`.  
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
