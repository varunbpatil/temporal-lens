// Package mcp provides the Model Context Protocol inbound adapter.
package mcp

import (
	"net/http"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/varunbpatil/temporal-lens/version"
)

// New creates a Temporal Lens MCP server. Domain packages register their
// tools with the returned server.
func New() *gomcp.Server {
	return gomcp.NewServer(&gomcp.Implementation{
		Name:        "temporal-lens",
		Title:       "Temporal Lens",
		Description: "Search indexed Temporal workflows and manage their executions.",
		Version:     version.Version,
	}, &gomcp.ServerOptions{
		Instructions: "Temporal Lens exposes domain-specific tools and documentation resources. " +
			"List resources to discover each domain's usage guide before calling its tools.",
	})
}

// Handler exposes server through the Streamable HTTP MCP transport.
func Handler(server *gomcp.Server) http.Handler {
	return gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server {
		return server
	}, nil)
}
