package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/adapters"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
)

func TestMemoryEventStore_Store(t *testing.T) {
	store := adapters.NewMemoryEventStore(0)
	ctx := context.Background()

	event := &domain.Event{
		ID:        "evt-1",
		Type:      domain.EventTypeTokenCreated,
		SessionID: "session-1",
		ToolName:  "greet",
		Timestamp: time.Now(),
	}

	if err := store.Store(ctx, event); err != nil {
		t.Fatalf("Store() error = %v", err)
	}
}

func TestMemoryEventStore_GetEvents_BySession(t *testing.T) {
	store := adapters.NewMemoryEventStore(0)
	ctx := context.Background()
	now := time.Now()

	events := []*domain.Event{
		{ID: "1", Type: domain.EventTypeTokenCreated, SessionID: "session-A", ToolName: "greet", Timestamp: now},
		{ID: "2", Type: domain.EventTypeToolCallSuccess, SessionID: "session-B", ToolName: "greet", Timestamp: now},
		{ID: "3", Type: domain.EventTypeTokenConsumed, SessionID: "session-A", ToolName: "get_user_name", Timestamp: now},
	}
	for _, e := range events {
		_ = store.Store(ctx, e)
	}

	got, err := store.GetEvents(ctx, "session-A", nil)
	if err != nil {
		t.Fatalf("GetEvents() error = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("GetEvents() for session-A returned %d events, want 2", len(got))
	}
}

func TestMemoryEventStore_GetEvents_FilterByEventType(t *testing.T) {
	store := adapters.NewMemoryEventStore(0)
	ctx := context.Background()
	now := time.Now()

	_ = store.Store(ctx, &domain.Event{ID: "1", Type: domain.EventTypeTokenCreated, SessionID: "s1", ToolName: "greet", Timestamp: now})
	_ = store.Store(ctx, &domain.Event{ID: "2", Type: domain.EventTypeToolCallFailure, SessionID: "s1", ToolName: "greet", Timestamp: now})
	_ = store.Store(ctx, &domain.Event{ID: "3", Type: domain.EventTypeTokenCreated, SessionID: "s1", ToolName: "get_user_name", Timestamp: now})

	filter := &secondary.EventFilter{EventTypes: []domain.EventType{domain.EventTypeTokenCreated}}
	got, err := store.GetEvents(ctx, "s1", filter)
	if err != nil {
		t.Fatalf("GetEvents() error = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("GetEvents() with EventType filter returned %d events, want 2", len(got))
	}
}

func TestMemoryEventStore_GetEvents_FilterByLimit(t *testing.T) {
	store := adapters.NewMemoryEventStore(0)
	ctx := context.Background()
	now := time.Now()

	for i := range 5 {
		_ = store.Store(ctx, &domain.Event{
			ID:        domain.EventID("evt-" + string(rune('0'+i))),
			Type:      domain.EventTypeToolCallAttempt,
			SessionID: "s1",
			ToolName:  "greet",
			Timestamp: now,
		})
	}

	filter := &secondary.EventFilter{Limit: 3}
	got, err := store.GetEvents(ctx, "s1", filter)
	if err != nil {
		t.Fatalf("GetEvents() error = %v", err)
	}
	if len(got) != 3 {
		t.Errorf("GetEvents() with Limit=3 returned %d events, want 3", len(got))
	}
}

func TestMemoryEventStore_GetStats(t *testing.T) {
	store := adapters.NewMemoryEventStore(0)
	ctx := context.Background()
	now := time.Now()

	_ = store.Store(ctx, &domain.Event{ID: "1", Type: domain.EventTypeTokenCreated, SessionID: "s1", ToolName: "greet", Timestamp: now})
	_ = store.Store(ctx, &domain.Event{ID: "2", Type: domain.EventTypeTokenCreated, SessionID: "s1", ToolName: "greet", Timestamp: now})
	_ = store.Store(ctx, &domain.Event{ID: "3", Type: domain.EventTypeTokenConsumed, SessionID: "s2", ToolName: "greet", Timestamp: now})
	_ = store.Store(ctx, &domain.Event{ID: "4", Type: domain.EventTypeToolCallFailure, SessionID: "s3", ToolName: "get_user_name", Timestamp: now})

	stats, err := store.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	if stats.TotalEvents != 4 {
		t.Errorf("TotalEvents = %d, want 4", stats.TotalEvents)
	}
	if stats.TokensIssued != 2 {
		t.Errorf("TokensIssued = %d, want 2", stats.TokensIssued)
	}
	if stats.TokensConsumed != 1 {
		t.Errorf("TokensConsumed = %d, want 1", stats.TokensConsumed)
	}
	if stats.ActiveSessions != 3 {
		t.Errorf("ActiveSessions = %d, want 3", stats.ActiveSessions)
	}
}

func TestMemoryEventStore_GetStats_Empty(t *testing.T) {
	store := adapters.NewMemoryEventStore(0)
	ctx := context.Background()

	stats, err := store.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.TotalEvents != 0 {
		t.Errorf("TotalEvents = %d, want 0", stats.TotalEvents)
	}
}
