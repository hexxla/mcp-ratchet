# MCP-Ratchet Improvement Plan

## Overview

This plan addresses 13 validated issues identified during codebase analysis. Each issue includes:
- **Problem description** with validation evidence
- **What the fix achieves** (behavioral/security/architectural improvement)
- **Estimated effort**

---

## Critical Priority

### 1. Test Coverage for Core Ratchet Logic

**Problem:**
- `ValidateToolCall` has **0% test coverage** (`@/home/anon/Documents/GitHub/mcp-ratchet/pkg/ratchet/services/ratchet_service.go:53`)
- `internal/`, `cmd/`, and adapter packages have **no tests at all**
- Overall coverage: **22.1%** (only `pkg/ratchet/domain` at 96.7%)

**Validation:**
```bash
github.com/hexxla/mcp-ratchet/pkg/ratchet/services/ratchet_service.go:53:  ValidateToolCall  0.0%
```

**What the Fix Achieves:**
- Verifies token validation logic (prerequisite checks, expiry, one-time-use)
- Catches regressions in security-critical ratchet mechanism
- Enables safe refactoring of service logic
- Documents expected behavior through executable specifications

**Test Cases Needed:**
- Tool with no rules (unrestricted access)
- Tool with prerequisite met/unmet
- Token expiry edge cases
- OneTimeUse token consumption
- Circular dependency detection
- Missing session handling

**Effort:** Medium (2-3 hours)

---

### 2. Fix Broken golangci-lint depguard Rules

**Problem:**
Two depguard rules reference wrong module paths copied from another project:
- Line 38: `github.com/sploitzberg/mcp-ratchet/internal/adapter` (wrong owner)
- Line 66: `github.com/sploitzberg/go-llm-project-structure/internal/adapter/primary` (different project entirely)

**Validation:**
```yaml
# @/home/anon/Documents/GitHub/mcp-ratchet/.golangci.yml:38-40
- pkg: "github.com/sploitzberg/mcp-ratchet/internal/adapte\\
    r"

# @/home/anon/Documents/GitHub/mcp-ratchet/.golangci.yml:66-67
- pkg: "github.com/sploitzberg/go-llm-project-structure/internal/adapte\\
    r/primary"
```

**What the Fix Achieves:**
- Restores hexagonal architecture enforcement at CI level
- Prevents accidental imports from wrong layers
- Catches architectural violations before merge
- The rules currently **silently fail** — no violations detected even if broken

**Fix:** Replace `sploitzberg` with `hexxla`, fix broken string continuations

**Effort:** Low (15 minutes)

---

### 3. Move OneTimeUse Token Consumption to Post-Success

**Problem:**
In `ValidateToolCall`, the prerequisite token is consumed immediately during validation (lines 104-121). If the actual tool handler fails afterward, the token is already gone and the user cannot retry.

**Validation:**
```go
// @/home/anon/Documents/GitHub/mcp-ratchet/pkg/ratchet/services/ratchet_service.go:104-121
if rule.OneTimeUse {
    // Remove token BEFORE handler executes
    err = s.tokenStore.RemoveToken(ctx, sessionID, rule.Prerequisite, lastToken)
    // ...
}
// Handler execution happens AFTER this block
```

**What the Fix Achieves:**
- **Prevents token loss on handler failure** — users can retry failed operations
- Maintains ratchet security (token still consumed, but only on success)
- Aligns with principle: consume resources only after work is done
- Fixes potential DoS: malicious/erroneous requests could burn tokens without completing work

**Approaches:**
1. **Option A:** Split validation and consumption — add `ConsumeToken()` method called by adapter after success
2. **Option B:** Pass a callback/closure to `ValidateToolCall` that executes handler, wrap in transaction
3. **Option C:** Make adapter responsible for calling `ConsumeToken` after successful handler execution

**Recommended:** Option C (clean separation, adapter already handles success/failure)

**Effort:** Medium (1-2 hours) — requires interface change and adapter updates

---

### 4. Fix MemorySessionStore.Get() Returning Mutable References

**Problem:**
`Get()` returns a direct pointer to the internal map entry. Callers mutate the session, then call `Update()` which is currently a no-op for memory store. This masks a real bug: with a real database, mutations would be lost.

**Validation:**
```go
// @/home/anon/Documents/GitHub/mcp-ratchet/pkg/ratchet/adapters/memory_session_store.go:34-42
func (m *MemorySessionStore) Get(...) (*domain.Session, error) {
    // Returns direct pointer from map
    return m.sessions[id], nil  // Caller can modify without Update()
}
```

**What the Fix Achieves:**
- **Prevents silent data loss** when switching to persistent session stores
- Makes in-memory behavior match database behavior (read-modify-write cycle)
- Enforces proper repository pattern usage
- Prevents accidental cross-request session pollution (if same session retrieved multiple times)

