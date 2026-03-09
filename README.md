# go-joplin

[![CI](https://github.com/jescarri/go-joplin/actions/workflows/ci.yml/badge.svg)](https://github.com/jescarri/go-joplin/actions/workflows/ci.yml)
[![Release](https://github.com/jescarri/go-joplin/actions/workflows/release.yml/badge.svg)](https://github.com/jescarri/go-joplin/actions/workflows/release.yml)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)

A Joplin Web Clipper server implementation in Go. It runs a local HTTP server that the [Joplin](https://joplinapp.org/) Web Clipper browser extension can talk to, and syncs your notes with either **Joplin Server** or an **S3-compatible bucket** (AWS S3, MinIO, etc.).

**Purpose:** This server and its MCP (Model Context Protocol) endpoint let you run **fully headless**—no Joplin desktop or mobile app required. Sync and serve your notes from a single binary, and expose them to **agents over the network** (e.g. AI assistants, automation) without installing Joplin on the same machine.

This project is not affiliated with Joplin. **Joplin** is the open-source note-taking app by [Laurent Cozic](https://github.com/laurent22). See [https://joplinapp.org/](https://joplinapp.org/) and the [Joplin repository](https://github.com/laurent22/joplin) for the official app, documentation, and community.

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

go-joplin uses the **existing Joplin configuration** from a machine where the Joplin desktop app is installed. The config file (e.g. `~/.config/joplin-desktop/settings.json` on Linux, `~/Library/Application Support/Joplin/` on macOS) contains sync targets, API tokens, and other settings. You can override the path with the `GOJOPLIN_CONFIG_PATH` environment variable.

When running in Docker, **mount the Joplin config directory as read-only (`:ro`)** so the container cannot modify your Joplin settings.

**Environment variables** (all optional; override config file and flags):

| Variable | Description |
|----------|-------------|
| `GOJOPLIN_CONFIG_PATH` | Path to Joplin settings file (default: `~/.config/joplin-desktop/settings.json`) |
| `GOJOPLIN_DATA_DIR` | Data directory for DB and resources (default: `~/.local/share/gojoplin`) |
| `GOJOPLIN_LISTEN_HOST` | Clipper server bind address host (default: `localhost`; use `0.0.0.0` to listen on all interfaces) |
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

## Docker

Pre-built images are available at `ghcr.io/jescarri/gojoplin`. Mount your Joplin config **read-only** and provide a writable data directory:

```bash
docker run -d \
  -p 41184:41184 \
  -v /path/to/joplin-desktop/config:/config:ro \
  -v gojoplin-data:/data \
  -e GOJOPLIN_CONFIG_PATH=/config/settings.json \
  -e GOJOPLIN_DATA_DIR=/data \
  ghcr.io/jescarri/gojoplin:latest serve --api-key YOUR_JOPLIN_API_KEY
```

- `/config:ro` — Joplin settings (sync targets, API token, etc.) mounted **read-only**
- `/data` — Writable volume for the local SQLite DB and resources

## Releases and CI

- **Branches and PRs**: CI runs tests and build (and validates the Dockerfile); no image is pushed.
- **Main branch**: On every push to `main`, the workflow runs tests, [semantic-release](https://github.com/semantic-release/semantic-release) (which may create a new version and GitHub Release from [Conventional Commits](https://www.conventionalcommits.org/)), and pushes the container image to GitHub Container Registry as `latest` and `sha-<short-sha>`. If semantic-release publishes a new version (e.g. `v1.2.3`), that tag is also pushed to GHCR.
- **Tag push (`v*`)**: Pushing a tag (e.g. `v1.0.0`) runs tests and pushes the image with that tag to GHCR.

Configuration for semantic-release is in [.releaserc.json](.releaserc.json).

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

This project is licensed under the same terms as Joplin: **AGPL-3.0-or-later**. See the [LICENSE](LICENSE) file in this repository. The [Joplin project](https://github.com/laurent22/joplin) is © 2016–2025 [Laurent Cozic](https://github.com/laurent22); Joplin® is a trademark of JOPLIN SAS.
