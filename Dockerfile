# Multi-stage Dockerfile for go-joplin
# Build stage uses upstream golang image; app stage uses distroless.
# Note: mattn/go-sqlite3 requires CGO and glibc, so we use base-debian13 (not static).

ARG GO_VERSION=1.24
FROM golang:${GO_VERSION}-bookworm AS builder

WORKDIR /build

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and run tests
COPY . .
RUN go test ./...

# Build binary (CGO required for sqlite3)
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o go-joplin .

# App stage: minimal distroless image with glibc (required for go-sqlite3)
FROM gcr.io/distroless/base-debian13:nonroot

COPY --from=builder /build/go-joplin /go-joplin

ENTRYPOINT ["/go-joplin"]
