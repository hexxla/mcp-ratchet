package secondary

import (
	"context"
	"time"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
)

// EventFilter specifies criteria for retrieving events from an EventStore.
type EventFilter struct {
	// EventTypes restricts results to specific event types. Empty means all types.
	EventTypes []domain.EventType

	// ToolNames restricts results to specific tools. Empty means all tools.
	ToolNames []domain.ToolName

	// StartTime restricts results to events at or after this time. Zero means no lower bound.
	StartTime time.Time

	// EndTime restricts results to events before or at this time. Zero means no upper bound.
	EndTime time.Time

	// Limit caps the number of results returned. Zero means no limit.
	Limit int
}

// EventStore defines the secondary port for storing and retrieving ratchet observability events.
// Implementations must be safe for concurrent use.
// The event store is a pure side-effect sink — it never influences core ratchet decisions.
type EventStore interface {
	// Store persists an event. Errors should be logged by the caller but never
	// cause the core ratchet operation to fail.
	Store(ctx context.Context, event *domain.Event) error

	// GetEvents retrieves events for a session matching the given filter.
	GetEvents(ctx context.Context, sessionID domain.SessionID, filter *EventFilter) ([]*domain.Event, error)

	// GetStats returns aggregate metrics across all stored events.
	GetStats(ctx context.Context) (*domain.EventStats, error)
}