**Fix Options:**
1. Return deep copy (safer, but allocation cost)
2. Document that `Update()` is mandatory after mutation (risky, relies on convention)
3. Remove `Update()` method entirely, make `Get()` return copy-on-write or immutable interface

**Recommended:** Option 1 — return shallow copy of Session struct ( Tokens map still shared — need deep copy for full safety)

**Effort:** Low-Medium (30-60 minutes)

---

## Medium Priority

### 5. Extract Generic Ratchet Wrapping Helper

**Problem:**
`greeting_tool.go` repeats ~20 lines of identical ratchet wrapping logic for each tool (4 tools). Only varying parts: tool name and handler signature.

**Validation:**
Lines 43-97, 127-181, 209-263, 293-347 are structurally identical.

**What the Fix Achieves:**
- **Reduces duplication** — single source of truth for ratchet integration
- Easier to modify ratchet behavior (change in one place)
- Lower barrier to adding new ratchet-protected tools
- Reduces copy-paste error risk

**Fix:** Create `WrapToolHandler(sessionStore, ratchet, toolName, handler)` helper

**Effort:** Low (30-45 minutes)

---

### 6. Remove Hardcoded Demo Data

**Problem:**
- `get_time` returns fixed string `"2026-05-02T15:00:00Z"`
- `get_date` returns fixed string `"2026-05-02"`
- Session ID hardcoded to `"demo-session"` in every handler

**Validation:**
```go
// @/home/anon/Documents/GitHub/mcp-ratchet/internal/adapter/primary/mcp/greeting_tool.go:201
resp := getTimeResponse{Time: "2026-05-02T15:00:00Z"}

// @/home/anon/Documents/GitHub/mcp-ratchet/internal/adapter/primary/mcp/greeting_tool.go:47
sessionID := ratchetDomain.SessionID("demo-session")
```

**What the Fix Achieves:**
- Makes demo server actually useful for testing time-based features
- Enables real session isolation testing
- Removes "this is clearly a toy" signal from the codebase

**Fix:**
- Use `clock.Now()` for time (already injected)
- Use `randomGen.GenerateSessionID()` or request-based session ID

**Effort:** Low (20-30 minutes)

---

### 7. Make NewSession Clock-Injectable

**Problem:**
`NewSession` uses `time.Now()` directly, making time-based tests non-deterministic.

**Validation:**
```go
// @/home/anon/Documents/GitHub/mcp-ratchet/pkg/ratchet/domain/session.go:22
CreatedAt: time.Now(),  // Not injectable
```

**What the Fix Achieves:**
- Enables deterministic testing of session expiry logic
- Aligns with existing `Clock` port pattern used elsewhere
- Allows testing "session created at specific time" scenarios

**Fix:** Add `func NewSessionWithTime(id SessionID, createdAt time.Time)` or pass Clock interface

**Effort:** Low (15-20 minutes)

---

### 8. Replace Getter/Setter Pattern with Exported Fields

**Problem:**
`internal/core/domain/greeting.go` uses unexported fields with Java-style Get/Set methods. Un-idiomatic in Go; `pkg/ratchet/domain/` correctly uses exported fields.

**Validation:**
```go
// @/home/anon/Documents/GitHub/mcp-ratchet/internal/core/domain/greeting.go:6-33
type GreetingRequest struct {
    name string  // unexported
}
func (r *GreetingRequest) Name() string { return r.name }
func (r *GreetingRequest) SetName(name string) { r.name = name }
```

**What the Fix Achieves:**
- **Idiomatic Go code** — exported fields with documentation
- Less boilerplate, more readable
- Consistent with `pkg/ratchet/domain/` style
- Zero behavioral change, pure maintainability improvement

**Fix:** Export fields, remove getter/setter methods

**Effort:** Low (15-20 minutes)

---

### 9. Replace Reflection-Based Wrapper with Typed Middleware

**Problem:**
`pkg/ratchet/services/wrapper.go` uses `reflect.MakeFunc` to wrap arbitrary functions. Bypasses compile-time type safety, hard to debug.

**Validation:**
```go
// @/home/anon/Documents/GitHub/mcp-ratchet/pkg/ratchet/services/wrapper.go:34
wrapper := reflect.MakeFunc(fnType, func(args []reflect.Value) []reflect.Value {
    // ...
})
```

**What the Fix Achieves:**
- **Compile-time type safety** — errors caught at build, not runtime
- Easier debugging (stack traces, breakpoints)
- Better IDE support (go-to-definition, refactoring)
- Removes runtime reflection overhead

**Fix:** Define explicit `ToolHandler` interface, use decorator pattern

**Effort:** Medium (1-2 hours) — requires redesigning handler interface

---

## Low Priority

### 10. Add YAML Struct Tags to Rule

**Problem:**
`Rule` struct relies on field-name matching for YAML unmarshaling. Renaming fields breaks config silently.

