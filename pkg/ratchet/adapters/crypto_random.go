package adapters

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
)

// CryptoRandomGenerator implements RandomGenerator using crypto/rand
type CryptoRandomGenerator struct{}

// NewCryptoRandomGenerator creates a new crypto random generator
func NewCryptoRandomGenerator() secondary.RandomGenerator {
	return &CryptoRandomGenerator{}
}

// GenerateToken generates a cryptographically secure random token
func (c *CryptoRandomGenerator) GenerateToken() (domain.TokenValue, error) {
	// Generate enough bytes to produce MinTokenLength hex characters (2 hex chars per byte)
	bytes := make([]byte, domain.MinTokenLength/2)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return domain.TokenValue(hex.EncodeToString(bytes)), nil
}

// GenerateSessionID generates a cryptographically secure session ID
func (c *CryptoRandomGenerator) GenerateSessionID() (domain.SessionID, error) {
	token, err := c.GenerateToken()
	if err != nil {
		return "", err
	}
	return domain.SessionID(token), nil
}
