package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/adapters"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
)

func TestRatchetServiceImpl_RegisterRule(t *testing.T) {
	service := NewRatchetService(
		adapters.NewYAMLConfigLoader(),
		adapters.NewMemoryTokenStore(),
		adapters.NewMemorySessionStore(),
		adapters.NewCryptoRandomGenerator(),
		adapters.NewRealClock(),
	)

	rule := domain.Rule{
		Tool:         "Create Cell",
		Prerequisite: "List Tags",
		Expiry:       5 * time.Minute,
	}

	err := service.RegisterRule(context.Background(), rule)
	if err != nil {
		t.Errorf("RegisterRule() error = %v", err)
	}

	// Invalid rule should fail
	invalidRule := domain.Rule{
		Tool:         "Create Cell",
		Prerequisite: "Create Cell", // Self-dependency
	}
	err = service.RegisterRule(context.Background(), invalidRule)
	if err == nil {
		t.Error("Expected error for self-dependency rule")
	}
}

func TestRatchetServiceImpl_IssueToken(t *testing.T) {
	sessionStore := adapters.NewMemorySessionStore()
	service := NewRatchetService(
		adapters.NewYAMLConfigLoader(),
		adapters.NewMemoryTokenStore(),
		sessionStore,
		adapters.NewCryptoRandomGenerator(),
		adapters.NewRealClock(),
	)

	sessionID := domain.SessionID("test-session")
	session := domain.NewSession(sessionID)

	if err := sessionStore.Create(context.Background(), session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	token, err := service.IssueToken(context.Background(), sessionID, "List Tags")
	if err != nil {
		t.Errorf("IssueToken() error = %v", err)
	}
	if len(token) < 32 {
		t.Error("Expected token to be at least 32 characters")
	}
}

func TestRatchetServiceImpl_LoadConfiguration(t *testing.T) {
	service := NewRatchetService(
		adapters.NewYAMLConfigLoader(),
		adapters.NewMemoryTokenStore(),
		adapters.NewMemorySessionStore(),
		adapters.NewCryptoRandomGenerator(),
		adapters.NewRealClock(),
	)

	yamlConfig := `
rules:
  - tool: "Create Cell"
    prerequisite: "List Tags"
    expiry: "5m"
    error_message: "Must call List Tags first"
`
	rules, err := service.LoadConfiguration(context.Background(), strings.NewReader(yamlConfig))
	if err != nil {
		t.Errorf("LoadConfiguration() error = %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(rules))
	}
}

func TestRatchetServiceImpl_GetRequiredPrerequisite(t *testing.T) {
	service := NewRatchetService(
		adapters.NewYAMLConfigLoader(),
		adapters.NewMemoryTokenStore(),
		adapters.NewMemorySessionStore(),
		adapters.NewCryptoRandomGenerator(),
		adapters.NewRealClock(),
	)

	rule := domain.Rule{
		Tool:         "Create Cell",
		Prerequisite: "List Tags",
		Expiry:       5 * time.Minute,
	}
	if err := service.RegisterRule(context.Background(), rule); err != nil {
		t.Fatalf("Failed to register rule: %v", err)
	}

	prereq, err := service.GetRequiredPrerequisite("Create Cell")
	if err != nil {
		t.Errorf("GetRequiredPrerequisite() error = %v", err)
	}
	if prereq != "List Tags" {
		t.Errorf("Expected prerequisite 'List Tags', got '%s'", prereq)
	}
}
