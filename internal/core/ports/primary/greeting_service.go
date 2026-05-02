package primary

import (
	"context"

	"github.com/hexxla/mcp-ratchet/internal/core/domain"
)

// GreetingService defines the primary port for greeting functionality
type GreetingService interface {
	// Greet returns a greeting message for the given name
	Greet(ctx context.Context, req domain.GreetingRequest) (domain.GreetingResponse, error)
}

// UserService defines the primary port for user identification functionality
type UserService interface {
	// IdentifyUser returns the current user's name
	IdentifyUser(ctx context.Context, req domain.UserIdentificationRequest) (domain.UserIdentificationResponse, error)
}
