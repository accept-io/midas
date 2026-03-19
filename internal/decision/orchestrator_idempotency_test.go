package decision_test

// Idempotency and replay semantics for scoped evaluation.
//
// Test matrix:
//   1. First submission creates a new envelope.
//   2. Exact replay (same source+id+payload) returns the existing result
//      without creating a second envelope.
//   3. Same (request_source, request_id) with a different payload returns
//      ErrScopedRequestConflict.
//   4. Same request_id under a different request_source is treated as an
//      independent request and creates a second envelope.
//   5. Retrieval by scoped request identity (GetEnvelopeByRequestScope)
//      returns the correct envelope.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/eval"
)

// ---------------------------------------------------------------------------
// Helpers shared across tests in this file
// ---------------------------------------------------------------------------

// buildPayload returns a JSON-encoded DecisionRequest suitable as the raw
// body argument to Evaluate. Fields are predictable so the hash is stable.
func buildPayload(t *testing.T, req eval.DecisionRequest) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return b
}

// seededStore returns a fakeStore pre-populated with a surface, agent, grant,
// and profile so that a full evaluation can succeed.
func seededStore() *fakeStore {
	st := newFakeStore()
	seedStore(st)
	return st
}

// evaluate1 runs an evaluation and fails the test if it returns an error.
func evaluate1(t *testing.T, o *decision.Orchestrator, req eval.DecisionRequest, raw json.RawMessage) decision.EvaluationResult {
	t.Helper()
	result, err := o.Evaluate(context.Background(), req, raw)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	return result
}

// ---------------------------------------------------------------------------
// Test 1: first submission creates a new envelope
// ---------------------------------------------------------------------------

func TestIdempotency_FirstSubmissionCreatesEnvelope(t *testing.T) {
	st := seededStore()
	o := buildOrchestrator(t, st, &allowAllPolicies{})

	req := eval.DecisionRequest{
		RequestSource: "svc-a",
		RequestID:     "req-001",
		SurfaceID:     testSurfaceID,
		AgentID:       testAgentID,
		Confidence:    0.95,
	}
	raw := buildPayload(t, req)

	result := evaluate1(t, o, req, raw)

	if result.EnvelopeID == "" {
		t.Fatal("expected a non-empty EnvelopeID on first submission")
	}
	if result.Outcome == "" {
		t.Fatal("expected a non-empty Outcome on first submission")
	}

	// Confirm exactly one envelope exists for the scope.
	env, err := o.GetEnvelopeByRequestScope(context.Background(), "svc-a", "req-001")
	if err != nil {
		t.Fatalf("GetEnvelopeByRequestScope: %v", err)
	}
	if env == nil {
		t.Fatal("expected envelope to be persisted, got nil")
	}
	if env.ID() != result.EnvelopeID {
		t.Errorf("envelope ID mismatch: got %q, want %q", env.ID(), result.EnvelopeID)
	}
}

// ---------------------------------------------------------------------------
// Test 2: exact replay returns the existing result without a second envelope
// ---------------------------------------------------------------------------

