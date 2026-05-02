# Configurable Ratchet System - Design Plan

## Overview

Create a configurable system where users can define tool dependencies via YAML configuration. The system will enforce that certain tools must be called before others are allowed, using a ratchet token mechanism. Users can import a Go package to wrap their functions/LLM tools to require ratchet values.

## Implementation Progress

- ✅ **Step 0: Package Structure** - Created `pkg/ratchet/` with domain, ports/primary, ports/secondary, services, adapters directories
- ✅ **Step 1: Domain** - Implemented value objects (ToolName, TokenValue, SessionID), Rule, RatchetToken, Session entities with validation and business rules
- ✅ **Step 2: Primary Ports** - Implemented RatchetService and Wrapper interfaces
- ✅ **Step 3: Secondary Ports** - Implemented ConfigLoader, TokenStore, SessionStore, RandomGenerator, Clock interfaces
- ✅ **Step 4: Services** - Implemented RatchetServiceImpl and WrapperImpl
- ✅ **Step 5: Adapters** - Implemented YAMLConfigLoader, MemoryTokenStore, MemorySessionStore, CryptoRandomGenerator, RealClock
- ✅ **Step 6: Tests** - Wrote unit tests for domain, services, and adapters
- ✅ **Step 7: Example Config** - Created example YAML configuration in configs/ratchet.yaml
- ✅ **Step 8: Example Usage** - Created example usage in pkg/ratchet/example_test.go
- ✅ **Step 9: Documentation** - Enhanced doc.go and created README.md

## Requirements

1. **YAML Configuration**: Users define tool dependencies in a YAML file
2. **Go Package**: Importable package for wrapping functions to require ratchet tokens
3. **Enforcement**: System enforces "do A before B" pattern using one-time ratchet tokens
4. **MCP Integration**: Tools called inside MCP server context

---

## Step 1: Define Business Concepts (Domain)

### Entities

#### Rule

- Defines the enforcement pattern: which tool depends on which prerequisite
- Has a tool name and prerequisite tool name
- Business rule: A tool cannot be called unless its prerequisite has been completed

#### RatchetToken

- One-time code issued after completing prerequisite
- Has a value (string), expiration time, and associated session
- Business rules:
  - Removed from array when used (absence signifies used or never created)
  - Expires after configured time period
  - New token issued after successful execution
  - Must match the expected value for the tool being called

#### Session

- Tracks state for a given interaction
- Contains session ID, current tokens (array per tool for concurrency), and tool call history
- Business rules:
  - Maintains mapping of tools to arrays of valid tokens (supports concurrent token usage)
  - Tracks which tools have been called in what order
  - Tokens are removed from array when used (absence signifies used or never created)
- **Concurrency**: Each tool can hold multiple tokens in an array for concurrent access
- **Lifecycle**: Tied to the lifecycle of the app, starting fresh each time the app starts

#### ToolName

- Value object representing the identifier for an MCP tool
- Examples: "Create Cell", "List Tags", "Put Cell"

#### TokenValue

- Value object representing the actual ratchet token string
- Cryptographically secure random string
- **Format**: Simple, random, short strings (not JWT)

#### SessionID

- Value object representing unique session identifier

### Value Objects

```go
// ToolName identifies an MCP tool
type ToolName string

func (t ToolName) Validate() error {
    if t == "" {
        return errors.New("tool name cannot be empty")
    }
    return nil
}

// TokenValue represents a ratchet token
type TokenValue string

func (t TokenValue) Validate() error {
    if len(t) < 32 {
        return errors.New("token must be at least 32 characters")
    }
    return nil
}

// SessionID uniquely identifies a session
type SessionID string

func (s SessionID) Validate() error {
    if s == "" {
        return errors.New("session ID cannot be empty")
    }
    return nil
}
```

### Domain Entities

```go
// Rule defines a tool dependency
type Rule struct {
    Tool         ToolName
    Prerequisite ToolName
}

func (r *Rule) Validate() error {
    if err := r.Tool.Validate(); err != nil {
        return err
    }
    if err := r.Prerequisite.Validate(); err != nil {
        return err
    }
    if r.Tool == r.Prerequisite {
        return errors.New("tool cannot depend on itself")
    }
    return nil
}

// RatchetToken represents a one-time use token
type RatchetToken struct {
    Value      TokenValue
    SessionID  SessionID
    Tool       ToolName
    ExpiresAt  time.Time
}

func (t *RatchetToken) IsValid() error {
    if time.Now().After(t.ExpiresAt) {
        return errors.New("token has expired")
    }
    return nil
}

// Session tracks state for an interaction
type Session struct {
    ID           SessionID
    Tokens       map[ToolName][]TokenValue // Array of tokens per tool for concurrency
    ToolHistory  []ToolName
    CreatedAt    time.Time
}

func (s *Session) RecordToolCall(tool ToolName) {
    s.ToolHistory = append(s.ToolHistory, tool)
}

func (s *Session) AddToken(tool ToolName, token TokenValue) {
    s.Tokens[tool] = append(s.Tokens[tool], token)
}

func (s *Session) RemoveToken(tool ToolName, token TokenValue) {
    tokens := s.Tokens[tool]
    for i, t := range tokens {
        if t == token {
            s.Tokens[tool] = append(tokens[:i], tokens[i+1:]...)
            break
        }
    }
}
```

