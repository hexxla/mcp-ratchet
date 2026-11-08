// Package mcp provides utilities for integrating the ratchet mechanism with MCP (Model Context Protocol) tools.
package mcp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/primary"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
)

// WrapWithRatchet wraps an MCP tool handler with ratchet validation logic.
// The service must implement [primary.WorkflowExecutor]. The store and logger
// arguments are retained for source compatibility.
//
// This helper eliminates ~50 lines of boilerplate code per tool when using ratchet rules.
//
// Example usage:
//
//	handler := func(ctx context.Context, req *mcp.CallToolRequest, input MyInput) (*mcp.CallToolResult, MyOutput, error) {
//	    // Your tool logic here
//	}
//
//	if ratchet != nil {
//	    handler = mcp.WrapWithRatchet("my_tool", handler, ratchet, sessionStore, log)
//	}
//
//	mcp.AddTool(server, &mcp.Tool{Name: "my_tool"}, handler)
func WrapWithRatchet[In any, Out any](
	toolName domain.ToolName,
	handler func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error),
	ratchet primary.RatchetService,
	_ secondary.SessionStore,
	_ *slog.Logger,
) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error) {
	workflow, supported := ratchet.(primary.WorkflowExecutor)
	return func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		var output Out
		if !supported {
			return nil, output, fmt.Errorf("ratchet: workflow execution is required for %q", toolName)
		}
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
