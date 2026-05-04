package mcp

import (
	"context"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hexxla/mcp-ratchet/internal/core/domain"
	"github.com/hexxla/mcp-ratchet/internal/core/ports/primary"
	ratchetMCP "github.com/hexxla/mcp-ratchet/pkg/ratchet/mcp"
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
				&mcp.TextContent{Text: resp.Message()},
			},
		}, domain.GreetingResponse{}, nil
	}

	// Wrap with ratchet if provided
	if ratchet != nil {
		handler = ratchetMCP.WrapWithRatchet("greet", handler, ratchet, sessionStore, log)
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
				&mcp.TextContent{Text: resp.UserName()},
			},
		}, domain.UserIdentificationResponse{}, nil
	}

	// Wrap with ratchet if provided
	if ratchet != nil {
		handler = ratchetMCP.WrapWithRatchet("get_user_name", handler, ratchet, sessionStore, log)
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

		resp := getTimeResponse{Time: time.Now().UTC().Format(time.RFC3339)}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: resp.Time},
			},
		}, resp, nil
	}

	// Wrap with ratchet if provided
	if ratchet != nil {
		handler = ratchetMCP.WrapWithRatchet("get_time", handler, ratchet, sessionStore, log)
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_time",
		Description: "Returns the current time (for demo purposes, returns a fixed time)",
	}, handler)
}

// RegisterGetDateTool registers the get_date tool on the MCP server
func RegisterGetDateTool(server *mcp.Server, ratchet ratchetPorts.RatchetService, sessionStore ratchetSecondary.SessionStore, log *slog.Logger) {
	type getDateInput struct{}
	type getDateResponse struct {
		Date string `json:"date"`
	}

	handler := func(ctx context.Context, req *mcp.CallToolRequest, _ getDateInput) (*mcp.CallToolResult, getDateResponse, error) {
		if log != nil {
			log.DebugContext(ctx, "get_date tool invoked")
		}

		resp := getDateResponse{Date: time.Now().UTC().Format("2006-01-02")}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: resp.Date},
			},
		}, resp, nil
	}

	// Wrap with ratchet if provided
	if ratchet != nil {
		handler = ratchetMCP.WrapWithRatchet("get_date", handler, ratchet, sessionStore, log)
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_date",
		Description: "Returns the current date (for demo purposes, returns a fixed date)",
	}, handler)
}
