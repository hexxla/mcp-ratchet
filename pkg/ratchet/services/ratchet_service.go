package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/primary"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
)

// RatchetServiceImpl implements the RatchetService interface
type RatchetServiceImpl struct {
	configLoader secondary.ConfigLoader
	tokenStore   secondary.TokenStore
	sessionStore secondary.SessionStore
	randomGen    secondary.RandomGenerator
	clock        secondary.Clock
	rules        []domain.Rule
	eventStore   secondary.EventStore // Optional, nil disables observability
}

// NewRatchetService creates a new ratchet service
func NewRatchetService(
	configLoader secondary.ConfigLoader,
	tokenStore secondary.TokenStore,
	sessionStore secondary.SessionStore,
	randomGen secondary.RandomGenerator,
	clock secondary.Clock,
) primary.RatchetService {
	return &RatchetServiceImpl{
		configLoader: configLoader,
		tokenStore:   tokenStore,
		sessionStore: sessionStore,
		randomGen:    randomGen,
		clock:        clock,
		rules:        make([]domain.Rule, 0),
	}
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
) primary.RatchetService {
	return &RatchetServiceImpl{
		configLoader: configLoader,
		tokenStore:   tokenStore,
		sessionStore: sessionStore,
		randomGen:    randomGen,
		clock:        clock,
		rules:        make([]domain.Rule, 0),
		eventStore:   eventStore,
	}
}

// RegisterRule adds a new tool dependency rule
func (s *RatchetServiceImpl) RegisterRule(ctx context.Context, rule domain.Rule) error {
	if err := rule.Validate(); err != nil {
		return fmt.Errorf("rule validation failed: %w", err)
	}
	s.rules = append(s.rules, rule)
	return nil
}

// ValidateToolCall checks if a tool can be called
func (s *RatchetServiceImpl) ValidateToolCall(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName, token domain.TokenValue) error {
	s.emitEvent(ctx, domain.EventTypeToolCallAttempt, sessionID, tool, token, nil)

	// Find rule for this tool
	rule := s.findRule(tool)
	if rule == nil {
		// No rule means tool is unrestricted
		s.emitEvent(ctx, domain.EventTypeToolCallSuccess, sessionID, tool, token, nil)
		return nil
	}

	// If tool has no prerequisites and no session, allow it (for demo/testing)
	if rule.Prerequisite == "" && sessionID == "" {
		s.emitEvent(ctx, domain.EventTypeToolCallSuccess, sessionID, tool, token, nil)
		return nil
	}

	// For demo: if tool has prerequisites but no session, return custom error message
	if rule.Prerequisite != "" && sessionID == "" {
		var validationErr error
		if rule.ErrorMessage != "" {
			validationErr = errors.New(rule.ErrorMessage)
		} else {
			validationErr = fmt.Errorf("tool %s requires a session and prerequisite %s", tool, rule.Prerequisite)
		}
		s.emitEvent(ctx, domain.EventTypeToolCallFailure, sessionID, tool, token, map[string]any{"error": validationErr.Error()})
		return validationErr
	}

	// Get session (only if we have a sessionID)
	session, err := s.sessionStore.Get(ctx, sessionID)
	if err != nil {
		s.emitEvent(ctx, domain.EventTypeToolCallFailure, sessionID, tool, token, map[string]any{"error": err.Error()})
		return fmt.Errorf("session not found: %w", err)
	}

	// Check if prerequisite has been called
	if rule.Prerequisite != "" && !session.HasToolBeenCalled(rule.Prerequisite) {
		var validationErr error
		if rule.ErrorMessage != "" {
			validationErr = errors.New(rule.ErrorMessage)
		} else {
			validationErr = fmt.Errorf("prerequisite tool %s must be called first", rule.Prerequisite)
		}
		s.emitEvent(ctx, domain.EventTypeToolCallFailure, sessionID, tool, token, map[string]any{"error": validationErr.Error()})
		return validationErr
	}

	// Check if prerequisite tool has a valid token
	if rule.Prerequisite != "" {
		// Check token expiry using tokenStore
		validTokens, err := s.tokenStore.GetValidTokens(ctx, sessionID, rule.Prerequisite)
		if err != nil {
			s.emitEvent(ctx, domain.EventTypeToolCallFailure, sessionID, tool, token, map[string]any{"error": err.Error()})
			return fmt.Errorf("failed to get valid tokens: %w", err)
		}

		if len(validTokens) == 0 {
			var validationErr error
			if rule.ErrorMessage != "" {
				validationErr = errors.New(rule.ErrorMessage)
			} else {
				validationErr = domain.ErrInvalidToken
			}
			s.emitEvent(ctx, domain.EventTypeToolCallFailure, sessionID, tool, token, map[string]any{"error": validationErr.Error()})
			return validationErr
		}
	}

	s.emitEvent(ctx, domain.EventTypeToolCallSuccess, sessionID, tool, token, nil)
	return nil
}

