package secondary

import (
	"context"
	"io"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
)

// ConfigLoader loads configuration from YAML
type ConfigLoader interface {
	Load(ctx context.Context, source io.Reader) ([]domain.Rule, error)
	Validate(rules []domain.Rule) error
}
