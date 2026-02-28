package mcp

import (
	"net/http"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server is the MCP server type from the official SDK. Exported so callers can type the SSE handler callback.
type Server = sdkmcp.Server

const (
	implementationName    = "joplingo"
	implementationVersion = "v1.0.0"
)

// NewServer creates an MCP server with all tools registered. Easy to modify: edit RegisterAll in tools.go.
func NewServer(d *Deps) *Server {
	server := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    implementationName,
		Version: implementationVersion,
	}, nil)
	RegisterAll(server, d)
	return server
}

// NewSSEHandler returns an http.Handler that serves MCP over SSE. Accepts GET (new session) and POST (message to session).
// Mount at e.g. /mcp. Caller must apply Bearer auth before this handler if required.
func NewSSEHandler(getServer func(*http.Request) *Server) http.Handler {
	return sdkmcp.NewSSEHandler(getServer, nil)
}
