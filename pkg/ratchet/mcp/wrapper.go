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
// It handles session management, token validation, prerequisite consumption, and token issuance.
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
	handler func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error),
	ratchet primary.RatchetService,
	sessionStore secondary.SessionStore,
	log *slog.Logger,
) func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		// Generate simple session ID from request (for demo, use fixed session)
		sessionID := domain.SessionID("demo-session")

		// Get or create session — creation goes through ratchet service to emit session_created event
		s, err := sessionStore.Get(ctx, sessionID)
		if err != nil {
			var createErr error
			s, createErr = ratchet.CreateSession(ctx, sessionID)
			if createErr != nil {
				if log != nil {
					log.WarnContext(ctx, "failed to create session", "error", createErr)
				}
				s = domain.NewSession(sessionID)
			}
		}

		// Get stored token for this tool from session
		var token domain.TokenValue
		if tokens, ok := s.Tokens[toolName]; ok && len(tokens) > 0 {
			token = tokens[len(tokens)-1] // Get the latest token
		}

		// Validate tool call
		err = ratchet.ValidateToolCall(ctx, sessionID, toolName, token)
		if err != nil {
			var zero Out
			return nil, zero, fmt.Errorf("ratchet validation failed: %w", err)
		}

		// Execute the handler
		result, resp, err := handler(ctx, req, input)
		if err != nil {
			return result, resp, err
		}

		// Consume prerequisite token for one-time-use rules after successful execution
		if err := ratchet.ConsumePrerequisiteToken(ctx, sessionID, toolName); err != nil {
			if log != nil {
				log.WarnContext(ctx, "failed to consume prerequisite token", "error", err)
			}
		}

		// Issue token after successful execution
		_, err = ratchet.IssueToken(ctx, sessionID, toolName)
		if err != nil {
			if log != nil {
				log.WarnContext(ctx, "failed to issue ratchet token", "error", err)
			}
		}

		// Re-fetch session after IssueToken to ensure we have the latest state
		s, err = sessionStore.Get(ctx, sessionID)
		if err != nil {
			if log != nil {
				log.WarnContext(ctx, "failed to get session after token issuance", "error", err)
			}
		} else {
			// Record tool call in session AFTER issuing token
			s.RecordToolCall(toolName)
			if updateErr := sessionStore.Update(ctx, s); updateErr != nil {
				if log != nil {
					log.WarnContext(ctx, "failed to update session with tool call", "error", updateErr)
				}
			}
		}

		return result, resp, nil
	}
}
