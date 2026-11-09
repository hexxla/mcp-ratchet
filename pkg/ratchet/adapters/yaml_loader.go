package adapters

import (
	"errors"
	"fmt"
	"io"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet"
)

type yamlPolicy struct {
	CompletionTTL *string    `yaml:"completion_ttl"`
	Rules         []yamlRule `yaml:"rules"`
}

type yamlRule struct {
	Tool     string   `yaml:"tool"`
	AfterAny []string `yaml:"after_any"`
	Consume  bool     `yaml:"consume"`
	TTL      *string  `yaml:"ttl"`
	Message  string   `yaml:"message"`
}

// LoadPolicy decodes exactly one YAML document, rejects unknown fields and
// validates the resulting policy. Durations, when supplied, must be positive.
// Only completion_ttl and rules with tool/after_any/consume/ttl/message are
// accepted; legacy policy and observability fields are rejected.
func LoadPolicy(source io.Reader) (ratchet.Policy, error) {
	var cfg *yamlPolicy
	decoder := yaml.NewDecoder(source)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return ratchet.Policy{}, fmt.Errorf("decode ratchet YAML: %w", err)
	}
	if cfg == nil {
		return ratchet.Policy{}, errors.New("ratchet policy must be a YAML mapping")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err != nil {
			return ratchet.Policy{}, fmt.Errorf("decode trailing YAML: %w", err)
		}
		return ratchet.Policy{}, errors.New("ratchet policy must contain exactly one YAML document")
	}
	ttl, err := parseTTL(cfg.CompletionTTL)
	if err != nil {
		return ratchet.Policy{}, fmt.Errorf("completion_ttl: %w", err)
	}
	policy := ratchet.Policy{CompletionTTL: ttl, Rules: make([]ratchet.Rule, 0, len(cfg.Rules))}
	for _, rule := range cfg.Rules {
		ttl, err := parseTTL(rule.TTL)
		if err != nil {
			return ratchet.Policy{}, fmt.Errorf("TTL for tool %q: %w", rule.Tool, err)
		}
		policy.Rules = append(policy.Rules, ratchet.Rule{
			Tool: rule.Tool, AfterAny: rule.AfterAny, Consume: rule.Consume, TTL: ttl, Message: rule.Message,
		})
	}
	if _, err := ratchet.New(policy, ratchet.Options{}); err != nil {
		return ratchet.Policy{}, fmt.Errorf("validate ratchet policy: %w", err)
	}
	return policy, nil
}

func parseTTL(value *string) (time.Duration, error) {
	if value == nil {
		return 0, nil
	}
	ttl, err := time.ParseDuration(*value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration: %w", err)
	}
	if ttl <= 0 {
		return 0, errors.New("duration must be positive")
	}
	return ttl, nil
}
