package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"time"
	"uuid"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/primary"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
)

// RatchetServiceImpl implements the RatchetService interface
type RatchetServiceImpl struct {
	configLoader  secondary.ConfigLoader
	tokenStore    secondary.TokenStore
	randomGen     secondary.RandomGenerator
	clock         secondary.Clock
	rulesMu       sync.RWMutex
	rules         []domain.Rule
	byTool        map[domain.ToolName][]domain.Rule
	prerequisites map[domain.ToolName]struct{}
	workflow      workflowSessions
	eventSink     secondary.EventSink
	eventStore    secondary.EventStore
}

// NewRatchetService creates a new ratchet service
func NewRatchetService(
	configLoader secondary.ConfigLoader,
	tokenStore secondary.TokenStore,
	randomGen secondary.RandomGenerator,
	clock secondary.Clock,
) *RatchetServiceImpl {
	return NewRatchetServiceWithEventSink(configLoader, tokenStore, randomGen, clock, nil)
}

// NewRatchetServiceWithObservability creates a new ratchet service with an optional event store.
// Pass a nil eventStore to disable observability.
func NewRatchetServiceWithObservability(
	configLoader secondary.ConfigLoader,
	tokenStore secondary.TokenStore,
	randomGen secondary.RandomGenerator,
	clock secondary.Clock,
	eventStore secondary.EventStore,
) *RatchetServiceImpl {
	service := NewRatchetServiceWithEventSink(configLoader, tokenStore, randomGen, clock, eventStore)
	service.eventStore = eventStore
	return service
}

// NewRatchetServiceWithEventSink creates a service that emits live workflow events.
func NewRatchetServiceWithEventSink(
	configLoader secondary.ConfigLoader,
	tokenStore secondary.TokenStore,
	randomGen secondary.RandomGenerator,
	clock secondary.Clock,
	eventSink secondary.EventSink,
) *RatchetServiceImpl {
	return &RatchetServiceImpl{
		configLoader: configLoader,
		tokenStore:   tokenStore,
		randomGen:    randomGen,
		clock:        clock,
		eventSink:    eventSink,
	}
}

// RegisterRule adds a new tool dependency rule
func (s *RatchetServiceImpl) RegisterRule(ctx context.Context, rule domain.Rule) error {
	if err := rule.Validate(); err != nil {
		return fmt.Errorf("rule validation failed: %w", err)
	}
	s.rulesMu.Lock()
	defer s.rulesMu.Unlock()
	rules := append(slices.Clone(s.rules), rule)
	if err := s.configLoader.Validate(rules); err != nil {
		return fmt.Errorf("rule validation failed: %w", err)
	}
	s.indexRules(rules)
	return nil
}

func (s *RatchetServiceImpl) selectPrerequisite(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName) (*domain.Rule, domain.TokenValue, error) {
	rules := s.findRules(tool)
	if len(rules) == 0 || rules[0].Prerequisite == "" {
		return nil, "", nil
	}
	if sessionID == "" {
		if rules[0].ErrorMessage != "" {
			return nil, "", errors.New(rules[0].ErrorMessage)
		}
		return nil, "", fmt.Errorf("tool %s requires a session", tool)
	}
	var lastErr error
	now := s.clock.Now()
	for _, rule := range rules {
		token, ok, err := s.tokenStore.LatestToken(ctx, sessionID, rule.Prerequisite, now)
		if err != nil {
			lastErr = fmt.Errorf("failed to get valid token: %w", err)
			continue
		}
		if ok {
			return &rule, token, nil
		}
		lastErr = fmt.Errorf("prerequisite tool %s must be called first: %w", rule.Prerequisite, domain.ErrInvalidToken)
		if rule.ErrorMessage != "" {
			lastErr = errors.New(rule.ErrorMessage)
		}
	}
	return nil, "", lastErr
}

func (s *RatchetServiceImpl) issueToken(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName, newWorkflow bool) error {
	expiry := 5 * time.Minute
	for _, rule := range s.findRules(tool) {
		if rule.Expiry > 0 {
			expiry = rule.Expiry
			break
		}
	}

	tokenValue, err := s.randomGen.GenerateToken()
	if err != nil {
		return err
	}

	expires := s.clock.Now().Add(expiry)
	if err := s.tokenStore.Store(ctx, sessionID, tool, tokenValue, expires); err != nil {
		return fmt.Errorf("failed to store token: %w", err)
	}
	if newWorkflow {
		s.emitEvent(ctx, domain.EventTypeSessionCreated, sessionID, "", "", nil)
	}
	s.emitEvent(ctx, domain.EventTypeTokenCreated, sessionID, tool, tokenValue, map[string]any{"expiry": expires.Format(time.RFC3339)})

	return nil
}

