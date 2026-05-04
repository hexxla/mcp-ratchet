package secondary

import (
	"context"
	"io"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
)

// RatchetConfig holds the full configuration loaded from the ratchet YAML file.
type RatchetConfig struct {
	Rules         []domain.Rule
	Observability domain.ObservabilityConfig
}

// ConfigLoader loads configuration from YAML
type ConfigLoader interface {
	// Load loads rules from a YAML source (legacy, rules-only).
	Load(ctx context.Context, source io.Reader) ([]domain.Rule, error)

	// LoadConfig loads the full ratchet configuration including observability settings.
	LoadConfig(ctx context.Context, source io.Reader) (*RatchetConfig, error)

	// Validate checks rules for circular dependencies.
	Validate(rules []domain.Rule) error
}
