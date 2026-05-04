package adapters

import (
	"fmt"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
)

// factoryOptions holds optional dependencies for the event store factory.
// Future backends (hexxladb, sql) register their clients here via Option functions.
type factoryOptions struct {
	customStore secondary.EventStore
}

// Option is a functional option for NewEventStore.
type Option func(*factoryOptions)

// WithCustomStore allows callers to provide a pre-built EventStore implementation.
// This is the primary extension point for HexxlaDB, SQL, or any other backend.
//
// Example:
//
//	eventStore, err := adapters.NewEventStore(cfg,
//	    adapters.WithCustomStore(myHexxlaDBEventStore),
//	)
func WithCustomStore(store secondary.EventStore) Option {
	return func(o *factoryOptions) {
		o.customStore = store
	}
}

// NewEventStore creates an EventStore based on the given ObservabilityConfig.
// Returns nil if observability is disabled — callers must handle a nil EventStore gracefully.
//
// Use Option functions to inject custom backends:
//
//	adapters.NewEventStore(cfg, adapters.WithCustomStore(hexxlaDBStore))
func NewEventStore(cfg domain.ObservabilityConfig, opts ...Option) (secondary.EventStore, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	options := &factoryOptions{}
	for _, opt := range opts {
		opt(options)
	}

	if options.customStore != nil {
		return options.customStore, nil
	}

	switch cfg.StorageType {
	case "", "memory":
		return NewMemoryEventStore(cfg.RetentionDays), nil
	default:
		return nil, fmt.Errorf("unsupported observability storage_type %q: use WithCustomStore() to register a custom backend", cfg.StorageType)
	}
}