// IssueToken creates a new ratchet token after successful execution
func (s *RatchetServiceImpl) IssueToken(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName) (domain.TokenValue, error) {
	// Find rule for this tool to get expiry
	rule := s.findRule(tool)
	expiry := 5 * time.Minute // 5 minutes default
	if rule != nil && rule.Expiry > 0 {
		expiry = rule.Expiry
	}

	tokenValue, err := s.randomGen.GenerateToken()
	if err != nil {
		return "", err
	}

	// Store token
	err = s.tokenStore.Store(ctx, sessionID, tool, tokenValue, s.clock.Now().Add(expiry))
	if err != nil {
		return "", fmt.Errorf("failed to store token: %w", err)
	}

	// Update session
	session, err := s.sessionStore.Get(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}
	session.AddToken(tool, tokenValue)
	err = s.sessionStore.Update(ctx, session)
	if err != nil {
		return "", fmt.Errorf("failed to update session: %w", err)
	}

	s.emitEvent(ctx, domain.EventTypeTokenCreated, sessionID, tool, tokenValue, map[string]any{"expiry": s.clock.Now().Add(expiry).Format(time.RFC3339)})

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

	s.rules = rules
	return rules, nil
}

// ConsumePrerequisiteToken consumes the prerequisite token for one-time-use rules.
// Should be called after successful tool execution to consume the prerequisite token.
func (s *RatchetServiceImpl) ConsumePrerequisiteToken(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName) error {
	// Find rule for this tool
	rule := s.findRule(tool)
	if rule == nil || rule.Prerequisite == "" || !rule.OneTimeUse {
		// No rule, no prerequisite, or not one-time-use: nothing to consume
		return nil
	}

	// Get valid tokens for the prerequisite tool
	validTokens, err := s.tokenStore.GetValidTokens(ctx, sessionID, rule.Prerequisite)
	if err != nil {
		return fmt.Errorf("failed to get valid tokens: %w", err)
	}

	if len(validTokens) == 0 {
		// Token already consumed or expired - this is okay, just nothing to do
		return nil
	}

	// Remove the last valid token from tokenStore
	lastToken := validTokens[len(validTokens)-1]
	if err := s.tokenStore.RemoveToken(ctx, sessionID, rule.Prerequisite, lastToken); err != nil {
		return fmt.Errorf("failed to remove token from store: %w", err)
	}

	s.emitEvent(ctx, domain.EventTypeTokenConsumed, sessionID, rule.Prerequisite, lastToken, map[string]any{"consumed_by": string(tool)})

	// Also remove from session
	session, err := s.sessionStore.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	prereqTokens, hasToken := session.Tokens[rule.Prerequisite]
	if hasToken && len(prereqTokens) > 0 {
		session.Tokens[rule.Prerequisite] = prereqTokens[:len(prereqTokens)-1]
		if err := s.sessionStore.Update(ctx, session); err != nil {
			return fmt.Errorf("failed to update session after token removal: %w", err)
		}
	}

	return nil
}

// GetRequiredPrerequisite returns the prerequisite tool for a given tool
func (s *RatchetServiceImpl) GetRequiredPrerequisite(tool domain.ToolName) (domain.ToolName, error) {
	rule := s.findRule(tool)
	if rule == nil {
		return "", fmt.Errorf("no rule found for tool %s", tool)
	}
	return rule.Prerequisite, nil
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
		ID:        domain.EventID(fmt.Sprintf("%s-%s-%d", eventType, tool, s.clock.Now().UnixNano())),
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

// findRule finds the rule for a given tool
func (s *RatchetServiceImpl) findRule(tool domain.ToolName) *domain.Rule {
	for i := range s.rules {
		if s.rules[i].Tool == tool {
			return &s.rules[i]
		}
	}
	return nil
}
