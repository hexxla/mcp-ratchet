package ratchet

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testClock struct{ nanos atomic.Int64 }

func (c *testClock) now() time.Time          { return time.Unix(0, c.nanos.Load()).UTC() }
func (c *testClock) advance(d time.Duration) { c.nanos.Add(int64(d)) }

func testEngine(t *testing.T, policy Policy, options Options) (*Engine, *testClock) {
	t.Helper()
	engine, err := New(policy, options)
	if err != nil {
		t.Fatal(err)
	}
	clock := new(testClock)
	clock.nanos.Store(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	engine.now = clock.now
	return engine, clock
}

func success() (bool, error) { return true, nil }

func TestPolicyValidation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		policy Policy
		want   string
	}{
		{"negative default", Policy{CompletionTTL: -time.Second}, "TTL"},
		{"empty tool", Policy{Rules: []Rule{{}}}, "tool"},
		{"negative producer TTL", Policy{Rules: []Rule{{Tool: "read", TTL: -time.Second}}}, "TTL"},
		{"consume unrestricted", Policy{Rules: []Rule{{Tool: "read", Consume: true}}}, "without a prerequisite"},
		{"empty alternative", Policy{Rules: []Rule{{Tool: "write", AfterAny: []string{""}}}}, "empty prerequisite"},
		{"duplicate grouped edge", Policy{Rules: []Rule{{Tool: "write", AfterAny: []string{"read", "read"}}}}, "duplicate"},
		{"duplicate across groups", Policy{Rules: []Rule{{Tool: "write", AfterAny: []string{"read"}}, {Tool: "write", AfterAny: []string{"read"}, Consume: true}}}, "duplicate"},
		{"duplicate free rule", Policy{Rules: []Rule{{Tool: "read"}, {Tool: "read"}}}, "duplicate"},
		{"mixed guarded", Policy{Rules: []Rule{{Tool: "write"}, {Tool: "write", AfterAny: []string{"read"}}}}, "mixes"},
		{"conflicting TTL", Policy{Rules: []Rule{{Tool: "write", AfterAny: []string{"a"}, TTL: time.Second}, {Tool: "write", AfterAny: []string{"b"}, TTL: time.Minute}}}, "conflicting"},
		{"self cycle", Policy{Rules: []Rule{{Tool: "read", AfterAny: []string{"read"}}}}, "circular"},
		{"cycle", Policy{Rules: []Rule{{Tool: "a", AfterAny: []string{"b"}}, {Tool: "b", AfterAny: []string{"c"}}, {Tool: "c", AfterAny: []string{"a"}}}}, "circular"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.policy, Options{})
			if !errors.Is(err, ErrInvalidPolicy) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("New error = %v, want invalid policy containing %q", err, tt.want)
			}
		})
	}
	if _, err := New(Policy{}, Options{}); err != nil {
		t.Fatalf("empty policy: %v", err)
	}
}

func TestEngineClonesPolicyAndPreservesAlternativeOrder(t *testing.T) {
	policy := Policy{Rules: []Rule{
		{Tool: "write", AfterAny: []string{"missing", "read"}, Consume: true},
		{Tool: "write", AfterAny: []string{"reusable"}},
	}}
	engine, _ := testEngine(t, policy, Options{})
	policy.Rules[0].AfterAny[1] = "other"
	policy.Rules[0].Tool = "other"
	policy.Rules[1].Consume = true
	for _, tool := range []string{"read", "reusable"} {
		if err := engine.ExecuteToolCall(t.Context(), "owner", tool, success); err != nil {
			t.Fatal(err)
		}
	}
	if err := engine.ExecuteToolCall(t.Context(), "other", "write", success); !errors.Is(err, ErrPrerequisiteRequired) {
		t.Fatalf("completion leaked: %v", err)
	}
	if err := engine.ExecuteToolCall(t.Context(), "owner", "write", success); err != nil {
		t.Fatal(err)
	}
	if values := engine.sessions["owner"].completions["read"]; len(values) != 0 {
		t.Fatal("did not consume the first eligible grouped alternative")
	}
	for range 2 {
		if err := engine.ExecuteToolCall(t.Context(), "owner", "write", success); err != nil {
			t.Fatalf("reusable alternative was consumed: %v", err)
		}
	}
}

