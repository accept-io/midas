package decision_test

// orchestrator_failmode_runtime_invariance_test.go — D29b runtime
// preservation pin.
//
// Goal: prove that D29b's three-axis rule admittance does NOT change
// orchestrator outcomes. Two policies with the same (id, version) but
// different rule contents — one closed+evidence_only+escalate (the
// pre-D29b shape) and one with a soft+enforced+permit_with_evidence
// rule on resource (a maximally-different post-D29b declaration) —
// must produce identical evaluation outcomes, reason codes, terminal
// states, POLICY_EVALUATED payloads, OUTCOME_RECORDED payloads, and
// FAIL_MODE_POLICY_RESOLVED payloads for the same evaluation request.
//
// Approach: spin up two parallel orchestrator harnesses with identical
// seed data except for the FailModePolicy's rules. Capture and compare
// the relevant audit-event payloads. EnvelopeID and per-event UUIDs
// necessarily differ between runs; this test compares only the payload
// fields that are content-deterministic.

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/failmode"
)

// seedFailModePolicyWithRules seeds an active policy whose Rules are the
// slice provided by the caller. Mirrors seedFailModePolicyForResolver
// but lets the caller supply explicit three-axis rule contents.
func seedFailModePolicyWithRules(t *testing.T, r testRepos, id string, version int, rules []failmode.FailModePolicyRule) {
	t.Helper()
	now := time.Now().UTC()
	if err := r.failModePolicies.Create(context.Background(), &failmode.FailModePolicy{
		ID:             id,
		Version:        version,
		Name:           "Test " + id,
		Status:         failmode.FailModePolicyStatusActive,
		EffectiveDate:  now.Add(-time.Hour),
		BusinessOwner:  "owner@example.com",
		TechnicalOwner: "platform-team",
		Rules:          rules,
		Origin:         "manual",
		Managed:        true,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "test",
	}); err != nil {
		t.Fatalf("seed FailModePolicy %s/v%d: %v", id, version, err)
	}
}

// closedEvidenceOnlyRules returns the pre-D29b closed-only rule shape.
// EnforcementState and Outcome are intentionally omitted so the
// repository read path applies the axis-aware defaults
// (evidence_only + escalate) — exercising the legacy-shape branch.
func closedEvidenceOnlyRules() []failmode.FailModePolicyRule {
	return []failmode.FailModePolicyRule{
		{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed},
		{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed},
		{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable},
		{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: failmode.PermittedModeClosed},
		{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeClosed},
	}
}

// softEnforcedPermitWithEvidenceRules returns a maximally-different
// post-D29b rule shape: resource is soft + enforced + permit_with_evidence.
// The runtime must not consult rule contents in D29b, so this declaration
// must produce byte-equal orchestrator output to closedEvidenceOnlyRules.
func softEnforcedPermitWithEvidenceRules() []failmode.FailModePolicyRule {
	return []failmode.FailModePolicyRule{
		{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: failmode.PermittedModeSoft, EnforcementState: failmode.EnforcementStateEnforced, Outcome: failmode.OutcomePermitWithEvidence},
		{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeOpen, EnforcementState: failmode.EnforcementStateDryRun, Outcome: failmode.OutcomePermitWithEvidence},
	}
}

// extractPayloadByType returns the first audit-event payload whose
// EventType matches the supplied type, or nil if no such event exists.
func extractPayloadByType(events []*audit.AuditEvent, t audit.AuditEventType) map[string]any {
	for _, e := range events {
		if e.EventType == t {
			return e.Payload
		}
	}
	return nil
}

// runFailModeFixture seeds and runs an identical evaluation against a
// fresh repo seeded with the supplied policy rules. Returns the
// orchestrator result and the full audit-event list.
func runFailModeFixture(t *testing.T, fixtureID string, rules []failmode.FailModePolicyRule) (eval.Outcome, eval.ReasonCode, []*audit.AuditEvent) {
	t.Helper()
	r := newRepos()
	const surfaceID = "surf-d29b-invariance"
	const agentID = "agent-1"
	const profID = "prof-1"
	const grantID = "grant-1"
	const policyID = "fmp-d29b-invariance"
	const policyVersion = 1

	seedActiveSurface(t, r, surfaceID)
	seedAgent(t, r, agentID)
	seedProfile(t, r, profID, surfaceID)
	seedActiveGrant(t, r, grantID, agentID, profID)
	seedFailModePolicyWithRules(t, r, policyID, policyVersion, rules)
	setSurfaceFailModePolicyID(t, r, surfaceID, policyID)

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest(surfaceID, agentID), rawPayload(t))
	if err != nil {
		t.Fatalf("[%s] Evaluate: %v", fixtureID, err)
	}

	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("[%s] ListByEnvelopeID: %v", fixtureID, err)
	}
	return res.Outcome, res.ReasonCode, events
}

// stripNonContentFields removes fields from an audit-event payload that
// are necessarily different between two runs (UUIDs, timestamps).
// Returns a new map so the originals are not mutated.
func stripNonContentFields(p map[string]any) map[string]any {
	out := make(map[string]any, len(p))
	for k, v := range p {
		switch k {
		// Per-run-identifying fields — exclude from byte-equality.
		case "envelope_id", "audit_event_id", "request_id",
			"resolved_at", "evaluation_time", "started_at", "ended_at",
			"created_at", "updated_at", "timestamp":
			continue
		}
		out[k] = v
	}
	return out
}

