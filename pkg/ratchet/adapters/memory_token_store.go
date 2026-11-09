package adapters

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
)

// MaxTokensPerTool bounds the number of pending completions per session and tool.
const MaxTokensPerTool = 64

// MemoryTokenStore stores recent tool completions in memory.
type MemoryTokenStore struct {
	mu     sync.Mutex
	tokens map[domain.SessionID]map[domain.ToolName][]tokenEntry
}

type tokenEntry struct {
	value   domain.TokenValue
	expires time.Time
}

// NewMemoryTokenStore creates an in-memory token store.
func NewMemoryTokenStore() *MemoryTokenStore {
	return &MemoryTokenStore{tokens: make(map[domain.SessionID]map[domain.ToolName][]tokenEntry)}
}

// Store records a completion, dropping the oldest entry at the per-tool limit.
func (m *MemoryTokenStore) Store(_ context.Context, sessionID domain.SessionID, tool domain.ToolName, token domain.TokenValue, expiry time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := m.tokens[sessionID][tool]
	if len(entries) == MaxTokensPerTool {
		entries = slices.Delete(entries, 0, 1)
	}
	m.setEntries(sessionID, tool, append(entries, tokenEntry{value: token, expires: expiry}))
	return nil
}

// LatestToken returns the newest valid token at the supplied time.
func (m *MemoryTokenStore) LatestToken(_ context.Context, sessionID domain.SessionID, tool domain.ToolName, now time.Time) (domain.TokenValue, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := liveEntries(m.tokens[sessionID][tool], now)
	m.setEntries(sessionID, tool, entries)
	if len(entries) == 0 {
		return "", false, nil
	}
	return entries[len(entries)-1].value, true, nil
}

// RemoveToken removes a specific token.
func (m *MemoryTokenStore) RemoveToken(_ context.Context, sessionID domain.SessionID, tool domain.ToolName, token domain.TokenValue) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := m.tokens[sessionID][tool]
	i := slices.IndexFunc(entries, func(entry tokenEntry) bool { return entry.value == token })
	if i < 0 {
		return secondary.ErrTokenNotFound
	}
	m.setEntries(sessionID, tool, slices.Delete(entries, i, i+1))
	return nil
}

// HasLiveTokens prunes expired completions and reports whether a session has state.
func (m *MemoryTokenStore) HasLiveTokens(_ context.Context, sessionID domain.SessionID, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for tool, entries := range m.tokens[sessionID] {
		m.setEntries(sessionID, tool, liveEntries(entries, now))
	}
	return len(m.tokens[sessionID]) != 0, nil
}

// DeleteSession releases the session's token state.
func (m *MemoryTokenStore) DeleteSession(_ context.Context, sessionID domain.SessionID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tokens, sessionID)
	return nil
}

func liveEntries(entries []tokenEntry, now time.Time) []tokenEntry {
	return slices.DeleteFunc(entries, func(entry tokenEntry) bool { return !now.Before(entry.expires) })
}

func (m *MemoryTokenStore) setEntries(sessionID domain.SessionID, tool domain.ToolName, entries []tokenEntry) {
	tools := m.tokens[sessionID]
	if len(entries) == 0 {
		delete(tools, tool)
		if len(tools) == 0 {
			delete(m.tokens, sessionID)
		}
		return
	}
	if tools == nil {
		tools = make(map[domain.ToolName][]tokenEntry)
		m.tokens[sessionID] = tools
	}
	tools[tool] = entries
}