func TestEngineFailuresSpendReservationAndIssueNoCompletion(t *testing.T) {
	cause := errors.New("handler failed")
	for _, tt := range []struct {
		name string
		run  func() (bool, error)
	}{
		{"false", func() (bool, error) { return false, nil }},
		{"error", func() (bool, error) { return false, cause }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			engine, _ := testEngine(t, Policy{Rules: []Rule{{Tool: "write", AfterAny: []string{"read"}, Consume: true}, {Tool: "final", AfterAny: []string{"write"}}}}, Options{})
			if err := engine.ExecuteToolCall(t.Context(), "s", "read", success); err != nil {
				t.Fatal(err)
			}
			err := engine.ExecuteToolCall(t.Context(), "s", "write", tt.run)
			if tt.name == "error" && !errors.Is(err, cause) {
				t.Fatalf("handler error lost: %v", err)
			}
			if tt.name == "false" && err != nil {
				t.Fatalf("false handler result changed: %v", err)
			}
			for _, tool := range []string{"write", "final"} {
				if err := engine.ExecuteToolCall(t.Context(), "s", tool, success); !errors.Is(err, ErrPrerequisiteRequired) {
					t.Fatalf("failed handler enabled %s: %v", tool, err)
				}
			}
		})
	}
}

func TestEnginePanicReleasesGateAndSpendsReservation(t *testing.T) {
	engine, _ := testEngine(t, Policy{Rules: []Rule{{Tool: "write", AfterAny: []string{"read"}, Consume: true}}}, Options{})
	if err := engine.ExecuteToolCall(t.Context(), "s", "read", success); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("handler panic disappeared")
			}
		}()
		_ = engine.ExecuteToolCall(t.Context(), "s", "write", func() (bool, error) { panic("failed") })
	}()
	if err := engine.ExecuteToolCall(t.Context(), "s", "write", success); !errors.Is(err, ErrPrerequisiteRequired) {
		t.Fatalf("panic stranded the gate or preserved reservation: %v", err)
	}
}

func TestEngineExpiryAndProducerTTL(t *testing.T) {
	for _, tt := range []struct {
		name                          string
		defaultTTL, producerTTL, want time.Duration
	}{
		{"default", 0, 0, 5 * time.Minute}, {"policy default", 3 * time.Minute, 0, 3 * time.Minute}, {"producer override", 3 * time.Minute, time.Second, time.Second},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var recorded Event
			engine, clock := testEngine(t, Policy{CompletionTTL: tt.defaultTTL, Rules: []Rule{{Tool: "read", TTL: tt.producerTTL}, {Tool: "write", AfterAny: []string{"read"}, TTL: time.Hour}}}, Options{Observer: func(event Event) {
				if event.Type == EventTypeCompletionRecorded {
					recorded = event
				}
			}})
			if err := engine.ExecuteToolCall(t.Context(), "s", "read", success); err != nil {
				t.Fatal(err)
			}
			if recorded.ExpiresAt.Sub(clock.now()) != tt.want {
				t.Fatalf("producer TTL = %v, want %v", recorded.ExpiresAt.Sub(clock.now()), tt.want)
			}
			clock.advance(tt.want - time.Nanosecond)
			if err := engine.ExecuteToolCall(t.Context(), "s", "write", success); err != nil {
				t.Fatalf("completion expired early: %v", err)
			}
			clock.advance(time.Nanosecond)
			if err := engine.ExecuteToolCall(t.Context(), "s", "write", success); !errors.Is(err, ErrPrerequisiteRequired) {
				t.Fatalf("completion remained valid at expiry: %v", err)
			}
		})
	}
}

func TestEngineCompletionBoundAndIdleExpiry(t *testing.T) {
	engine, clock := testEngine(t, Policy{Rules: []Rule{{Tool: "read", TTL: time.Hour}, {Tool: "write", AfterAny: []string{"read"}, Consume: true}}}, Options{})
	for range 2 * maxCompletionsPerTool {
		if err := engine.ExecuteToolCall(t.Context(), "s", "read", success); err != nil {
			t.Fatal(err)
		}
	}
	for range maxCompletionsPerTool {
		if err := engine.ExecuteToolCall(t.Context(), "s", "write", success); err != nil {
			t.Fatal(err)
		}
	}
	if err := engine.ExecuteToolCall(t.Context(), "s", "write", success); !errors.Is(err, ErrPrerequisiteRequired) {
		t.Fatalf("completion bound not enforced: %v", err)
	}
	if len(engine.sessions) != 0 {
		t.Fatal("spent completions retained a session slot")
	}
	if err := engine.ExecuteToolCall(t.Context(), "s", "read", success); err != nil {
		t.Fatal(err)
	}
	clock.advance(idleTimeout)
	if err := engine.ExecuteToolCall(t.Context(), "s", "write", success); !errors.Is(err, ErrPrerequisiteRequired) {
		t.Fatalf("idle completion authorized call: %v", err)
	}
}

