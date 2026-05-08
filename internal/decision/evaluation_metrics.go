package decision

import "time"

// EvaluationRecorder tracks business outcomes from evaluations.
// Implementations should be safe for concurrent use.
//
// Duration Semantics:
// RecordEvaluationDuration measures end-to-end committed evaluation latency
// as seen by the caller, including domain logic execution and transaction
// commit overhead. This will be close to but slightly higher than
// TransactionRecorder.RecordTransactionDuration for the same operation.
//
// Outcome / FailureKind labelling on RecordEvaluationDuration:
//   - On success: outcome is the evaluation outcome (accept/escalate/reject/
//     request_clarification); failureKind is "none".
//   - On failure: outcome is "none"; failureKind is one of the typed failure
//     categories (envelope_persistence, audit_append, idempotency_conflict,
//     authority_resolution, invalid_transition, policy_evaluation, …).
// Labels are intentionally low-cardinality and code-controlled.
//
// Failure Counter Semantics (D27i-c chunk 4):
// IncrementEvaluationFailure carries two labels: failureKind (the typed
// FailureCategory string) and correctnessClass (the chunk-1 FailureClass
// derived via ClassifyFailure). Cardinality bound on correctnessClass is 5
// (governance_integrity, persistence, input, resource, consistency). The
// two labels are deterministically related per the chunk-1 mapping but are
// emitted independently so dashboards can group on either axis.
type EvaluationRecorder interface {
	RecordEvaluationDuration(outcome string, failureKind string, duration time.Duration)
	IncrementEvaluationOutcome(outcome string, reasonCode string)
	IncrementEvaluationFailure(failureKind string, correctnessClass string)
}

// NoOpEvaluationRecorder is a no-op implementation used as the default.
type NoOpEvaluationRecorder struct{}

func (NoOpEvaluationRecorder) RecordEvaluationDuration(string, string, time.Duration) {}
func (NoOpEvaluationRecorder) IncrementEvaluationOutcome(string, string)               {}
func (NoOpEvaluationRecorder) IncrementEvaluationFailure(string, string)               {}

var _ EvaluationRecorder = NoOpEvaluationRecorder{}
