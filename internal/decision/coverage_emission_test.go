package decision_test

// coverage_emission_test.go — focused tests for #54
// (`GOVERNANCE_CONDITION_DETECTED` runtime audit emission). The wiring
// under test is decision.Orchestrator.WithCoverageService; the matcher
// itself is exercised in internal/governancecoverage/.
//
// Test bias: drive the wiring through the orchestrator's public
// Evaluate path and assert against the audit chain via the existing
// fakeStore + auditEventsFor helpers. Coverage events are observational;
// the orchestrator must emit them without altering the evaluation
// outcome or reason code.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/governancecoverage"
	"github.com/accept-io/midas/internal/governanceexpectation"
	"github.com/accept-io/midas/internal/store/memory"
	"github.com/accept-io/midas/internal/value"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// activeExpectation returns a fully-populated GovernanceExpectation in
// active state, scoped to testProcessID, requiring testSurfaceID, with
// the supplied condition payload bytes. Tests use direct repo seeding
// because no approval endpoint exists yet (#54 close-out).
func activeExpectation(id string, version int, payload string) *governanceexpectation.GovernanceExpectation {
	return &governanceexpectation.GovernanceExpectation{
		ID:                id,
		Version:           version,
		ScopeKind:         governanceexpectation.ScopeKindProcess,
		ScopeID:           testProcessID,
		RequiredSurfaceID: testSurfaceID,
		Name:              id,
		Status:            governanceexpectation.ExpectationStatusActive,
		EffectiveDate:     testNow.Add(-time.Hour),
		ConditionType:     governanceexpectation.ConditionTypeRiskCondition,
		ConditionPayload:  json.RawMessage(payload),
		BusinessOwner:     "biz",
		TechnicalOwner:    "tech",
	}
}

// orchestratorWithExpectations builds an orchestrator wired with a
// fresh memory governance-expectation repo, seeded with the supplied
// expectations, then wraps it in a coverage service via
// WithCoverageService. The store is the canonical seededStore() so the
// rest of the evaluation chain resolves cleanly.
func orchestratorWithExpectations(
	t *testing.T,
	expectations ...*governanceexpectation.GovernanceExpectation,
) (*decision.Orchestrator, *fakeStore, *memory.GovernanceExpectationRepo) {
	t.Helper()
	st := seededStore()
	repo := memory.NewGovernanceExpectationRepo()
	ctx := context.Background()
	for _, e := range expectations {
		if err := repo.Create(ctx, e); err != nil {
			t.Fatalf("seed expectation %s: %v", e.ID, err)
		}
	}
	o := buildOrchestrator(t, st, &allowAllPolicies{}).
		WithCoverageService(governancecoverage.NewService(repo))
	return o, st, repo
}

// coverageRequest returns a request fixture that resolves the canonical
// surface/agent and carries a typed Consequence the matcher can evaluate.
func coverageRequest(requestID string) eval.DecisionRequest {
	return eval.DecisionRequest{
		RequestSource: "test-source",
		RequestID:     requestID,
		SurfaceID:     testSurfaceID,
		AgentID:       testAgentID,
		Confidence:    0.9,
		Consequence: &eval.Consequence{
			Type:     value.ConsequenceTypeMonetary,
			Amount:   7500,
			Currency: "GBP",
		},
	}
}