**Validation:**
```go
// @/home/anon/Documents/GitHub/mcp-ratchet/pkg/ratchet/domain/rule.go:10-16
type Rule struct {
    Tool         ToolName      // no yaml:"tool" tag
    Prerequisite ToolName      // no yaml:"prerequisite" tag
    // ...
}
```

**What the Fix Achieves:**
- Explicit contract between config file and struct
- Allows field renaming without breaking YAML compatibility
- Documents expected YAML structure in code

**Fix:** Add `yaml:"field_name"` tags to all fields

**Effort:** Trivial (5 minutes)

---

### 11. Make RemoveToken/Delete Return Errors on Missing Keys

**Problem:**
Both `MemoryTokenStore.RemoveToken` and `MemorySessionStore.Delete` return `nil` when key doesn't exist, masking logic errors.

**Validation:**
```go
// @/home/anon/Documents/GitHub/mcp-ratchet/pkg/ratchet/adapters/memory_token_store.go:80
if !ok {
    return nil  // Silently succeeds on missing session
}
```

**What the Fix Achieves:**
- **Fail-fast behavior** — callers know if they're operating on non-existent data
- Consistent with typical repository pattern expectations
- Helps catch bugs like double-delete, wrong session IDs

**Fix:** Return `ErrSessionNotFound` / `ErrTokenNotFound` or similar

**Effort:** Low (15-20 minutes) — requires interface audit for callers

---

### 12. Fix Token Length Validation vs Generation Mismatch

**Problem:**
`TokenValue.Validate()` requires ≥32 characters, and `GenerateToken()` produces exactly 32 hex characters. Minor change to generation would break validation.

**Validation:**
```go
// @/home/anon/Documents/GitHub/mcp-ratchet/pkg/ratchet/domain/value_objects.go:23
if len(t) < 32 {
    return errors.New("token must be at least 32 characters")
}

// @/home/anon/Documents/GitHub/mcp-ratchet/pkg/ratchet/adapters/crypto_random.go:21
bytes := make([]byte, 16) // 32 hex characters
```

**What the Fix Achieves:**
- **Consistent token contract** — generation and validation in sync
- Prevents accidental breakage when modifying token generation
- Either use shared constant or make validation accept range

**Fix:** Define `const MinTokenLength = 32`, use in both places

**Effort:** Trivial (5 minutes)

---

### 13. Remove Duplicate clean-llm Target

**Problem:**
`Makefile` defines `clean-llm` twice (lines 88 and 91).

**Validation:**
```makefile
# @/home/anon/Documents/GitHub/mcp-ratchet/Makefile:88
clean-llm: clean-llm-all

# @/home/anon/Documents/GitHub/mcp-ratchet/Makefile:91
clean-llm: clean-llm-all  # Duplicate!
```

**What the Fix Achieves:**
- Clean build file without redundancy
- Removes confusion about which definition applies

**Fix:** Delete line 91

**Effort:** Trivial (1 minute)

---

## Implementation Order

### Phase 1: Critical Fixes (Security & Correctness)
1. **Issue #3** — OneTimeUse token consumption timing (behavioral bug)
2. **Issue #4** — MemorySessionStore mutable reference (data integrity)
3. **Issue #2** — Fix depguard rules (CI enforcement)
4. **Issue #1** — Test coverage for ValidateToolCall (regression prevention)

### Phase 2: Maintainability
5. **Issue #5** — Extract wrapping helper (DRY)
6. **Issue #8** — Remove getter/setter boilerplate (idiomatic Go)
7. **Issue #9** — Replace reflection wrapper (type safety)
8. **Issue #7** — Clock-injectable NewSession (testability)

### Phase 3: Polish
9. **Issue #6** — Remove hardcoded data (demo quality)
10. **Issue #10, #11, #12, #13** — Minor cleanups (tags, errors, consts, Makefile)

---

## Estimated Total Effort

- **Phase 1:** 4-6 hours
- **Phase 2:** 3-5 hours
- **Phase 3:** 1-2 hours
- **Total:** ~8-13 hours

---

## Files Requiring Changes

| File | Issues |
|------|--------|
| `.golangci.yml` | #2 |
| `pkg/ratchet/services/ratchet_service.go` | #1, #3 |
| `pkg/ratchet/services/ratchet_service_test.go` | #1 (new tests) |
| `pkg/ratchet/adapters/memory_session_store.go` | #4 |
| `internal/adapter/primary/mcp/greeting_tool.go` | #5, #6 |
| `internal/core/domain/greeting.go` | #8 |
| `pkg/ratchet/services/wrapper.go` | #9 |
| `pkg/ratchet/domain/session.go` | #7 |
| `pkg/ratchet/domain/rule.go` | #10 |
| `pkg/ratchet/adapters/memory_token_store.go` | #11 |
| `pkg/ratchet/domain/value_objects.go` | #12 |
| `pkg/ratchet/adapters/crypto_random.go` | #12 |
| `Makefile` | #13 |
