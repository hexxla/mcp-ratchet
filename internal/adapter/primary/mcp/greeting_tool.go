package mcp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hexxla/mcp-ratchet/internal/core/domain"
	"github.com/hexxla/mcp-ratchet/internal/core/ports/primary"
	ratchetDomain "github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	ratchetPorts "github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/primary"
	ratchetSecondary "github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
)

// RegisterGreetingTool registers the greet tool on the MCP server
func RegisterGreetingTool(server *mcp.Server, greeting primary.GreetingService, ratchet ratchetPorts.RatchetService, sessionStore ratchetSecondary.SessionStore, log *slog.Logger) {
	type greetInput struct {
		Name string `json:"name" jsonschema:"The name to greet"`
	}

	handler := func(ctx context.Context, req *mcp.CallToolRequest, input greetInput) (*mcp.CallToolResult, domain.GreetingResponse, error) {
		if log != nil {
			log.DebugContext(ctx, "greet tool invoked", "name", input.Name)
		}

		domainReq := domain.GreetingRequest{}
		domainReq.SetName(input.Name)
		resp, err := greeting.Greet(ctx, domainReq)
		if err != nil {
			return nil, domain.GreetingResponse{}, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: resp.GetMessage()},
			},
		}, domain.GreetingResponse{}, nil
	}

	// Wrap with ratchet if provided
	if ratchet != nil {
		originalHandler := handler
		wrappedHandler := func(ctx context.Context, req *mcp.CallToolRequest, input greetInput) (*mcp.CallToolResult, domain.GreetingResponse, error) {
			// Generate simple session ID from request (for demo, use fixed session)
			sessionID := ratchetDomain.SessionID("demo-session")

			// Get or create session
			session, err := sessionStore.Get(ctx, sessionID)
			if err != nil {
				session = ratchetDomain.NewSession(sessionID)
				if createErr := sessionStore.Create(ctx, session); createErr != nil {
					log.WarnContext(ctx, "failed to create session", "error", createErr)
				}
			}

			// Get stored token for this tool from session
			var token ratchetDomain.TokenValue
			if tokens, ok := session.Tokens["greet"]; ok && len(tokens) > 0 {
				token = tokens[len(tokens)-1] // Get the latest token
			}

			// Validate tool call
			err = ratchet.ValidateToolCall(ctx, sessionID, "greet", token)
			if err != nil {
				return nil, domain.GreetingResponse{}, fmt.Errorf("ratchet validation failed: %w", err)
			}

			// Execute the handler
			result, resp, err := originalHandler(ctx, req, input)
			if err != nil {
				return result, resp, err
			}

			// Issue token after successful execution (this retrieves and updates the session)
			_, err = ratchet.IssueToken(ctx, sessionID, "greet")
			if err != nil {
				log.WarnContext(ctx, "failed to issue ratchet token", "error", err)
			}

			// Re-fetch session after IssueToken to ensure we have the latest state
			session, err = sessionStore.Get(ctx, sessionID)
			if err != nil {
				log.WarnContext(ctx, "failed to get session after token issuance", "error", err)
			} else {
				// Record tool call in session AFTER issuing token
				session.RecordToolCall("greet")
				if updateErr := sessionStore.Update(ctx, session); updateErr != nil {
					log.WarnContext(ctx, "failed to update session with tool call", "error", updateErr)
				}
			}

			return result, resp, nil
		}
		handler = wrappedHandler
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "greet",
		Description: "Returns a greeting message for the given name",
	}, handler)
}

