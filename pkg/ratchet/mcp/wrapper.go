// Package mcp provides utilities for integrating the ratchet mechanism with MCP (Model Context Protocol) tools.
package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/primary"
)

// WrapWithRatchet gates an MCP tool using the transport session and the workflow executor.
func WrapWithRatchet[In any, Out any](
	toolName domain.ToolName,
	handler func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error),
	workflow primary.WorkflowExecutor,
) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		var output Out
		if req == nil || req.Session == nil || req.Session.ID() == "" {
			return nil, output, fmt.Errorf("ratchet: tool %q requires an MCP session", toolName)
		}
		var result *mcp.CallToolResult
		err := workflow.ExecuteToolCall(ctx, domain.SessionID(req.Session.ID()), toolName, func() (bool, error) {
			var err error
			result, output, err = handler(ctx, req, input)
			return err == nil && (result == nil || !result.IsError), err
		})
		return result, output, err
	}
}
