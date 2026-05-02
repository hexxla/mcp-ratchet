// Package ratchet provides a configurable system for enforcing tool call order
// using ratchet tokens.
//
// # Overview
//
// The ratchet system enforces that certain tools must be called before others
// are allowed. This is achieved through a ratchet token mechanism:
//
// 1. Define tool dependencies in YAML configuration
// 2. When a prerequisite tool completes successfully, issue a ratchet token
// 3. The dependent tool requires this token to be called
// 4. Tokens are single-use and expire after a configured time
//
// # Architecture
//
// The package follows hexagonal architecture:
//
//   - Domain: Core business entities (Rule, RatchetToken, Session)
//   - Primary Ports: Use case interfaces (RatchetService, Wrapper)
//   - Secondary Ports: External dependencies (ConfigLoader, TokenStore, etc.)
//   - Services: Business logic implementation
//   - Adapters: Concrete implementations (YAML loader, in-memory stores)
//
// # Quick Start
//
//	// Create adapters
//	configLoader := adapters.NewYAMLConfigLoader()
//	tokenStore := adapters.NewMemoryTokenStore()
//	sessionStore := adapters.NewMemorySessionStore()
//	randomGen := adapters.NewCryptoRandomGenerator()
//	clock := adapters.NewRealClock()
//
//	// Create service
//	service := services.NewRatchetService(configLoader, tokenStore, sessionStore, randomGen, clock)
//
//	// Load configuration
//	configFile, _ := os.Open("configs/ratchet.yaml")
//	rules, _ := service.LoadConfiguration(ctx, configFile)
//
//	// Create session
//	sessionID := domain.SessionID("session-123")
//	session := domain.NewSession(sessionID)
//	sessionStore.Create(ctx, session)
//
//	// Issue token after prerequisite completes
//	token, _ := service.IssueToken(ctx, sessionID, "PrerequisiteTool")
//
//	// Validate before calling dependent tool
//	err := service.ValidateToolCall(ctx, sessionID, "DependentTool", token)
//
// # Configuration
//
// YAML configuration defines tool dependencies:
//
//	rules:
//	  - tool: "Create Cell"
//	    prerequisite: "List Tags"
//	    expiry: "5m"
//	    error_message: "Must call List Tags first"
//
// # Token Lifecycle
//
// - Tokens are issued after successful tool execution
// - Tokens are removed from the session when used (absence = used or never created)
// - Tokens expire after configured duration
// - Each tool can have multiple valid tokens for concurrent access
//
// # Concurrency
//
// The Session struct uses `map[ToolName][]TokenValue` to support
// concurrent token usage. Multiple agents can have valid tokens for the
// same tool simultaneously.
package ratchet