// TestRuntimeInvariance_D29bAxisDeclarationDoesNotAlterOutcome is the
// brief-required strongest-runtime-boundary test. Two evaluations with
// identical input data and identical FailModePolicy identity but with
// maximally-different rule declarations must produce byte-equal
// orchestrator output across:
//
//   - eval.Outcome
//   - eval.ReasonCode
//   - terminal envelope state (via OUTCOME_RECORDED to-state)
//   - POLICY_EVALUATED payload (content-stable fields)
//   - OUTCOME_RECORDED payload (content-stable fields)
//   - FAIL_MODE_POLICY_RESOLVED payload (content-stable fields)
//   - no new audit-event kinds appear (no TRIGGER_FIRED /
//     DRY_RUN_DECISION / ENFORCED)
//
// The two fixtures use the same (policy_id, version) but different
// Rules. The orchestrator must not consult Rules in D29b, so the two
// runs must be indistinguishable at the runtime boundary.
func TestRuntimeInvariance_D29bAxisDeclarationDoesNotAlterOutcome(t *testing.T) {
	baselineOutcome, baselineReason, baselineEvents := runFailModeFixture(t,
		"closed+evidence_only+escalate", closedEvidenceOnlyRules())
	threeAxisOutcome, threeAxisReason, threeAxisEvents := runFailModeFixture(t,
		"soft+enforced+permit_with_evidence", softEnforcedPermitWithEvidenceRules())

	if baselineOutcome != threeAxisOutcome {
		t.Errorf("Outcome diverged: baseline=%q three_axis=%q", baselineOutcome, threeAxisOutcome)
	}
	if baselineReason != threeAxisReason {
		t.Errorf("ReasonCode diverged: baseline=%q three_axis=%q", baselineReason, threeAxisReason)
	}

	// Compare POLICY_EVALUATED payloads byte-for-byte after stripping
	// per-run-identifying fields.
	basePE := stripNonContentFields(extractPayloadByType(baselineEvents, audit.AuditEventPolicyEvaluated))
	axisPE := stripNonContentFields(extractPayloadByType(threeAxisEvents, audit.AuditEventPolicyEvaluated))
	if !reflect.DeepEqual(basePE, axisPE) {
		t.Errorf("POLICY_EVALUATED payload diverged:\nbaseline=%v\nthree_axis=%v", basePE, axisPE)
	}

	// Compare OUTCOME_RECORDED payloads.
	baseOR := stripNonContentFields(extractPayloadByType(baselineEvents, audit.AuditEventOutcomeRecorded))
	axisOR := stripNonContentFields(extractPayloadByType(threeAxisEvents, audit.AuditEventOutcomeRecorded))
	if !reflect.DeepEqual(baseOR, axisOR) {
		t.Errorf("OUTCOME_RECORDED payload diverged:\nbaseline=%v\nthree_axis=%v", baseOR, axisOR)
	}

	// Compare FAIL_MODE_POLICY_RESOLVED payloads. The forbidden-key
	// pin in orchestrator_failmode_audit_test.go already guarantees
	// rules / permitted_mode / outcome are absent from this payload;
	// here we additionally pin that the per-content fields are
	// identical between the two rule shapes.
	baseFMR := stripNonContentFields(extractPayloadByType(baselineEvents, audit.AuditEventFailModePolicyResolved))
	axisFMR := stripNonContentFields(extractPayloadByType(threeAxisEvents, audit.AuditEventFailModePolicyResolved))
	if baseFMR == nil || axisFMR == nil {
		t.Fatalf("FAIL_MODE_POLICY_RESOLVED missing on one side: baseline=%v three_axis=%v", baseFMR, axisFMR)
	}
	if !reflect.DeepEqual(baseFMR, axisFMR) {
		t.Errorf("FAIL_MODE_POLICY_RESOLVED payload diverged:\nbaseline=%v\nthree_axis=%v", baseFMR, axisFMR)
	}

	// Pin that no new audit-event kinds were introduced. D29b must
	// not add FAIL_MODE_POLICY_TRIGGER_FIRED / _DRY_RUN_DECISION /
	// _ENFORCED. Compare the set of event kinds between the two runs;
	// any divergence indicates a runtime branch the operator did not
	// authorise.
	baseKinds := eventKindSet(baselineEvents)
	axisKinds := eventKindSet(threeAxisEvents)
	if !reflect.DeepEqual(baseKinds, axisKinds) {
		t.Errorf("audit event kind set diverged:\nbaseline=%v\nthree_axis=%v", baseKinds, axisKinds)
	}
	// Explicit absence pins for the forbidden new event kinds.
	for _, forbidden := range []audit.AuditEventType{
		"FAIL_MODE_POLICY_TRIGGER_FIRED",
		"FAIL_MODE_POLICY_DRY_RUN_DECISION",
		"FAIL_MODE_POLICY_ENFORCED",
	} {
		if _, present := axisKinds[forbidden]; present {
			t.Errorf("D29b must not emit %q; found in three-axis run", forbidden)
		}
		if _, present := baseKinds[forbidden]; present {
			t.Errorf("D29b must not emit %q; found in baseline run", forbidden)
		}
	}
}

func eventKindSet(events []*audit.AuditEvent) map[audit.AuditEventType]int {
	m := make(map[audit.AuditEventType]int, len(events))
	for _, e := range events {
		m[e.EventType]++
	}
	return m
}
