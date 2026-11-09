package adapters

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/secondary"
)

// YAMLConfigLoader implements ConfigLoader for YAML files
type YAMLConfigLoader struct{}

// NewYAMLConfigLoader creates a new YAML config loader
func NewYAMLConfigLoader() secondary.ConfigLoader {
	return &YAMLConfigLoader{}
}

// config represents the YAML structure
type config struct {
	Rules         []ruleConfig               `yaml:"rules"`
	Observability domain.ObservabilityConfig `yaml:"observability"`
}

type ruleConfig struct {
	Tool         string `yaml:"tool"`
	Prerequisite string `yaml:"prerequisite"`
	Expiry       string `yaml:"expiry"`
	ErrorMessage string `yaml:"error_message"`
	OneTimeUse   bool   `yaml:"one_time_use"`
}

// LoadConfig loads the full ratchet configuration including observability settings.
func (y *YAMLConfigLoader) LoadConfig(ctx context.Context, source io.Reader) (*secondary.RatchetConfig, error) {
	var cfg config
	decoder := yaml.NewDecoder(source)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode YAML: %w", err)
	}

	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, fmt.Errorf("decode trailing YAML: %w", err)
		}
		return nil, errors.New("ratchet policy must contain exactly one YAML document")
	}

	rules := make([]domain.Rule, 0, len(cfg.Rules))
	for _, rc := range cfg.Rules {
		var expiry time.Duration
		if rc.Expiry != "" {
			d, err := time.ParseDuration(rc.Expiry)
			if err != nil {
				return nil, fmt.Errorf("invalid expiry duration for tool %s: %w", rc.Tool, err)
			}
			if d <= 0 {
				return nil, fmt.Errorf("expiry for tool %q must be positive", rc.Tool)
			}
			expiry = d
		}

		rule := domain.Rule{
			Tool:         domain.ToolName(rc.Tool),
			Prerequisite: domain.ToolName(rc.Prerequisite),
			Expiry:       expiry,
			ErrorMessage: rc.ErrorMessage,
			OneTimeUse:   rc.OneTimeUse,
		}

		if err := rule.Validate(); err != nil {
			return nil, fmt.Errorf("invalid rule for tool %s: %w", rc.Tool, err)
		}

		rules = append(rules, rule)
	}

	return &secondary.RatchetConfig{
		Rules:         rules,
		Observability: cfg.Observability,
	}, nil
}

// Load loads rules from YAML
func (y *YAMLConfigLoader) Load(ctx context.Context, source io.Reader) ([]domain.Rule, error) {
	cfg, err := y.LoadConfig(ctx, source)
	if err != nil {
		return nil, err
	}
	return cfg.Rules, nil
}

// Validate checks for circular dependencies in rules
func (y *YAMLConfigLoader) Validate(rules []domain.Rule) error {
	// Build adjacency map
	adj := make(map[domain.ToolName][]domain.ToolName)
	expiries := make(map[domain.ToolName]time.Duration)
	seen := make(map[[2]domain.ToolName]struct{}, len(rules))
	for _, rule := range rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("invalid rule for tool %q: %w", rule.Tool, err)
		}
		key := [2]domain.ToolName{rule.Tool, rule.Prerequisite}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("duplicate prerequisite %q for tool %q", rule.Prerequisite, rule.Tool)
		}
		seen[key] = struct{}{}
		if rule.Expiry > 0 {
			if expiry := expiries[rule.Tool]; expiry > 0 && expiry != rule.Expiry {
				return fmt.Errorf("conflicting token expiry for tool %q", rule.Tool)
			}
			expiries[rule.Tool] = rule.Expiry
		}
		if rule.Prerequisite != "" {
			adj[rule.Tool] = append(adj[rule.Tool], rule.Prerequisite)
		}
	}
	for tool := range adj {
		if _, unrestricted := seen[[2]domain.ToolName{tool, ""}]; unrestricted {
			return fmt.Errorf("tool %q mixes unrestricted and guarded rules", tool)
		}
	}

	// Detect cycles using DFS
	visited := make(map[domain.ToolName]bool)
	recursionStack := make(map[domain.ToolName]bool)

	var hasCycle func(tool domain.ToolName) bool
	hasCycle = func(tool domain.ToolName) bool {
		visited[tool] = true
		recursionStack[tool] = true

		for _, prereq := range adj[tool] {
			if !visited[prereq] {
				if hasCycle(prereq) {
					return true
				}
			} else if recursionStack[prereq] {
				return true
			}
		}

		recursionStack[tool] = false
		return false
	}

	for tool := range adj {
		if !visited[tool] {
			if hasCycle(tool) {
				return domain.ErrCircularDependency
			}
		}
	}

	return nil
}
