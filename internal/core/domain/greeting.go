package domain

import "errors"

// GreetingRequest represents a request for a greeting
type GreetingRequest struct {
	name string
}

// Name returns the name from the request
func (r *GreetingRequest) Name() string {
	return r.name
}

// SetName sets the name on the request
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

// GreetingResponse represents a greeting response
type GreetingResponse struct {
	message string
}

// Message returns the message from the response
func (r *GreetingResponse) Message() string {
	return r.message
}

// SetMessage sets the message on the response
func (r *GreetingResponse) SetMessage(message string) {
	r.message = message
}

// UserIdentificationRequest represents a request to identify a user
type UserIdentificationRequest struct{}

// UserIdentificationResponse represents a user identification response
type UserIdentificationResponse struct {
	userName string
}

// UserName returns the user name from the response
func (r *UserIdentificationResponse) UserName() string {
	return r.userName
}

// SetUserName sets the user name on the response
func (r *UserIdentificationResponse) SetUserName(userName string) {
	r.userName = userName
}
