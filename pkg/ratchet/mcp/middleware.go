// Package mcp integrates Ratchet workflows with the MCP SDK.
package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Executor is the workflow behavior consumed by this MCP adapter. Applications
// may implement it to filter calls before delegating to a ratchet.Engine.
type Executor interface {
	ExecuteToolCall(context.Context, string, string, func() (bool, error)) error
}

// NewMiddleware gates tools/call requests and records success only after the SDK
// has validated and serialized the tool output. Install it once with
// Server.AddReceivingMiddleware, and register tools with the SDK normally.
// The transport must supply an isolated, nonempty session ID.
// Executed results become private JSON snapshots implementing mcp.Result;
// their dynamic Go type is not *mcp.CallToolResult. Inspect output through SDK
// interfaces; output-decorating application middleware must run inside Ratchet.
// SDK server-info metadata decoration remains supported. Clients still receive
// ordinary CallToolResult values. Input-required responses are not completions;
// a continuation RPC must satisfy the policy again, with fresh consumed prerequisites.
func NewMiddleware(workflow Executor) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}
			call, ok := req.(*mcp.CallToolRequest)
			if !ok || call == nil || call.Params == nil || call.Params.Name == "" {
				return toolError(fmt.Errorf("ratchet: invalid tools/call request")), nil
			}
			if call.Session == nil || call.Session.ID() == "" {
				return toolError(fmt.Errorf("ratchet: tool %q requires an MCP session", call.Params.Name)), nil
			}
			if workflow == nil {
				return toolError(fmt.Errorf("ratchet: workflow executor is required")), nil
			}
			var result mcp.Result
			var handlerErr error
			err := workflow.ExecuteToolCall(ctx, call.Session.ID(), call.Params.Name, func() (bool, error) {
				result, handlerErr = next(ctx, method, req)
				if handlerErr != nil {
					return false, handlerErr
				}
				toolResult, ok := result.(*mcp.CallToolResult)
				if !ok || toolResult == nil {
					handlerErr = fmt.Errorf("ratchet: invalid tools/call result")
					return false, handlerErr
				}
				frozen, err := freezeToolResult(toolResult)
				if err != nil {
					handlerErr = err
					return false, err
				}
				result = frozen
				return !frozen.IsError && !frozen.NeedsInput(), nil
			})
			if handlerErr != nil {
				return nil, handlerErr
			}
			if err != nil {
				return toolError(err), nil
			}
			if result == nil {
				return toolError(fmt.Errorf("ratchet: workflow did not execute the tool")), nil
			}
			return result, nil
		}
	}
}

func toolError(err error) *mcp.CallToolResult {
	result := &mcp.CallToolResult{}
	result.SetError(err)
	return result
}
