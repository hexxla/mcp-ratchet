package adapters

import (
	"slices"
	"sync"
	"time"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet"
)

const pruneInterval = 5 * time.Minute

// EventFilter restricts historical results. Zero fields impose no restriction.
// Time bounds are inclusive; Limit returns the first matching events in storage
// order. A nonpositive Limit imposes no limit.
type EventFilter struct {
	EventTypes []ratchet.EventType
	ToolNames  []string
	StartTime  time.Time
	EndTime    time.Time
	Limit      int
}

// EventStats describes all retained history. Sessions counts distinct session
// IDs in that history, rather than sessions currently active in an engine.
type EventStats struct {
	TotalEvents         int
	EventsByType        map[ratchet.EventType]int
	EventsByTool        map[string]int
	Sessions            int
	CompletionsRecorded int
	CompletionsConsumed int
}

// MemoryEventStore is an optional process-local historical observer. Event
// values and query slices are detached; concurrent calls are safe. Nonpositive
// retentionDays keeps all history. Positive retention starts a five-minute
// pruning loop; call Stop on shutdown. This adapter is independent of Engine.
type MemoryEventStore struct {
	mu            sync.RWMutex
	events        []ratchet.Event
	retentionDays int
	stopCh        chan struct{}
	stop          func()
}

// NewMemoryEventStore creates an observer suitable for demos and tests. Pass its
// Observe method as ratchet.Options.Observer to capture engine events.
func NewMemoryEventStore(retentionDays int) *MemoryEventStore {
	stopCh := make(chan struct{})
	m := &MemoryEventStore{
		events: make([]ratchet.Event, 0), retentionDays: retentionDays,
		stopCh: stopCh, stop: sync.OnceFunc(func() { close(stopCh) }),
	}
	if retentionDays > 0 {
		go m.runPruner()
	}
	return m
}

// Stop signals the pruning goroutine to exit. Concurrent calls are safe.
func (m *MemoryEventStore) Stop() { m.stop() }

// Observe appends a value snapshot. It does not retain any caller-owned pointer.
func (m *MemoryEventStore) Observe(event ratchet.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *MemoryEventStore) runPruner() {
	ticker := time.NewTicker(pruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.mu.Lock()
			m.pruneExpired()
			m.mu.Unlock()
		case <-m.stopCh:
			return
		}
	}
}

// pruneExpired runs with mu held. DeleteFunc clears discarded tail references.
func (m *MemoryEventStore) pruneExpired() {
	cutoff := time.Now().AddDate(0, 0, -m.retentionDays)
	m.events = slices.DeleteFunc(m.events, func(event ratchet.Event) bool {
		return !event.Timestamp.After(cutoff)
	})
}

// Events returns detached event values for one session matching filter.
func (m *MemoryEventStore) Events(sessionID string, filter *EventFilter) []ratchet.Event {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]ratchet.Event, 0)
	for _, event := range m.events {
		if event.SessionID != sessionID || (filter != nil && !matchesFilter(event, filter)) {
			continue
		}
		result = append(result, event)
		if filter != nil && filter.Limit > 0 && len(result) >= filter.Limit {
			break
		}
	}
	return result
}

// Stats returns detached aggregate maps across all retained events.
func (m *MemoryEventStore) Stats() EventStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	stats := EventStats{EventsByType: make(map[ratchet.EventType]int), EventsByTool: make(map[string]int)}
	sessions := make(map[string]struct{})
	for _, event := range m.events {
		stats.TotalEvents++
		stats.EventsByType[event.Type]++
		if event.ToolName != "" {
			stats.EventsByTool[event.ToolName]++
		}
		sessions[event.SessionID] = struct{}{}
		switch event.Type {
		case ratchet.EventTypeCompletionRecorded:
			stats.CompletionsRecorded++
		case ratchet.EventTypeCompletionConsumed:
			stats.CompletionsConsumed++
		}
	}
	stats.Sessions = len(sessions)
	return stats
}

func matchesFilter(event ratchet.Event, filter *EventFilter) bool {
	return (len(filter.EventTypes) == 0 || slices.Contains(filter.EventTypes, event.Type)) &&
		(len(filter.ToolNames) == 0 || slices.Contains(filter.ToolNames, event.ToolName)) &&
		(filter.StartTime.IsZero() || !event.Timestamp.Before(filter.StartTime)) &&
		(filter.EndTime.IsZero() || !event.Timestamp.After(filter.EndTime))
}
