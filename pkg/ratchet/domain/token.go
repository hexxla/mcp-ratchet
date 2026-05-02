package domain

import (
	"errors"
	"time"
)

// RatchetToken represents a one-time use token
type RatchetToken struct {
	Value     TokenValue
	SessionID SessionID
	Tool      ToolName
	ExpiresAt time.Time
}

// IsValid checks if the token is valid (not expired).
func (t *RatchetToken) IsValid() error {
	if time.Now().After(t.ExpiresAt) {
		return errors.New("token has expired")
	}
	return nil
}
