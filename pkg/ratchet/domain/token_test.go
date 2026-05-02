package domain

import (
	"testing"
	"time"
)

func TestRatchetTokenIsValid(t *testing.T) {
	tests := []struct {
		name    string
		token   *RatchetToken
		wantErr bool
	}{
		{"valid token", &RatchetToken{ExpiresAt: time.Now().Add(5 * time.Minute)}, false},
		{"expired token", &RatchetToken{ExpiresAt: time.Now().Add(-5 * time.Minute)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.token.IsValid(); (err != nil) != tt.wantErr {
				t.Errorf("RatchetToken.IsValid() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
