package store

import "time"

// TransactionRecorder tracks database transaction outcomes.
// Implementations should be safe for concurrent use.
//
// Duration Semantics:
// RecordTransactionDuration measures transaction lifecycle latency at the
// persistence boundary, including begin, callback execution, and commit/rollback.
type TransactionRecorder interface {
	RecordTransactionDuration(operation string, outcome string, d time.Duration)
	IncrementTransactionCommit(operation string)
	IncrementTransactionRollback(operation string)
	IncrementTransactionError(operation string, stage string)
}

// NoOpTransactionRecorder is the default recorder when metrics are not configured.
type NoOpTransactionRecorder struct{}

func (NoOpTransactionRecorder) RecordTransactionDuration(string, string, time.Duration) {}
func (NoOpTransactionRecorder) IncrementTransactionCommit(string)                       {}
func (NoOpTransactionRecorder) IncrementTransactionRollback(string)                     {}
func (NoOpTransactionRecorder) IncrementTransactionError(string, string)                {}

var _ TransactionRecorder = NoOpTransactionRecorder{}
