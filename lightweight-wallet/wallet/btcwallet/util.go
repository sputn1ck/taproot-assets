package btcwallet

import (
	"context"
	"time"
)

// contextWithTimeout is a helper that creates a context with a standard timeout.
type timedContext struct {
	context.Context
	cancel context.CancelFunc
}

// contextWithTimeout creates a context with a 30 second timeout.
func contextWithTimeout() *timedContext {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	return &timedContext{
		Context: ctx,
		cancel:  cancel,
	}
}
