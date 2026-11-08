package adapters

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
)

func TestMemoryEventStoreConcurrentStop(t *testing.T) {
	t.Parallel()
	for range 100 {
		store := NewMemoryEventStore(1).(*MemoryEventStore)
		start := make(chan struct{})
		var callers sync.WaitGroup
		for range 32 {
			callers.Go(func() {
				<-start
				store.Stop()
			})
		}
		close(start)
		callers.Wait()
		select {
		case <-store.stopCh:
		default:
			t.Fatal("Stop left the pruning goroutine's stop channel open")
		}
	}
}

func TestMemoryEventStorePruneClearsExpiredTail(t *testing.T) {
	t.Parallel()
	now := time.Now()
	expired := &domain.Event{Timestamp: now.AddDate(0, 0, -2)}
	first := &domain.Event{Timestamp: now}
	second := &domain.Event{Timestamp: now}
	backing := []*domain.Event{expired, first, expired, second}
	store := &MemoryEventStore{retentionDays: 1, events: backing}

	store.mu.Lock()
	store.pruneExpired()
	store.mu.Unlock()

	if !slices.Equal(store.events, []*domain.Event{first, second}) {
		t.Fatalf("pruneExpired kept %v, want %v then %v", store.events, first, second)
	}
	for _, event := range backing[len(store.events):] {
		if event != nil {
			t.Fatal("pruneExpired left an event referenced in the discarded tail")
		}
	}
}
