package domain

import (
	"errors"
)

// ToolName identifies an MCP tool
type ToolName string

// Validate checks if the tool name is valid.
func (t ToolName) Validate() error {
	if t == "" {
		return errors.New("tool name cannot be empty")
	}
	return nil
}

// TokenValue represents a ratchet token
type TokenValue string

// Validate checks if the token value is valid.
func (t TokenValue) Validate() error {
	if len(t) < 32 {
		return errors.New("token must be at least 32 characters")
	}
	return nil
}

// SessionID uniquely identifies a session
type SessionID string

// Validate checks if the session ID is valid.
func (s SessionID) Validate() error {
	if s == "" {
		return errors.New("session ID cannot be empty")
	}
	return nil
}
