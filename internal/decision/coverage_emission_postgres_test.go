package decision_test

// coverage_emission_postgres_test.go — Postgres-backed parity test for #54.
//
// Reuses the existing Postgres orchestrator harness
// (openPostgresTestDB, cleanupPostgresTestData, mustPostgresStore,
// mustRepositories, seedPostgresHappyPathData, basePostgresRequest)
// from orchestrator_postgres_test.go and the canonical
// decisionTestProcessID seeded by seedSurfaceParents.
//
// One end-to-end test only: emit a GOVERNANCE_CONDITION_DETECTED audit
// event through the Postgres-backed audit_events table, prove its
// payload round-trips JSONB, and prove the matcher's repository query
// (governance_expectations + idx_governance_expectations_scope) wires
// cleanly into the orchestrator. Memory-backed coverage in
// coverage_emission_test.go provides the broader matrix.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/governancecoverage"
	"github.com/accept-io/midas/internal/governanceexpectation"
	"github.com/accept-io/midas/internal/policy"
)

func TestCoverageEmission_Postgres_PersistsGovernanceConditionDetectedEvent(t *testing.T) {
	db := openPostgresTestDB(t)
	defer db.Close()

	cleanupPostgresTestData(t, db)
	// governance_expectations is not in the shared cleanup list. Use a
	// per-test DELETE to avoid drift between tests; the shared cleanup
	// helper stays untouched.
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM governance_expectations`)
	})

	pgStore := mustPostgresStore(t, db)
	repos := mustRepositories(t, pgStore)
	seedPostgresHappyPathData(t, repos)

	now := time.Now().UTC().Truncate(time.Millisecond)
	expectation := &governanceexpectation.GovernanceExpectation{
		ID:                "ge-pg-001",
		Version:           2,
		ScopeKind:         governanceexpectation.ScopeKindProcess,
		ScopeID:           decisionTestProcessID, // matches seeded surface's ProcessID
		RequiredSurfaceID: "surf-1",              // matches seedPostgresHappyPathData's surface
		Name:              "Postgres coverage emission expectation",
		Status:            governanceexpectation.ExpectationStatusActive,
		EffectiveDate:     now.Add(-time.Hour),
		ConditionType:     governanceexpectation.ConditionTypeRiskCondition,
		ConditionPayload:  json.RawMessage(`{"min_confidence": 0.5}`),
		BusinessOwner:     "biz",
		TechnicalOwner:    "tech",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := repos.GovernanceExpectations.Create(context.Background(), expectation); err != nil {
		t.Fatalf("seed GovernanceExpectation: %v", err)
	}

	orch, err := decision.NewOrchestrator(pgStore, policy.NoOpPolicyEvaluator{}, nil)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	orch = orch.WithCoverageService(governancecoverage.NewService(repos.GovernanceExpectations))

	req := basePostgresRequest()
	raw := []byte(`{"request_source":"test-source","request_id":"req-postgres-1","surface_id":"surf-1","agent_id":"agent-1"}`)
	result, err := orch.Evaluate(context.Background(), req, raw)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.EnvelopeID == "" {
		t.Fatal("expected non-empty envelope id")
	}

	events, err := repos.Audit.ListByEnvelopeID(context.Background(), result.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}

	var coverageEvents []*audit.AuditEvent
	for _, ev := range events {
		if ev.EventType == audit.AuditEventGovernanceConditionDetected {
			coverageEvents = append(coverageEvents, ev)
		}
	}
	if len(coverageEvents) != 1 {
		t.Fatalf("want 1 GOVERNANCE_CONDITION_DETECTED event in audit_events, got %d", len(coverageEvents))
	}
	ev := coverageEvents[0]

	// Payload round-trip through JSONB. Numeric fields decode back as
	// float64 because that is the JSON unmarshal default — assert with
	// that in mind.
	want := map[string]any{
		"expectation_id":      "ge-pg-001",
		"expectation_version": float64(2),
		"process_id":          decisionTestProcessID,
		"required_surface_id": "surf-1",
		"condition_type":      "risk_condition",
	}
	for k, v := range want {
		if ev.Payload[k] != v {
			t.Errorf("payload[%q]: got %v (%T), want %v (%T)", k, ev.Payload[k], ev.Payload[k], v, v)
		}
	}

	summary, ok := ev.Payload["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary: want map, got %T", ev.Payload["summary"])
	}
	if summary["confidence"] != float64(0.9) {
		t.Errorf("summary.confidence: got %v, want 0.9", summary["confidence"])
	}
	cons, ok := summary["consequence"].(map[string]any)
	if !ok {
		t.Fatalf("summary.consequence: want map, got %T", summary["consequence"])
	}
	if cons["risk_rating"] != "medium" {
		t.Errorf("summary.consequence.risk_rating: got %v, want medium", cons["risk_rating"])
	}

	// Hash chain still validates with the new event embedded.
	for i := 1; i < len(events); i++ {
		if events[i].PrevHash != events[i-1].EventHash {
			t.Errorf("hash chain broken at sequence %d", events[i].SequenceNo)
		}
	}
}

// TestCoverageGap_Postgres_PersistsGovernanceCoverageGapEvent is the
// Postgres-backed parity test for #55. It mirrors the structure of its
// detected-event sibling above but seeds an active GovernanceExpectation
// whose RequiredSurfaceID differs from the request's actual SurfaceID,
// then asserts both events appear in the live audit_events table with
// the JSONB payloads the helpers produce.
//
// One end-to-end test only: prove the gap event persists with the
// expected payload through the same audit_events / JSONB path as #54.
// The memory-backed tests above already cover the broader test matrix.
func TestCoverageGap_Postgres_PersistsGovernanceCoverageGapEvent(t *testing.T) {
	db := openPostgresTestDB(t)
	defer db.Close()

	cleanupPostgresTestData(t, db)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM governance_expectations`)
	})

	pgStore := mustPostgresStore(t, db)
	repos := mustRepositories(t, pgStore)
	seedPostgresHappyPathData(t, repos)

	now := time.Now().UTC().Truncate(time.Millisecond)
	expectation := &governanceexpectation.GovernanceExpectation{
		ID:        "ge-pg-gap-001",
		Version:   4,
		ScopeKind: governanceexpectation.ScopeKindProcess,
		ScopeID:   decisionTestProcessID,
		// Required surface differs from the request's "surf-1" — this
		// is the load-bearing condition for the gap event to fire.
		RequiredSurfaceID: "surf-required-but-not-invoked",
		Name:              "Postgres coverage gap expectation",
		Status:            governanceexpectation.ExpectationStatusActive,
		EffectiveDate:     now.Add(-time.Hour),
		ConditionType:     governanceexpectation.ConditionTypeRiskCondition,
		ConditionPayload:  json.RawMessage(`{"min_confidence": 0.5}`),
		BusinessOwner:     "biz",
		TechnicalOwner:    "tech",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := repos.GovernanceExpectations.Create(context.Background(), expectation); err != nil {
		t.Fatalf("seed GovernanceExpectation: %v", err)
	}

	orch, err := decision.NewOrchestrator(pgStore, policy.NoOpPolicyEvaluator{}, nil)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	orch = orch.WithCoverageService(governancecoverage.NewService(repos.GovernanceExpectations))

	req := basePostgresRequest()
	raw := []byte(`{"request_source":"test-source","request_id":"req-postgres-1","surface_id":"surf-1","agent_id":"agent-1"}`)
	result, err := orch.Evaluate(context.Background(), req, raw)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	events, err := repos.Audit.ListByEnvelopeID(context.Background(), result.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}

	var gaps []*audit.AuditEvent
	for _, ev := range events {
		if ev.EventType == audit.AuditEventGovernanceCoverageGap {
			gaps = append(gaps, ev)
		}
	}
	if len(gaps) != 1 {
		t.Fatalf("want 1 GOVERNANCE_COVERAGE_GAP event in audit_events, got %d", len(gaps))
	}
	ev := gaps[0]

	want := map[string]any{
		"expectation_id":      "ge-pg-gap-001",
		"expectation_version": float64(4),
		"missing_surface_id":  "surf-required-but-not-invoked",
		"actual_surface_id":   "surf-1",
		"process_id":          decisionTestProcessID,
		"condition_type":      "risk_condition",
	}
	for k, v := range want {
		if ev.Payload[k] != v {
			t.Errorf("payload[%q]: got %v (%T), want %v (%T)", k, ev.Payload[k], ev.Payload[k], v, v)
		}
	}

	cb, ok := ev.Payload["correlation_basis"].(map[string]any)
	if !ok {
		t.Fatalf("correlation_basis: want map, got %T", ev.Payload["correlation_basis"])
	}
	if cb["type"] != "same_evaluation" {
		t.Errorf("correlation_basis.type: got %v, want same_evaluation", cb["type"])
	}
	if cb["envelope_id"] != result.EnvelopeID {
		t.Errorf("correlation_basis.envelope_id: got %v, want %v", cb["envelope_id"], result.EnvelopeID)
	}
	if cb["request_source"] != "test-source" {
		t.Errorf("correlation_basis.request_source: got %v", cb["request_source"])
	}
	if cb["request_id"] != "req-postgres-1" {
		t.Errorf("correlation_basis.request_id: got %v", cb["request_id"])
	}
	if _, present := cb["elapsed_ms"]; present {
		t.Errorf("correlation_basis.elapsed_ms must be omitted in same_evaluation MVP; got %v", cb["elapsed_ms"])
	}

	// Hash chain validates with detected + gap events present.
	for i := 1; i < len(events); i++ {
		if events[i].PrevHash != events[i-1].EventHash {
			t.Errorf("hash chain broken at sequence %d", events[i].SequenceNo)
		}
	}
}
