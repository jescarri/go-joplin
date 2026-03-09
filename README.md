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

You can use either:

1. **Joplin settings.json** — The config file from a machine where the Joplin desktop app is installed (e.g. `~/.config/joplin-desktop/settings.json` on Linux). Contains sync targets, API token, and (for Joplin desktop) optionally S3 credentials.
2. **Native YAML config** — A dedicated YAML file with no secrets in the file; all secrets are provided via environment variables. Use `--config config.yaml` or `GOJOPLIN_CONFIG_PATH=config.yaml`. See [Native YAML config](#native-yaml-config) below.

The config path defaults to `~/.config/joplin-desktop/settings.json`. Override with `GOJOPLIN_CONFIG_PATH` or the `--config` flag. If the path has a `.yaml` or `.yml` extension, the YAML format is used; otherwise Joplin JSON is expected.

When running in Docker, **mount the config file or directory read-only (`:ro`)** so the container cannot modify your settings.

### Getting values from Joplin settings.json

Joplin stores its configuration in **`settings.json`** (not `config.json`). Use this file either as the config source for go-joplin or to copy values into env vars / a YAML config.

**Where to find it:**

| Platform | Path |
|----------|------|
| Linux | `~/.config/joplin-desktop/settings.json` |
| macOS | `~/Library/Application Support/Joplin/settings.json` |
| Windows | `%APPDATA%\Joplin\settings.json` |

**View or copy values:**

- Open the file in a text editor, or
- From a shell (Linux/macOS): `cat ~/.config/joplin-desktop/settings.json` (or use `jq` for pretty-print: `jq . ~/.config/joplin-desktop/settings.json`).

**Keys used by go-joplin:**

| Key in settings.json | Used for | Env var / YAML equivalent |
|----------------------|----------|----------------------------|
| `api.token` | Web Clipper token (returned by `/auth`). | `GOJOPLIN_API_TOKEN` or `api.token` / `api.key` in YAML with `${GOJOPLIN_API_TOKEN}`. |
| `sync.target` | Sync backend: `8` = S3, `9` = Joplin Server. | YAML: `sync.target`. |
| `sync.8.path` | S3 bucket name. | YAML: `sync.s3.bucket`. |
| `sync.8.url` | S3 endpoint URL (e.g. MinIO or AWS). | YAML: `sync.s3.url`. |
| `sync.8.region` | S3 region. | YAML: `sync.s3.region`. |
| `sync.8.username` | S3 access key (if not using env). | `AWS_ACCESS_KEY_ID`. |
| `sync.8.password` | S3 secret key (if not using env). | `AWS_SECRET_ACCESS_KEY`. |
| `sync.8.forcePathStyle` | Use path-style URLs (e.g. MinIO). | YAML: `sync.s3.force_path_style`. |
| `sync.9.path` | Joplin Server URL. | YAML: `sync.joplin_server.url`. |
| `sync.9.username` | Joplin Server username. | `GOJOPLIN_USERNAME`. |
| `sync.9.password` | Joplin Server password. | `GOJOPLIN_PASSWORD`. |

For **Bearer auth** (clipper/MCP), use the same token as `api.token` or set a dedicated key via `GOJOPLIN_API_KEY` or `--api-key`. The `api.token` value is the one Joplin shows in **Tools → Web Clipper** after authorizing the clipper.

### Environment variables

Precedence: **env vars > CLI flags > config file**. All secrets should be provided via env vars when using the YAML config; when using Joplin settings.json, env vars override the file.

#### Required (secrets)

| Variable | Description | When |
|----------|-------------|------|
| `GOJOPLIN_API_TOKEN` | Joplin Web Clipper token (same as Joplin’s `api.token`). Used by `/auth` and `/auth/check`. | Always when using YAML config; otherwise from Joplin `api.token`. |
| `GOJOPLIN_API_KEY` | Bearer token for clipper and MCP requests. Can be the same as `GOJOPLIN_API_TOKEN`. | Required for non-`/health` requests; if unset, server returns 401. |
| `AWS_ACCESS_KEY_ID` | S3 access key. | When sync target is S3 (target 8). |
| `AWS_SECRET_ACCESS_KEY` | S3 secret key. | When sync target is S3 (target 8). |
| `GOJOPLIN_USERNAME` | Joplin Server username. | When sync target is Joplin Server (target 9). |
| `GOJOPLIN_PASSWORD` | Joplin Server password. | When sync target is Joplin Server (target 9). |

S3 credentials can also be provided as `ACCESS_KEY_ID` and `SECRET_ACCESS_KEY`. When using Joplin settings.json, `sync.8.username` and `sync.8.password` (and `sync.9.username` / `sync.9.password`) are read from the file if env vars are not set.

#### Optional (overrides and runtime)

| Variable | Description |
|----------|-------------|
| `GOJOPLIN_CONFIG_PATH` | Config file path (default: `~/.config/joplin-desktop/settings.json`). Use a `.yaml`/`.yml` path for native YAML config. |
| `GOJOPLIN_DATA_DIR` | Data directory for DB and resources (default: `~/.local/share/gojoplin`). |
| `GOJOPLIN_LISTEN_HOST` | Clipper server bind address (default: `localhost`; use `0.0.0.0` to listen on all interfaces). |
| `GOJOPLIN_PORT` | Clipper server port (default: 41184). |
| `GOJOPLIN_MASTER_PASSWORD` | E2EE master password for decrypting notes. |
| `GOJOPLIN_TRACING_ENABLED` | Enable OpenTelemetry tracing (default: true). |
| `GOJOPLIN_TRACING_PROTOCOL` | OTLP protocol: `http` or `grpc` (default: http). |
| `GOJOPLIN_TRACING_SERVICE_NAME` | Service name for traces (default: go-joplin). |
| `GOJOPLIN_TRACING_SAMPLE_RATE` | Trace sample rate 0–1 (default: 1.0). |
| `GOJOPLIN_METRICS_ENABLED` | Enable Prometheus metrics (default: true). |
| `GOJOPLIN_METRICS_PROMETHEUS_PORT` | Port for /metrics (default: 9091). |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP endpoint for traces (e.g. http://localhost:4318). |
| `GOJOPLIN_MCP_ALLOW_FOLDERS` | Comma-separated folder IDs or titles where notes can be created/updated; use `*` to allow all. |
| `GOJOPLIN_MCP_ALLOW_TAGS` | Comma-separated tag IDs or titles that can be attached to notes; use `*` to allow all. |
| `GOJOPLIN_MCP_ALLOW_CREATE_TAG` | Allow creating new tags (default: false). |
| `GOJOPLIN_MCP_ALLOW_CREATE_FOLDER` | Allow creating new folders (default: false). |

### Native YAML config

Use a YAML file when you want a single config file without storing secrets on disk. Copy `config.yaml.example` to e.g. `config.yaml`, set sync target and endpoints in the file, and provide all secrets via environment variables.

Example:

```bash
cp config.yaml.example config.yaml
# Edit config.yaml (sync target, bucket, URLs). Do not put secrets in the file.
export GOJOPLIN_API_TOKEN="your-joplin-web-clipper-token"
export GOJOPLIN_API_KEY="your-bearer-key"
export AWS_ACCESS_KEY_ID="your-s3-access-key"      # when using S3
export AWS_SECRET_ACCESS_KEY="your-s3-secret-key"
./go-joplin serve --config config.yaml
```

In the YAML file you can use `${VAR}` for values that come from the environment (e.g. `api.token: "${GOJOPLIN_API_TOKEN}"`). See `config.yaml.example` for the full structure.

- **Sync target 9 (Joplin Server)**  
  Set `sync.target` to 9 and configure server URL. With Joplin settings.json: `sync.9.path` (server URL), `sync.9.username`, `sync.9.password`. With YAML: `sync.joplin_server.url`; set `GOJOPLIN_USERNAME` and `GOJOPLIN_PASSWORD`. The clipper also needs the API token (Joplin’s Web Clipper `api.token` or `GOJOPLIN_API_TOKEN`).

- **Sync target 8 (S3)**  
  Set `sync.target` to 8. With Joplin settings.json: `sync.8.path` (bucket), `sync.8.url`, `sync.8.region`, `sync.8.forcePathStyle`; credentials can be in the file (`sync.8.username`, `sync.8.password`) or in env. With YAML: set `sync.s3.bucket`, `sync.s3.url`, `sync.s3.region`, `sync.s3.force_path_style`; credentials must be in env: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` (or `ACCESS_KEY_ID`, `SECRET_ACCESS_KEY`).

  The S3 client works with both official AWS S3 and S3-compatible storage (e.g. MinIO).

  **S3 503 / “XML syntax error … element &lt;hr&gt;”:** That usually means the endpoint URL is returning an **HTML error page** (e.g. from a proxy or load balancer) instead of the S3 API. Check that `sync.s3.url` (or `sync.8.url`) points at the real S3-compatible API root (e.g. `https://s3.example.com` with no extra path), that the service is up, and that it speaks S3 XML (not an HTML 503 page). You can probe the endpoint with `curl -I "https://your-endpoint/bucket"` (with auth if required).

- **Mutation allow-list (MCP and Clipper API)**  
  By default all mutations (create/update notes, folders, tags) are **read-only**. To allow writes:

  - `GOJOPLIN_MCP_ALLOW_FOLDERS`: Comma-separated folder names or IDs where notes can be created/updated. Use `*` to allow all folders.
  - `GOJOPLIN_MCP_ALLOW_TAGS`: Comma-separated tag names or IDs that can be attached to notes. Use `*` to allow all tags.
  - `GOJOPLIN_MCP_ALLOW_CREATE_TAG`: Set to `true` to allow creating new tags.
  - `GOJOPLIN_MCP_ALLOW_CREATE_FOLDER`: Set to `true` to allow creating new folders.

  Example (allow all mutations):
  ```bash
  export GOJOPLIN_MCP_ALLOW_FOLDERS="*"
  export GOJOPLIN_MCP_ALLOW_TAGS="*"
  export GOJOPLIN_MCP_ALLOW_CREATE_TAG=true
  export GOJOPLIN_MCP_ALLOW_CREATE_FOLDER=true
  ```

  Example (restrict to specific folders and tags):
  ```bash
  export GOJOPLIN_MCP_ALLOW_FOLDERS="Inbox,Notes"
  export GOJOPLIN_MCP_ALLOW_TAGS="work,personal"
  export GOJOPLIN_MCP_ALLOW_CREATE_TAG=true
  ```

  LLMs can discover allowed operations via the MCP resource `joplingo://capabilities` or the `get_capabilities` tool.

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
- `internal/config`: Load and validate config from Joplin settings.json (JSON) or native YAML (see `config.yaml.example`); observability and overrides from env.
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
