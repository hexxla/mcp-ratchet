package adapters

import (
	"testing"
)

func TestCryptoRandomGenerator_GenerateToken(t *testing.T) {
	gen := NewCryptoRandomGenerator()

	token, err := gen.GenerateToken()
	if err != nil {
		t.Errorf("GenerateToken() error = %v", err)
	}
	if len(token) < 32 {
		t.Errorf("Expected token to be at least 32 characters, got %d", len(token))
	}
}

func TestCryptoRandomGenerator_GenerateSessionID(t *testing.T) {
	gen := NewCryptoRandomGenerator()

	id, err := gen.GenerateSessionID()
	if err != nil {
		t.Errorf("GenerateSessionID() error = %v", err)
	}
	if len(id) < 32 {
		t.Errorf("Expected session ID to be at least 32 characters, got %d", len(id))
	}
}