// RegisterGetUserNameTool registers the get_user_name tool on the MCP server
func RegisterGetUserNameTool(server *mcp.Server, user primary.UserService, ratchet ratchetPorts.RatchetService, sessionStore ratchetSecondary.SessionStore, log *slog.Logger) {
	type getUserInput struct{}

	handler := func(ctx context.Context, req *mcp.CallToolRequest, _ getUserInput) (*mcp.CallToolResult, domain.UserIdentificationResponse, error) {
		if log != nil {
			log.DebugContext(ctx, "get_user_name tool invoked")
		}

		resp, err := user.IdentifyUser(ctx, domain.UserIdentificationRequest{})
		if err != nil {
			return nil, domain.UserIdentificationResponse{}, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: resp.GetUserName()},
			},
		}, domain.UserIdentificationResponse{}, nil
	}

	// Wrap with ratchet if provided
	if ratchet != nil {
		originalHandler := handler
		wrappedHandler := func(ctx context.Context, req *mcp.CallToolRequest, input getUserInput) (*mcp.CallToolResult, domain.UserIdentificationResponse, error) {
			// Generate simple session ID from request (for demo, use fixed session)
			sessionID := ratchetDomain.SessionID("demo-session")

			// Get or create session
			session, err := sessionStore.Get(ctx, sessionID)
			if err != nil {
				session = ratchetDomain.NewSession(sessionID)
				if createErr := sessionStore.Create(ctx, session); createErr != nil {
					log.WarnContext(ctx, "failed to create session", "error", createErr)
				}
			}

			// Get stored token for this tool from session
			var token ratchetDomain.TokenValue
			if tokens, ok := session.Tokens["get_user_name"]; ok && len(tokens) > 0 {
				token = tokens[len(tokens)-1] // Get the latest token
			}

			// Validate tool call
			err = ratchet.ValidateToolCall(ctx, sessionID, "get_user_name", token)
			if err != nil {
				return nil, domain.UserIdentificationResponse{}, fmt.Errorf("ratchet validation failed: %w", err)
			}

			// Execute the handler
			result, resp, err := originalHandler(ctx, req, input)
			if err != nil {
				return result, resp, err
			}

			// Issue token after successful execution (this retrieves and updates the session)
			_, err = ratchet.IssueToken(ctx, sessionID, "get_user_name")
			if err != nil {
				log.WarnContext(ctx, "failed to issue ratchet token", "error", err)
			}

			// Re-fetch session after IssueToken to ensure we have the latest state
			session, err = sessionStore.Get(ctx, sessionID)
			if err != nil {
				log.WarnContext(ctx, "failed to get session after token issuance", "error", err)
			} else {
				// Record tool call in session AFTER issuing token
				session.RecordToolCall("get_user_name")
				if updateErr := sessionStore.Update(ctx, session); updateErr != nil {
					log.WarnContext(ctx, "failed to update session with tool call", "error", updateErr)
				}
			}

			return result, resp, nil
		}
		handler = wrappedHandler
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_user_name",
		Description: "Returns the current user's name (for demo purposes, returns 'DemoUser')",
	}, handler)
}

// RegisterGetTimeTool registers the get_time tool on the MCP server
func RegisterGetTimeTool(server *mcp.Server, ratchet ratchetPorts.RatchetService, sessionStore ratchetSecondary.SessionStore, log *slog.Logger) {
	type getTimeInput struct{}
	type getTimeResponse struct {
		Time string `json:"time"`
	}

	handler := func(ctx context.Context, req *mcp.CallToolRequest, _ getTimeInput) (*mcp.CallToolResult, getTimeResponse, error) {
		if log != nil {
			log.DebugContext(ctx, "get_time tool invoked")
		}

		resp := getTimeResponse{Time: "2026-05-02T15:00:00Z"}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: resp.Time},
			},
		}, resp, nil
	}

	// Wrap with ratchet if provided
	if ratchet != nil {
		originalHandler := handler
		wrappedHandler := func(ctx context.Context, req *mcp.CallToolRequest, input getTimeInput) (*mcp.CallToolResult, getTimeResponse, error) {
			// Generate simple session ID from request (for demo, use fixed session)
			sessionID := ratchetDomain.SessionID("demo-session")

			// Get or create session
			session, err := sessionStore.Get(ctx, sessionID)
			if err != nil {
				session = ratchetDomain.NewSession(sessionID)
				if createErr := sessionStore.Create(ctx, session); createErr != nil {
					log.WarnContext(ctx, "failed to create session", "error", createErr)
				}
			}

			// Get stored token for this tool from session
			var token ratchetDomain.TokenValue
			if tokens, ok := session.Tokens["get_time"]; ok && len(tokens) > 0 {
				token = tokens[len(tokens)-1] // Get the latest token
			}

			// Validate tool call
			err = ratchet.ValidateToolCall(ctx, sessionID, "get_time", token)
			if err != nil {
				return nil, getTimeResponse{}, fmt.Errorf("ratchet validation failed: %w", err)
			}

			// Execute the handler
			result, resp, err := originalHandler(ctx, req, input)
			if err != nil {
				return result, resp, err
			}

			// Issue token after successful execution (this retrieves and updates the session)
			_, err = ratchet.IssueToken(ctx, sessionID, "get_time")
			if err != nil {
				log.WarnContext(ctx, "failed to issue ratchet token", "error", err)
			}

			// Re-fetch session after IssueToken to ensure we have the latest state
			session, err = sessionStore.Get(ctx, sessionID)
			if err != nil {
				log.WarnContext(ctx, "failed to get session after token issuance", "error", err)
			} else {
				// Record tool call in session AFTER issuing token
				session.RecordToolCall("get_time")
				if updateErr := sessionStore.Update(ctx, session); updateErr != nil {
					log.WarnContext(ctx, "failed to update session with tool call", "error", updateErr)
				}
			}

			return result, resp, nil
		}
		handler = wrappedHandler
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_time",
		Description: "Returns the current time (for demo purposes, returns a fixed time)",
	}, handler)
}
