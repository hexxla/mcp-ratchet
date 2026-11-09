package ratchet

import "time"

// EventType identifies an informational workflow transition.
type EventType string

// Engine lifecycle event types.
const (
	EventTypeSessionCreated     EventType = "session_created"
	EventTypeCompletionRecorded EventType = "completion_recorded"
	EventTypeCompletionConsumed EventType = "completion_consumed"
	EventTypeToolCallAttempt    EventType = "tool_call_attempt"
	EventTypeToolCallSuccess    EventType = "tool_call_success"
	EventTypeToolCallFailure    EventType = "tool_call_failure"
)

// Event is an immutable-by-ownership value: it contains no mutable collections
// or opaque completion credentials. Observers receive independent values.
// ExpiresAt is set for completion events; ConsumedBy identifies the dependent
// tool on completion_consumed. Error contains a denial or handler error.
type Event struct {
	ID         string
	Type       EventType
	SessionID  string
	ToolName   string
	Timestamp  time.Time
	ExpiresAt  time.Time
	ConsumedBy string
	Error      string
}
