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

Use Go 1.27.3 or newer.

```bash
go get github.com/hexxla/mcp-ratchet
```

The library is distributed through Go module tags. The
[module tag workflow](.github/workflows/tag.yml) validates tags reachable
from `main`. Verify CI on `main` before pushing a version tag. To build the
demonstration binary locally, run `make build`.


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

#### Custom transports

Use the service's [WorkflowExecutor](pkg/ratchet/ports/primary/ratchet_service.go) with a stable, isolated transport session ID. The callback reports both handler errors and application-level failure. The service selects and reserves one-time alternatives before invoking the callback and records successful execution afterwards.

The public MCP wrapper requires a transport that exposes a session ID, such as stateful Streamable HTTP. For transports that omit IDs, supply a session identity through your own adapter and call the workflow port directly. The wrapper's legacy store and logger arguments remain accepted for source compatibility.

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

Ratchet's service is framework-agnostic. Transport adapters call `WorkflowExecutor.ExecuteToolCall` with the session ID, tool name and callback. The service handles the session, reservation and completion as one workflow.

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

### Configuration Separation

mcp-ratchet separates configuration into two distinct concerns:

**ratchet.yaml** - Core ratchet library settings:

```yaml
# Core ratchet settings - the library captures and stores events
observability:
  enabled: true # Ratchet: capture events?
  storage_type: memory # Ratchet: where to store?
  retention_days: 0 # Ratchet: how long to keep?

rules:
  - tool: greet
    prerequisite: ""
    expiry: 15s
    one_time_use: false
```

**mcp-config.yaml** - Server/presentation layer settings:

```yaml
# Server-level settings - how to expose ratchet functionality
observability:
  http_enabled: true # Server: expose REST endpoints?
  websocket_enabled: true # Server: expose WebSocket?
  websocket_path: "/observability/stream"

server:
  addr: "127.0.0.1:8080"
  mcp_path: "/mcp"
```

This separation keeps ratchet core independent of how events are exposed (HTTP, WebSocket, etc.).

### Factory Pattern for Custom Event Stores

Use the functional options pattern to inject custom EventStore implementations:

```go
import (
    "github.com/hexxla/mcp-ratchet/pkg/ratchet/adapters"
)

// Create your custom EventStore (e.g., HexxlaDB)
hexxlaStore := NewHexxlaDBEventStore(hexxlaClient, cfg)

// Pass it via the factory
eventStore, err := adapters.NewEventStore(cfg,
    adapters.WithCustomStore(hexxlaStore),
)
```

This enables any EventStore implementation without modifying the factory code.

The demo binds to a loopback IP and redacts token values and event metadata from its HTTP and WebSocket output. Use the session ID from the MCP transport to filter demo events. Run `MCP_SESSION_ID=<id> make run-ws-test` to watch that session.

### Background Pruning

The `MemoryEventStore` automatically prunes expired events in the background when `retention_days > 0`:

```go
// Events older than 7 days are pruned every 5 minutes
store := adapters.NewMemoryEventStore(7)

// Graceful shutdown when done
if memStore, ok := store.(*adapters.MemoryEventStore); ok {
    memStore.Stop() // Stops the background pruner
}
```

### Event IDs

All events are assigned collision-safe UUIDs for production use:

```json
{
  "ID": "e6d23263-9266-43a6-bf8d-dac08c75ea8b",
  "Type": "tool_call_success",
  "SessionID": "<mcp-session-id>",
  ...
}
```

### Session Creation Events

When using `CreateSession`, a `session_created` event is emitted automatically:

```go
// Creates session AND emits session_created event
session, err := ratchetSvc.CreateSession(ctx, sessionID)
```

This provides complete visibility into session lifecycle.

### HTTP Endpoint (Web UI)