### Business Rules

1. **Token Uniqueness**: A token is removed from the array when used (absence signifies used or never created)
2. **Token Expiration**: Tokens expire after a configured time period
3. **Dependency Validation**: A tool cannot be called unless its prerequisite has been completed
4. **Session Isolation**: Tokens are scoped to a specific session
5. **Circular Dependencies**: Rules must not create circular dependencies (A depends on B, B depends on A)
6. **Concurrency**: Each tool can have multiple valid tokens in its array for concurrent access

### Error Handling

- Return errors for anything that can go wrong
- For invalid tool use without token, return the error message defined in the YAML configuration file
- Error messages can be instructional to inform the LLM what it needs to do (e.g., "must call List Tags first")

---

## Step 2: Define Use Cases (Primary Ports)

### Use Cases

#### Register Rule

- Add a new rule to the system
- Input: Rule (tool, prerequisite)
- Output: error if validation fails

#### Validate Tool Call

- Check if a tool can be called based on rules and session state
- Input: session ID, tool name, ratchet token (if provided)
- Output: error if tool cannot be called, or success

#### Issue Token

- Issue a new ratchet token after successful tool execution
- Input: session ID, tool name
- Output: new RatchetToken

#### Load Configuration

- Load rules from YAML configuration file
- Input: file path or reader
- Output: list of Rules, error if parsing fails

#### Wrap Function

- Wrap a user's function to require ratchet token validation
- Input: function to wrap, tool name, rule enforcer
- Output: wrapped function

### Interface Definition

```go
// RatchetService defines the primary port for ratchet operations
type RatchetService interface {
    // RegisterRule adds a new tool dependency rule
    RegisterRule(ctx context.Context, rule Rule) error

    // ValidateToolCall checks if a tool can be called
    ValidateToolCall(ctx context.Context, sessionID SessionID, tool ToolName, token TokenValue) error

    // IssueToken creates a new ratchet token after successful execution
    IssueToken(ctx context.Context, sessionID SessionID, tool ToolName) (*RatchetToken, error)

    // LoadConfiguration loads rules from YAML
    LoadConfiguration(ctx context.Context, config io.Reader) ([]Rule, error)
}

// Wrapper defines the primary port for function wrapping
type Wrapper interface {
    // Wrap wraps a function to require ratchet token validation
    Wrap(fn interface{}, tool ToolName) (interface{}, error)
}
```

---

## Step 3: Define External Dependencies (Secondary Ports)

### External Dependencies

#### Configuration Storage

- Read YAML configuration files
- Interface: `ConfigLoader`

#### Token Storage

- Store and retrieve tokens for sessions
- Interface: `TokenStore`

#### Session Storage

- Store and retrieve session state
- Interface: `SessionStore`

#### Random Generator

- Generate cryptographically secure random tokens
- Interface: `RandomGenerator`

#### Clock

- Get current time for token expiration
- Interface: `Clock`

### Interface Definitions

```go
// ConfigLoader loads configuration from YAML
type ConfigLoader interface {
    Load(ctx context.Context, source io.Reader) ([]Rule, error)
}

// TokenStore stores and retrieves ratchet tokens
type TokenStore interface {
    Store(ctx context.Context, sessionID SessionID, token *RatchetToken) error
    Retrieve(ctx context.Context, sessionID SessionID, tool ToolName) (*RatchetToken, error)
    Delete(ctx context.Context, sessionID SessionID, tool ToolName) error
}

// SessionStore stores and retrieves session state
type SessionStore interface {
    Create(ctx context.Context, session *Session) error
    Get(ctx context.Context, id SessionID) (*Session, error)
    Update(ctx context.Context, session *Session) error
    Delete(ctx context.Context, id SessionID) error
}

// RandomGenerator generates cryptographically secure random values
type RandomGenerator interface {
    GenerateToken() (TokenValue, error)
    GenerateSessionID() (SessionID, error)
}

// Clock provides time functionality
type Clock interface {
    Now() time.Time
}
```

---

## Step 4: Plan Services Implementation

### Service Implementation

#### RatchetServiceImpl

- Implements `RatchetService` interface
- Coordinates domain entities with secondary ports
- Core logic:
  1. Validate tool call by checking rules and session state
  2. Issue new tokens after successful execution
  3. Load and validate configuration

