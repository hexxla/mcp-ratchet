package secondary

import (
	"context"
	"errors"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
)

// ErrSessionNotFound is returned when a session is not found.
var ErrSessionNotFound = errors.New("session not found")

// SessionStore stores and retrieves session state
type SessionStore interface {
	Create(ctx context.Context, session *domain.Session) error
	Get(ctx context.Context, id domain.SessionID) (*domain.Session, error)
	Update(ctx context.Context, session *domain.Session) error
	Delete(ctx context.Context, id domain.SessionID) error
}
