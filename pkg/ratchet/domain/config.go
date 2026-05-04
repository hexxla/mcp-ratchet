package domain

// ObservabilityConfig controls the observability event store behaviour.
// It is loaded from the ratchet YAML configuration file.
type ObservabilityConfig struct {
	// Enabled controls whether event capture is active.
	// When false, no events are stored and there is zero overhead.
	Enabled bool `yaml:"enabled"`

	// StorageType selects the event store backend.
	// Supported values: "memory" (default), "hexxladb", "sql"
	StorageType string `yaml:"storage_type"`

	// RetentionDays specifies how long events are kept before pruning.
	// A value of 0 disables pruning (keep all events).
	RetentionDays int `yaml:"retention_days"`

	// WebSocketEnabled controls whether the WebSocket streaming endpoint is active.
	// When true, clients can connect to /observability/stream for real-time events.
	WebSocketEnabled bool `yaml:"websocket_enabled"`
}
