# mcp-ratchet

A Go package for enforcing tool call order in MCP servers using configurable token-based dependencies.

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
    "fmt"

    "github.com/modelcontextprotocol/go-sdk/mcp"
    ratchetDomain "github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
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
func RegisterMyTool(server *mcp.Server, ratchet ratchetPorts.RatchetService, sessionStore ratchetSecondary.SessionStore) {
    originalHandler := myToolHandler

    // Wrap with ratchet validation
    wrappedHandler := func(ctx context.Context, req *mcp.CallToolRequest, input MyToolInput) (*mcp.CallToolResult, MyToolOutput, error) {
        sessionID := ratchetDomain.SessionID("session-123")

        // Get or create session
        session, err := sessionStore.Get(ctx, sessionID)
        if err != nil {
            session = ratchetDomain.NewSession(sessionID)
            sessionStore.Create(ctx, session)
        }

        // Get existing token for this tool
        var token ratchetDomain.TokenValue
        if tokens, ok := session.Tokens["my_tool"]; ok && len(tokens) > 0 {
            token = tokens[len(tokens)-1]
        }

        // Validate tool call
        err = ratchet.ValidateToolCall(ctx, sessionID, "my_tool", token)
        if err != nil {
            return nil, MyToolOutput{}, fmt.Errorf("ratchet validation failed: %w", err)
        }

        // Execute original handler
        result, output, err := originalHandler(ctx, req, input)
        if err != nil {
            return result, output, err
        }

        // Issue token after successful execution
        _, err = ratchet.IssueToken(ctx, sessionID, "my_tool")
        if err != nil {
            return result, output, fmt.Errorf("failed to issue token: %w", err)
        }

        // Update session
        session.RecordToolCall("my_tool")
        sessionStore.Update(ctx, session)

        return result, output, nil
    }

    // Register wrapped tool with MCP SDK
    mcp.AddTool(server, &mcp.Tool{
        Name:        "my_tool",
        Description: "Description of your tool",
    }, wrappedHandler)
}
```

### Configuration

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