// LoadConfiguration loads rules from YAML
func (s *RatchetServiceImpl) LoadConfiguration(ctx context.Context, config io.Reader) ([]domain.Rule, error) {
	rules, err := s.configLoader.Load(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Validate rules for circular dependencies
	if err := s.configLoader.Validate(rules); err != nil {
		return nil, fmt.Errorf("rule validation failed: %w", err)
	}

	s.rulesMu.Lock()
	s.indexRules(slices.Clone(rules))
	s.rulesMu.Unlock()
	return rules, nil
}

func (s *RatchetServiceImpl) consume(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName, rule *domain.Rule, token domain.TokenValue) error {
	if rule == nil || !rule.OneTimeUse {
		return nil
	}
	if err := s.tokenStore.RemoveToken(ctx, sessionID, rule.Prerequisite, token); err != nil {
		return fmt.Errorf("failed to reserve prerequisite token: %w", err)
	}
	s.emitEvent(ctx, domain.EventTypeTokenConsumed, sessionID, rule.Prerequisite, token, map[string]any{"consumed_by": string(tool)})
	return nil
}

// ExecuteToolCall selects and reserves the exact eligible prerequisite before
// running the tool. A failed call leaves its one-time prerequisite spent.
func (s *RatchetServiceImpl) ExecuteToolCall(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName, run func() (bool, error)) error {
	if err := sessionID.Validate(); err != nil {
		return err
	}
	release, retained, err := s.acquireWorkflowSession(ctx, sessionID)
	if err != nil {
		return err
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return err
	}

	s.emitEvent(ctx, domain.EventTypeToolCallAttempt, sessionID, tool, "", nil)
	rule, token, err := s.selectPrerequisite(ctx, sessionID, tool)
	if err == nil {
		err = s.consume(ctx, sessionID, tool, rule, token)
	}
	if err != nil {
		s.emitEvent(ctx, domain.EventTypeToolCallFailure, sessionID, tool, "", map[string]any{"error": err.Error()})
		return err
	}

	ok, err := run()
	if err != nil || !ok {
		s.emitEvent(ctx, domain.EventTypeToolCallFailure, sessionID, tool, "", nil)
		return err
	}
	if s.isPrerequisite(tool) {
		if err := s.issueToken(ctx, sessionID, tool, !retained); err != nil {
			return fmt.Errorf("tool completed but failed to record Ratchet state: %w", err)
		}
	}
	s.emitEvent(ctx, domain.EventTypeToolCallSuccess, sessionID, tool, "", nil)
	return nil
}

var (
	_ primary.WorkflowExecutor = (*RatchetServiceImpl)(nil)
	_ primary.RatchetService   = (*RatchetServiceImpl)(nil)
)

// GetObservabilityStats returns aggregate event metrics.
// Returns nil stats without error if observability is disabled.
func (s *RatchetServiceImpl) GetObservabilityStats(ctx context.Context) (*domain.EventStats, error) {
	if s.eventStore == nil {
		return nil, nil
	}
	return s.eventStore.GetStats(ctx)
}

// GetObservabilityEvents retrieves events for a session matching the given filter.
// Returns an empty slice without error if observability is disabled.
func (s *RatchetServiceImpl) GetObservabilityEvents(ctx context.Context, sessionID domain.SessionID, filter *secondary.EventFilter) ([]*domain.Event, error) {
	if s.eventStore == nil {
		return []*domain.Event{}, nil
	}
	return s.eventStore.GetEvents(ctx, sessionID, filter)
}

// emitEvent stores an observability event if an eventStore is configured.
// Errors are logged but never propagate — observability must not affect core behaviour.
func (s *RatchetServiceImpl) emitEvent(ctx context.Context, eventType domain.EventType, sessionID domain.SessionID, tool domain.ToolName, token domain.TokenValue, metadata map[string]any) {
	if s.eventSink == nil {
		return
	}

	event := &domain.Event{
		ID:        domain.EventID(uuid.NewV4().String()),
		Type:      eventType,
		SessionID: sessionID,
		ToolName:  tool,
		Token:     token,
		Timestamp: s.clock.Now(),
		Metadata:  metadata,
	}

	if err := s.eventSink.Store(ctx, event); err != nil {
		slog.WarnContext(ctx, "failed to store observability event", "event_type", eventType, "tool", tool, "error", err)
	}
}

// indexRules publishes a fresh policy index. The caller holds rulesMu.
func (s *RatchetServiceImpl) indexRules(rules []domain.Rule) {
	s.rules = rules
	s.byTool = make(map[domain.ToolName][]domain.Rule)
	s.prerequisites = make(map[domain.ToolName]struct{})
	for _, rule := range rules {
		s.byTool[rule.Tool] = append(s.byTool[rule.Tool], rule)
		if rule.Prerequisite != "" {
			s.prerequisites[rule.Prerequisite] = struct{}{}
		}
	}
}

func (s *RatchetServiceImpl) findRules(tool domain.ToolName) []domain.Rule {
	s.rulesMu.RLock()
	defer s.rulesMu.RUnlock()
	return s.byTool[tool]
}

func (s *RatchetServiceImpl) isPrerequisite(tool domain.ToolName) bool {
	s.rulesMu.RLock()
	defer s.rulesMu.RUnlock()
	_, ok := s.prerequisites[tool]
	return ok
}
