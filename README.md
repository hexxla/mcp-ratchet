# mcp-ratchet

A Go package for enforcing tool call order in MCP servers using configurable token-based dependencies.

## Problem

LLMs often attempt to call tools in the wrong order or without necessary prerequisites, leading to:

- Failed API calls due to missing authentication or setup
- Incorrect results from tools that depend on prior state
- Confusion when complex workflows require specific sequences
- Difficulty enforcing systematic patterns in agent behavior
- **Contextless use of critical tools** that require proper setup or state

## Solution

mcp-ratchet enforces tool call order and compliance through a token-based system:

- Tools require tokens from prerequisite tools before they can be called
- Tokens are issued after successful tool execution
- Tokens can expire or be one-time use
- Multi-level dependency chains are supported
- Configuration is via simple YAML files
- **Custom error messages guide LLMs to the correct next step**
- **Enforces compliance for critical tools that cannot afford contextless use**

This ensures LLMs follow the correct workflow every time, with clear error messages guiding them when prerequisites are missing. Critical tools (e.g., authentication, setup, data modification) are protected from being called without the necessary context or prior steps.

## Quick Start

```bash
go get github.com/hexxla/mcp-ratchet
```

## Usage

Import the package:

```go
import (
    "github.com/hexxla/mcp-ratchet/pkg/ratchet/adapters"
    ratchetDomain "github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
    ratchetPorts "github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/primary"
    ratchetSecondary "github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
    "github.com/hexxla/mcp-ratchet/pkg/ratchet/services"
)
```

### Initialize the Service

```go
configLoader := adapters.NewYAMLConfigLoader()
tokenStore := adapters.NewMemoryTokenStore()
sessionStore := adapters.NewMemorySessionStore()
randomGen := adapters.NewCryptoRandomGenerator()
clock := adapters.NewRealClock()

ratchetSvc := services.NewRatchetService(configLoader, tokenStore, sessionStore, randomGen, clock)

// Load configuration
rules, err := ratchetSvc.LoadConfiguration(ctx, configFile)
```

### Wrap Your Tool Handler

Using the Go MCP SDK, wrap your tool handler with ratchet validation:

```go
import (
    "context"
    "log/slog"

    "github.com/modelcontextprotocol/go-sdk/mcp"
    ratchetMCP "github.com/hexxla/mcp-ratchet/pkg/ratchet/mcp"
    ratchetPorts "github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/primary"
    ratchetSecondary "github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
)

// Original tool handler (MCP SDK format)
func myToolHandler(ctx context.Context, req *mcp.CallToolRequest, input MyToolInput) (*mcp.CallToolResult, MyToolOutput, error) {
    // Your tool logic here
    output := MyToolOutput{Result: "success"}
    return &mcp.CallToolResult{
        Content: []mcp.Content{
            &mcp.TextContent{Text: output.Result},
        },
    }, output, nil
}

// Register tool with ratchet wrapping
func RegisterMyTool(server *mcp.Server, ratchet ratchetPorts.RatchetService, sessionStore ratchetSecondary.SessionStore, log *slog.Logger) {
    handler := myToolHandler

    // Wrap with ratchet validation in one line
    // This handles: session management, token validation, prerequisite consumption,
    // token issuance, and session updates automatically
    if ratchet != nil {
        handler = ratchetMCP.WrapWithRatchet("my_tool", handler, ratchet, sessionStore, log)
    }

    // Register wrapped tool with MCP SDK
    mcp.AddTool(server, &mcp.Tool{
        Name:        "my_tool",
        Description: "Description of your tool",
    }, handler)
}
```

#### Advanced: Manual Wrapping

For custom session ID derivation or fine-grained control, you can implement wrapping manually:

```go
import (
    "context"
    "fmt"

    "github.com/modelcontextprotocol/go-sdk/mcp"
    ratchetDomain "github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
)

func manualWrap(ctx context.Context, req *mcp.CallToolRequest, input MyToolInput,
    originalHandler MyToolHandler, ratchet RatchetService, sessionStore SessionStore) (*mcp.CallToolResult, MyToolOutput, error) {

    // Custom session ID derivation from request context
    sessionID := deriveSessionIDFromRequest(req) // e.g., from headers, auth, etc.

    // Get or create session
    session, err := sessionStore.Get(ctx, sessionID)
    if err != nil {
        session = ratchetDomain.NewSession(sessionID)
        sessionStore.Create(ctx, session)
    }

    // Validate and execute
    var token ratchetDomain.TokenValue
    if tokens, ok := session.Tokens["my_tool"]; ok && len(tokens) > 0 {
        token = tokens[len(tokens)-1]
    }

    if err := ratchet.ValidateToolCall(ctx, sessionID, "my_tool", token); err != nil {
        return nil, MyToolOutput{}, fmt.Errorf("ratchet validation failed: %w", err)
    }

    result, output, err := originalHandler(ctx, req, input)
    if err != nil {
        return result, output, err
    }

    // Consume prerequisite and issue new token
    ratchet.ConsumePrerequisiteToken(ctx, sessionID, "my_tool")
    ratchet.IssueToken(ctx, sessionID, "my_tool")

    return result, output, nil
}
```

