package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
)

const (
	maxWorkflowSessions = 1024
	workflowIdleTimeout = 30 * time.Minute
	workflowSweepPeriod = time.Minute
)

type workflowSessions struct {
	mu        sync.Mutex
	entries   map[domain.SessionID]*workflowSession
	nextSweep time.Time
}

type workflowSession struct {
	running chan struct{}
	users   int
	idleAt  time.Time
}

// acquireWorkflowSession serializes one session without blocking unrelated
// sessions. Idle state is reclaimed during subsequent activity.
func (s *RatchetServiceImpl) acquireWorkflowSession(ctx context.Context, id domain.SessionID) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	state := &s.workflow
	state.mu.Lock()
	now := s.clock.Now()
	if state.entries == nil {
		state.entries = make(map[domain.SessionID]*workflowSession)
	}
	entry := state.entries[id]
	if !now.Before(state.nextSweep) ||
		(entry != nil && entry.users == 0 && !now.Before(entry.idleAt.Add(workflowIdleTimeout))) {
		if err := s.sweepWorkflowSessions(ctx, now); err != nil {
			state.mu.Unlock()
			return nil, err
		}
		state.nextSweep = now.Add(workflowSweepPeriod)
	}
	entry = state.entries[id]
	if entry == nil {
		if len(state.entries) >= maxWorkflowSessions {
			state.mu.Unlock()
			return nil, errors.New("ratchet session limit reached")
		}
		entry = &workflowSession{running: make(chan struct{}, 1)}
		state.entries[id] = entry
	}
	entry.users++
	state.mu.Unlock()

	releaseUser := func() {
		state.mu.Lock()
		entry.users--
		if entry.users == 0 {
			entry.idleAt = s.clock.Now()
		}
		state.mu.Unlock()
	}
	select {
	case entry.running <- struct{}{}:
		return func() {
			<-entry.running
			releaseUser()
		}, nil
	case <-ctx.Done():
		releaseUser()
		return nil, ctx.Err()
	}
}

// sweepWorkflowSessions runs with workflow.mu held. Queued and running calls
// retain their session until the last caller releases it.
func (s *RatchetServiceImpl) sweepWorkflowSessions(ctx context.Context, now time.Time) error {
	for id, entry := range s.workflow.entries {
		if entry.users != 0 || now.Before(entry.idleAt.Add(workflowIdleTimeout)) {
			continue
		}
		if err := s.tokenStore.DeleteSession(ctx, id); err != nil {
			return fmt.Errorf("clear idle Ratchet tokens: %w", err)
		}
		if err := s.sessionStore.Delete(ctx, id); err != nil && !errors.Is(err, secondary.ErrSessionNotFound) {
			return fmt.Errorf("clear idle Ratchet session: %w", err)
		}
		delete(s.workflow.entries, id)
	}
	return nil
}
