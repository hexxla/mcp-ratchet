package secondary

import (
	"context"
	"errors"
	"time"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
)

// ErrTokenNotFound is returned when a token is not found.
var ErrTokenNotFound = errors.New("token not found")

// TokenStore stores and retrieves ratchet tokens
type TokenStore interface {
	Store(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName, token domain.TokenValue, expiry time.Time) error
	GetValidTokens(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName) ([]domain.TokenValue, error)
	RemoveToken(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName, token domain.TokenValue) error
}
