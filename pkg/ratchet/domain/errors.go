package domain

import "errors"

var (
	// ErrInvalidToken is returned when a token is invalid.
	ErrInvalidToken = errors.New("invalid token")
	// ErrExpiredToken is returned when a token has expired.
	ErrExpiredToken = errors.New("token has expired")
	// ErrMissingPrerequisite is returned when a prerequisite tool hasn't been called.
	ErrMissingPrerequisite = errors.New("prerequisite tool must be called first")
	// ErrCircularDependency is returned when rules contain circular dependencies.
	ErrCircularDependency = errors.New("circular dependency detected in rules")
)
