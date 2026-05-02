package domain

import (
	"testing"
)

func TestToolNameValidate(t *testing.T) {
	tests := []struct {
		name    string
		tool    ToolName
		wantErr bool
	}{
		{"valid tool", ToolName("List Tags"), false},
		{"empty tool", ToolName(""), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.tool.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("ToolName.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTokenValueValidate(t *testing.T) {
	tests := []struct {
		name    string
		token   TokenValue
		wantErr bool
	}{
		{"valid token", TokenValue("a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6"), false},
		{"short token", TokenValue("short"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.token.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("TokenValue.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSessionIDValidate(t *testing.T) {
	tests := []struct {
		name    string
		id      SessionID
		wantErr bool
	}{
		{"valid session", SessionID("session-123"), false},
		{"empty session", SessionID(""), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.id.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("SessionID.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
