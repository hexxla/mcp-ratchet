package adapters

import (
	"context"
	"maps"
	"slices"
	"sync"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
)

// MemorySessionStore implements SessionStore using in-memory storage
type MemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[domain.SessionID]*domain.Session
}

// NewMemorySessionStore creates a new in-memory session store
func NewMemorySessionStore() secondary.SessionStore {
	return &MemorySessionStore{
		sessions: make(map[domain.SessionID]*domain.Session),
	}
}

// Create creates a new session. Returns ErrSessionAlreadyExists if a session
// with the same ID already exists (use Update to overwrite).
func (m *MemorySessionStore) Create(ctx context.Context, session *domain.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[session.ID]; ok {
		return secondary.ErrSessionAlreadyExists
	}
	m.sessions[session.ID] = cloneSession(session)
	return nil
}

// Get retrieves a session (returns a copy to enforce proper Update() usage)
func (m *MemorySessionStore) Get(ctx context.Context, id domain.SessionID) (*domain.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[id]
	if !ok {
		return nil, secondary.ErrSessionNotFound
	}

	return cloneSession(session), nil
}

// Update updates a session
func (m *MemorySessionStore) Update(ctx context.Context, session *domain.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[session.ID] = cloneSession(session)
	return nil
}

// Delete deletes a session.
// Returns ErrSessionNotFound if the session does not exist.
func (m *MemorySessionStore) Delete(ctx context.Context, id domain.SessionID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[id]; !ok {
		return secondary.ErrSessionNotFound
	}

	delete(m.sessions, id)
	return nil
}

// cloneSession detaches both the map and its token slices from the caller.
func cloneSession(session *domain.Session) *domain.Session {
	snapshot := *session
	snapshot.Tokens = maps.Clone(session.Tokens)
	for tool, tokens := range snapshot.Tokens {
		snapshot.Tokens[tool] = slices.Clone(tokens)
	}
	snapshot.ToolHistory = slices.Clone(session.ToolHistory)
	return &snapshot
}
