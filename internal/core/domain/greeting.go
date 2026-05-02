package domain

import "errors"

// GreetingRequest represents a request for a greeting
type GreetingRequest struct {
	name string
}

// GreetingResponse represents a greeting response
type GreetingResponse struct {
	message string
}

// GetMessage returns the message from the response
func (r *GreetingResponse) GetMessage() string {
	return r.message
}

// SetMessage sets the message for the response
func (r *GreetingResponse) SetMessage(msg string) {
	r.message = msg
}

// Name returns the name from the request
func (r *GreetingRequest) Name() string {
	return r.name
}

// SetName sets the name for the request
func (r *GreetingRequest) SetName(name string) {
	r.name = name
}

// Validate checks if the request is valid
func (r *GreetingRequest) Validate() error {
	if r.name == "" {
		return errors.New("name cannot be empty")
	}
	return nil
}

// Greet creates a greeting message
func (r *GreetingRequest) Greet() string {
	return "Hello, " + r.name + "!"
}

// UserIdentificationRequest represents a request to identify a user
type UserIdentificationRequest struct{}

// UserIdentificationResponse represents a user identification response
type UserIdentificationResponse struct {
	userName string
}

// GetUserName returns the user name
func (r *UserIdentificationResponse) GetUserName() string {
	return r.userName
}

// SetUserName sets the user name
func (r *UserIdentificationResponse) SetUserName(name string) {
	r.userName = name
}
