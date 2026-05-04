package adapters

import (
	"context"
	"fmt"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
)

// EventBroadcaster defines the interface for broadcasting events to subscribers.
// The MCP server layer implements this to push events to WebSocket clients.
type EventBroadcaster interface {
	// Broadcast sends an event to all subscribers for the given session.
	Broadcast(sessionID domain.SessionID, event *domain.Event)
}

// BroadcastingEventStore wraps an EventStore and broadcasts events to subscribers.
// This bridges the gap between ratchet's event emission and real-time streaming.
//
// Usage in MCP server (main.go):
//
//	broadcaster := newEventBroadcaster()
//	baseStore := adapters.NewMemoryEventStore(retentionDays)
//	store := adapters.NewBroadcastingEventStore(baseStore, broadcaster)
//	ratchetSvc := services.NewRatchetServiceWithObservability(..., store)
//
// Now when ratchet emits events, they are both stored AND broadcast.
type BroadcastingEventStore struct {
	base        secondary.EventStore
	broadcaster EventBroadcaster
}

// NewBroadcastingEventStore creates a wrapping EventStore that broadcasts events.
func NewBroadcastingEventStore(base secondary.EventStore, broadcaster EventBroadcaster) secondary.EventStore {
	return &BroadcastingEventStore{
		base:        base,
		broadcaster: broadcaster,
	}
}

// Store persists the event and broadcasts it to subscribers.
// The broadcast is best-effort; errors don't affect the store operation.
func (b *BroadcastingEventStore) Store(ctx context.Context, event *domain.Event) error {
	// Store first
	if err := b.base.Store(ctx, event); err != nil {
		return fmt.Errorf("failed to store event: %w", err)
	}
	// Broadcast after successful store
	if b.broadcaster != nil {
		b.broadcaster.Broadcast(event.SessionID, event)
	}
	return nil
}

// GetEvents delegates to the underlying store.
func (b *BroadcastingEventStore) GetEvents(ctx context.Context, sessionID domain.SessionID, filter *secondary.EventFilter) ([]*domain.Event, error) {
	return b.base.GetEvents(ctx, sessionID, filter)
}

// GetStats delegates to the underlying store.
func (b *BroadcastingEventStore) GetStats(ctx context.Context) (*domain.EventStats, error) {
	return b.base.GetStats(ctx)
}
