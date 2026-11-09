package adapters_test

import (
	"sync"
	"testing"
	"time"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/adapters"
)

func TestMemoryEventStoreHistoryAndFilters(t *testing.T) {
	store := adapters.NewMemoryEventStore(0)
	now := time.Now()
	for _, event := range []ratchet.Event{
		{ID: "1", Type: ratchet.EventTypeCompletionRecorded, SessionID: "a", ToolName: "read", Timestamp: now},
		{ID: "2", Type: ratchet.EventTypeToolCallSuccess, SessionID: "b", ToolName: "read", Timestamp: now},
		{ID: "3", Type: ratchet.EventTypeCompletionConsumed, SessionID: "a", ToolName: "read", Timestamp: now.Add(time.Second)},
		{ID: "4", Type: ratchet.EventTypeToolCallFailure, SessionID: "a", ToolName: "write", Timestamp: now.Add(2 * time.Second)},
	} {
		store.Observe(event)
	}
	for _, tt := range []struct {
		name, session string
		filter        *adapters.EventFilter
		want          int
	}{
		{"session", "a", nil, 3}, {"missing session", "other", nil, 0},
		{"type", "a", &adapters.EventFilter{EventTypes: []ratchet.EventType{ratchet.EventTypeCompletionRecorded}}, 1},
		{"tool", "a", &adapters.EventFilter{ToolNames: []string{"write"}}, 1},
		{"inclusive time", "a", &adapters.EventFilter{StartTime: now, EndTime: now.Add(time.Second)}, 2},
		{"limit", "a", &adapters.EventFilter{Limit: 2}, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := store.Events(tt.session, tt.filter); len(got) != tt.want {
				t.Fatalf("Events = %v, want %d", got, tt.want)
			}
		})
	}
	stats := store.Stats()
	if stats.TotalEvents != 4 || stats.Sessions != 2 || stats.CompletionsRecorded != 1 || stats.CompletionsConsumed != 1 || stats.EventsByTool["read"] != 3 || stats.EventsByType[ratchet.EventTypeToolCallFailure] != 1 {
		t.Fatalf("Stats = %+v", stats)
	}
	stats.EventsByTool["read"] = 999
	if store.Stats().EventsByTool["read"] != 3 {
		t.Fatal("statistics maps alias store state")
	}
}

func TestMemoryEventStoreDetachedValues(t *testing.T) {
	store := adapters.NewMemoryEventStore(0)
	event := ratchet.Event{ID: "original", SessionID: "owner", ToolName: "read"}
	store.Observe(event)
	event.ID = "changed"
	event.SessionID = "other"
	first := store.Events("owner", nil)
	if len(first) != 1 || first[0].ID != "original" {
		t.Fatalf("input mutation changed stored event: %v", first)
	}
	first[0].ID = "reader changed"
	first[0].SessionID = "other"
	second := store.Events("owner", nil)
	if len(second) != 1 || second[0].ID != "original" || len(store.Events("other", nil)) != 0 {
		t.Fatalf("output mutation changed stored event: %v", second)
	}
}

func TestMemoryEventStoreConcurrentDetachedMutation(t *testing.T) {
	store := adapters.NewMemoryEventStore(0)
	store.Observe(ratchet.Event{ID: "original", SessionID: "s", ToolName: "read"})
	first := store.Events("s", nil)
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Go(func() {
		<-start
		for range 1000 {
			first[0].ID = "changed"
			first[0].SessionID = "other"
		}
	})
	workers.Go(func() {
		<-start
		for range 1000 {
			store.Observe(ratchet.Event{SessionID: "different"})
		}
	})
	close(start)
	for range 1000 {
		_ = store.Stats()
		if got := store.Events("s", nil); len(got) != 1 || got[0].ID != "original" {
			t.Errorf("concurrent mutation changed history: %v", got)
		}
	}
	workers.Wait()
}

func TestMemoryEventStoreEmpty(t *testing.T) {
	store := adapters.NewMemoryEventStore(0)
	if got := store.Events("missing", nil); got == nil || len(got) != 0 {
		t.Fatalf("empty Events = %v", got)
	}
	if stats := store.Stats(); stats.TotalEvents != 0 || stats.Sessions != 0 {
		t.Fatalf("empty Stats = %+v", stats)
	}
}
