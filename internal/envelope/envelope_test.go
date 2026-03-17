package envelope_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
)

var testTime = time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

// ---------------------------------------------------------------------------
// Constructor tests
// ---------------------------------------------------------------------------

func TestNew_CreatesEnvelopeInReceivedState(t *testing.T) {
	raw := json.RawMessage(`{"action":"test"}`)

	env, err := envelope.New("env-1", "test-source", "req-1", raw, testTime)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if env.ID() != "env-1" {
		t.Errorf("ID: got %q, want %q", env.ID(), "env-1")
	}
	if env.RequestID() != "req-1" {
		t.Errorf("RequestID: got %q, want %q", env.RequestID(), "req-1")
	}
	if env.RequestSource() != "test-source" {
		t.Errorf("RequestSource: got %q, want %q", env.RequestSource(), "test-source")
	}
	if env.State != envelope.EnvelopeStateReceived {
		t.Errorf("State: got %q, want %q", env.State, envelope.EnvelopeStateReceived)
	}
	if env.Identity.SchemaVersion != envelope.SchemaVersion {
		t.Errorf("SchemaVersion: got %d, want %d", env.Identity.SchemaVersion, envelope.SchemaVersion)
	}
	if string(env.Submitted.Raw) != string(raw) {
		t.Errorf("Submitted.Raw: got %q, want %q", env.Submitted.Raw, raw)
	}
	if !env.Submitted.ReceivedAt.Equal(testTime) {
		t.Errorf("ReceivedAt: got %v, want %v", env.Submitted.ReceivedAt, testTime)
	}
	if !env.CreatedAt.Equal(testTime) {
		t.Errorf("CreatedAt: got %v, want %v", env.CreatedAt, testTime)
	}
	if !env.UpdatedAt.Equal(testTime) {
		t.Errorf("UpdatedAt: got %v, want %v", env.UpdatedAt, testTime)
	}
	if env.ClosedAt != nil {
		t.Errorf("ClosedAt: got %v, want nil", env.ClosedAt)
	}
}

func TestNew_RejectsInvalidJSON(t *testing.T) {
	invalidJSON := json.RawMessage(`{invalid`)

	_, err := envelope.New("env-1", "test-source", "req-1", invalidJSON, testTime)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// ---------------------------------------------------------------------------
// State transition tests - Happy paths
// ---------------------------------------------------------------------------

func TestTransition_ReceivedToEvaluating(t *testing.T) {
	env := mustEnvelope(t, "env-1", "req-1")
	now := testTime.Add(time.Second)

	if err := env.Transition(envelope.EnvelopeStateEvaluating, now); err != nil {
		t.Fatalf("Transition: %v", err)
	}

	if env.State != envelope.EnvelopeStateEvaluating {
		t.Errorf("State: got %q, want %q", env.State, envelope.EnvelopeStateEvaluating)
	}
	if !env.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt: got %v, want %v", env.UpdatedAt, now)
	}
	if env.ClosedAt != nil {
		t.Errorf("ClosedAt should remain nil, got %v", env.ClosedAt)
	}
}

func TestTransition_EvaluatingToOutcomeRecorded_RequiresExplanation(t *testing.T) {
	env := mustEnvelope(t, "env-1", "req-1")
	mustTransition(t, env, envelope.EnvelopeStateEvaluating)

	// Missing explanation - should fail
	err := env.Transition(envelope.EnvelopeStateOutcomeRecorded, testTime.Add(2*time.Second))
	if err != envelope.ErrMissingExplanation {
		t.Fatalf("expected ErrMissingExplanation, got %v", err)
	}

	// Add explanation - should succeed
	env.Evaluation.Explanation = &envelope.DecisionExplanation{
		Result: "Execute",
		Reason: "WITHIN_AUTHORITY",
	}

	if err := env.Transition(envelope.EnvelopeStateOutcomeRecorded, testTime.Add(2*time.Second)); err != nil {
		t.Fatalf("Transition with explanation: %v", err)
	}

	if env.State != envelope.EnvelopeStateOutcomeRecorded {
		t.Errorf("State: got %q, want %q", env.State, envelope.EnvelopeStateOutcomeRecorded)
	}
}

