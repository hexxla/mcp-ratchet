package primary

import (
	"context"
	"io"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
)

// RatchetService defines the primary port for ratchet operations
type RatchetService interface {
	// RegisterRule adds a new tool dependency rule
	RegisterRule(ctx context.Context, rule domain.Rule) error

	// ValidateToolCall checks if a tool can be called
	ValidateToolCall(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName, token domain.TokenValue) error

	// ConsumePrerequisiteToken consumes the prerequisite token for one-time-use rules.
	// Should be called after successful tool execution.
	ConsumePrerequisiteToken(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName) error

	// IssueToken creates a new ratchet token after successful execution
	IssueToken(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName) (domain.TokenValue, error)

	// LoadConfiguration loads rules from YAML
	LoadConfiguration(ctx context.Context, config io.Reader) ([]domain.Rule, error)

	// CreateSession creates a new session and emits a session_created observability event.
	CreateSession(ctx context.Context, sessionID domain.SessionID) (*domain.Session, error)

	// GetRequiredPrerequisite returns the prerequisite tool for a given tool
	GetRequiredPrerequisite(tool domain.ToolName) (domain.ToolName, error)

	// GetObservabilityStats returns aggregate event metrics.
	// Returns nil stats without error if observability is disabled.
	GetObservabilityStats(ctx context.Context) (*domain.EventStats, error)

	// GetObservabilityEvents retrieves events for a session matching the given filter.
	// Returns an empty slice without error if observability is disabled.
	GetObservabilityEvents(ctx context.Context, sessionID domain.SessionID, filter *secondary.EventFilter) ([]*domain.Event, error)
}
