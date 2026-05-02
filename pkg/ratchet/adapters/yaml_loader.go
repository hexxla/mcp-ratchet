package adapters

import (
	"context"
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
	Rules []ruleConfig `yaml:"rules"`
}

type ruleConfig struct {
	Tool         string `yaml:"tool"`
	Prerequisite string `yaml:"prerequisite"`
	Expiry       string `yaml:"expiry"`
	ErrorMessage string `yaml:"error_message"`
	OneTimeUse   bool   `yaml:"one_time_use"`
}

// Load loads rules from YAML
func (y *YAMLConfigLoader) Load(ctx context.Context, source io.Reader) ([]domain.Rule, error) {
	var cfg config
	if err := yaml.NewDecoder(source).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode YAML: %w", err)
	}

	rules := make([]domain.Rule, 0, len(cfg.Rules))
	for _, rc := range cfg.Rules {
		expiry := 5 * time.Minute // default
		if rc.Expiry != "" {
			d, err := time.ParseDuration(rc.Expiry)
			if err != nil {
				return nil, fmt.Errorf("invalid expiry duration for tool %s: %w", rc.Tool, err)
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

	return rules, nil
}

// Validate checks for circular dependencies in rules
func (y *YAMLConfigLoader) Validate(rules []domain.Rule) error {
	// Build adjacency map
	adj := make(map[domain.ToolName][]domain.ToolName)
	for _, rule := range rules {
		adj[rule.Tool] = append(adj[rule.Tool], rule.Prerequisite)
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
