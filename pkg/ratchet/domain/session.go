package domain

import (
	"slices"
	"time"
)

// Session tracks state for an interaction
type Session struct {
	ID          SessionID
	Tokens      map[ToolName][]TokenValue // Array of tokens per tool for concurrency
	ToolHistory []ToolName
	CreatedAt   time.Time
}

// NewSession creates a new session with the given ID using the current time.
// For testable code, prefer NewSessionWithTime.
func NewSession(id SessionID) *Session {
	return NewSessionWithTime(id, time.Now())
}

// NewSessionWithTime creates a new session with the given ID and creation time.
// This allows tests to inject specific times for deterministic testing.
func NewSessionWithTime(id SessionID, createdAt time.Time) *Session {
	return &Session{
		ID:          id,
		Tokens:      make(map[ToolName][]TokenValue),
		ToolHistory: make([]ToolName, 0),
		CreatedAt:   createdAt,
	}
}

// RecordToolCall records that a tool was called in this session.
func (s *Session) RecordToolCall(tool ToolName) {
	s.ToolHistory = append(s.ToolHistory, tool)
}

// AddToken adds a token to the session for the given tool.
func (s *Session) AddToken(tool ToolName, token TokenValue) {
	s.Tokens[tool] = append(s.Tokens[tool], token)
}

// RemoveToken removes a token from the session for the given tool.
func (s *Session) RemoveToken(tool ToolName, token TokenValue) {
	tokens := s.Tokens[tool]
	for i, t := range tokens {
		if t == token {
			s.Tokens[tool] = append(tokens[:i], tokens[i+1:]...)
			break
		}
	}
}

// HasValidToken checks if the given token is valid for the specified tool.
func (s *Session) HasValidToken(tool ToolName, token TokenValue) bool {
	return slices.Contains(s.Tokens[tool], token)
}

// HasToolBeenCalled checks if the given tool has been called in this session.
func (s *Session) HasToolBeenCalled(tool ToolName) bool {
	return slices.Contains(s.ToolHistory, tool)
}
