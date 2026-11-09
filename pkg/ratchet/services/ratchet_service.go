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
	sessionStore  secondary.SessionStore
	randomGen     secondary.RandomGenerator
	clock         secondary.Clock
	rulesMu       sync.RWMutex
	rules         []domain.Rule
	byTool        map[domain.ToolName][]domain.Rule
	prerequisites map[domain.ToolName]struct{}
	workflow      workflowSessions
	eventStore    secondary.EventStore // Optional, nil disables observability
}

// NewRatchetService creates a new ratchet service
func NewRatchetService(
	configLoader secondary.ConfigLoader,
	tokenStore secondary.TokenStore,
	sessionStore secondary.SessionStore,
	randomGen secondary.RandomGenerator,
	clock secondary.Clock,
) *RatchetServiceImpl {
	return NewRatchetServiceWithObservability(configLoader, tokenStore, sessionStore, randomGen, clock, nil)
}

// NewRatchetServiceWithObservability creates a new ratchet service with an optional event store.
// Pass a nil eventStore to disable observability.
func NewRatchetServiceWithObservability(
	configLoader secondary.ConfigLoader,
	tokenStore secondary.TokenStore,
	sessionStore secondary.SessionStore,
	randomGen secondary.RandomGenerator,
	clock secondary.Clock,
	eventStore secondary.EventStore,
) *RatchetServiceImpl {
	return &RatchetServiceImpl{
		configLoader: configLoader,
		tokenStore:   tokenStore,
		sessionStore: sessionStore,
		randomGen:    randomGen,
		clock:        clock,
		eventStore:   eventStore,
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

// ValidateToolCall checks if a tool can be called.
// When multiple rules exist for the same tool, ANY satisfied rule allows the call (OR logic).
func (s *RatchetServiceImpl) ValidateToolCall(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName, token domain.TokenValue) error {
	s.emitEvent(ctx, domain.EventTypeToolCallAttempt, sessionID, tool, token, nil)
	_, _, err := s.selectPrerequisite(ctx, sessionID, tool)
	if err != nil {
		s.emitEvent(ctx, domain.EventTypeToolCallFailure, sessionID, tool, token, map[string]any{"error": err.Error()})
		return err
	}
	s.emitEvent(ctx, domain.EventTypeToolCallSuccess, sessionID, tool, token, nil)
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
	session, err := s.sessionStore.Get(ctx, sessionID)
	if err != nil {
		return nil, "", fmt.Errorf("session not found: %w", err)
	}

	var lastErr error
	for _, rule := range rules {
		if !session.HasToolBeenCalled(rule.Prerequisite) {
			lastErr = fmt.Errorf("prerequisite tool %s must be called first", rule.Prerequisite)
		} else {
			token, ok, err := s.tokenStore.LatestToken(ctx, sessionID, rule.Prerequisite, s.clock.Now())
			if err != nil {
				lastErr = fmt.Errorf("failed to get valid tokens: %w", err)
				continue
			}
			if ok {
				return &rule, token, nil
			}
			lastErr = domain.ErrInvalidToken
		}
		if rule.ErrorMessage != "" {
			lastErr = errors.New(rule.ErrorMessage)
		}
	}
	return nil, "", lastErr
}

// IssueToken creates a new ratchet token after successful execution
func (s *RatchetServiceImpl) IssueToken(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName) (domain.TokenValue, error) {
	return s.issueToken(ctx, sessionID, tool, false)
}

func (s *RatchetServiceImpl) issueToken(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName, completed bool) (domain.TokenValue, error) {
	expiry := 5 * time.Minute
	for _, rule := range s.findRules(tool) {
		if rule.Expiry > 0 {
			expiry = rule.Expiry
			break
		}
	}

	tokenValue, err := s.randomGen.GenerateToken()
	if err != nil {
		return "", err
	}

	session, err := s.sessionStore.Get(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}
	expires := s.clock.Now().Add(expiry)
	if err := s.tokenStore.Store(ctx, sessionID, tool, tokenValue, expires); err != nil {
		return "", fmt.Errorf("failed to store token: %w", err)
	}
	if completed && !session.HasToolBeenCalled(tool) {
		session.RecordToolCall(tool)
		if err := s.sessionStore.Update(ctx, session); err != nil {
			cleanupErr := s.tokenStore.RemoveToken(context.WithoutCancel(ctx), sessionID, tool, tokenValue)
			return "", errors.Join(fmt.Errorf("failed to update session: %w", err), cleanupErr)
		}
	}
	s.emitEvent(ctx, domain.EventTypeTokenCreated, sessionID, tool, tokenValue, map[string]any{"expiry": expires.Format(time.RFC3339)})

	return tokenValue, nil
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

// ConsumePrerequisiteToken consumes the prerequisite token for one-time-use rules.
// Prefer ExecuteToolCall to select and reserve a prerequisite in one operation.
func (s *RatchetServiceImpl) ConsumePrerequisiteToken(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName) error {
	rule, token, err := s.selectPrerequisite(ctx, sessionID, tool)
	if err != nil {
		return err
	}
	return s.consume(ctx, sessionID, tool, rule, token)
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
	release, err := s.acquireWorkflowSession(ctx, sessionID)
	if err != nil {
		return err
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := s.CreateSession(ctx, sessionID); err != nil {
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
		if _, err := s.issueToken(ctx, sessionID, tool, true); err != nil {
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

// CreateSession creates a new session and emits a session_created observability event.
// If the session already exists, returns the existing session (idempotent).
func (s *RatchetServiceImpl) CreateSession(ctx context.Context, sessionID domain.SessionID) (*domain.Session, error) {
	// Try to get existing session first
	existingSession, err := s.sessionStore.Get(ctx, sessionID)
	if err == nil {
		// Session already exists, return it
		return existingSession, nil
	}

	if !errors.Is(err, secondary.ErrSessionNotFound) {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Session doesn't exist, create a new one
	session := domain.NewSessionWithTime(sessionID, s.clock.Now())
	if err := s.sessionStore.Create(ctx, session); err != nil {
		if errors.Is(err, secondary.ErrSessionAlreadyExists) {
			return s.sessionStore.Get(ctx, sessionID)
		}
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	s.emitEvent(ctx, domain.EventTypeSessionCreated, sessionID, "", "", nil)
	return session, nil
}

// GetRequiredPrerequisite returns the prerequisite tool for a given tool
func (s *RatchetServiceImpl) GetRequiredPrerequisite(tool domain.ToolName) (domain.ToolName, error) {
	rules := s.findRules(tool)
	if len(rules) == 0 {
		return "", fmt.Errorf("no rule found for tool %s", tool)
	}
	return rules[0].Prerequisite, nil
}

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
	if s.eventStore == nil {
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

	if err := s.eventStore.Store(ctx, event); err != nil {
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