func TestEngineCapacityReclaimsExpiredCompletions(t *testing.T) {
	engine, clock := testEngine(t, Policy{CompletionTTL: time.Second, Rules: []Rule{{Tool: "write", AfterAny: []string{"read"}}}}, Options{})
	for i := range maxSessions {
		if err := engine.ExecuteToolCall(t.Context(), fmt.Sprint(i), "read", success); err != nil {
			t.Fatal(err)
		}
	}
	if err := engine.ExecuteToolCall(t.Context(), "extra", "read", success); !errors.Is(err, ErrSessionLimit) {
		t.Fatalf("live session bound not enforced: %v", err)
	}
	// Capacity pressure forces reclamation even before the periodic sweep.
	clock.advance(time.Second)
	if err := engine.ExecuteToolCall(t.Context(), "extra", "read", success); err != nil {
		t.Fatalf("expired state blocked admission: %v", err)
	}
	if len(engine.sessions) != 1 {
		t.Fatalf("expired state retained %d sessions", len(engine.sessions))
	}
}

func TestEngineDeniedAndUntrackedCallsReleaseCapacity(t *testing.T) {
	engine, _ := testEngine(t, Policy{Rules: []Rule{{Tool: "write", AfterAny: []string{"read"}, Message: "discover first"}}}, Options{})
	for i := range maxSessions + 1 {
		err := engine.ExecuteToolCall(t.Context(), fmt.Sprint(i), "write", func() (bool, error) { t.Fatal("denied handler executed"); return false, nil })
		if !errors.Is(err, ErrPrerequisiteRequired) || !strings.Contains(err.Error(), "discover first") {
			t.Fatalf("denial %d: %v", i, err)
		}
		if err := engine.ExecuteToolCall(t.Context(), fmt.Sprint(i), "untracked", success); err != nil {
			t.Fatal(err)
		}
	}
	if len(engine.sessions) != 0 {
		t.Fatal("calls without completions exhausted session capacity")
	}
}

func TestEngineMaintenanceDoesNotRefreshIdleTimeout(t *testing.T) {
	engine, clock := testEngine(t, Policy{CompletionTTL: time.Hour, Rules: []Rule{{Tool: "write", AfterAny: []string{"read"}}}}, Options{})
	if err := engine.ExecuteToolCall(t.Context(), "idle", "read", success); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		clock.advance(idleTimeout / 3)
		if err := engine.ExecuteToolCall(t.Context(), "other", "free", success); err != nil {
			t.Fatal(err)
		}
	}
	clock.advance(idleTimeout / 3)
	if err := engine.ExecuteToolCall(t.Context(), "idle", "write", success); !errors.Is(err, ErrPrerequisiteRequired) {
		t.Fatalf("maintenance extended idle lifetime: %v", err)
	}
}

func waitUsers(t *testing.T, engine *Engine, id string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		engine.mu.Lock()
		entry := engine.sessions[id]
		got := 0
		if entry != nil {
			got = entry.users
		}
		engine.mu.Unlock()
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("session %q never had %d users", id, want)
}

func TestEngineQueuedCancellationAndUnrelatedProgress(t *testing.T) {
	engine, _ := testEngine(t, Policy{}, Options{})
	entered, finish := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(finish) })
	defer unblock()
	first := make(chan error, 1)
	go func() {
		first <- engine.ExecuteToolCall(t.Context(), "busy", "free", func() (bool, error) { close(entered); <-finish; return true, nil })
	}()
	<-entered
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	waiting := make(chan error, 1)
	go func() {
		waiting <- engine.ExecuteToolCall(ctx, "busy", "free", func() (bool, error) { t.Error("canceled waiter ran"); return true, nil })
	}()
	waitUsers(t, engine, "busy", 2)
	cancel()
	select {
	case err := <-waiting:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued cancellation blocked")
	}
	if err := engine.ExecuteToolCall(t.Context(), "other", "free", success); err != nil {
		t.Fatalf("unrelated session blocked: %v", err)
	}
	unblock()
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if len(engine.sessions) != 0 {
		t.Fatal("finished session retained its slot")
	}
}