func TestIdempotency_ExactReplayReturnsSameResult(t *testing.T) {
	st := seededStore()
	o := buildOrchestrator(t, st, &allowAllPolicies{})

	req := eval.DecisionRequest{
		RequestSource: "svc-a",
		RequestID:     "req-002",
		SurfaceID:     testSurfaceID,
		AgentID:       testAgentID,
		Confidence:    0.95,
	}
	raw := buildPayload(t, req)

	first := evaluate1(t, o, req, raw)

	// Submit the identical request again.
	second, err := o.Evaluate(context.Background(), req, raw)
	if err != nil {
		t.Fatalf("second Evaluate: %v", err)
	}

	if second.EnvelopeID != first.EnvelopeID {
		t.Errorf("replay must return the same EnvelopeID: got %q, want %q",
			second.EnvelopeID, first.EnvelopeID)
	}
	if second.Outcome != first.Outcome {
		t.Errorf("replay must return the same Outcome: got %q, want %q",
			second.Outcome, first.Outcome)
	}
	if second.ReasonCode != first.ReasonCode {
		t.Errorf("replay must return the same ReasonCode: got %q, want %q",
			second.ReasonCode, first.ReasonCode)
	}

	// Confirm only one envelope was created (no duplicates).
	envs, err := o.ListEnvelopes(context.Background())
	if err != nil {
		t.Fatalf("ListEnvelopes: %v", err)
	}
	count := 0
	for _, e := range envs {
		if e.RequestSource() == "svc-a" && e.RequestID() == "req-002" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 envelope for scope svc-a/req-002, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Test 3: same scope, different payload → ErrScopedRequestConflict
// ---------------------------------------------------------------------------

func TestIdempotency_DifferentPayloadReturnsScopedConflictError(t *testing.T) {
	st := seededStore()
	o := buildOrchestrator(t, st, &allowAllPolicies{})

	req1 := eval.DecisionRequest{
		RequestSource: "svc-a",
		RequestID:     "req-003",
		SurfaceID:     testSurfaceID,
		AgentID:       testAgentID,
		Confidence:    0.95,
	}
	raw1 := buildPayload(t, req1)

	// First submission succeeds.
	evaluate1(t, o, req1, raw1)

	// Second submission: same scope but different confidence → different hash.
	req2 := eval.DecisionRequest{
		RequestSource: "svc-a",
		RequestID:     "req-003",
		SurfaceID:     testSurfaceID,
		AgentID:       testAgentID,
		Confidence:    0.50, // mutated
	}
	raw2 := buildPayload(t, req2)

	_, err := o.Evaluate(context.Background(), req2, raw2)
	if err == nil {
		t.Fatal("expected ErrScopedRequestConflict, got nil")
	}
	if !errors.Is(err, decision.ErrScopedRequestConflict) {
		t.Errorf("expected errors.Is(err, ErrScopedRequestConflict), got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test 4: same request_id under a different request_source is independent
// ---------------------------------------------------------------------------

func TestIdempotency_SameRequestIDDifferentSourceIsIndependent(t *testing.T) {
	st := seededStore()
	o := buildOrchestrator(t, st, &allowAllPolicies{})

	sharedRequestID := "req-004"

	reqA := eval.DecisionRequest{
		RequestSource: "svc-a",
		RequestID:     sharedRequestID,
		SurfaceID:     testSurfaceID,
		AgentID:       testAgentID,
		Confidence:    0.95,
	}
	rawA := buildPayload(t, reqA)

	reqB := eval.DecisionRequest{
		RequestSource: "svc-b", // different source
		RequestID:     sharedRequestID,
		SurfaceID:     testSurfaceID,
		AgentID:       testAgentID,
		Confidence:    0.95,
	}
	rawB := buildPayload(t, reqB)

	resultA := evaluate1(t, o, reqA, rawA)
	resultB := evaluate1(t, o, reqB, rawB)

	// Both should succeed and produce distinct envelopes.
	if resultA.EnvelopeID == "" {
		t.Fatal("svc-a result has empty EnvelopeID")
	}
	if resultB.EnvelopeID == "" {
		t.Fatal("svc-b result has empty EnvelopeID")
	}
	if resultA.EnvelopeID == resultB.EnvelopeID {
		t.Errorf("different sources must produce distinct envelopes, but both returned %q",
			resultA.EnvelopeID)
	}

	// Each scope should resolve to its own envelope.
	envA, err := o.GetEnvelopeByRequestScope(context.Background(), "svc-a", sharedRequestID)
	if err != nil {
		t.Fatalf("GetEnvelopeByRequestScope(svc-a): %v", err)
	}
	if envA == nil || envA.ID() != resultA.EnvelopeID {
		t.Errorf("svc-a scope resolved wrong envelope: got %v, want %q", envA, resultA.EnvelopeID)
	}

	envB, err := o.GetEnvelopeByRequestScope(context.Background(), "svc-b", sharedRequestID)
	if err != nil {
		t.Fatalf("GetEnvelopeByRequestScope(svc-b): %v", err)
	}
	if envB == nil || envB.ID() != resultB.EnvelopeID {
		t.Errorf("svc-b scope resolved wrong envelope: got %v, want %q", envB, resultB.EnvelopeID)
	}
}

// ---------------------------------------------------------------------------
// Test 5: GetEnvelopeByRequestScope retrieval works correctly
// ---------------------------------------------------------------------------

func TestIdempotency_GetEnvelopeByRequestScopeReturnsCorrectEnvelope(t *testing.T) {
	st := seededStore()
	o := buildOrchestrator(t, st, &allowAllPolicies{})

	req := eval.DecisionRequest{
		RequestSource: "svc-retrieval",
		RequestID:     "req-005",
		SurfaceID:     testSurfaceID,
		AgentID:       testAgentID,
		Confidence:    0.95,
	}
	raw := buildPayload(t, req)

	result := evaluate1(t, o, req, raw)

	// Lookup by scope.
	env, err := o.GetEnvelopeByRequestScope(context.Background(), "svc-retrieval", "req-005")
	if err != nil {
		t.Fatalf("GetEnvelopeByRequestScope: %v", err)
	}
	if env == nil {
		t.Fatal("expected envelope, got nil")
	}
	if env.ID() != result.EnvelopeID {
		t.Errorf("ID mismatch: got %q, want %q", env.ID(), result.EnvelopeID)
	}
	if env.RequestSource() != "svc-retrieval" {
		t.Errorf("RequestSource: got %q, want %q", env.RequestSource(), "svc-retrieval")
	}
	if env.RequestID() != "req-005" {
		t.Errorf("RequestID: got %q, want %q", env.RequestID(), "req-005")
	}

	// A non-existent scope returns nil, not an error.
	missing, err := o.GetEnvelopeByRequestScope(context.Background(), "svc-retrieval", "no-such-id")
	if err != nil {
		t.Fatalf("GetEnvelopeByRequestScope for missing: %v", err)
	}
	if missing != nil {
		t.Errorf("expected nil for missing scope, got envelope %q", missing.ID())
	}
}

// ---------------------------------------------------------------------------
// Test 6: conflict error is not returned on replay (regression guard)
// ---------------------------------------------------------------------------

// Replay must never return an error — it must silently return the original.
func TestIdempotency_ReplayNeverReturnsError(t *testing.T) {
	st := seededStore()
	o := buildOrchestrator(t, st, &allowAllPolicies{})

	req := eval.DecisionRequest{
		RequestSource: "svc-replay",
		RequestID:     "req-006",
		SurfaceID:     testSurfaceID,
		AgentID:       testAgentID,
		Confidence:    0.95,
	}
	raw := buildPayload(t, req)

	evaluate1(t, o, req, raw)

	// Replay five times — must never error.
	for i := range 5 {
		_, err := o.Evaluate(context.Background(), req, raw)
		if err != nil {
			t.Errorf("replay %d returned unexpected error: %v", i+1, err)
		}
	}
}
