# mcp-ratchet

A Go project implementing a configurable **MCP Ratchet** system with strict **Hexagonal Architecture** (Ports & Adapters pattern) for enforcing tool call order in MCP servers.

## Overview

The `pkg/ratchet` package provides a configurable token-based system for enforcing tool call order in Model Context Protocol (MCP) servers. It ensures that tools are called in a specific sequence by requiring tokens from prerequisite tools before allowing execution.

## Quick Start

```bash
# Clone the repository
git clone https://github.com/hexxla/mcp-ratchet.git
cd mcp-ratchet

# Run the demo server (without ratchet)
go run cmd/mcp-ratchet-demo/main.go
```

## Using the Ratchet Package

The ratchet package is located in `pkg/ratchet/` and can be imported as:

```go
import (
    "github.com/hexxla/mcp-ratchet/pkg/ratchet/adapters"
    "github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/primary"
    "github.com/hexxla/mcp-ratchet/pkg/ratchet/services"
)
```

### Basic Usage

```go
// 1. Initialize the ratchet service
configLoader := adapters.NewYAMLConfigLoader()
tokenStore := adapters.NewMemoryTokenStore()
sessionStore := adapters.NewMemorySessionStore()
randomGen := adapters.NewCryptoRandomGenerator()
clock := adapters.NewRealClock()

ratchetSvc := services.NewRatchetService(configLoader, tokenStore, sessionStore, randomGen, clock)

// 2. Load configuration from YAML
configFile, err := os.Open("ratchet.yaml")
if err != nil {
    log.Fatal(err)
}
defer configFile.Close()

rules, err := ratchetSvc.LoadConfiguration(ctx, configFile)
if err != nil {
    log.Fatal(err)
}

// 3. Validate tool calls before execution
sessionID := domain.SessionID("session-123")
toolName := domain.ToolName("my_tool")

// Check if tool can be called
err := ratchetSvc.ValidateToolCall(ctx, sessionID, toolName, nil)
if err != nil {
    // Tool call blocked - missing prerequisite token
    return err
}

// 4. After successful execution, issue token for dependent tools
token, err := ratchetSvc.IssueToken(ctx, sessionID, toolName)
if err != nil {
    return err
}

// Return token to client for next call
```

### Configuration Format

Create a YAML file defining tool dependencies:

```yaml
# ratchet.yaml
rules:
  # Tool with no prerequisites (can be called first)
  - tool: greet
    prerequisite: ""
    expiry: 5m
    error_message: ""

  # Tool that requires a prerequisite
  - tool: get_user_name
    prerequisite: greet
    expiry: 10m
    error_message: "You must call the 'greet' tool before calling 'get_user_name'. Please call greet first to establish the required session."
    one_time_use: false
```

**Fields:**

- `tool` - The name of the tool being defined
- `prerequisite` - The name of the tool that must be called first (empty string for no prerequisite)
- `expiry` - Token expiration time (e.g., `5m`, `10m`, `1h`)
- `error_message` - Custom error message to return when validation fails (instructional for LLMs)
- `one_time_use` - If `true`, the token is consumed after one use and cannot be reused (default: `false`)

### Wrapping Tool Handlers

Use the reflection-based wrapper to automatically enforce ratchet rules:

```go
wrapper := services.NewWrapper(ratchetSvc, "my_tool")
wrappedHandler := wrapper.Wrap(originalHandler)

// The wrapped handler will automatically:
// 1. Validate prerequisites before execution
// 2. Extract token from last argument
// 3. Issue token after successful execution
```

## Demo Server

The demo server (`cmd/mcp-ratchet-demo`) showcases basic MCP server functionality with optional ratchet enforcement.

```bash
# Run without ratchet enforcement
go run cmd/mcp-ratchet-demo/main.go

# Run with ratchet enforcement
go run cmd/mcp-ratchet-demo/main.go --ratchet-config configs/ratchet.yaml
```

The demo server includes two tools:

- `greet` - Returns a greeting message (no prerequisites)
- `get_user_name` - Returns the current user's name (requires `greet` to be called first)

When ratchet is enabled, `get_user_name` will fail if `greet` hasn't been called first.

## Guardrails & CI/CD

This project includes a comprehensive CI/CD pipeline with automated quality checks:

### Architecture Guardrails

- **Hexagonal Architecture Guardrail** - Enforces dependency rules (domain purity, correct layer dependencies)
- **Import Cycle Detection** - Prevents circular dependencies
- **Framework Leak Detection** - Ensures core packages don't depend on frameworks

### Code Quality Checks

- **Formatting** - gofmt, goimports (auto-format on save)
- **Linting** - golangci-lint with comprehensive rule set
- **Static Analysis** - go vet, gosec (security scanner)
- **Error Wrapping** - Validates proper error handling with `%w`
- **Struct Fields** - Ensures exported struct fields have proper tags
- **Import Order** - Enforces consistent import ordering

### Go Conventions Validation

- No `context.Background()` in exported functions
- No `panic()` in production code
- No `time.Sleep()` in production code
- No `os.Exit()` in non-main packages
- No `init()` functions in production code

### Security Checks

- Secret scanning (AWS credentials, GitHub tokens, API keys)
- Dependency vulnerability scanning
- Input validation enforcement

### Git Hooks

- **pre-commit** - Fast checks (formatting, linting, unit tests)
- **pre-push** - Comprehensive checks (build, all tests, coverage, outdated dependencies)
- **commit-msg** - Conventional commits format validation

## Architecture

This project follows Hexagonal Architecture with these layers:

- `internal/core/domain/` — Pure business entities and rules (zero dependencies)
- `internal/core/ports/` — Interface definitions (primary/secondary)
- `internal/core/services/` — Application use case orchestration
- `internal/adapter/` — Concrete implementations (HTTP, database, etc.)
- `internal/config/` — Configuration structures

## Documentation

- [`pkg/ratchet/README.md`](pkg/ratchet/README.md) — Ratchet package documentation
- [`pkg/ratchet/doc.go`](pkg/ratchet/doc.go) — Package documentation
- [`AGENTS.md`](AGENTS.md) — Instructions for AI coding assistants
- [`SECURITY.md`](SECURITY.md) — Security policy and vulnerability reporting
- [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) — Contributor code of conduct

## Makefile Targets

- `make build` — Build the binary
- `make test` — Run unit tests
- `make test-all` — Run both unit and integration tests
- `make integration` — Run integration tests (slower, external dependencies)
- `make lint` — Run golangci-lint
- `make fmt` — Format code
- `make ci` — Run full CI checks
- `make install` — Install locally

## Integration Tests

Integration tests are separated from unit tests to keep CI fast:

- **Unit tests** (`make test`) — Run on every commit (pre-commit hook)
- **Integration tests** (`make integration`) — Run on every push (pre-push hook)

To create an integration test, add the build tag to the top of your test file:

```go
//go:build integration

package mypackage

import "testing"

func TestMyIntegration(t *testing.T) {
    // Test with external dependencies (databases, APIs, etc.)
}
```

Run integration tests locally:

```bash
make integration
# or
go test -tags=integration ./...
```
