# Observability Improvements

**Status:** Planned
**Priority:** Medium

## Overview

This plan addresses inconsistencies and gaps identified in the initial observability implementation. The core design is sound, but several improvements will enhance robustness, extensibility, and production readiness.

---

## Issues Identified

### 1. Missing `session_created` Event Emission
**Severity:** Medium  
**Impact:** Incomplete event coverage

The domain defines `EventTypeSessionCreated` but the service never emits it when a session is created. This makes it impossible to track when sessions are initialized via observability.

**Files affected:**
- `pkg/ratchet/services/ratchet_service.go`
- `pkg/ratchet/domain/event.go` (already has the event type)

---

### 2. Factory Signature Limits Extensibility
**Severity:** High  
**Impact:** Cannot add HexxlaDB/SQL backends without breaking the factory

`NewEventStore(cfg)` only accepts configuration. To add HexxlaDB or SQL backends, we need to pass database clients. The current factory signature doesn't support optional dependencies.

**Files affected:**
- `pkg/ratchet/adapters/event_store_factory.go`

---

### 3. Retention Pruning on Every `Store`
**Severity:** Low  
**Impact:** Performance inefficiency under high volume

`MemoryEventStore` prunes expired events on every `Store` call. For high-volume scenarios, a background goroutine with periodic pruning would be more efficient.

**Files affected:**
- `pkg/ratchet/adapters/memory_event_store.go`

---

### 4. Event ID Generation Not Collision-Safe
**Severity:** Low  
**Impact:** Potential ID collisions under high concurrency

Current ID generation uses `fmt.Sprintf("%s-%s-%d", eventType, tool, s.clock.Now().UnixNano())`. While nanosecond timestamps reduce collision probability, UUIDs are safer for production systems.

**Files affected:**
- `pkg/ratchet/services/ratchet_service.go`

---

### 5. HTTP Endpoint Limitations
**Severity:** Low  
**Impact:** Limited usability for production web UI

Current endpoints lack:
- Pagination for large event sets
- Real-time streaming (WebSocket/SSE)
- Filtering by metadata fields
- Sorting options

**Files affected:**
- `cmd/mcp-ratchet-demo/main.go`

---

## Implementation Plan

### Phase 1: Fix Missing Event Emission (High Value, Low Effort)

**Task:** Emit `session_created` event when sessions are created

**Steps:**
1. Add `sessionStore` reference to `RatchetServiceImpl` (or pass as parameter to emit function)
2. In session creation flow (currently in adapter layer), emit event via ratchet service
3. Add test for `session_created` event emission

**Alternatives:**
- Move session creation into service layer for better control
- Add callback/hook in SessionStore for event emission

**Decision:** Add callback/hook in SessionStore to avoid breaking adapter layer changes.

---

### Phase 2: Refactor Factory for Extensibility (High Value, Medium Effort)

**Task:** Support optional dependencies in factory via functional options pattern

**Steps:**
1. Define `Option` type: `type Option func(*factoryConfig) struct`
2. Add options: `WithHexxlaDBClient(client)`, `WithDBClient(db)`
3. Update `NewEventStore(cfg, opts ...Option)` signature
4. Implement HexxlaDB backend (separate task, this just enables it)

**Example:**
```go
eventStore, err := adapters.NewEventStore(cfg,
    adapters.WithHexxlaDBClient(hexxlaClient),
)
```

---

### Phase 3: Background Pruning (Medium Value, Medium Effort)

**Task:** Move retention pruning to background goroutine

**Steps:**
1. Add `Stop()` method to `MemoryEventStore` for graceful shutdown
2. Start background goroutine with `time.Ticker` in constructor
3. Prune on ticker interval (e.g., every 5 minutes)
4. Add context cancellation support for goroutine lifecycle

---

### Phase 4: UUID Event IDs (Low Value, Low Effort)

**Task:** Use UUIDs for event IDs

**Steps:**
1. Add `github.com/google/uuid` dependency
2. Update `emitEvent` to use `uuid.New()`
3. Update tests to validate UUID format

---

### Phase 5: HTTP Endpoint Enhancements (Low Value, High Effort)

**Task:** Add pagination and better filtering

**Steps:**
1. Add `page` and `page_size` query parameters
2. Add `sort_by` and `sort_order` parameters
3. Add metadata filtering support (e.g., `?metadata_key=error`)
4. Consider WebSocket/SSE for real-time streaming (future work)

---

## File Inventory

| Phase | Files |
|-------|-------|
| 1 | `pkg/ratchet/ports/secondary/session_store.go`, `pkg/ratchet/services/ratchet_service.go` |
| 2 | `pkg/ratchet/adapters/event_store_factory.go` |
| 3 | `pkg/ratchet/adapters/memory_event_store.go` |
| 4 | `pkg/ratchet/services/ratchet_service.go`, `go.mod` |
| 5 | `cmd/mcp-ratchet-demo/main.go` |

---

## Priority Order

1. **Phase 1** (Fix missing event) — Completes event coverage
2. **Phase 2** (Factory extensibility) — Enables HexxlaDB/SQL backends
3. **Phase 3** (Background pruning) — Performance improvement
4. **Phase 4** (UUID IDs) — Production safety
5. **Phase 5** (HTTP enhancements) — Nice-to-have, can defer

---

## Out of Scope

- HexxlaDBEventStore implementation (separate feature)
- SQLEventStore implementation (separate feature)
- Real-time streaming (WebSocket/SSE) — defer to future
- Advanced analytics/aggregation queries — defer to future