func TestEngineConcurrentFailureSpendsOnlyReservationAndPinsSession(t *testing.T) {
	engine, clock := testEngine(t, Policy{Rules: []Rule{{Tool: "write", AfterAny: []string{"missing", "read"}, Consume: true}, {Tool: "final", AfterAny: []string{"write"}}}}, Options{})
	if err := engine.ExecuteToolCall(t.Context(), "s", "read", success); err != nil {
		t.Fatal(err)
	}
	entered, finish := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(finish) })
	defer unblock()
	cause := errors.New("handler failed")
	first := make(chan error, 1)
	go func() {
		first <- engine.ExecuteToolCall(t.Context(), "s", "write", func() (bool, error) { close(entered); <-finish; return false, cause })
	}()
	<-entered
	const count = 16
	results := make(chan error, count)
	var handlers atomic.Int32
	var callers sync.WaitGroup
	for range count {
		callers.Go(func() {
			results <- engine.ExecuteToolCall(t.Context(), "s", "write", func() (bool, error) { handlers.Add(1); return true, nil })
		})
	}
	waitUsers(t, engine, "s", count+1)
	clock.advance(idleTimeout)
	if err := engine.ExecuteToolCall(t.Context(), "other", "free", success); err != nil {
		t.Fatal(err)
	}
	unblock()
	if err := <-first; !errors.Is(err, cause) {
		t.Fatal(err)
	}
	callers.Wait()
	for range count {
		if err := <-results; !errors.Is(err, ErrPrerequisiteRequired) {
			t.Fatalf("queued call bypassed reservation: %v", err)
		}
	}
	if handlers.Load() != 0 {
		t.Fatal("a second handler spent the same completion")
	}
	if err := engine.ExecuteToolCall(t.Context(), "s", "final", success); !errors.Is(err, ErrPrerequisiteRequired) {
		t.Fatalf("failed handler issued completion: %v", err)
	}
}

func TestEngineObserverValuesAndLifecycle(t *testing.T) {
	var events []Event
	engine, _ := testEngine(t, Policy{Rules: []Rule{{Tool: "write", AfterAny: []string{"read"}, Consume: true}}}, Options{Observer: func(event Event) { events = append(events, event); event.SessionID = "changed" }})
	for _, tool := range []string{"read", "write"} {
		if err := engine.ExecuteToolCall(t.Context(), "s", tool, success); err != nil {
			t.Fatal(err)
		}
	}
	types := make([]EventType, 0, len(events))
	ids := make(map[string]bool)
	for _, event := range events {
		types = append(types, event.Type)
		if event.ID == "" || ids[event.ID] || event.Timestamp.IsZero() || event.SessionID != "s" {
			t.Fatalf("invalid event: %+v", event)
		}
		ids[event.ID] = true
	}
	want := []EventType{EventTypeToolCallAttempt, EventTypeSessionCreated, EventTypeCompletionRecorded, EventTypeToolCallSuccess, EventTypeToolCallAttempt, EventTypeCompletionConsumed, EventTypeToolCallSuccess}
	if !slices.Equal(types, want) {
		t.Fatalf("lifecycle = %v, want %v", types, want)
	}
	if events[5].ConsumedBy != "write" || events[5].ToolName != "read" || !events[5].ExpiresAt.Equal(events[2].ExpiresAt) {
		t.Fatalf("consumption event = %+v", events[5])
	}
}

func TestEngineBlockedObserverDoesNotHoldGlobalLock(t *testing.T) {
	entered, finish := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(finish) })
	defer unblock()
	engine, _ := testEngine(t, Policy{}, Options{Observer: func(event Event) {
		if event.SessionID == "busy" && event.Type == EventTypeToolCallAttempt {
			close(entered)
			<-finish
		}
	}})
	done := make(chan error, 1)
	go func() { done <- engine.ExecuteToolCall(t.Context(), "busy", "free", success) }()
	<-entered
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := engine.ExecuteToolCall(ctx, "other", "free", success); err != nil {
		t.Fatalf("observer blocked unrelated session: %v", err)
	}
	unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestEngineInvalidAndCanceledCalls(t *testing.T) {
	engine, _ := testEngine(t, Policy{}, Options{})
	for _, tt := range []struct {
		id, tool string
		run      func() (bool, error)
	}{{"", "tool", success}, {"s", "", success}, {"s", "tool", nil}} {
		if err := engine.ExecuteToolCall(t.Context(), tt.id, tt.tool, tt.run); !errors.Is(err, ErrInvalidCall) {
			t.Fatalf("invalid call accepted: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := engine.ExecuteToolCall(ctx, "s", "tool", success); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled call accepted: %v", err)
	}
	if len(engine.sessions) != 0 {
		t.Fatal("invalid calls retained state")
	}
}

func TestEngineObserverDelayRechecksExpiry(t *testing.T) {
	var clock *testClock
	engine, clock := testEngine(t, Policy{CompletionTTL: time.Second, Rules: []Rule{{Tool: "write", AfterAny: []string{"read"}, Consume: true}}}, Options{Observer: func(event Event) {
		if event.Type == EventTypeToolCallAttempt && event.ToolName == "write" {
			clock.advance(time.Second)
		}
	}})
	if err := engine.ExecuteToolCall(t.Context(), "s", "read", success); err != nil {
		t.Fatal(err)
	}
	err := engine.ExecuteToolCall(t.Context(), "s", "write", func() (bool, error) {
		t.Error("expired prerequisite authorized a call after its observer returned")
		return true, nil
	})
	if !errors.Is(err, ErrPrerequisiteRequired) {
		t.Fatalf("delayed attempt error = %v", err)
	}
}
