package services

import (
	"context"
	"errors"
	"fmt"
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
		adapters.NewCryptoRandomGenerator(),
		adapters.NewRealClock(),
	)

	rule := domain.Rule{
		Tool:         "Create Cell",
		Prerequisite: "List Tags",
		Expiry:       5 * time.Minute,
	}

	err := service.RegisterRule(t.Context(), rule)
	if err != nil {
		t.Errorf("RegisterRule() error = %v", err)
	}

	// Invalid rule should fail
	invalidRule := domain.Rule{
		Tool:         "Create Cell",
		Prerequisite: "Create Cell", // Self-dependency
	}
	err = service.RegisterRule(t.Context(), invalidRule)
	if err == nil {
		t.Error("Expected error for self-dependency rule")
	}
}

func TestRatchetServiceImpl_LoadConfiguration(t *testing.T) {
	service := NewRatchetService(
		adapters.NewYAMLConfigLoader(),
		adapters.NewMemoryTokenStore(),
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
	rules, err := service.LoadConfiguration(t.Context(), strings.NewReader(yamlConfig))
	if err != nil {
		t.Errorf("LoadConfiguration() error = %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(rules))
	}
}

func TestRatchetServiceImpl_ObservabilityEvents(t *testing.T) {
	ctx := t.Context()
	sessionID := domain.SessionID("obs-session")

	eventStore := adapters.NewMemoryEventStore(0)
	tokenStore := adapters.NewMemoryTokenStore()

	service := NewRatchetServiceWithObservability(
		adapters.NewYAMLConfigLoader(),
		tokenStore,
		adapters.NewCryptoRandomGenerator(),
		adapters.NewRealClock(),
		eventStore,
	)

	if err := service.RegisterRule(ctx, domain.Rule{Tool: "tool_b", Prerequisite: "tool_a", ErrorMessage: "discover first"}); err != nil {
		t.Fatal(err)
	}
	success := func() (bool, error) { return true, nil }
	if err := service.ExecuteToolCall(ctx, sessionID, "tool_b", func() (bool, error) {
		t.Fatal("denied tool ran")
		return false, nil
	}); err == nil || err.Error() != "discover first" {
		t.Fatalf("expected configured denial, got %v", err)
	}
	for _, tool := range []domain.ToolName{"tool_a", "tool_b"} {
		if err := service.ExecuteToolCall(ctx, sessionID, tool, success); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := service.GetObservabilityStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.EventsByType[domain.EventTypeToolCallAttempt] != 3 ||
		stats.EventsByType[domain.EventTypeToolCallFailure] != 1 ||
		stats.EventsByType[domain.EventTypeToolCallSuccess] != 2 ||
		stats.EventsByType[domain.EventTypeSessionCreated] != 1 || stats.TokensIssued != 1 {
		t.Fatalf("unexpected workflow stats: %+v", stats)
	}
}

func TestRatchetServiceImpl_ObservabilityDisabled(t *testing.T) {
	ctx := t.Context()
	service := NewRatchetService(
		adapters.NewYAMLConfigLoader(),
		adapters.NewMemoryTokenStore(),
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

// mockClock implements the Clock interface for testing
type mockClock struct {
	now time.Time
}

func (m *mockClock) Now() time.Time {
	return m.now
}

func TestWorkflowBoundsAndExpiresPrerequisites(t *testing.T) {
	clock := &mockClock{now: time.Now()}
	tokens := adapters.NewMemoryTokenStore()
	service := NewRatchetService(adapters.NewYAMLConfigLoader(), tokens, adapters.NewCryptoRandomGenerator(), clock)
	for _, rule := range []domain.Rule{
		{Tool: "read", Expiry: time.Hour},
		{Tool: "write", Prerequisite: "read", OneTimeUse: true},
	} {
		if err := service.RegisterRule(t.Context(), rule); err != nil {
			t.Fatal(err)
		}
	}
	const id = "test-session"
	run := service.ExecuteToolCall
	success := func() (bool, error) { return true, nil }
	for range 2 * adapters.MaxTokensPerTool {
		if err := run(t.Context(), id, "read", success); err != nil {
			t.Fatal(err)
		}
	}
	for range adapters.MaxTokensPerTool {
		if err := run(t.Context(), id, "write", success); err != nil {
			t.Fatal(err)
		}
	}
	if err := run(t.Context(), id, "write", success); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expected denial after consuming the bounded set, got %v", err)
	}
	if err := run(t.Context(), id, "read", success); err != nil {
		t.Fatal(err)
	}
	clock.now = clock.now.Add(workflowIdleTimeout)
	if err := run(t.Context(), id, "write", func() (bool, error) {
		t.Error("idle session authorized a write")
		return true, nil
	}); err == nil {
		t.Fatal("expected expired session to require a new read")
	}
	if live, err := tokens.HasLiveTokens(t.Context(), id, clock.now); err != nil || live {
		t.Fatalf("live idle tokens = %v, error = %v", live, err)
	}
}

func TestWorkflowWaitCancellationAndSessionIndependence(t *testing.T) {
	service := NewRatchetService(adapters.NewYAMLConfigLoader(), adapters.NewMemoryTokenStore(), adapters.NewCryptoRandomGenerator(), adapters.NewRealClock())
	run := service.ExecuteToolCall
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, "first", "read", func() (bool, error) {
			close(started)
			<-release
			return true, nil
		})
	}()
	defer func() {
		close(release)
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	<-started
	wait, stopWaiting := context.WithCancel(ctx)
	stopWaiting()
	err := run(wait, "first", "read", func() (bool, error) {
		t.Error("canceled waiter executed")
		return true, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting call error = %v", err)
	}
	if err := run(ctx, "second", "read", func() (bool, error) { return true, nil }); err != nil {
		t.Fatalf("unrelated session could not proceed: %v", err)
	}
}

func TestWorkflowUsesIssuingToolExpiry(t *testing.T) {
	clock := &mockClock{now: time.Now()}
	service := NewRatchetService(adapters.NewYAMLConfigLoader(), adapters.NewMemoryTokenStore(), adapters.NewCryptoRandomGenerator(), clock)
	for _, rule := range []domain.Rule{
		{Tool: "read", Expiry: time.Second},
		{Tool: "write", Prerequisite: "read"},
	} {
		if err := service.RegisterRule(t.Context(), rule); err != nil {
			t.Fatal(err)
		}
	}
	run := service.ExecuteToolCall
	if err := run(t.Context(), "session", "read", func() (bool, error) { return true, nil }); err != nil {
		t.Fatal(err)
	}
	clock.now = clock.now.Add(time.Second)
	if err := run(t.Context(), "session", "write", func() (bool, error) {
		t.Error("expired prerequisite authorized a write")
		return true, nil
	}); err == nil {
		t.Fatal("expected token expiry at the configured boundary")
	}
}

func TestDeniedWorkflowsDoNotConsumeSessionCapacity(t *testing.T) {
	service := NewRatchetService(adapters.NewYAMLConfigLoader(), adapters.NewMemoryTokenStore(), adapters.NewCryptoRandomGenerator(), adapters.NewRealClock())
	if err := service.RegisterRule(t.Context(), domain.Rule{Tool: "write", Prerequisite: "read"}); err != nil {
		t.Fatal(err)
	}
	for i := range maxWorkflowSessions + 1 {
		id := domain.SessionID(fmt.Sprintf("denied-%d", i))
		err := service.ExecuteToolCall(t.Context(), id, "write", func() (bool, error) {
			t.Fatal("denied handler executed")
			return false, nil
		})
		if !errors.Is(err, domain.ErrInvalidToken) {
			t.Fatalf("denial %d: %v", i, err)
		}
	}
	if err := service.ExecuteToolCall(t.Context(), "new-client", "read", func() (bool, error) { return true, nil }); err != nil {
		t.Fatalf("new session failed to start discovery: %v", err)
	}
}
