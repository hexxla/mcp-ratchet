package ratchet

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// Policy describes session-scoped tool prerequisites. Zero CompletionTTL uses
// five minutes. A Rule's TTL overrides the lifetime of its Tool's completions.
type Policy struct {
	CompletionTTL time.Duration
	Rules         []Rule
}

// Rule permits Tool after any one of AfterAny has completed in the same session.
// An empty AfterAny permits Tool without a prerequisite. Consume spends the exact
// selected completion before the call, even if the call subsequently fails.
// Repeated blocks for one tool are alternatives, evaluated in policy order;
// each block's AfterAny is evaluated in slice order.
type Rule struct {
	Tool     string
	AfterAny []string
	Consume  bool
	TTL      time.Duration
	Message  string
}

var (
	// ErrInvalidPolicy identifies policies rejected by New.
	ErrInvalidPolicy = errors.New("invalid ratchet policy")
	// ErrPrerequisiteRequired identifies calls without an eligible completion.
	ErrPrerequisiteRequired = errors.New("ratchet prerequisite required")
	// ErrSessionLimit identifies calls denied by the in-memory session bound.
	ErrSessionLimit = errors.New("ratchet session limit reached")
	// ErrInvalidCall identifies missing session IDs, tool names or handlers.
	ErrInvalidCall = errors.New("invalid ratchet call")
)

func compilePolicy(policy Policy) (map[string][]Rule, map[string]time.Duration, error) {
	if policy.CompletionTTL < 0 {
		return nil, nil, fmt.Errorf("%w: completion TTL cannot be negative", ErrInvalidPolicy)
	}
	defaultTTL := policy.CompletionTTL
	if defaultTTL == 0 {
		defaultTTL = 5 * time.Minute
	}
	byTool := make(map[string][]Rule)
	producerTTL := make(map[string]time.Duration)
	explicitTTL := make(map[string]time.Duration)
	seen := make(map[[2]string]struct{})
	guarded := make(map[string]bool)
	unrestricted := make(map[string]bool)
	for _, rule := range policy.Rules {
		if rule.Tool == "" || rule.TTL < 0 {
			return nil, nil, fmt.Errorf("%w: rules require a tool and nonnegative TTL", ErrInvalidPolicy)
		}
		if len(rule.AfterAny) == 0 {
			if rule.Consume {
				return nil, nil, fmt.Errorf("%w: tool %q consumes without a prerequisite", ErrInvalidPolicy, rule.Tool)
			}
			if unrestricted[rule.Tool] {
				return nil, nil, fmt.Errorf("%w: duplicate unrestricted rule for tool %q", ErrInvalidPolicy, rule.Tool)
			}
			unrestricted[rule.Tool] = true
		} else {
			guarded[rule.Tool] = true
		}
		if rule.TTL > 0 {
			if ttl := explicitTTL[rule.Tool]; ttl != 0 && ttl != rule.TTL {
				return nil, nil, fmt.Errorf("%w: conflicting completion TTL for tool %q", ErrInvalidPolicy, rule.Tool)
			}
			explicitTTL[rule.Tool] = rule.TTL
		}
		for _, prerequisite := range rule.AfterAny {
			if prerequisite == "" {
				return nil, nil, fmt.Errorf("%w: tool %q has an empty prerequisite", ErrInvalidPolicy, rule.Tool)
			}
			key := [2]string{rule.Tool, prerequisite}
			if _, duplicate := seen[key]; duplicate {
				return nil, nil, fmt.Errorf("%w: duplicate prerequisite %q for tool %q", ErrInvalidPolicy, prerequisite, rule.Tool)
			}
			seen[key] = struct{}{}
			producerTTL[prerequisite] = defaultTTL
		}
		rule.AfterAny = slices.Clone(rule.AfterAny)
		byTool[rule.Tool] = append(byTool[rule.Tool], rule)
	}
	for tool := range guarded {
		if unrestricted[tool] {
			return nil, nil, fmt.Errorf("%w: tool %q mixes guarded and unrestricted rules", ErrInvalidPolicy, tool)
		}
	}
	for tool := range producerTTL {
		if ttl := explicitTTL[tool]; ttl != 0 {
			producerTTL[tool] = ttl
		}
	}
	// All declared edges participate, even when another alternative could break
	// a cycle at runtime. Policy cycles are rejected rather than silently ignored.
	visited := make(map[string]bool)
	visiting := make(map[string]bool)
	var visit func(string) bool
	visit = func(tool string) bool {
		if visiting[tool] {
			return true
		}
		if visited[tool] {
			return false
		}
		visiting[tool] = true
		for _, rule := range byTool[tool] {
			if slices.ContainsFunc(rule.AfterAny, visit) {
				return true
			}
		}
		delete(visiting, tool)
		visited[tool] = true
		return false
	}
	for tool := range byTool {
		if visit(tool) {
			return nil, nil, fmt.Errorf("%w: circular dependency involving tool %q", ErrInvalidPolicy, tool)
		}
	}
	return byTool, producerTTL, nil
}
