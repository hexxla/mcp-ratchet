package domain

import "time"

// EventType represents the type of a ratchet lifecycle event.
type EventType string

const (
	// EventTypeSessionCreated is emitted when a new session is created.
	EventTypeSessionCreated EventType = "session_created"

	// EventTypeTokenCreated is emitted when a token is issued after successful tool execution.
	EventTypeTokenCreated EventType = "token_created"

	// EventTypeTokenConsumed is emitted when a one-time-use token is consumed.
	EventTypeTokenConsumed EventType = "token_consumed"

	// EventTypeToolCallAttempt is emitted when a tool call is validated (before execution).
	EventTypeToolCallAttempt EventType = "tool_call_attempt"

	// EventTypeToolCallSuccess is emitted after a tool call passes validation and executes.
	EventTypeToolCallSuccess EventType = "tool_call_success"

	// EventTypeToolCallFailure is emitted when a tool call fails ratchet validation.
	EventTypeToolCallFailure EventType = "tool_call_failure"
)

// EventID uniquely identifies an observability event.
type EventID string

// Event represents a ratchet lifecycle event captured for observability.
// Events are purely informational — they do not affect core ratchet behaviour.
type Event struct {
	ID        EventID
	Type      EventType
	SessionID SessionID
	ToolName  ToolName
	Token     TokenValue
	Timestamp time.Time
	Metadata  map[string]any
}

// EventStats provides aggregate metrics across all captured events.
type EventStats struct {
	TotalEvents    int
	EventsByType   map[EventType]int
	EventsByTool   map[ToolName]int
	ActiveSessions int
	TokensIssued   int
	TokensConsumed int
}
