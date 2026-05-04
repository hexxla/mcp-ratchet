package services

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	// Find rule for this tool
	rule := s.findRule(tool)
	if rule == nil {
		// No rule means tool is unrestricted
		return nil
	}

	// If tool has no prerequisites and no session, allow it (for demo/testing)
	if rule.Prerequisite == "" && sessionID == "" {
		return nil
	}

	// For demo: if tool has prerequisites but no session, return custom error message
	if rule.Prerequisite != "" && sessionID == "" {
		if rule.ErrorMessage != "" {
			return errors.New(rule.ErrorMessage)
		}
		return fmt.Errorf("tool %s requires a session and prerequisite %s", tool, rule.Prerequisite)
	}

	// Get session (only if we have a sessionID)
	session, err := s.sessionStore.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Check if prerequisite has been called
	if rule.Prerequisite != "" && !session.HasToolBeenCalled(rule.Prerequisite) {
		if rule.ErrorMessage != "" {
			return errors.New(rule.ErrorMessage)
		}
		return fmt.Errorf("prerequisite tool %s must be called first", rule.Prerequisite)
	}

	// Check if prerequisite tool has a valid token
	if rule.Prerequisite != "" {
		// Check token expiry using tokenStore
		validTokens, err := s.tokenStore.GetValidTokens(ctx, sessionID, rule.Prerequisite)
		if err != nil {
			return fmt.Errorf("failed to get valid tokens: %w", err)
		}

		if len(validTokens) == 0 {
			if rule.ErrorMessage != "" {
				return errors.New(rule.ErrorMessage)
			}
			return domain.ErrInvalidToken
		}
	}

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

// findRule finds the rule for a given tool
func (s *RatchetServiceImpl) findRule(tool domain.ToolName) *domain.Rule {
	for i := range s.rules {
		if s.rules[i].Tool == tool {
			return &s.rules[i]
		}
	}
	return nil
}