// coverageEventsFor extracts every GOVERNANCE_CONDITION_DETECTED event
// for the envelope, in audit-chain order.
func coverageEventsFor(t *testing.T, st *fakeStore, envelopeID string) []*audit.AuditEvent {
	t.Helper()
	all := auditEventsFor(t, st, envelopeID)
	var out []*audit.AuditEvent
	for _, ev := range all {
		if ev.EventType == audit.AuditEventGovernanceConditionDetected {
			out = append(out, ev)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// 1. Active matching expectation emits exactly one event.
// 2. Event payload contains every required field including summary.
// ---------------------------------------------------------------------------

func TestCoverageEmission_ActiveMatch_EmitsOneEventWithFullPayload(t *testing.T) {
	o, st, _ := orchestratorWithExpectations(t,
		activeExpectation("ge-active-1", 3, `{"min_confidence": 0.5, "consequence_amount_at_least": 1000, "consequence_currency": "GBP"}`),
	)

	req := coverageRequest("req-cov-001")
	result := evaluate1(t, o, req, buildPayload(t, req))

	got := coverageEventsFor(t, st, result.EnvelopeID)
	if len(got) != 1 {
		t.Fatalf("want 1 GOVERNANCE_CONDITION_DETECTED event, got %d", len(got))
	}
	ev := got[0]

	if ev.RequestSource != "test-source" {
		t.Errorf("RequestSource: got %q", ev.RequestSource)
	}
	if ev.RequestID != "req-cov-001" {
		t.Errorf("RequestID: got %q", ev.RequestID)
	}
	if ev.EnvelopeID != result.EnvelopeID {
		t.Errorf("EnvelopeID: got %q, want %q", ev.EnvelopeID, result.EnvelopeID)
	}

	wantPayload := map[string]any{
		"expectation_id":      "ge-active-1",
		"expectation_version": 3,
		"process_id":          testProcessID,
		"required_surface_id": testSurfaceID,
		"condition_type":      "risk_condition",
	}
	for k, want := range wantPayload {
		got := ev.Payload[k]
		if !equalAny(got, want) {
			t.Errorf("payload[%q]: got %v (%T), want %v (%T)", k, got, got, want, want)
		}
	}

	summary, ok := ev.Payload["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary: want map, got %T", ev.Payload["summary"])
	}
	if !equalAny(summary["confidence"], 0.9) {
		t.Errorf("summary.confidence: got %v", summary["confidence"])
	}
	cons, ok := summary["consequence"].(map[string]any)
	if !ok {
		t.Fatalf("summary.consequence: want map, got %T", summary["consequence"])
	}
	if !equalAny(cons["type"], "monetary") {
		t.Errorf("summary.consequence.type: got %v", cons["type"])
	}
	if !equalAny(cons["amount"], 7500.0) {
		t.Errorf("summary.consequence.amount: got %v", cons["amount"])
	}
	if !equalAny(cons["currency"], "GBP") {
		t.Errorf("summary.consequence.currency: got %v", cons["currency"])
	}
	if !equalAny(cons["risk_rating"], "") {
		t.Errorf("summary.consequence.risk_rating: got %v", cons["risk_rating"])
	}
}

// ---------------------------------------------------------------------------
// 3. Multiple matched expectations emit multiple events in
// lexicographic ExpectationID order.
// ---------------------------------------------------------------------------

func TestCoverageEmission_MultipleMatches_EmitInLexicographicOrder(t *testing.T) {
	o, st, _ := orchestratorWithExpectations(t,
		activeExpectation("ge-z", 1, `{}`),
		activeExpectation("ge-a", 1, `{}`),
		activeExpectation("ge-m", 1, `{}`),
	)

	req := coverageRequest("req-cov-multi")
	result := evaluate1(t, o, req, buildPayload(t, req))

	got := coverageEventsFor(t, st, result.EnvelopeID)
	if len(got) != 3 {
		t.Fatalf("want 3 events, got %d", len(got))
	}
	wantOrder := []string{"ge-a", "ge-m", "ge-z"}
	for i, ev := range got {
		id, _ := ev.Payload["expectation_id"].(string)
		if id != wantOrder[i] {
			t.Errorf("event %d expectation_id: got %q, want %q", i, id, wantOrder[i])
		}
	}

	// Sequence numbers must be contiguous within the audit chain (the
	// accumulator's flushEventsAndUpdate guarantees declaration order).
	for i := 1; i < len(got); i++ {
		if got[i].SequenceNo != got[i-1].SequenceNo+1 {
			t.Errorf("non-contiguous sequence: got[%d].SequenceNo=%d, got[%d].SequenceNo=%d",
				i-1, got[i-1].SequenceNo, i, got[i].SequenceNo)
		}
	}
}

// ---------------------------------------------------------------------------
// 4. No matching expectation under scope → no event.
// ---------------------------------------------------------------------------

func TestCoverageEmission_NoMatch_NoEvent(t *testing.T) {
	o, st, _ := orchestratorWithExpectations(t,
		activeExpectation("ge-no-match", 1, `{"min_confidence": 0.99}`),
	)

	req := coverageRequest("req-cov-nomatch")
	req.Confidence = 0.5 // below threshold
	result := evaluate1(t, o, req, buildPayload(t, req))

	if got := coverageEventsFor(t, st, result.EnvelopeID); len(got) != 0 {
		t.Fatalf("want 0 events, got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// 5. Review/deprecated/retired expectations emit no event.
// ---------------------------------------------------------------------------

func TestCoverageEmission_NonActiveStatuses_NoEvent(t *testing.T) {
	cases := []governanceexpectation.ExpectationStatus{
		governanceexpectation.ExpectationStatusReview,
		governanceexpectation.ExpectationStatusDeprecated,
		governanceexpectation.ExpectationStatusRetired,
		governanceexpectation.ExpectationStatusDraft,
	}
	for _, status := range cases {
		t.Run(string(status), func(t *testing.T) {
			e := activeExpectation("ge-nonactive", 1, `{}`)
			e.Status = status
			o, st, _ := orchestratorWithExpectations(t, e)

			req := coverageRequest("req-cov-status")
			result := evaluate1(t, o, req, buildPayload(t, req))

			if got := coverageEventsFor(t, st, result.EnvelopeID); len(got) != 0 {
				t.Errorf("status=%s: want 0 events, got %d", status, len(got))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 6. Future-dated and expired expectations emit no event.
// ---------------------------------------------------------------------------

func TestCoverageEmission_OutOfWindow_NoEvent(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*governanceexpectation.GovernanceExpectation)
	}{
		{"future_dated", func(e *governanceexpectation.GovernanceExpectation) {
			e.EffectiveDate = testNow.Add(time.Hour)
		}},
		{"expired", func(e *governanceexpectation.GovernanceExpectation) {
			past := testNow.Add(-time.Minute)
			e.EffectiveUntil = &past
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := activeExpectation("ge-window", 1, `{}`)
			tc.mutate(e)
			o, st, _ := orchestratorWithExpectations(t, e)

			req := coverageRequest("req-cov-window")
			result := evaluate1(t, o, req, buildPayload(t, req))

			if got := coverageEventsFor(t, st, result.EnvelopeID); len(got) != 0 {
				t.Errorf("%s: want 0 events, got %d", tc.name, len(got))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 7. Nil coverage service emits no event and produces no error.
// ---------------------------------------------------------------------------

func TestCoverageEmission_NilService_NoEventNoError(t *testing.T) {
	st := seededStore()
	o := buildOrchestrator(t, st, &allowAllPolicies{}) // no WithCoverageService

	req := coverageRequest("req-cov-nilsvc")
	result := evaluate1(t, o, req, buildPayload(t, req))

	if got := coverageEventsFor(t, st, result.EnvelopeID); len(got) != 0 {
		t.Errorf("want 0 events with nil coverage service, got %d", len(got))
	}
	if result.Outcome == "" {
		t.Error("evaluation must still complete with nil coverage service")
	}
}

// ---------------------------------------------------------------------------
// 8. Coverage service repository error logs/skips and does not change
// evaluation outcome.
// ---------------------------------------------------------------------------

// failingExpRepo returns the configured err from ListActiveByScope and
// no-ops every other method. Used to prove the orchestrator swallows
// repo errors without failing the evaluation.
type failingExpRepo struct{ err error }

func (failingExpRepo) Create(_ context.Context, _ *governanceexpectation.GovernanceExpectation) error {
	return nil
}

func (failingExpRepo) FindByID(_ context.Context, _ string) (*governanceexpectation.GovernanceExpectation, error) {
	return nil, nil
}

func (failingExpRepo) FindByIDAndVersion(_ context.Context, _ string, _ int) (*governanceexpectation.GovernanceExpectation, error) {
	return nil, nil
}

func (failingExpRepo) ListVersions(_ context.Context, _ string) ([]*governanceexpectation.GovernanceExpectation, error) {
	return nil, nil
}

func (failingExpRepo) Update(_ context.Context, _ *governanceexpectation.GovernanceExpectation) error {
	return nil
}

func (r failingExpRepo) ListActiveByScope(_ context.Context, _ governanceexpectation.ScopeKind, _ string, _ time.Time) ([]*governanceexpectation.GovernanceExpectation, error) {
	return nil, r.err
}

var _ governanceexpectation.Repository = failingExpRepo{}

func TestCoverageEmission_RepoError_LogsAndSkips(t *testing.T) {
	st := seededStore()
	repo := failingExpRepo{err: errors.New("simulated repo failure")}
	o := buildOrchestrator(t, st, &allowAllPolicies{}).
		WithCoverageService(governancecoverage.NewService(repo))

	req := coverageRequest("req-cov-repofail")
	result, err := o.Evaluate(context.Background(), req, buildPayload(t, req))
	if err != nil {
		t.Fatalf("Evaluate must succeed despite coverage repo failure: %v", err)
	}
	if result.Outcome == "" {
		t.Error("evaluation must complete with a real outcome")
	}
	if got := coverageEventsFor(t, st, result.EnvelopeID); len(got) != 0 {
		t.Errorf("repo error must produce no coverage events, got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// 9. Idempotent replay does not emit duplicate coverage events.
// ---------------------------------------------------------------------------

func TestCoverageEmission_IdempotentReplay_NoDuplicate(t *testing.T) {
	o, st, _ := orchestratorWithExpectations(t,
		activeExpectation("ge-replay", 1, `{}`),
	)

	req := coverageRequest("req-cov-replay")
	raw := buildPayload(t, req)
	first := evaluate1(t, o, req, raw)

	if got := coverageEventsFor(t, st, first.EnvelopeID); len(got) != 1 {
		t.Fatalf("first apply: want 1 coverage event, got %d", len(got))
	}

	// Replay the exact same payload. The orchestrator's idempotency
	// short-circuit returns the original result without re-running
	// evaluation, so no new audit events of any kind should appear.
	second, err := o.Evaluate(context.Background(), req, raw)
	if err != nil {
		t.Fatalf("replay Evaluate: %v", err)
	}
	if second.EnvelopeID != first.EnvelopeID {
		t.Errorf("replay must return the same envelope; got %q != %q", second.EnvelopeID, first.EnvelopeID)
	}
	if got := coverageEventsFor(t, st, first.EnvelopeID); len(got) != 1 {
		t.Errorf("replay must not emit new coverage events; got %d total", len(got))
	}
}

// ---------------------------------------------------------------------------
// 10. Envelope audit hash chain remains valid with coverage events present.
// ---------------------------------------------------------------------------

func TestCoverageEmission_HashChainStillValid(t *testing.T) {
	o, st, _ := orchestratorWithExpectations(t,
		activeExpectation("ge-chain-1", 1, `{}`),
		activeExpectation("ge-chain-2", 1, `{}`),
	)

	req := coverageRequest("req-cov-chain")
	result := evaluate1(t, o, req, buildPayload(t, req))

	all := auditEventsFor(t, st, result.EnvelopeID)
	if len(all) < 3 {
		t.Fatalf("want at least 3 audit events in the chain, got %d", len(all))
	}
	// First event has empty PrevHash; every subsequent event's PrevHash
	// must equal the previous event's EventHash. This is the chain
	// invariant fakeAuditRepo enforces too — so a coverage event landing
	// inside the chain without breaking it pins the load-bearing
	// integrity guarantee.
	if all[0].PrevHash != "" {
		t.Errorf("first event PrevHash: want empty, got %q", all[0].PrevHash)
	}
	for i := 1; i < len(all); i++ {
		if all[i].PrevHash != all[i-1].EventHash {
			t.Errorf("event %d PrevHash mismatch: got %q, want %q",
				i, all[i].PrevHash, all[i-1].EventHash)
		}
	}
}

// ---------------------------------------------------------------------------
// 11. Evaluation outcome / reason code is unchanged with and without
// coverage matching.
// ---------------------------------------------------------------------------

func TestCoverageEmission_OutcomeUnchanged_WithVsWithoutCoverage(t *testing.T) {
	// Without coverage service.
	stNoCoverage := seededStore()
	oNoCoverage := buildOrchestrator(t, stNoCoverage, &allowAllPolicies{})
	reqA := coverageRequest("req-cov-outcome-a")
	noCovResult := evaluate1(t, oNoCoverage, reqA, buildPayload(t, reqA))

	// With coverage service producing matches.
	oWithCoverage, _, _ := orchestratorWithExpectations(t,
		activeExpectation("ge-outcome", 1, `{}`),
	)
	reqB := coverageRequest("req-cov-outcome-b")
	covResult := evaluate1(t, oWithCoverage, reqB, buildPayload(t, reqB))

	if noCovResult.Outcome != covResult.Outcome {
		t.Errorf("Outcome differs: without=%q, with=%q", noCovResult.Outcome, covResult.Outcome)
	}
	if noCovResult.ReasonCode != covResult.ReasonCode {
		t.Errorf("ReasonCode differs: without=%q, with=%q", noCovResult.ReasonCode, covResult.ReasonCode)
	}
	if noCovResult.State != covResult.State {
		t.Errorf("State differs: without=%q, with=%q", noCovResult.State, covResult.State)
	}
}

// ---------------------------------------------------------------------------
// Bonus: nil Consequence on the request causes the summary.consequence
// key to be omitted entirely. Pins the per-prompt rule that
// `payload_json -> 'summary' ? 'consequence'` is true only when
// consequence facts were actually present at evaluation time.
// ---------------------------------------------------------------------------

func TestCoverageEmission_NilConsequence_OmitsConsequenceKey(t *testing.T) {
	o, st, _ := orchestratorWithExpectations(t,
		activeExpectation("ge-nilcons", 1, `{"min_confidence": 0.5}`),
	)

	req := coverageRequest("req-cov-nilcons")
	req.Consequence = nil // grammar's min_confidence still satisfies → match
	result := evaluate1(t, o, req, buildPayload(t, req))

	got := coverageEventsFor(t, st, result.EnvelopeID)
	if len(got) != 1 {
		t.Fatalf("want 1 event, got %d", len(got))
	}
	summary, ok := got[0].Payload["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary: want map, got %T", got[0].Payload["summary"])
	}
	if _, present := summary["consequence"]; present {
		t.Errorf("nil Consequence: summary.consequence must be omitted; got %v", summary["consequence"])
	}
	if _, present := summary["confidence"]; !present {
		t.Errorf("summary.confidence must always be present even when Consequence is nil")
	}
}

// ---------------------------------------------------------------------------
// Test-local helpers
// ---------------------------------------------------------------------------

// equalAny compares two any-typed values for testing. It treats numeric
// types pragmatically (int → float64 etc.) so payloads built from
// map[string]any with int literals compare cleanly against expectation
// values built with int or float64 literals in tests.
func equalAny(got, want any) bool {
	if got == nil && want == nil {
		return true
	}
	switch w := want.(type) {
	case int:
		if g, ok := got.(int); ok {
			return g == w
		}
		if g, ok := got.(float64); ok {
			return g == float64(w)
		}
	case float64:
		if g, ok := got.(float64); ok {
			return g == w
		}
		if g, ok := got.(int); ok {
			return float64(g) == w
		}
	case string:
		if g, ok := got.(string); ok {
			return g == w
		}
	}
	return false
}
