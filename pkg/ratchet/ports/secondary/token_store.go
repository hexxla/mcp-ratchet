package secondary

import (
	"context"
	"time"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
)

// TokenStore stores and retrieves ratchet tokens
type TokenStore interface {
	Store(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName, token domain.TokenValue, expiry time.Time) error
	GetValidTokens(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName) ([]domain.TokenValue, error)
	RemoveToken(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName, token domain.TokenValue) error
}
