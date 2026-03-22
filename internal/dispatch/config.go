package dispatch

import "time"

// DispatcherConfig controls the runtime behaviour of the Dispatcher.
type DispatcherConfig struct {
	// BatchSize is the maximum number of outbox rows claimed and processed per
	// poll cycle. Must be greater than zero.
	BatchSize int

	// PollInterval is the duration the dispatcher sleeps between poll cycles
	// when the queue is empty. Must be greater than zero.
	PollInterval time.Duration

	// MaxBackoff is the upper bound for the exponential sleep when consecutive
	// poll cycles encounter errors. The dispatcher resets to PollInterval on
	// any successful empty-queue or publish cycle.
	MaxBackoff time.Duration
}

// DefaultDispatcherConfig returns sensible production defaults.
func DefaultDispatcherConfig() DispatcherConfig {
	return DispatcherConfig{
		BatchSize:    100,
		PollInterval: 2 * time.Second,
		MaxBackoff:   30 * time.Second,
	}
}
