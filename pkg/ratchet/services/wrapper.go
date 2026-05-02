package services

import (
	"context"
	"errors"
	"reflect"

	"github.com/hexxla/mcp-ratchet/pkg/ratchet/domain"
	"github.com/hexxla/mcp-ratchet/pkg/ratchet/ports/primary"
)

// WrapperImpl implements the Wrapper interface
type WrapperImpl struct {
	ratchetService primary.RatchetService
}

// NewWrapper creates a new wrapper
func NewWrapper(ratchetService primary.RatchetService) primary.Wrapper {
	return &WrapperImpl{
		ratchetService: ratchetService,
	}
}

// Wrap wraps a function to require ratchet token validation
func (w *WrapperImpl) Wrap(fn any, tool domain.ToolName) (any, error) {
	fnValue := reflect.ValueOf(fn)
	fnType := fnValue.Type()

	if fnType.Kind() != reflect.Func {
		return nil, errors.New("input must be a function")
	}

	// Create a wrapper function with the same signature but with token parameter
	wrapper := reflect.MakeFunc(fnType, func(args []reflect.Value) []reflect.Value {
		ctx := args[0].Interface().(context.Context)

		// Extract token from last argument
		token := args[len(args)-1].Interface().(domain.TokenValue)
		sessionID := domain.SessionID("default")

		// Validate token before calling original function
		err := w.ratchetService.ValidateToolCall(ctx, sessionID, tool, token)
		if err != nil {
			// Return error in the same format as original function
			return []reflect.Value{reflect.Zero(fnType.Out(0)), reflect.ValueOf(err)}
		}

		// Call original function without token
		originalArgs := args[:len(args)-1]
		results := fnValue.Call(originalArgs)

		// Check if function returned an error
		if len(results) > 0 && results[len(results)-1].CanInterface() {
			if err, ok := results[len(results)-1].Interface().(error); ok && err == nil {
				// Function succeeded, issue new token
				_, err := w.ratchetService.IssueToken(ctx, sessionID, tool)
				if err != nil {
					// Return error in the same format as original function
					return []reflect.Value{reflect.Zero(fnType.Out(0)), reflect.ValueOf(err)}
				}
			}
		}

		return results
	})

	return wrapper.Interface(), nil
}
