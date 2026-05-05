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

func TestRatchetServiceImpl_ObservabilityEvents(t *testing.T) {
	ctx := context.Background()
	sessionID := domain.SessionID("obs-session")

	eventStore := adapters.NewMemoryEventStore(0)
	sessionStore := adapters.NewMemorySessionStore()
	tokenStore := adapters.NewMemoryTokenStore()

	service := NewRatchetServiceWithObservability(
		adapters.NewYAMLConfigLoader(),
		tokenStore,
		sessionStore,
		adapters.NewCryptoRandomGenerator(),
		adapters.NewRealClock(),
		eventStore,
	)

	// Register a rule
	rule := domain.Rule{Tool: "tool_b", Prerequisite: "tool_a", Expiry: time.Minute}
	_ = service.RegisterRule(ctx, rule)

	// Create session
	session := domain.NewSession(sessionID)
	_ = sessionStore.Create(ctx, session)

	// Attempt tool_b without prerequisite — should emit failure event
	_ = service.ValidateToolCall(ctx, sessionID, "tool_b", "")

	stats, err := service.GetObservabilityStats(ctx)
	if err != nil {
		t.Fatalf("GetObservabilityStats() error = %v", err)
	}
	if stats.EventsByType[domain.EventTypeToolCallAttempt] != 1 {
		t.Errorf("want 1 tool_call_attempt event, got %d", stats.EventsByType[domain.EventTypeToolCallAttempt])
	}
	if stats.EventsByType[domain.EventTypeToolCallFailure] != 1 {
		t.Errorf("want 1 tool_call_failure event, got %d", stats.EventsByType[domain.EventTypeToolCallFailure])
	}

	// Issue a token for tool_a and record the call, then validate tool_b
	session, _ = sessionStore.Get(ctx, sessionID)
	session.RecordToolCall("tool_a")
	_ = sessionStore.Update(ctx, session)
	_, _ = service.IssueToken(ctx, sessionID, "tool_a")

	_ = service.ValidateToolCall(ctx, sessionID, "tool_b", "")

	stats, _ = service.GetObservabilityStats(ctx)
	if stats.EventsByType[domain.EventTypeToolCallSuccess] != 1 {
		t.Errorf("want 1 tool_call_success event, got %d", stats.EventsByType[domain.EventTypeToolCallSuccess])
	}
	if stats.TokensIssued != 1 {
		t.Errorf("want 1 token issued, got %d", stats.TokensIssued)
	}
}

func TestRatchetServiceImpl_ObservabilityDisabled(t *testing.T) {
	ctx := context.Background()
	service := NewRatchetService(
		adapters.NewYAMLConfigLoader(),
		adapters.NewMemoryTokenStore(),
		adapters.NewMemorySessionStore(),
		adapters.NewCryptoRandomGenerator(),
		adapters.NewRealClock(),
	)

	// Stats should be nil without error when observability disabled
	stats, err := service.GetObservabilityStats(ctx)
	if err != nil {
		t.Fatalf("GetObservabilityStats() error = %v, want nil", err)
	}
	if stats != nil {
		t.Errorf("GetObservabilityStats() = %v, want nil when disabled", stats)
	}

	// Events should be empty without error when observability disabled
	events, err := service.GetObservabilityEvents(ctx, "any-session", nil)
	if err != nil {
		t.Fatalf("GetObservabilityEvents() error = %v, want nil", err)
	}
	if len(events) != 0 {
		t.Errorf("GetObservabilityEvents() returned %d events, want 0 when disabled", len(events))
	}
}

func TestRatchetServiceImpl_ValidateToolCall_MultipleRules_ORLogic(t *testing.T) {
	sessionStore := adapters.NewMemorySessionStore()
	tokenStore := adapters.NewMemoryTokenStore()
	service := NewRatchetService(
		adapters.NewYAMLConfigLoader(),
		tokenStore,
		sessionStore,
		adapters.NewCryptoRandomGenerator(),
		adapters.NewRealClock(),
	)

	ctx := context.Background()

	// ToolC can be called after ToolA OR ToolB (two rules, OR logic)
	rules := []domain.Rule{
		{Tool: "ToolC", Prerequisite: "ToolA", Expiry: 5 * time.Minute, ErrorMessage: "need ToolA or ToolB"},
		{Tool: "ToolC", Prerequisite: "ToolB", Expiry: 5 * time.Minute, ErrorMessage: "need ToolA or ToolB"},
	}
	for _, r := range rules {
		if err := service.RegisterRule(ctx, r); err != nil {
			t.Fatalf("RegisterRule: %v", err)
		}
	}

	sessionID := domain.SessionID("test-or-session")

	t.Run("fails when neither prerequisite called", func(t *testing.T) {
		session := domain.NewSession(sessionID)
		if err := sessionStore.Create(ctx, session); err != nil {
			t.Fatalf("Create session: %v", err)
		}
		err := service.ValidateToolCall(ctx, sessionID, "ToolC", "")
		if err == nil {
			t.Error("expected error when no prerequisite called")
		}
	})

	t.Run("succeeds when only ToolB called (second rule satisfied)", func(t *testing.T) {
		// Reset session with ToolB called
		session := domain.NewSession(sessionID)
		session.RecordToolCall("ToolB")
		if err := sessionStore.Update(ctx, session); err != nil {
			t.Fatalf("Update session: %v", err)
		}
		_, err := service.IssueToken(ctx, sessionID, "ToolB")
		if err != nil {
			t.Fatalf("IssueToken: %v", err)
		}
		if err := service.ValidateToolCall(ctx, sessionID, "ToolC", ""); err != nil {
			t.Errorf("expected success when ToolB called, got: %v", err)
		}
	})

	t.Run("succeeds when only ToolA called (first rule satisfied)", func(t *testing.T) {
		sessionID2 := domain.SessionID("test-or-session-2")
		session := domain.NewSession(sessionID2)
		session.RecordToolCall("ToolA")
		if err := sessionStore.Create(ctx, session); err != nil {
			t.Fatalf("Create session: %v", err)
		}
		_, err := service.IssueToken(ctx, sessionID2, "ToolA")
		if err != nil {
			t.Fatalf("IssueToken: %v", err)
		}
		if err := service.ValidateToolCall(ctx, sessionID2, "ToolC", ""); err != nil {
			t.Errorf("expected success when ToolA called, got: %v", err)
		}
	})
}

// mockClock implements the Clock interface for testing
type mockClock struct {
	now time.Time
}

func (m *mockClock) Now() time.Time {
	return m.now
}
