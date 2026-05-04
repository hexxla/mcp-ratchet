package adapters

import (
	"fmt"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
)

// NewEventStore creates an EventStore based on the given ObservabilityConfig.
// Returns nil if observability is disabled — callers must handle a nil EventStore gracefully.
// This factory is the extension point for future backends (hexxladb, sql).
func NewEventStore(cfg domain.ObservabilityConfig) (secondary.EventStore, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	switch cfg.StorageType {
	case "", "memory":
		return NewMemoryEventStore(cfg.RetentionDays), nil
	default:
		return nil, fmt.Errorf("unsupported observability storage_type %q: supported values are \"memory\"", cfg.StorageType)
	}
}
