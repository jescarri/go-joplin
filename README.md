# go-joplin

A Joplin Web Clipper server implementation in Go. It runs a local HTTP server that the Joplin Web Clipper browser extension can talk to, and syncs your notes with either **Joplin Server** or an **S3-compatible bucket** (AWS S3, MinIO, etc.).

![go-joplin](go-joplin.png)

## Features

- **Web Clipper API**: Notes, folders, tags, resources, search, and events endpoints compatible with the Joplin clipper.
- **MCP (Model Context Protocol)**: SSE endpoint at `/mcp` with Bearer token auth; tools for notes, folders, tags, resources, search, and sync. Tool registration and prompts are easy to modify in `internal/mcp`.
- **Observability**: OpenTelemetry tracing (OTLP HTTP), Prometheus metrics on a separate port; `/health` returns 200 (no trace/log). p99 and other quantiles via Prometheus recording rules (see `docs/prometheus-recording-rules.yaml`).
- **Sync targets**:
  - **Joplin Server** (sync target 9): Sync over HTTP with a Joplin Server instance.
  - **S3** (sync target 8): Sync with any S3-compatible storage (AWS S3, MinIO, Backblaze B2, etc.). Uses the same object layout as Joplin’s built-in S3 sync.
- **End-to-end encryption**: Uses existing Joplin E2EE; no changes to crypto.

## Requirements

- Go 1.24 or later
- For **Joplin Server** sync: Joplin Server URL, username, password, and API token from your Joplin config.
- For **S3** sync: Bucket name, region, endpoint URL (and optional force path style for MinIO). **Credentials must be provided via environment variables** (see below).

## Build

From the project root:

```bash
go build -o go-joplin .
```

To install into `$GOPATH/bin` (or `$HOME/go/bin`):

```bash
go install .
```

## Configuration

go-joplin reads the Joplin desktop app config (e.g. `~/.config/joplin-desktop/settings.json`). You can override the path with the `GOJOPLIN_CONFIG_PATH` environment variable.

**Environment variables** (all optional; override config file and flags):

| Variable | Description |
|----------|-------------|
| `GOJOPLIN_CONFIG_PATH` | Path to Joplin settings file (default: `~/.config/joplin-desktop/settings.json`) |
| `GOJOPLIN_DATA_DIR` | Data directory for DB and resources (default: `~/.local/share/joplingo`) |
| `GOJOPLIN_PORT` | Clipper server port (default: 41184) |
| `GOJOPLIN_USERNAME` | Joplin Server username (overrides config) |
| `GOJOPLIN_PASSWORD` | Joplin Server password (overrides config) |
| `GOJOPLIN_API_KEY` | API key for clipper and MCP authentication (overrides config) |
| `GOJOPLIN_MASTER_PASSWORD` | E2EE master password for decrypting notes (overrides config) |
| `GOJOPLIN_TRACING_ENABLED` | Enable OpenTelemetry tracing (default: true) |
| `GOJOPLIN_TRACING_PROTOCOL` | OTLP protocol: `http` or `grpc` (default: http) |
| `GOJOPLIN_TRACING_SERVICE_NAME` | Service name for traces (default: niper-agent) |
| `GOJOPLIN_TRACING_SAMPLE_RATE` | Trace sample rate 0–1 (default: 1.0) |
| `GOJOPLIN_METRICS_ENABLED` | Enable Prometheus metrics (default: true) |
| `GOJOPLIN_METRICS_PROMETHEUS_PORT` | Port for /metrics (default: 9091) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP endpoint for traces (e.g. http://localhost:4318) |

- **Sync target 9 (Joplin Server)**
  Set in Joplin: sync target = Joplin Server, and set `sync.9.path` (server URL), `sync.9.username`, `sync.9.password`. The clipper also needs `api.token` (from Joplin’s Web Clipper auth).

- **Sync target 8 (S3)**
  Set in Joplin: sync target = S3, and in settings:
  - `sync.8.path`: bucket name
  - `sync.8.url`: endpoint URL (e.g. `https://s3.amazonaws.com/` for AWS, or your MinIO URL)
  - `sync.8.region`: region (e.g. `us-east-1`)
  - `sync.8.forcePathStyle`: set to `true` for MinIO and some other S3-compatible backends

  **Credentials for S3** follow Joplin config: use `sync.8.username` and `sync.8.password` from your Joplin settings file. Environment variables override if set: `AWS_ACCESS_KEY_ID` (or `ACCESS_KEY_ID`) and `AWS_SECRET_ACCESS_KEY` (or `SECRET_ACCESS_KEY`).

  The S3 client works with both official AWS S3 and S3-compatible storage (e.g. MinIO).

## Usage

- **Serve (clipper + background sync)**
  Start the clipper server and sync in the background:

  ```bash
  export AWS_ACCESS_KEY_ID=your_key      # when using S3
  export AWS_SECRET_ACCESS_KEY=your_secret
  ./go-joplin serve --api-key YOUR_JOPLIN_API_KEY
  ```

  The server listens on `localhost:41184` by default. Override with `GOJOPLIN_PORT` or the `--port` flag if supported.

- **One-shot sync**
  Run a single sync (no HTTP server):

  ```bash
  ./go-joplin sync
  ```

- **Config**
  Print resolved configuration:

  ```bash
  ./go-joplin config
  ```

## Project layout

- `cmd/`: CLI commands (`serve`, `sync`, `config`, etc.)
- `internal/config`: Load and validate config from Joplin settings (including observability from env).
- `internal/store`: SQLite store (notes, folders, tags, resources, sync state).
- `internal/sync`: Sync engine; HTTP client for Joplin Server and S3 backend for S3/MinIO; traced backend wrapper.
- `internal/s3`: S3 client (AWS SDK v2) for official S3 and S3-compatible endpoints.
- `internal/clipper`: Web Clipper HTTP API and handlers; `/health`, metrics middleware.
- `internal/mcp`: MCP server over SSE; tool registry and handlers (easy to modify).
- `internal/telemetry`: OpenTelemetry tracer provider, Prometheus registry and request-duration histogram.
- `internal/e2ee`, `internal/models`: E2EE and shared models (E2EE calls traced).
- `docs/prometheus-recording-rules.yaml`: Example recording rules for p99/p95/p50 of API latency.

## License

See the repository license file.
