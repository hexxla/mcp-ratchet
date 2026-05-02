package primary

import (
	"context"
	"io"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
)

// RatchetService defines the primary port for ratchet operations
type RatchetService interface {
	// RegisterRule adds a new tool dependency rule
	RegisterRule(ctx context.Context, rule domain.Rule) error

	// ValidateToolCall checks if a tool can be called
	ValidateToolCall(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName, token domain.TokenValue) error

	// IssueToken creates a new ratchet token after successful execution
	IssueToken(ctx context.Context, sessionID domain.SessionID, tool domain.ToolName) (domain.TokenValue, error)

	// LoadConfiguration loads rules from YAML
	LoadConfiguration(ctx context.Context, config io.Reader) ([]domain.Rule, error)

	// GetRequiredPrerequisite returns the prerequisite tool for a given tool
	GetRequiredPrerequisite(tool domain.ToolName) (domain.ToolName, error)
}
