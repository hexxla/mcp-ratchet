package primary

import (
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
)

// Wrapper defines the primary port for function wrapping
type Wrapper interface {
	// Wrap wraps a function to require ratchet token validation
	Wrap(fn any, tool domain.ToolName) (any, error)
}
