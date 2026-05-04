package services

import (
	"context"

	"github.com/hexxla/mcp-ratchet/internal/core/domain"
	"github.com/hexxla/mcp-ratchet/internal/core/ports/primary"
)

// GreetingServiceImpl implements the GreetingService interface
type GreetingServiceImpl struct{}

// NewGreetingService creates a new greeting service
func NewGreetingService() primary.GreetingService {
	return &GreetingServiceImpl{}
}

// Greet generates a greeting message
func (s *GreetingServiceImpl) Greet(ctx context.Context, req domain.GreetingRequest) (domain.GreetingResponse, error) {
	if err := req.Validate(); err != nil {
		return domain.GreetingResponse{}, err
	}
	return domain.GreetingResponse{Message: req.Greet()}, nil
}

// UserServiceImpl implements the UserService interface
type UserServiceImpl struct{}

// NewUserService creates a new user service
func NewUserService() primary.UserService {
	return &UserServiceImpl{}
}

// IdentifyUser identifies the current user
func (s *UserServiceImpl) IdentifyUser(ctx context.Context, req domain.UserIdentificationRequest) (domain.UserIdentificationResponse, error) {
	return domain.UserIdentificationResponse{UserName: "DemoUser"}, nil
}
