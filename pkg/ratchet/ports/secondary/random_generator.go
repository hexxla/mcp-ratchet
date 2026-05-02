package secondary

import (
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
)

// RandomGenerator generates cryptographically secure random values
type RandomGenerator interface {
	GenerateToken() (domain.TokenValue, error)
	GenerateSessionID() (domain.SessionID, error)
}
