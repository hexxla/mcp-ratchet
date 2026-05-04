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

func TestRatchetServiceImpl_ValidateToolCall_Unrestricted(t *testing.T) {
	// Tool with no rules should be allowed
	service := NewRatchetService(
		adapters.NewYAMLConfigLoader(),
		adapters.NewMemoryTokenStore(),
		adapters.NewMemorySessionStore(),
		adapters.NewCryptoRandomGenerator(),
		adapters.NewRealClock(),
	)

	err := service.ValidateToolCall(context.Background(), "session-1", "UnrestrictedTool", "")
	if err != nil {
		t.Errorf("ValidateToolCall() for unrestricted tool error = %v", err)
	}
}

func TestRatchetServiceImpl_ValidateToolCall_PrerequisiteNotMet(t *testing.T) {
	sessionStore := adapters.NewMemorySessionStore()
	service := NewRatchetService(
		adapters.NewYAMLConfigLoader(),
		adapters.NewMemoryTokenStore(),
		sessionStore,
		adapters.NewCryptoRandomGenerator(),
		adapters.NewRealClock(),
	)

	// Register rule: ToolB requires ToolA
	rule := domain.Rule{
		Tool:         "ToolB",
		Prerequisite: "ToolA",
		Expiry:       5 * time.Minute,
	}
	if err := service.RegisterRule(context.Background(), rule); err != nil {
		t.Fatalf("Failed to register rule: %v", err)
	}

	// Create empty session (ToolA never called)
	sessionID := domain.SessionID("test-session")
	session := domain.NewSession(sessionID)
	if err := sessionStore.Create(context.Background(), session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Should fail - prerequisite not called
	err := service.ValidateToolCall(context.Background(), sessionID, "ToolB", "")
	if err == nil {
		t.Error("Expected error when prerequisite not met")
	}
}

func TestRatchetServiceImpl_ValidateToolCall_PrerequisiteMet_WithToken(t *testing.T) {
	sessionStore := adapters.NewMemorySessionStore()
	tokenStore := adapters.NewMemoryTokenStore()
	service := NewRatchetService(
		adapters.NewYAMLConfigLoader(),
		tokenStore,
		sessionStore,
		adapters.NewCryptoRandomGenerator(),
		adapters.NewRealClock(),
	)

	// Register rule: ToolB requires ToolA
	rule := domain.Rule{
		Tool:         "ToolB",
		Prerequisite: "ToolA",
		Expiry:       5 * time.Minute,
	}
	if err := service.RegisterRule(context.Background(), rule); err != nil {
		t.Fatalf("Failed to register rule: %v", err)
	}

	// Create session with ToolA called and valid token
	sessionID := domain.SessionID("test-session")
	session := domain.NewSession(sessionID)
	session.RecordToolCall("ToolA")
	if err := sessionStore.Create(context.Background(), session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Issue token for ToolA
	token, err := service.IssueToken(context.Background(), sessionID, "ToolA")
	if err != nil {
		t.Fatalf("Failed to issue token: %v", err)
	}

	// Should succeed - prerequisite met with valid token
	err = service.ValidateToolCall(context.Background(), sessionID, "ToolB", token)
	if err != nil {
		t.Errorf("ValidateToolCall() error = %v", err)
	}
}

func TestRatchetServiceImpl_ValidateToolCall_ExpiredToken(t *testing.T) {
	sessionStore := adapters.NewMemorySessionStore()
	tokenStore := adapters.NewMemoryTokenStore()

	// Use a clock that returns expired time
	mockClock := &mockClock{now: time.Now().Add(-10 * time.Minute)}

	service := NewRatchetService(
		adapters.NewYAMLConfigLoader(),
		tokenStore,
		sessionStore,
		adapters.NewCryptoRandomGenerator(),
		mockClock,
	)

	// Register rule with 1 minute expiry
	rule := domain.Rule{
		Tool:         "ToolB",
		Prerequisite: "ToolA",
		Expiry:       1 * time.Minute,
	}
	if err := service.RegisterRule(context.Background(), rule); err != nil {
		t.Fatalf("Failed to register rule: %v", err)
	}

	// Create session
	sessionID := domain.SessionID("test-session")
	session := domain.NewSession(sessionID)
	session.RecordToolCall("ToolA")
	if err := sessionStore.Create(context.Background(), session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Issue token (will be immediately expired due to mock clock)
	token, err := service.IssueToken(context.Background(), sessionID, "ToolA")
	if err != nil {
		t.Fatalf("Failed to issue token: %v", err)
	}

	// Reset clock to real time for validation
	mockClock.now = time.Now()

	// Should fail - token expired
	err = service.ValidateToolCall(context.Background(), sessionID, "ToolB", token)
	if err == nil {
		t.Error("Expected error for expired token")
	}
}

func TestRatchetServiceImpl_ValidateToolCall_CustomErrorMessage(t *testing.T) {
	sessionStore := adapters.NewMemorySessionStore()
	service := NewRatchetService(
		adapters.NewYAMLConfigLoader(),
		adapters.NewMemoryTokenStore(),
		sessionStore,
		adapters.NewCryptoRandomGenerator(),
		adapters.NewRealClock(),
	)

	// Register rule with custom error message
	rule := domain.Rule{
		Tool:         "ToolB",
		Prerequisite: "ToolA",
		Expiry:       5 * time.Minute,
		ErrorMessage: "You must call ToolA first!",
	}
	if err := service.RegisterRule(context.Background(), rule); err != nil {
		t.Fatalf("Failed to register rule: %v", err)
	}

	// Create empty session
	sessionID := domain.SessionID("test-session")
	session := domain.NewSession(sessionID)
	if err := sessionStore.Create(context.Background(), session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Should fail with custom message
	err := service.ValidateToolCall(context.Background(), sessionID, "ToolB", "")
	if err == nil {
		t.Fatal("Expected error")
	}
	if err.Error() != "You must call ToolA first!" {
		t.Errorf("Expected custom error message, got: %v", err)
	}
}

func TestRatchetServiceImpl_ValidateToolCall_MissingSession(t *testing.T) {
	service := NewRatchetService(
		adapters.NewYAMLConfigLoader(),
		adapters.NewMemoryTokenStore(),
		adapters.NewMemorySessionStore(),
		adapters.NewCryptoRandomGenerator(),
		adapters.NewRealClock(),
	)

	// Register rule requiring prerequisite
	rule := domain.Rule{
		Tool:         "ToolB",
		Prerequisite: "ToolA",
		Expiry:       5 * time.Minute,
	}
	if err := service.RegisterRule(context.Background(), rule); err != nil {
		t.Fatalf("Failed to register rule: %v", err)
	}

	// No session created - should fail
	err := service.ValidateToolCall(context.Background(), "nonexistent-session", "ToolB", "")
	if err == nil {
		t.Error("Expected error for missing session")
	}
}

func TestRatchetServiceImpl_ConsumePrerequisiteToken_OneTimeUse(t *testing.T) {
	sessionStore := adapters.NewMemorySessionStore()
	tokenStore := adapters.NewMemoryTokenStore()
	service := NewRatchetService(
		adapters.NewYAMLConfigLoader(),
		tokenStore,
		sessionStore,
		adapters.NewCryptoRandomGenerator(),
		adapters.NewRealClock(),
	)

	// Register one-time-use rule: ToolB requires ToolA (one-time-use)
	rule := domain.Rule{
		Tool:         "ToolB",
		Prerequisite: "ToolA",
		Expiry:       5 * time.Minute,
		OneTimeUse:   true,
	}
	if err := service.RegisterRule(context.Background(), rule); err != nil {
		t.Fatalf("Failed to register rule: %v", err)
	}

	// Create session with ToolA called
	sessionID := domain.SessionID("test-session")
	session := domain.NewSession(sessionID)
	session.RecordToolCall("ToolA")
	if err := sessionStore.Create(context.Background(), session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Issue token for ToolA
	token, err := service.IssueToken(context.Background(), sessionID, "ToolA")
	if err != nil {
		t.Fatalf("Failed to issue token: %v", err)
	}

	// Validate should succeed (does not consume with new implementation)
	err = service.ValidateToolCall(context.Background(), sessionID, "ToolB", token)
	if err != nil {
		t.Fatalf("ValidateToolCall() error = %v", err)
	}

	// Token should still be valid before consumption
	validTokens, _ := tokenStore.GetValidTokens(context.Background(), sessionID, "ToolA")
	if len(validTokens) == 0 {
		t.Error("Token should exist before consumption")
	}

	// Consume the token (simulating post-success)
	err = service.ConsumePrerequisiteToken(context.Background(), sessionID, "ToolB")
	if err != nil {
		t.Errorf("ConsumePrerequisiteToken() error = %v", err)
	}

	// Token should now be consumed
	validTokens, _ = tokenStore.GetValidTokens(context.Background(), sessionID, "ToolA")
	if len(validTokens) != 0 {
		t.Error("Token should be consumed after ConsumePrerequisiteToken")
	}
}

// mockClock implements the Clock interface for testing
type mockClock struct {
	now time.Time
}

func (m *mockClock) Now() time.Time {
	return m.now
}
