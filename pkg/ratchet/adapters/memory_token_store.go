package adapters

import (
	"context"
	"sync"
	"time"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
)

// MemoryTokenStore implements TokenStore using in-memory storage
type MemoryTokenStore struct {
	mu     sync.RWMutex
	tokens map[domain.SessionID]map[domain.ToolName][]tokenEntry
}

type tokenEntry struct {
	value   domain.TokenValue
	expires time.Time
}

// NewMemoryTokenStore creates a new in-memory token store
func NewMemoryTokenStore() secondary.TokenStore {
	return &MemoryTokenStore{
		tokens: make(map[domain.SessionID]map[domain.ToolName][]tokenEntry),
	}
}

// Store stores a token
func (m *MemoryTokenStore) Store(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName, token domain.TokenValue, expiry time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tokens[sessionID]; !ok {
		m.tokens[sessionID] = make(map[domain.ToolName][]tokenEntry)
	}

	m.tokens[sessionID][tool] = append(m.tokens[sessionID][tool], tokenEntry{
		value:   token,
		expires: expiry,
	})

	return nil
}

// GetValidTokens returns all valid (non-expired) tokens for a tool
func (m *MemoryTokenStore) GetValidTokens(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName) ([]domain.TokenValue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessionTokens, ok := m.tokens[sessionID]
	if !ok {
		return []domain.TokenValue{}, nil
	}

	toolTokens, ok := sessionTokens[tool]
	if !ok {
		return []domain.TokenValue{}, nil
	}

	now := time.Now()
	validTokens := make([]domain.TokenValue, 0)
	for _, entry := range toolTokens {
		if now.Before(entry.expires) {
			validTokens = append(validTokens, entry.value)
		}
	}

	return validTokens, nil
}

// RemoveToken removes a specific token.
// Returns ErrTokenNotFound if the token does not exist.
func (m *MemoryTokenStore) RemoveToken(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName, token domain.TokenValue) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessionTokens, ok := m.tokens[sessionID]
	if !ok {
		return secondary.ErrTokenNotFound
	}

	toolTokens, ok := sessionTokens[tool]
	if !ok {
		return secondary.ErrTokenNotFound
	}

	for i, entry := range toolTokens {
		if entry.value == token {
			m.tokens[sessionID][tool] = append(toolTokens[:i], toolTokens[i+1:]...)
			return nil
		}
	}

	return secondary.ErrTokenNotFound
}