func TestTransition_EvaluatingToEscalated_RequiresExplanation(t *testing.T) {
	env := mustEnvelope(t, "env-1", "req-1")
	mustTransition(t, env, envelope.EnvelopeStateEvaluating)

	// Missing explanation - should fail
	err := env.Transition(envelope.EnvelopeStateEscalated, testTime.Add(2*time.Second))
	if err != envelope.ErrMissingExplanation {
		t.Fatalf("expected ErrMissingExplanation, got %v", err)
	}

	// Add explanation - should succeed
	env.Evaluation.Explanation = &envelope.DecisionExplanation{
		Result: "Escalate",
		Reason: "CONFIDENCE_BELOW_THRESHOLD",
	}

	if err := env.Transition(envelope.EnvelopeStateEscalated, testTime.Add(2*time.Second)); err != nil {
		t.Fatalf("Transition with explanation: %v", err)
	}

	if env.State != envelope.EnvelopeStateEscalated {
		t.Errorf("State: got %q, want %q", env.State, envelope.EnvelopeStateEscalated)
	}
}

func TestTransition_OutcomeRecordedToClosed_RequiresOutcome(t *testing.T) {
	env := mustEnvelope(t, "env-1", "req-1")
	mustTransition(t, env, envelope.EnvelopeStateEvaluating)
	env.Evaluation.Explanation = &envelope.DecisionExplanation{Result: "Execute", Reason: "WITHIN_AUTHORITY"}
	mustTransition(t, env, envelope.EnvelopeStateOutcomeRecorded)

	// Missing outcome - should fail
	err := env.Transition(envelope.EnvelopeStateClosed, testTime.Add(3*time.Second))
	if err != envelope.ErrMissingOutcome {
		t.Fatalf("expected ErrMissingOutcome, got %v", err)
	}

	// Add outcome and reason - should succeed
	env.Evaluation.Outcome = eval.OutcomeAccept
	env.Evaluation.ReasonCode = eval.ReasonWithinAuthority
	now := testTime.Add(3 * time.Second)

	if err := env.Transition(envelope.EnvelopeStateClosed, now); err != nil {
		t.Fatalf("Transition with outcome: %v", err)
	}

	if env.State != envelope.EnvelopeStateClosed {
		t.Errorf("State: got %q, want %q", env.State, envelope.EnvelopeStateClosed)
	}
	if env.ClosedAt == nil {
		t.Fatal("ClosedAt should be set")
	}
	if !env.ClosedAt.Equal(now) {
		t.Errorf("ClosedAt: got %v, want %v", *env.ClosedAt, now)
	}
}

func TestTransition_EscalatedToAwaitingReview(t *testing.T) {
	env := mustEnvelope(t, "env-1", "req-1")
	mustTransition(t, env, envelope.EnvelopeStateEvaluating)
	env.Evaluation.Explanation = &envelope.DecisionExplanation{Result: "Escalate", Reason: "CONFIDENCE_BELOW_THRESHOLD"}
	mustTransition(t, env, envelope.EnvelopeStateEscalated)

	if err := env.Transition(envelope.EnvelopeStateAwaitingReview, testTime.Add(3*time.Second)); err != nil {
		t.Fatalf("Transition: %v", err)
	}

	if env.State != envelope.EnvelopeStateAwaitingReview {
		t.Errorf("State: got %q, want %q", env.State, envelope.EnvelopeStateAwaitingReview)
	}
}

func TestTransition_AwaitingReviewToClosed_RequiresReview(t *testing.T) {
	env := mustEnvelope(t, "env-1", "req-1")
	mustTransition(t, env, envelope.EnvelopeStateEvaluating)
	env.Evaluation.Explanation = &envelope.DecisionExplanation{Result: "Escalate", Reason: "CONFIDENCE_BELOW_THRESHOLD"}
	mustTransition(t, env, envelope.EnvelopeStateEscalated)
	mustTransition(t, env, envelope.EnvelopeStateAwaitingReview)

	env.Evaluation.Outcome = eval.OutcomeEscalate
	env.Evaluation.ReasonCode = eval.ReasonConfidenceBelowThreshold

	// Missing review - should fail
	err := env.Transition(envelope.EnvelopeStateClosed, testTime.Add(4*time.Second))
	if err != envelope.ErrMissingReview {
		t.Fatalf("expected ErrMissingReview, got %v", err)
	}

	// Add review - should succeed
	env.Review = &envelope.EscalationReview{
		Decision:     envelope.ReviewDecisionApproved,
		ReviewerID:   "reviewer-1",
		ReviewerKind: "human",
		ReviewedAt:   testTime.Add(4 * time.Second),
	}

	now := testTime.Add(5 * time.Second)
	if err := env.Transition(envelope.EnvelopeStateClosed, now); err != nil {
		t.Fatalf("Transition with review: %v", err)
	}

	if env.State != envelope.EnvelopeStateClosed {
		t.Errorf("State: got %q, want %q", env.State, envelope.EnvelopeStateClosed)
	}
	if env.ClosedAt == nil {
		t.Fatal("ClosedAt should be set")
	}
	if !env.ClosedAt.Equal(now) {
		t.Errorf("ClosedAt: got %v, want %v", *env.ClosedAt, now)
	}
}

