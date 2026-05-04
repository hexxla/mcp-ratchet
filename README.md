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

## Observability

mcp-ratchet captures lifecycle events for visibility into tool usage patterns, token flow, and validation failures. Events are **non-intrusive** — they are emitted as side-effects and never affect core ratchet behavior.

### Enabling Observability

Add the `observability` section to your YAML configuration:

```yaml
observability:
  enabled: true
  storage_type: memory # Options: memory, hexxladb, sql
  retention_days: 7 # 0 = keep all events

rules:
  - tool: greet
    prerequisite: ""
    expiry: 5m
```

### Captured Events

| Event Type          | Triggered When               | Data Included                        |
| ------------------- | ---------------------------- | ------------------------------------ |
| `tool_call_attempt` | Tool validation starts       | SessionID, ToolName, Token           |
| `tool_call_success` | Tool validation passes       | SessionID, ToolName                  |
| `tool_call_failure` | Tool validation fails        | SessionID, ToolName, Error message   |
| `token_created`     | Token issued after execution | SessionID, ToolName, Token, Expiry   |
| `token_consumed`    | One-time-use token consumed  | SessionID, ToolName, Token, Consumer |
| `session_created`   | New session initialized      | SessionID                            |

### Service Layer Integration

Import mcp-ratchet and use the observability-enabled constructor:

```go
import (
    "github.com/hexxla/mcp-ratchet/pkg/ratchet/adapters"
    ratchetDomain "github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
    ratchetPorts "github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/primary"
    ratchetSecondary "github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
    ratchetServices "github.com/hexxla/mcp-ratchet/pkg/ratchet/services"
)

// Load configuration (includes observability settings)
configLoader := adapters.NewYAMLConfigLoader()
fullCfg, err := configLoader.LoadConfig(ctx, configFile)

// Create EventStore from factory (memory, hexxladb, or sql)
eventStore, err := adapters.NewEventStore(fullCfg.Observability)

// Initialize service with observability
ratchetSvc := ratchetServices.NewRatchetServiceWithObservability(
    configLoader,
    adapters.NewMemoryTokenStore(),
    adapters.NewMemorySessionStore(),
    adapters.NewCryptoRandomGenerator(),
    adapters.NewRealClock(),
    eventStore,  // nil = disabled
)
```

### Querying Events

Your service layer can query captured events for dashboards, debugging, or analytics:

```go
// Get aggregate statistics
stats, _ := ratchetSvc.GetObservabilityStats(ctx)
fmt.Printf("Total events: %d, Tokens issued: %d, Failures: %d\n",
    stats.TotalEvents, stats.TokensIssued, stats.EventsByType["tool_call_failure"])

// Get events for a specific session
events, _ := ratchetSvc.GetObservabilityEvents(ctx, sessionID, &ratchetSecondary.EventFilter{
    EventTypes: []ratchetDomain.EventType{
        ratchetDomain.EventTypeToolCallFailure,
    },
    Limit: 10,
})

for _, e := range events {
    fmt.Printf("%s: %s failed - %v\n", e.Timestamp, e.ToolName, e.Metadata["error"])
}
```

### Custom Event Store (Database Integration)

Implement the `EventStore` interface for your database:

```go
// HexxlaDBEventStore implements EventStore for HexxlaDB
type HexxlaDBEventStore struct {
    client *hexxladb.Client
    config ratchetDomain.ObservabilityConfig
}

func (h *HexxlaDBEventStore) Store(ctx context.Context, event *ratchetDomain.Event) error {
    // Store event as a cell in HexxlaDB
    cell := hexxladb.Cell{
        Tags: []string{"ratchet_event", string(event.Type)},
        Data: event,
    }
    return h.client.PutCell(ctx, cell)
}

func (h *HexxlaDBEventStore) GetEvents(ctx context.Context,
    sessionID ratchetDomain.SessionID,
    filter *ratchetSecondary.EventFilter) ([]*ratchetDomain.Event, error) {
    // Query cells by tags and session
    return h.client.QueryCells(ctx, hexxladb.Query{
        Tags: []string{"ratchet_event"},
        SessionID: string(sessionID),
    })
}

func (h *HexxlaDBEventStore) GetStats(ctx context.Context) (*ratchetDomain.EventStats, error) {
    // Aggregate from stored cells
    // ... implementation
}
```

Register in the factory:

```go
func NewEventStore(cfg ratchetDomain.ObservabilityConfig, hexxlaClient *hexxladb.Client) (ratchetSecondary.EventStore, error) {
    if !cfg.Enabled {
        return nil, nil
    }
    switch cfg.StorageType {
    case "memory":
        return NewMemoryEventStore(cfg.RetentionDays), nil
    case "hexxladb":
        return NewHexxlaDBEventStore(hexxlaClient, cfg), nil
    default:
        return nil, fmt.Errorf("unsupported storage_type: %s", cfg.StorageType)
    }
}
```

### HTTP Endpoint (Web UI)

Add an HTTP endpoint to expose events for a web UI:

```go
// ObservabilityHandler serves ratchet events via HTTP
type ObservabilityHandler struct {
    ratchetSvc ratchetPorts.RatchetService
}

func (h *ObservabilityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    switch r.URL.Path {
    case "/observability/stats":
        stats, err := h.ratchetSvc.GetObservabilityStats(r.Context())
        if err != nil {
            http.Error(w, err.Error(), 500)
            return
        }
        json.NewEncoder(w).Encode(stats)

    case "/observability/events":
        sessionID := r.URL.Query().Get("session_id")
        events, err := h.ratchetSvc.GetObservabilityEvents(r.Context(),
            ratchetDomain.SessionID(sessionID), nil)
        if err != nil {
            http.Error(w, err.Error(), 500)
            return
        }
        json.NewEncoder(w).Encode(events)
    }
}
```

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
