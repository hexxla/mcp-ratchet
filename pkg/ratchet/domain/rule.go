package domain

import (
	"errors"
	"fmt"
	"time"
)

// Rule defines a tool dependency
type Rule struct {
	Tool         ToolName      `yaml:"tool"`
	Prerequisite ToolName      `yaml:"prerequisite"`
	Expiry       time.Duration `yaml:"expiry"`
	ErrorMessage string        `yaml:"error_message"`
	OneTimeUse   bool          `yaml:"one_time_use"` // If true, token is consumed after use
}

// Validate checks if the rule is valid.
func (r *Rule) Validate() error {
	if err := r.Tool.Validate(); err != nil {
		return fmt.Errorf("tool validation failed: %w", err)
	}
	// Prerequisite can be empty for tools with no dependencies
	if r.Prerequisite != "" {
		if err := r.Prerequisite.Validate(); err != nil {
			return fmt.Errorf("prerequisite validation failed: %w", err)
		}
	}
	if r.Tool == r.Prerequisite {
		return errors.New("tool cannot depend on itself")
	}
	return nil
}
