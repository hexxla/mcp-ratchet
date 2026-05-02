package mcp

import (
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewServer creates a new MCP server with the given name and version
func NewServer(name, version string, opts *mcp.ServerOptions) *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{
		Name:    name,
		Version: version,
	}, opts)
}

// StreamableHTTPHandler creates an HTTP handler for the MCP server
func StreamableHTTPHandler(srv *mcp.Server, log *slog.Logger) http.Handler {
	opts := &mcp.StreamableHTTPOptions{}
	if log != nil {
		opts.Logger = log
	}
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, opts)
}
