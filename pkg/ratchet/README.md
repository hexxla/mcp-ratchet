# Ratchet Package

A configurable Go package for enforcing tool call order using ratchet tokens.

## Overview

The ratchet system enforces that certain tools must be called before others are allowed through a token-based mechanism. This is particularly useful for MCP (Model Context Protocol) servers where you want to guide LLMs through specific workflows.

## Features

- **YAML Configuration**: Define tool dependencies declaratively
- **Hexagonal Architecture**: Clean separation of concerns with ports and adapters
- **Token-based Enforcement**: Single-use tokens that expire after configured time
- **Concurrency Support**: Multiple agents can have valid tokens simultaneously
- **Circular Dependency Detection**: Validates rule graphs at startup

## Installation

```bash
go get github.com/hexxla/mcp-ratchet/pkg/ratchet
```

## Quick Start

```go
import (
    "context"
    "os"

    "github.com/hexxla/mcp-ratchet/pkg/ratchet/adapters"
    "github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
    "github.com/hexxla/mcp-ratchet/pkg/ratchet/services"
)

func main() {
    ctx := context.Background()

    // Create adapters
    configLoader := adapters.NewYAMLConfigLoader()
    tokenStore := adapters.NewMemoryTokenStore()
    sessionStore := adapters.NewMemorySessionStore()
    randomGen := adapters.NewCryptoRandomGenerator()
    clock := adapters.NewRealClock()

    // Create service
    service := services.NewRatchetService(
        configLoader,
        tokenStore,
        sessionStore,
        randomGen,
        clock,
    )

    // Load configuration
    configFile, _ := os.Open("configs/ratchet.yaml")
    rules, _ := service.LoadConfiguration(ctx, configFile)

    // Create session
    sessionID := domain.SessionID("session-123")
    session := domain.NewSession(sessionID)
    sessionStore.Create(ctx, session)

    // Issue token after prerequisite completes
    token, _ := service.IssueToken(ctx, sessionID, "List Tags")

    // Validate before calling dependent tool
    err := service.ValidateToolCall(ctx, sessionID, "Create Cell", token)
    if err != nil {
        // Handle validation error
    }
}
```

## Configuration

Create a YAML file to define tool dependencies:

```yaml
rules:
  - tool: "Create Cell"
    prerequisite: "List Tags"
    expiry: "5m"
    error_message: "Must call List Tags tool before Create Cell"
```

## Architecture

The package follows hexagonal architecture:

- **Domain**: Core business entities (Rule, RatchetToken, Session)
- **Primary Ports**: Use case interfaces (RatchetService, Wrapper)
- **Secondary Ports**: External dependencies (ConfigLoader, TokenStore, SessionStore, RandomGenerator, Clock)
- **Services**: Business logic implementation
- **Adapters**: Concrete implementations (YAML loader, in-memory stores, crypto random generator, real clock)

## Token Lifecycle

1. **Issue**: Token is issued after successful tool execution via `IssueToken()`
2. **Validate**: Token is validated before calling dependent tool via `ValidateToolCall()`
3. **Consume**: Token is removed from session when used (absence signifies used)
4. **Expire**: Tokens expire after configured duration

## License

MIT License - see LICENSE file for details.
