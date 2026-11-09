package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	ratchetMCP "github.com/hexxla/mcp-ratchet/pkg/ratchet/mcp"
	ratchetPorts "github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/primary"
)

// registerGreetingTool registers the greet tool on the MCP server
func registerGreetingTool(server *mcp.Server, ratchet ratchetPorts.WorkflowExecutor, log *slog.Logger) {
	type greetInput struct {
		Name string `json:"name" jsonschema:"The name to greet"`
	}

	handler := func(ctx context.Context, req *mcp.CallToolRequest, input greetInput) (*mcp.CallToolResult, struct{}, error) {
		if log != nil {
			log.DebugContext(ctx, "greet tool invoked", "name", input.Name)
		}

		if input.Name == "" {
			return nil, struct{}{}, fmt.Errorf("name cannot be empty")
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Hello, " + input.Name + "!"},
			},
		}, struct{}{}, nil
	}

	// Wrap with ratchet if provided
	if ratchet != nil {
		handler = ratchetMCP.WrapWithRatchet("greet", handler, ratchet)
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "greet",
		Description: "Returns a greeting message for the given name",
	}, handler)
}

// registerGetUserNameTool registers the get_user_name tool on the MCP server
func registerGetUserNameTool(server *mcp.Server, ratchet ratchetPorts.WorkflowExecutor, log *slog.Logger) {
	type getUserInput struct{}

	handler := func(ctx context.Context, req *mcp.CallToolRequest, _ getUserInput) (*mcp.CallToolResult, struct{}, error) {
		if log != nil {
			log.DebugContext(ctx, "get_user_name tool invoked")
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "DemoUser"},
			},
		}, struct{}{}, nil
	}

	// Wrap with ratchet if provided
	if ratchet != nil {
		handler = ratchetMCP.WrapWithRatchet("get_user_name", handler, ratchet)
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_user_name",
		Description: "Returns the current user's name (for demo purposes, returns 'DemoUser')",
	}, handler)
}

// registerGetTimeTool registers the get_time tool on the MCP server
func registerGetTimeTool(server *mcp.Server, ratchet ratchetPorts.WorkflowExecutor, log *slog.Logger) {
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
		handler = ratchetMCP.WrapWithRatchet("get_time", handler, ratchet)
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_time",
		Description: "Returns the current time",
	}, handler)
}

// registerGetDateTool registers the get_date tool on the MCP server
func registerGetDateTool(server *mcp.Server, ratchet ratchetPorts.WorkflowExecutor, log *slog.Logger) {
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
		handler = ratchetMCP.WrapWithRatchet("get_date", handler, ratchet)
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_date",
		Description: "Returns the current date",
	}, handler)
}
