package decision

import "time"

// EvaluationRecorder tracks business outcomes from evaluations.
// Implementations should be safe for concurrent use.
//
// Duration Semantics:
// RecordEvaluationDuration measures end-to-end committed evaluation latency as seen by the caller,
// including domain logic execution and transaction commit overhead. This will be close to but slightly
// higher than TransactionRecorder.RecordTransactionDuration for the same operation.
//
// Failure Stage Semantics:
// IncrementEvaluationFailure distinguishes system failures from business rejections.
// Current stages:
//   - "persistence": Repository operation, audit append, or policy infrastructure failure
//   Future stages might include: "validation", "policy_engine", "external_service"
type EvaluationRecorder interface {
	RecordEvaluationDuration(outcome string, duration time.Duration)
	IncrementEvaluationOutcome(outcome string, reasonCode string)
	IncrementEvaluationFailure(stage string)
}

// NoOpEvaluationRecorder is a no-op implementation used as the default.
type NoOpEvaluationRecorder struct{}

func (NoOpEvaluationRecorder) RecordEvaluationDuration(string, time.Duration) {}
func (NoOpEvaluationRecorder) IncrementEvaluationOutcome(string, string)      {}
func (NoOpEvaluationRecorder) IncrementEvaluationFailure(string)              {}

var _ EvaluationRecorder = NoOpEvaluationRecorder{}