// ---------------------------------------------------------------------------
// State transition tests - Invalid transitions
// ---------------------------------------------------------------------------

func TestTransition_RejectsInvalidStateChange(t *testing.T) {
	env := mustEnvelope(t, "env-1", "req-1")

	// RECEIVED -> CLOSED is not allowed
	err := env.Transition(envelope.EnvelopeStateClosed, testTime.Add(time.Second))
	if err != envelope.ErrInvalidTransition {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestTransition_ClosedEnvelopeCannotTransition(t *testing.T) {
	env := mustEnvelope(t, "env-1", "req-1")
	mustTransition(t, env, envelope.EnvelopeStateEvaluating)
	env.Evaluation.Explanation = &envelope.DecisionExplanation{Result: "Execute", Reason: "WITHIN_AUTHORITY"}
	mustTransition(t, env, envelope.EnvelopeStateOutcomeRecorded)
	env.Evaluation.Outcome = eval.OutcomeAccept
	env.Evaluation.ReasonCode = eval.ReasonWithinAuthority
	mustTransition(t, env, envelope.EnvelopeStateClosed)

	// Attempt to transition closed envelope
	err := env.Transition(envelope.EnvelopeStateEvaluating, testTime.Add(10*time.Second))
	if err != envelope.ErrEnvelopeClosed {
		t.Errorf("expected ErrEnvelopeClosed, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Convenience accessor tests
// ---------------------------------------------------------------------------

func TestAccessors(t *testing.T) {
	env := mustEnvelope(t, "env-123", "req-456")
	env.Evaluation.Outcome = eval.OutcomeAccept
	env.Evaluation.ReasonCode = eval.ReasonWithinAuthority

	if got := env.ID(); got != "env-123" {
		t.Errorf("ID(): got %q, want %q", got, "env-123")
	}
	if got := env.RequestSource(); got != "test-source" {
		t.Errorf("RequestSource(): got %q, want %q", got, "test-source")
	}
	if got := env.RequestID(); got != "req-456" {
		t.Errorf("RequestID(): got %q, want %q", got, "req-456")
	}
	if got := env.Outcome(); got != eval.OutcomeAccept {
		t.Errorf("Outcome(): got %q, want %q", got, eval.OutcomeAccept)
	}
	if got := env.ReasonCode(); got != eval.ReasonWithinAuthority {
		t.Errorf("ReasonCode(): got %q, want %q", got, eval.ReasonWithinAuthority)
	}
}

// ---------------------------------------------------------------------------
// Five-section structure tests
// ---------------------------------------------------------------------------

func TestEnvelope_FiveSectionStructure(t *testing.T) {
	raw := json.RawMessage(`{"surface_id":"surf-1","agent_id":"agent-1","confidence":0.9}`)
	env, err := envelope.New("env-1", "test-source", "req-1", raw, testTime)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Section 1: Identity
	if env.Identity.ID != "env-1" {
		t.Errorf("Identity.ID: got %q, want %q", env.Identity.ID, "env-1")
	}
	if env.Identity.RequestSource != "test-source" {
		t.Errorf("Identity.RequestSource: got %q, want %q", env.Identity.RequestSource, "test-source")
	}
	if env.Identity.RequestID != "req-1" {
		t.Errorf("Identity.RequestID: got %q, want %q", env.Identity.RequestID, "req-1")
	}
	if env.Identity.SchemaVersion != envelope.SchemaVersion {
		t.Errorf("Identity.SchemaVersion: got %d, want %d", env.Identity.SchemaVersion, envelope.SchemaVersion)
	}

	// Section 2: Submitted
	if string(env.Submitted.Raw) != string(raw) {
		t.Errorf("Submitted.Raw: got %q, want %q", env.Submitted.Raw, raw)
	}
	if !env.Submitted.ReceivedAt.Equal(testTime) {
		t.Errorf("Submitted.ReceivedAt: got %v, want %v", env.Submitted.ReceivedAt, testTime)
	}

	// Section 3: Resolved (initially empty, populated during evaluation)
	env.Resolved.Authority = envelope.ResolvedAuthority{
		SurfaceID:      "surf-1",
		SurfaceVersion: 1,
		ProfileID:      "prof-1",
		ProfileVersion: 1,
		AgentID:        "agent-1",
		GrantID:        "grant-1",
	}
	if env.Resolved.Authority.SurfaceID != "surf-1" {
		t.Errorf("Resolved.Authority.SurfaceID: got %q, want %q", env.Resolved.Authority.SurfaceID, "surf-1")
	}

	// Section 4: Evaluation (populated during evaluation)
	env.Evaluation.Outcome = eval.OutcomeAccept
	env.Evaluation.ReasonCode = eval.ReasonWithinAuthority
	env.Evaluation.Explanation = &envelope.DecisionExplanation{
		Result: string(eval.OutcomeAccept),
		Reason: string(eval.ReasonWithinAuthority),
	}
	if env.Evaluation.Outcome != eval.OutcomeAccept {
		t.Errorf("Evaluation.Outcome: got %q, want %q", env.Evaluation.Outcome, eval.OutcomeAccept)
	}

	// Section 5: Integrity (populated during audit event appends)
	env.Integrity.AuditEventIDs = []string{"evt-1", "evt-2", "evt-3"}
	env.Integrity.FirstEventHash = "hash-first"
	env.Integrity.FinalEventHash = "hash-final"
	if len(env.Integrity.AuditEventIDs) != 3 {
		t.Errorf("Integrity.AuditEventIDs: got %d events, want 3", len(env.Integrity.AuditEventIDs))
	}
	if env.Integrity.FinalEventHash != "hash-final" {
		t.Errorf("Integrity.FinalEventHash: got %q, want %q", env.Integrity.FinalEventHash, "hash-final")
	}
}

// ---------------------------------------------------------------------------
// Review metadata tests
// ---------------------------------------------------------------------------

func TestEnvelope_ReviewMetadata(t *testing.T) {
	env := mustEnvelope(t, "env-1", "req-1")

	// Initially no review
	if env.Review != nil {
		t.Errorf("Review should be nil initially, got %+v", env.Review)
	}

	// Set review (as would happen during ResolveEscalation)
	reviewTime := testTime.Add(10 * time.Second)
	env.Review = &envelope.EscalationReview{
		Decision:     envelope.ReviewDecisionApproved,
		ReviewerID:   "reviewer-jane",
		ReviewerKind: "human",
		Notes:        "Approved after manual review",
		ReviewedAt:   reviewTime,
	}

	if env.Review == nil {
		t.Fatal("Review should be set")
	}
	if env.Review.Decision != envelope.ReviewDecisionApproved {
		t.Errorf("Review.Decision: got %q, want %q", env.Review.Decision, envelope.ReviewDecisionApproved)
	}
	if env.Review.ReviewerID != "reviewer-jane" {
		t.Errorf("Review.ReviewerID: got %q, want %q", env.Review.ReviewerID, "reviewer-jane")
	}
	if env.Review.ReviewerKind != "human" {
		t.Errorf("Review.ReviewerKind: got %q, want %q", env.Review.ReviewerKind, "human")
	}
	if env.Review.Notes != "Approved after manual review" {
		t.Errorf("Review.Notes: got %q, want %q", env.Review.Notes, "Approved after manual review")
	}
	if !env.Review.ReviewedAt.Equal(reviewTime) {
		t.Errorf("Review.ReviewedAt: got %v, want %v", env.Review.ReviewedAt, reviewTime)
	}
}

func TestEnvelope_ReviewDecisions(t *testing.T) {
	tests := []struct {
		name     string
		decision envelope.ReviewDecision
	}{
		{"Approved", envelope.ReviewDecisionApproved},
		{"Rejected", envelope.ReviewDecisionRejected},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := mustEnvelope(t, "env-1", "req-1")
			env.Review = &envelope.EscalationReview{
				Decision:   tt.decision,
				ReviewerID: "reviewer-1",
				ReviewedAt: testTime,
			}

			if env.Review.Decision != tt.decision {
				t.Errorf("Decision: got %q, want %q", env.Review.Decision, tt.decision)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integrity tracking tests
// ---------------------------------------------------------------------------

func TestEnvelope_IntegrityTracking(t *testing.T) {
	env := mustEnvelope(t, "env-1", "req-1")

	// Initially empty
	if len(env.Integrity.AuditEventIDs) != 0 {
		t.Errorf("AuditEventIDs should be empty initially, got %d", len(env.Integrity.AuditEventIDs))
	}

	// Simulate audit events being appended
	env.Integrity.AuditEventIDs = []string{"evt-1", "evt-2", "evt-3", "evt-4"}
	env.Integrity.FirstEventHash = "hash-1"
	env.Integrity.FinalEventHash = "hash-4"
	env.Integrity.SubmittedHash = "raw-hash"

	if len(env.Integrity.AuditEventIDs) != 4 {
		t.Errorf("AuditEventIDs: got %d, want 4", len(env.Integrity.AuditEventIDs))
	}
	if env.Integrity.FirstEventHash != "hash-1" {
		t.Errorf("FirstEventHash: got %q, want %q", env.Integrity.FirstEventHash, "hash-1")
	}
	if env.Integrity.FinalEventHash != "hash-4" {
		t.Errorf("FinalEventHash: got %q, want %q", env.Integrity.FinalEventHash, "hash-4")
	}
	if env.Integrity.SubmittedHash != "raw-hash" {
		t.Errorf("SubmittedHash: got %q, want %q", env.Integrity.SubmittedHash, "raw-hash")
	}
}

// ---------------------------------------------------------------------------
// Decision explanation tests
// ---------------------------------------------------------------------------

func TestEnvelope_DecisionExplanation(t *testing.T) {
	env := mustEnvelope(t, "env-1", "req-1")

	explanation := &envelope.DecisionExplanation{
		ConfidenceProvided:  0.75,
		ConfidenceThreshold: 0.8,

		ConsequenceProvidedType:      "financial_amount",
		ConsequenceProvidedAmount:    5000.00,
		ConsequenceProvidedCurrency:  "USD",
		ConsequenceThresholdType:     "financial_amount",
		ConsequenceThresholdAmount:   10000.00,
		ConsequenceThresholdCurrency: "USD",

		PolicyEvaluated:      true,
		PolicyReference:      "lending/approve-limits",
		PolicyPackageVersion: "v1.2.3",

		DelegationValidated: true,
		ActionWithinScope:   true,

		OutcomeDriver: "threshold.confidence",
		Result:        "Escalate",
		Reason:        "CONFIDENCE_BELOW_THRESHOLD",
	}

	env.Evaluation.Explanation = explanation

	if env.Evaluation.Explanation.ConfidenceProvided != 0.75 {
		t.Errorf("ConfidenceProvided: got %f, want 0.75", env.Evaluation.Explanation.ConfidenceProvided)
	}
	if env.Evaluation.Explanation.ConfidenceThreshold != 0.8 {
		t.Errorf("ConfidenceThreshold: got %f, want 0.8", env.Evaluation.Explanation.ConfidenceThreshold)
	}
	if env.Evaluation.Explanation.OutcomeDriver != "threshold.confidence" {
		t.Errorf("OutcomeDriver: got %q, want %q", env.Evaluation.Explanation.OutcomeDriver, "threshold.confidence")
	}
	if env.Evaluation.Explanation.Result != "Escalate" {
		t.Errorf("Result: got %q, want %q", env.Evaluation.Explanation.Result, "Escalate")
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func mustEnvelope(t *testing.T, id, requestID string) *envelope.Envelope {
	t.Helper()

	raw := json.RawMessage(`{"action":"test"}`)
	env, err := envelope.New(id, "test-source", requestID, raw, testTime)
	if err != nil {
		t.Fatalf("mustEnvelope: %v", err)
	}
	return env
}

func mustTransition(t *testing.T, env *envelope.Envelope, next envelope.EnvelopeState) {
	t.Helper()

	if err := env.Transition(next, testTime.Add(time.Second)); err != nil {
		t.Fatalf("mustTransition to %s: %v", next, err)
	}
}