Add an HTTP endpoint to expose events for a web UI, with pagination and filtering:

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

        // Build filter from query parameters
        filter := &ratchetSecondary.EventFilter{}

        // Filter by event types (comma-separated)
        if raw := r.URL.Query().Get("event_type"); raw != "" {
            for _, t := range strings.Split(raw, ",") {
                filter.EventTypes = append(filter.EventTypes, ratchetDomain.EventType(strings.TrimSpace(t)))
            }
        }

        // Filter by tool names (comma-separated)
        if raw := r.URL.Query().Get("tool_name"); raw != "" {
            for _, t := range strings.Split(raw, ",") {
                filter.ToolNames = append(filter.ToolNames, ratchetDomain.ToolName(strings.TrimSpace(t)))
            }
        }

        // Pagination (limit and offset)
        limit := 100
        if raw := r.URL.Query().Get("limit"); raw != "" {
            if n, err := strconv.Atoi(raw); err == nil && n > 0 {
                limit = n
            }
        }
        offset := 0
        if raw := r.URL.Query().Get("offset"); raw != "" {
            if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
                offset = n
            }
        }
        filter.Limit = limit + offset

        events, err := h.ratchetSvc.GetObservabilityEvents(r.Context(),
            ratchetDomain.SessionID(sessionID), filter)
        if err != nil {
            http.Error(w, err.Error(), 500)
            return
        }

        // Apply offset for pagination
        if offset > 0 && offset < len(events) {
            events = events[offset:]
        } else if offset >= len(events) {
            events = []*ratchetDomain.Event{}
        }

        json.NewEncoder(w).Encode(events)
    }
}
```

**Usage examples:**

```bash
# Get all events for a session
curl "http://localhost:8080/observability/events?session_id=$MCP_SESSION_ID"

# Get only failures, last 10
curl "http://localhost:8080/observability/events?session_id=$MCP_SESSION_ID&event_type=tool_call_failure&limit=10"

# Get events for specific tool, page 2 (10 per page)
curl "http://localhost:8080/observability/events?session_id=$MCP_SESSION_ID&tool_name=greet&limit=10&offset=10"
```

### WebSocket Streaming (Real-Time)

For real-time event streaming, use the `BroadcastingEventStore` wrapper. This bridges ratchet's event emission with WebSocket broadcasting.

**Architecture:**

```
RatchetService.emitEvent() → BroadcastingEventStore.Store()
                                  ↓
                           ┌──────────────┐
                           │  Store to    │
                           │  EventStore  │
                           └──────────────┘
                                  ↓
                           ┌──────────────┐
                           │  Broadcast   │→ WebSocket clients
                           │  to subs     │
                           └──────────────┘
```

**Implementation:**

```go
import (
    "github.com/gorilla/websocket"
    "github.com/hexxla/mcp-ratchet/pkg/ratchet/adapters"
)

// 1. Create your broadcaster (manages WebSocket connections)
type eventBroadcaster struct {
    connections map[ratchetDomain.SessionID][]*websocket.Conn
}

func (b *eventBroadcaster) Broadcast(sessionID ratchetDomain.SessionID, event *ratchetDomain.Event) {
    // Send to all connected WebSocket clients for this session
    for _, conn := range b.connections[sessionID] {
        conn.WriteJSON(event)
    }
}

// 2. Wrap your EventStore with broadcasting
baseStore := adapters.NewMemoryEventStore(0)
broadcaster := &eventBroadcaster{connections: make(map[ratchetDomain.SessionID][]*websocket.Conn)}

store := adapters.NewBroadcastingEventStore(baseStore, broadcaster)

// 3. Pass wrapped store to ratchet service
ratchetSvc := ratchetServices.NewRatchetServiceWithObservability(
    configLoader, tokenStore, sessionStore, randomGen, clock, store,
)

// 4. Now all events are both stored AND broadcast in real-time!
```

**WebSocket Handler:**

```go
mux.HandleFunc("GET /observability/stream", func(w http.ResponseWriter, r *http.Request) {
    sessionID := ratchetDomain.SessionID(r.URL.Query().Get("session_id"))

    conn, err := websocketUpgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer conn.Close()

    // Subscribe to this session's events
    broadcaster.subscribe(sessionID, conn)
    defer broadcaster.unsubscribe(sessionID, conn)

    // Keep connection open (events pushed via broadcaster.Broadcast)
    for {
        _, _, err := conn.ReadMessage()
        if err != nil {
            break
        }
    }
})
```

**Config Separation:**

```yaml
# configs/ratchet.yaml - Core ratchet settings
observability:
  enabled: true
  storage_type: memory
  retention_days: 0

# configs/mcp-config.yaml - Server/presentation settings
observability:
  http_enabled: true
  websocket_enabled: true
  websocket_path: "/observability/stream"
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