#### WrapperImpl

- Implements `Wrapper` interface
- Uses reflection to wrap functions
- Validates ratchet token before calling wrapped function
- Issues new token after successful execution

### Key Logic

```go
type RatchetServiceImpl struct {
    configLoader  ConfigLoader
    tokenStore    TokenStore
    sessionStore  SessionStore
    randomGen     RandomGenerator
    clock         Clock
    rules         []Rule
}

func (s *RatchetServiceImpl) ValidateToolCall(ctx context.Context, sessionID SessionID, tool ToolName, token TokenValue) error {
    // 1. Find rule for this tool
    rule := s.findRule(tool)
    if rule == nil {
        // No rule means tool is unrestricted
        return nil
    }

    // 2. Get session
    session, err := s.sessionStore.Get(ctx, sessionID)
    if err != nil {
        return err
    }

    // 3. Check if prerequisite has been called
    if !s.hasToolBeenCalled(session, rule.Prerequisite) {
        return fmt.Errorf("prerequisite tool %s must be called first", rule.Prerequisite)
    }

    // 4. Validate token
    storedToken, err := s.tokenStore.Retrieve(ctx, sessionID, tool)
    if err != nil {
        return err
    }

    if storedToken.Value != token {
        return errors.New("invalid token")
    }

    if err := storedToken.IsValid(); err != nil {
        return err
    }

    return nil
}

func (s *RatchetServiceImpl) IssueToken(ctx context.Context, sessionID SessionID, tool ToolName) (*RatchetToken, error) {
    tokenValue, err := s.randomGen.GenerateToken()
    if err != nil {
        return nil, err
    }

    token := &RatchetToken{
        Value:     tokenValue,
        SessionID: sessionID,
        Tool:      tool,
        ExpiresAt: s.clock.Now().Add(5 * time.Minute), // Configurable
        Used:      false,
    }

    if err := s.tokenStore.Store(ctx, sessionID, token); err != nil {
        return nil, err
    }

    return token, nil
}
```

---

## Step 5: Plan Adapters Implementation

### Primary Adapters

#### YAML Config Loader

- Implements `ConfigLoader`
- Parses YAML files into Rule entities
- Validates rules (no circular dependencies)
- **Circular Dependency Detection**: Go script at startup validates the rule graph to detect and prevent circular dependencies

#### In-Memory Token Store

- Implements `TokenStore`
- Stores tokens in memory map
- Suitable for single-process deployments
- **Persistence**: No disk persistence required - tokens are in-memory only for the session

#### In-Memory Session Store

- Implements `SessionStore`
- Stores sessions in memory map
- Suitable for single-process deployments

#### Crypto Random Generator

- Implements `RandomGenerator`
- Uses crypto/rand for secure random generation

#### Real Clock

- Implements `Clock`
- Uses time.Now()

### Secondary Adapters

#### HTTP Handler (if needed)

- Exposes ratchet service via HTTP API
- Handles session management
- Validates tokens

#### Middleware

- HTTP middleware to wrap endpoints with ratchet validation
- Automatically handles token validation and issuance

### Example Usage

```go
// Setup
configLoader := NewYAMLConfigLoader()
tokenStore := NewMemoryTokenStore()
sessionStore := NewMemorySessionStore()
randomGen := NewCryptoRandomGenerator()
clock := NewRealClock()

service := NewRatchetService(configLoader, tokenStore, sessionStore, randomGen, clock)

// Load configuration
rules, err := service.LoadConfiguration(ctx, configFile)
if err != nil {
    log.Fatal(err)
}

// Wrap a function
wrapper := NewWrapper(service, randomGen)

originalFunc := func(ctx context.Context, arg string) error {
    // Do something
    return nil
}

wrappedFunc, err := wrapper.Wrap(originalFunc, "MyTool")
if err != nil {
    log.Fatal(err)
}

// Use wrapped function
sessionID := SessionID("session-123")
token, err := service.IssueToken(ctx, sessionID, "PrerequisiteTool")
if err != nil {
    log.Fatal(err)
}

// Call with token
err = wrappedFunc.(func(context.Context, string, TokenValue) error)(ctx, "arg", token.Value)
```

---

## YAML Configuration Example

```yaml
rules:
  - tool: "Create Cell"
    prerequisite: "List Tags"
    expiry: "5m" # Optional, defaults to sensible default
    error_message: "Must call List Tags before Create Cell" # Instructional error for LLM
  - tool: "Put Cell"
    prerequisite: "List Tags"
    expiry: "10m" # Optional, tool-specific expiry
    error_message: "Must call List Tags before Put Cell"
  - tool: "Delete Cell"
    prerequisite: "Get Cell"
    # No expiry specified, uses default
    error_message: "Must call Get Cell before Delete Cell"
```

---

## Open Questions

None - all questions have been resolved.
