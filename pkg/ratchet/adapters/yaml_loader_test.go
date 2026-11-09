package adapters

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet"
)

func TestLoadPolicy(t *testing.T) {
	policy, err := LoadPolicy(strings.NewReader(`
completion_ttl: 10m
rules:
  - tool: list_tags
    ttl: 1m
  - tool: write
    after_any: [list_tags, discover]
    consume: true
    message: Discover first.
  - tool: write
    after_any: [cached]
`))
	if err != nil {
		t.Fatal(err)
	}
	if policy.CompletionTTL != 10*time.Minute || len(policy.Rules) != 3 || policy.Rules[0].TTL != time.Minute {
		t.Fatalf("decoded policy = %+v", policy)
	}
	if got := policy.Rules[1]; len(got.AfterAny) != 2 || got.AfterAny[0] != "list_tags" || got.AfterAny[1] != "discover" || !got.Consume || got.Message != "Discover first." {
		t.Fatalf("grouped rule = %+v", got)
	}
	if policy.Rules[2].Consume {
		t.Fatal("repeated alternatives lost mixed consumption")
	}
}

func TestLoadPolicyRejectsInvalidYAMLAndPolicies(t *testing.T) {
	for _, tt := range []struct{ name, yaml, want string }{
		{"unknown field", "rules: [{tool: write, after_any: [read], consuem: true}]", "consuem"},
		{"legacy prerequisite", "rules: [{tool: write, prerequisite: read}]", "prerequisite"},
		{"legacy expiry", "rules: [{tool: read, expiry: 5m}]", "expiry"},
		{"legacy consumption", "rules: [{tool: write, one_time_use: true}]", "one_time_use"},
		{"legacy message", "rules: [{tool: write, error_message: denied}]", "error_message"},
		{"inert observability", "observability: {enabled: true}", "observability"},
		{"extra document", "rules: []\n---\nrules: []", "exactly one"},
		{"extra null document", "rules: []\n---\nnull", "exactly one"},
		{"empty document", "", "decode ratchet YAML"},
		{"null document", "null", "mapping"},
		{"sequence document", "[]", "unmarshal"},
		{"duplicate YAML key", "rules: []\nrules: []", "already defined"},
		{"scalar alternatives", "rules: [{tool: write, after_any: read}]", "unmarshal"},
		{"zero default TTL", "completion_ttl: 0s", "positive"},
		{"negative default TTL", "completion_ttl: -1s", "positive"},
		{"zero tool TTL", "rules: [{tool: read, ttl: 0s}]", "positive"},
		{"invalid TTL", "rules: [{tool: read, ttl: later}]", "invalid duration"},
		{"duplicate edge", "rules: [{tool: write, after_any: [read, read]}]", "duplicate"},
		{"conflicting TTL", "rules: [{tool: write, after_any: [first], ttl: 1s}, {tool: write, after_any: [second], ttl: 2s}]", "conflicting"},
		{"mixed free pass", "rules: [{tool: write, after_any: [read]}, {tool: write}]", "mixes"},
		{"self cycle", "rules: [{tool: read, after_any: [read]}]", "circular"},
		{"cycle", "rules: [{tool: a, after_any: [b]}, {tool: b, after_any: [a]}]", "circular"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadPolicy(strings.NewReader(tt.yaml))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadPolicy error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadPolicyPreservesValidationCause(t *testing.T) {
	_, err := LoadPolicy(strings.NewReader("rules: [{tool: a, after_any: [a]}]"))
	if !errors.Is(err, ratchet.ErrInvalidPolicy) {
		t.Fatalf("validation cause lost: %v", err)
	}
}