### YAML Configuration

Define tool dependencies in YAML:

```yaml
rules:
  - tool: greet
    prerequisite: ""
    expiry: 5m
    error_message: ""
    one_time_use: false

  - tool: get_user_name
    prerequisite: greet
    expiry: 10m
    error_message: "You must call the 'greet' tool before calling 'get_user_name'."
    one_time_use: false
```

**Fields:**

- `tool` - Tool name
- `prerequisite` - Tool that must be called first (empty for none)
- `expiry` - Token expiration (e.g., `5m`, `10m`, `1h`)
- `error_message` - Custom error message for validation failures
- `one_time_use` - If `true`, token is consumed after one use (default: `false`)

### Multi-level Chains

Tools can depend on tools that have prerequisites:

```yaml
rules:
  - tool: greet # Level 1
    prerequisite: ""
  - tool: get_user_name # Level 2
    prerequisite: greet
  - tool: get_time # Level 3
    prerequisite: get_user_name
  - tool: get_date # Level 4
    prerequisite: get_time
```

Each tool validates against its direct prerequisite only.

### MCP Tool Agnostic

The Ratchet package is framework-agnostic - it doesn't depend on any specific MCP SDK or tool system. It provides two main methods:

- `ValidateToolCall(ctx, sessionID, toolName, token)` - Validates prerequisites and token validity
- `IssueToken(ctx, sessionID, toolName)` - Issues a token after successful tool execution

Your tool system (MCP server, gRPC service, HTTP API, CLI tool, etc.) integrates with ratchet by:

1. Providing a session ID for tracking
2. Calling `ValidateToolCall()` before tool execution
3. Calling `IssueToken()` after successful execution

### Default Behavior

If no config rule is defined for a tool, it is unrestricted and works without prerequisites.

## How It Works

### Token Flow

1. **Tool Called Without Prerequisite**: Validation passes immediately
2. **Tool With Prerequisite Called**:
   - Validates that prerequisite tool has been called
   - Checks that prerequisite's token is valid (not expired)
   - If one-time use is enabled, consumes the token
3. **After Successful Execution**: Issues a new token for this tool
4. **Token Storage**: Tokens stored in both session and token store for expiry tracking

### Managing Sessions

Sessions track:

- Which tools have been called (`ToolHistory`)
- Valid tokens for each tool (`Tokens` map)
- Session creation time (`CreatedAt`)

Sessions are identified by a `SessionID` (string) that you provide in your application code. Common sources:

- User ID from authentication
- Conversation ID from a chat system
- Request ID or correlation ID
- Any unique identifier for the session/workflow

## Best Practices

### Rule Configuration

- Use descriptive error messages that guide LLMs to the correct next step
- Set appropriate expiry times based on your workflow (short for sensitive operations, longer for setup steps)
- Use `one_time_use: true` for operations that should not be repeated (e.g., setup, authentication)
- Test dependency chains incrementally to ensure each step works before adding complexity

### Managing Sessions

- Use consistent session IDs (e.g., user ID, conversation ID)
- Consider session lifecycle (when to create, when to expire)
- Monitor token usage patterns to identify workflow issues

### Error Messages

- Write error messages that are actionable for LLMs
- Include the exact tool name that needs to be called
- Provide context about what the prerequisite accomplishes

## Troubleshooting

### Common Issues

**Issue**: Tool call fails with "You must call the 'X' tool before calling 'Y'"

- **Cause**: Prerequisite tool hasn't been called or token expired
- **Solution**: Call the prerequisite tool first, or check token expiry configuration

**Issue**: Token consumed after one use but should be reusable

- **Cause**: `one_time_use: true` is set in config
- **Solution**: Set `one_time_use: false` for reusable tokens

**Issue**: Multi-level chain not working

- **Cause**: Tools depending on wrong prerequisite in chain
- **Solution**: Ensure each tool depends on its direct predecessor, not the chain root

**Issue**: Token expiry too short/long

- **Cause**: Expiry duration in config doesn't match workflow needs
- **Solution**: Adjust `expiry` field (e.g., `5m`, `1h`, `24h`)
