# Observability Event Store

**Branch:** `feature/observability-event-store`
**Status:** In Progress

## Goal

Add a non-intrusive observability layer to mcp-ratchet that captures ratchet lifecycle
events (token creation, consumption, tool call attempts/results) into a pluggable event
store. The event store is a pure side-effect sink — it never affects the core ratchet
mechanism (SessionStore, TokenStore, RatchetService decisions).

Storage backend and enabling/disabling is controlled via the existing ratchet YAML config.

---

## Design (Inside-Out, following Hexagonal Architecture)

### Core Guarantee
The event store is **optional and additive**:
- `SessionStore` — unchanged
- `TokenStore` — unchanged
- `RatchetService` logic — unchanged; events are emitted **after** core operations succeed
- If eventStore is nil or Write fails → log warning, continue normally

---

## Implementation Steps

### Step 1: Domain — Define Event types
**File:** `pkg/ratchet/domain/event.go`

- `EventType` string type + constants:
  - `token_created`
  - `token_consumed`
  - `tool_call_attempt`
  - `tool_call_success`
  - `tool_call_failure`
  - `session_created`
- `EventID` string type
- `Event` struct: `{ID, Type, SessionID, ToolName, Token, Timestamp, Metadata map[string]any}`
- `EventStats` struct: `{TotalEvents, EventsByType, EventsByTool, ActiveSessions, TokensIssued, TokensConsumed}`

**Validates:** Domain is pure, no external dependencies.

---

### Step 2: Domain — Add ObservabilityConfig
**File:** `pkg/ratchet/domain/config.go`

- `ObservabilityConfig` struct:
  ```go
  type ObservabilityConfig struct {
      Enabled       bool          `yaml:"enabled"`
      StorageType   string        `yaml:"storage_type"` // memory | hexxladb | sql
      RetentionDays int           `yaml:"retention_days"`
  }
  ```

**Validates:** Config is a domain concept, independent of implementation.

---

### Step 3: Secondary Port — EventStore interface
**File:** `pkg/ratchet/ports/secondary/event_store.go`

```go
type EventStore interface {
    Store(ctx context.Context, event *domain.Event) error
    GetEvents(ctx context.Context, sessionID domain.SessionID, filter EventFilter) ([]*domain.Event, error)
    GetStats(ctx context.Context) (*domain.EventStats, error)
}

type EventFilter struct {
    EventTypes []domain.EventType
    ToolNames  []domain.ToolName
    StartTime  time.Time
    EndTime    time.Time
    Limit      int
}
```

**Validates:** Interface defined before implementation, no adapter details.

---

### Step 4: Update YAML Config
**File:** `pkg/ratchet/adapters/yaml_loader.go`

- Add `Observability ObservabilityConfig` field to the loaded YAML struct
- Add `yaml:"observability"` tag
- No breaking changes to existing `rules:` parsing

**YAML shape:**
```yaml
observability:
  enabled: true
  storage_type: "memory"
  retention_days: 7

rules:
  - tool: greet
    ...
```

---

### Step 5: Adapter — MemoryEventStore
**File:** `pkg/ratchet/adapters/memory_event_store.go`

- In-memory implementation of `EventStore`
- Thread-safe with `sync.RWMutex`
- Implements `Store`, `GetEvents`, `GetStats`
- Default adapter for demos and testing
- Optional: prune events older than `RetentionDays`

---

### Step 6: Adapter — EventStore Factory
**File:** `pkg/ratchet/adapters/event_store_factory.go`

```go
func NewEventStore(cfg domain.ObservabilityConfig) (secondary.EventStore, error) {
    if !cfg.Enabled {
        return nil, nil
    }
    switch cfg.StorageType {
    case "memory":
        return NewMemoryEventStore(cfg), nil
    default:
        return nil, fmt.Errorf("unknown storage_type: %s", cfg.StorageType)
    }
}
```

**Note:** HexxlaDB and SQL adapters are future work — factory is the extension point.

---

### Step 7: Update RatchetService
**File:** `pkg/ratchet/services/ratchet_service.go`

- Add optional `eventStore secondary.EventStore` field to `RatchetServiceImpl`
- Update `NewRatchetService` to accept optional `EventStore` (nil = disabled)
- Emit events after core operations in:
  - `ValidateToolCall` → emit `tool_call_attempt` + `tool_call_success` / `tool_call_failure`
  - `IssueToken` → emit `token_created`
  - `ConsumePrerequisiteToken` → emit `token_consumed`
- Event emission pattern: non-blocking, warn on error, never fail core operation

---

### Step 8: Update RatchetService Primary Port
**File:** `pkg/ratchet/ports/primary/ratchet_service.go`

- Add `GetObservabilityStats(ctx context.Context) (*domain.EventStats, error)` to interface
- Add `GetEvents(ctx context.Context, sessionID domain.SessionID, filter secondary.EventFilter) ([]*domain.Event, error)` to interface

---

### Step 9: Update main.go wiring
**File:** `cmd/mcp-ratchet-demo/main.go`

- Read `ObservabilityConfig` from loaded YAML config
- Use factory to create `EventStore`
- Pass to `NewRatchetService`

---

### Step 10: Tests
- Unit tests for `MemoryEventStore` (store, filter, stats)
- Unit tests for `RatchetService` event emission (mock EventStore)
- Integration: verify events emitted during normal ratchet flow

---

### Step 11: Update configs/ratchet.yaml + README
- Add `observability` block to demo config (enabled, memory)
- Update README with observability section and YAML reference

---

## File Inventory

| File | Action |
|------|--------|
| `pkg/ratchet/domain/event.go` | Create |
| `pkg/ratchet/domain/config.go` | Create |
| `pkg/ratchet/ports/secondary/event_store.go` | Create |
| `pkg/ratchet/ports/primary/ratchet_service.go` | Update |
| `pkg/ratchet/adapters/memory_event_store.go` | Create |
| `pkg/ratchet/adapters/event_store_factory.go` | Create |
| `pkg/ratchet/adapters/yaml_loader.go` | Update |
| `pkg/ratchet/services/ratchet_service.go` | Update |
| `cmd/mcp-ratchet-demo/main.go` | Update |
| `configs/ratchet.yaml` | Update |
| `README.md` | Update |

---

## Out of Scope (Future Work)

- HexxlaDB event store adapter (Mosaic integration)
- SQL event store adapter (production persistence)
- Real-time event streaming
- HTTP API to query events
- Web UI for visualisation
