package adapters

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
)

const pruneInterval = 5 * time.Minute

// MemoryEventStore implements EventStore using in-memory storage.
// It is safe for concurrent use and suitable for demos and testing.
// When retentionDays > 0, a background goroutine prunes expired events every 5 minutes.
// Call Stop() to release the background goroutine on shutdown.
type MemoryEventStore struct {
	mu            sync.RWMutex
	events        []*domain.Event
	retentionDays int
	stopCh        chan struct{}
}

// NewMemoryEventStore creates a new in-memory event store.
// retentionDays controls how long events are kept; 0 means keep all events.
// When retentionDays > 0, a background pruning goroutine is started — call Stop() to clean up.
func NewMemoryEventStore(retentionDays int) secondary.EventStore {
	m := &MemoryEventStore{
		events:        make([]*domain.Event, 0),
		retentionDays: retentionDays,
		stopCh:        make(chan struct{}),
	}

	if retentionDays > 0 {
		go m.runPruner()
	}

	return m
}

// Stop stops the background pruning goroutine. Safe to call multiple times.
func (m *MemoryEventStore) Stop() {
	select {
	case <-m.stopCh:
	default:
		close(m.stopCh)
	}
}

// Store persists an event in memory.
func (m *MemoryEventStore) Store(_ context.Context, event *domain.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

// runPruner runs in a background goroutine, pruning expired events on a fixed interval.
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

// pruneExpired removes events older than retentionDays.
// Must be called with m.mu held (write lock).
func (m *MemoryEventStore) pruneExpired() {
	cutoff := time.Now().AddDate(0, 0, -m.retentionDays)
	kept := m.events[:0]
	for _, e := range m.events {
		if e.Timestamp.After(cutoff) {
			kept = append(kept, e)
		}
	}
	m.events = kept
}

// GetEvents retrieves events for a session matching the given filter.
func (m *MemoryEventStore) GetEvents(_ context.Context, sessionID domain.SessionID, filter *secondary.EventFilter) ([]*domain.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*domain.Event, 0)
	for _, e := range m.events {
		if e.SessionID != sessionID {
			continue
		}
		if filter != nil && !matchesFilter(e, filter) {
			continue
		}
		result = append(result, e)
		if filter != nil && filter.Limit > 0 && len(result) >= filter.Limit {
			break
		}
	}

	return result, nil
}

// GetStats returns aggregate metrics across all stored events.
func (m *MemoryEventStore) GetStats(_ context.Context) (*domain.EventStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &domain.EventStats{
		EventsByType: make(map[domain.EventType]int),
		EventsByTool: make(map[domain.ToolName]int),
	}

	sessions := make(map[domain.SessionID]struct{})
	for _, e := range m.events {
		stats.TotalEvents++
		stats.EventsByType[e.Type]++
		if e.ToolName != "" {
			stats.EventsByTool[e.ToolName]++
		}
		sessions[e.SessionID] = struct{}{}

		switch e.Type {
		case domain.EventTypeTokenCreated:
			stats.TokensIssued++
		case domain.EventTypeTokenConsumed:
			stats.TokensConsumed++
		}
	}

	stats.ActiveSessions = len(sessions)

	return stats, nil
}

// matchesFilter returns true if the event matches the filter criteria.
func matchesFilter(e *domain.Event, f *secondary.EventFilter) bool {
	if len(f.EventTypes) > 0 && !slices.Contains(f.EventTypes, e.Type) {
		return false
	}
	if len(f.ToolNames) > 0 && !slices.Contains(f.ToolNames, e.ToolName) {
		return false
	}
	if !f.StartTime.IsZero() && e.Timestamp.Before(f.StartTime) {
		return false
	}
	if !f.EndTime.IsZero() && e.Timestamp.After(f.EndTime) {
		return false
	}
	return true
}
