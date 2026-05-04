package domain

import "errors"

// GreetingRequest represents a request for a greeting
type GreetingRequest struct {
	Name string
}

// Validate checks if the request is valid
func (r *GreetingRequest) Validate() error {
	if r.Name == "" {
		return errors.New("name cannot be empty")
	}
	return nil
}

// Greet creates a greeting message
func (r *GreetingRequest) Greet() string {
	return "Hello, " + r.Name + "!"
}

// GreetingResponse represents a greeting response
type GreetingResponse struct {
	Message string
}

// UserIdentificationRequest represents a request to identify a user
type UserIdentificationRequest struct{}

// UserIdentificationResponse represents a user identification response
type UserIdentificationResponse struct {
	UserName string
}
