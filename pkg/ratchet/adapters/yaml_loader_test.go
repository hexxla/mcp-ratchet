package adapters

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
)

func TestYAMLConfigLoader_Load(t *testing.T) {
	loader := NewYAMLConfigLoader()

	yamlConfig := `
rules:
  - tool: "Create Cell"
    prerequisite: "List Tags"
    expiry: "5m"
    error_message: "Must call List Tags first"
`
	rules, err := loader.Load(context.Background(), strings.NewReader(yamlConfig))
	if err != nil {
		t.Errorf("Load() error = %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(rules))
	}
	if rules[0].Tool != "Create Cell" {
		t.Errorf("Expected tool 'Create Cell', got '%s'", rules[0].Tool)
	}
	if rules[0].Expiry != 5*time.Minute {
		t.Errorf("Expected expiry 5m, got %v", rules[0].Expiry)
	}
}

func TestYAMLConfigLoader_ValidateCircularDependency(t *testing.T) {
	loader := NewYAMLConfigLoader()

	rules := []domain.Rule{
		{Tool: "A", Prerequisite: "B", Expiry: 5 * time.Minute},
		{Tool: "B", Prerequisite: "A", Expiry: 5 * time.Minute},
	}

	err := loader.Validate(rules)
	if !errors.Is(err, domain.ErrCircularDependency) {
		t.Errorf("Expected ErrCircularDependency, got %v", err)
	}
}

func TestYAMLConfigLoader_ValidateNoCycle(t *testing.T) {
	loader := NewYAMLConfigLoader()

	rules := []domain.Rule{
		{Tool: "A", Prerequisite: "B", Expiry: 5 * time.Minute},
		{Tool: "B", Prerequisite: "C", Expiry: 5 * time.Minute},
	}

	err := loader.Validate(rules)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestYAMLConfigLoaderRejectsAmbiguousPolicies(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"unknown field", "rules: [{tool: write, prerequisite: read, one_time_ues: true}]", "one_time_ues"},
		{"extra document", "rules: []\n---\nrules: []", "exactly one"},
		{"nonpositive expiry", "rules: [{tool: read, expiry: 0s}]", "must be positive"},
		{"conflicting expiry", "rules: [{tool: write, prerequisite: first, expiry: 1s}, {tool: write, prerequisite: second, expiry: 2s}]", "conflicting token expiry"},
		{"mixed free pass", "rules: [{tool: write, prerequisite: read}, {tool: write}]", "mixes unrestricted and guarded rules"},
	}
	loader := NewYAMLConfigLoader()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, err := loader.Load(t.Context(), strings.NewReader(tt.yaml))
			if err == nil {
				err = loader.Validate(rules)
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
