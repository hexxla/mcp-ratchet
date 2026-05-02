package secondary

import "time"

// Clock provides time functionality
type Clock interface {
	Now() time.Time
}
