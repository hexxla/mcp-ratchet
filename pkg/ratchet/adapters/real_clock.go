package adapters

import "time"

// RealClock implements Clock using time.Now()
type RealClock struct{}

// NewRealClock creates a new real clock
func NewRealClock() *RealClock {
	return &RealClock{}
}

// Now returns the current time
func (r *RealClock) Now() time.Time {
	return time.Now()
}
