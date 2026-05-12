package decision_test

// orchestrator_failmode_dry_run_test.go — D29e pins.
//
// Asserts:
//
//   - FAIL_MODE_POLICY_DRY_RUN_DECISION is emitted ONLY when (a) the
//     trigger-fired conditions hold (resolved policy + evaluator
//     error), (b) the matching rule is found, AND (c) the matching
//     rule's enforcement_state == dry_run.
//   - The payload carries exactly the brief-required fields. Forbidden
//     keys are absent.
//   - configured_outcome → dry_run_outcome mapping is correct for the
//     four valid D29b combinations under dry_run.
//   - Divergence is computed on outcome equality only.
//   - authority.FailMode behaviour is preserved exactly (FailModeOpen
//     accepts, FailModeClosed escalates with POLICY_ERROR).
//   - OUTCOME_RECORDED never carries a reason_code starting with
//     "FAIL_MODE_POLICY_" — the dry-run reason codes are scoped to
//     the dry-run event payload only.
//   - Event ordering: TRIGGER_FIRED → DRY_RUN_DECISION → OUTCOME_RECORDED.
//   - Two evaluations differing only in enforcement_state (evidence_only
//     vs dry_run) produce identical Outcome / ReasonCode /
//     OUTCOME_RECORDED / POLICY_EVALUATED / FAIL_MODE_POLICY_RESOLVED /
//     FAIL_MODE_POLICY_TRIGGER_FIRED payloads — the only delta is the
//     inserted dry-run event in the dry_run case.
//   - /v1/evaluate response body never contains dry-run payload key
//     names or any FAIL_MODE_POLICY_* reason-code constants.

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/failmode"
	"github.com/accept-io/midas/internal/policy"
)

// ---------------------------------------------------------------------------
// Test fixtures — rule builders for each valid D29b dry_run combination
// ---------------------------------------------------------------------------

// dryRunResourceRules returns five rules covering all five correctness
// classes. The resource rule carries the supplied (posture, outcome)
// combination under EnforcementStateDryRun; the other four are
// closed+evidence_only+escalate (input is not_applicable). The resource
// rule is the one the policy_evaluator_error trigger selects (per
// D29c-2 mapping); the others are present only to satisfy the
// exhaustive-class invariant when the validator is bypassed via direct
// repo Create.
func dryRunResourceRules(posture failmode.PermittedMode, outcome failmode.Outcome) []failmode.FailModePolicyRule {
	return []failmode.FailModePolicyRule{
		{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: posture, EnforcementState: failmode.EnforcementStateDryRun, Outcome: outcome},
		{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
	}
}

// enforcementStateResourceRules returns the same shape as
// dryRunResourceRules but with the resource rule carrying a caller-
// supplied EnforcementState. Used to exercise the evidence_only and
// enforced no-emission paths.
func enforcementStateResourceRules(es failmode.EnforcementState, posture failmode.PermittedMode, outcome failmode.Outcome) []failmode.FailModePolicyRule {
	return []failmode.FailModePolicyRule{
		{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: posture, EnforcementState: es, Outcome: outcome},
		{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
	}
}

// dryRunEvents filters audit events down to the dry-run kind.
func dryRunEvents(events []*audit.AuditEvent) []*audit.AuditEvent {
	out := make([]*audit.AuditEvent, 0, 1)
	for _, e := range events {
		if e.EventType == audit.AuditEventFailModePolicyDryRunDecision {
			out = append(out, e)
		}
	}
	return out
}

// runDryRunScenario seeds a fresh repo set, attaches a FailModePolicy
// with the supplied rules as the surface override, runs Evaluate with
// the supplied policy evaluator and authority.FailMode, and returns
// the result plus the full audit-event list.
func runDryRunScenario(
	t *testing.T,
	surfaceID, policyRef string,
	fm authority.FailMode,
	evaluator policy.PolicyEvaluator,
	rules []failmode.FailModePolicyRule,
) (decision.EvaluationResult, []*audit.AuditEvent) {
	t.Helper()
	r := newRepos()
	seedActiveSurface(t, r, surfaceID)
	seedAgent(t, r, "agent-1")
	seedProfileForTrigger(t, r, "prof-1", surfaceID, policyRef, fm)
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	seedFailModePolicyWithRules(t, r, "fmp-dryrun", 7, rules)
	setSurfaceFailModePolicyID(t, r, surfaceID, "fmp-dryrun")

	res, err := newOrchestratorWithEvaluator(t, r, evaluator).Evaluate(
		context.Background(),
		baseRequest(surfaceID, "agent-1"),
		rawPayload(t),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	return res, events
}

// failingEvaluator returns a fresh evaluator that always errors. Each
// caller gets its own instance so the simulated error is independent
// across subtests.
func failingEvaluator() policy.PolicyEvaluator {
	return &failingPolicyEvaluator{mode: policy.PolicyModeNoop, err: errors.New("simulated evaluator failure")}
}

// ---------------------------------------------------------------------------
// Emission — positive scenarios for each valid D29b dry_run combination
// ---------------------------------------------------------------------------

func TestDryRun_EmitsOnClosedDryRunDeny(t *testing.T) {
	_, events := runDryRunScenario(t,
		"surf-dryrun-cd", "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
	)
	if len(dryRunEvents(events)) != 1 {
		t.Fatalf("expected exactly 1 dry-run event for closed+dry_run+deny; got %d", len(dryRunEvents(events)))
	}
}

func TestDryRun_EmitsOnClosedDryRunEscalate(t *testing.T) {
	_, events := runDryRunScenario(t,
		"surf-dryrun-ce", "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeEscalate),
	)
	if len(dryRunEvents(events)) != 1 {
		t.Fatalf("expected exactly 1 dry-run event for closed+dry_run+escalate; got %d", len(dryRunEvents(events)))
	}
}

func TestDryRun_EmitsOnClosedDryRunManualReview(t *testing.T) {
	_, events := runDryRunScenario(t,
		"surf-dryrun-cm", "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeManualReview),
	)
	if len(dryRunEvents(events)) != 1 {
		t.Fatalf("expected exactly 1 dry-run event for closed+dry_run+manual_review; got %d", len(dryRunEvents(events)))
	}
}

func TestDryRun_EmitsOnSoftDryRunPermitWithEvidence(t *testing.T) {
	_, events := runDryRunScenario(t,
		"surf-dryrun-sp", "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeSoft, failmode.OutcomePermitWithEvidence),
	)
	if len(dryRunEvents(events)) != 1 {
		t.Fatalf("expected exactly 1 dry-run event for soft+dry_run+permit_with_evidence; got %d", len(dryRunEvents(events)))
	}
}

func TestDryRun_EmitsOnOpenDryRunPermitWithEvidence(t *testing.T) {
	_, events := runDryRunScenario(t,
		"surf-dryrun-op", "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeOpen, failmode.OutcomePermitWithEvidence),
	)
	if len(dryRunEvents(events)) != 1 {
		t.Fatalf("expected exactly 1 dry-run event for open+dry_run+permit_with_evidence; got %d", len(dryRunEvents(events)))
	}
}

// ---------------------------------------------------------------------------
// Non-emission scenarios — every preconditional gate
// ---------------------------------------------------------------------------

func TestDryRun_NoEmissionOnEvidenceOnly(t *testing.T) {
	_, events := runDryRunScenario(t,
		"surf-dryrun-eo", "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		enforcementStateResourceRules(failmode.EnforcementStateEvidenceOnly, failmode.PermittedModeClosed, failmode.OutcomeEscalate),
	)
	if got := len(dryRunEvents(events)); got != 0 {
		t.Errorf("must not emit dry-run event when enforcement_state=evidence_only; got %d", got)
	}
}

func TestDryRun_NoEmissionOnEnforced(t *testing.T) {
	// Enforced is reserved for D29f. D29e must still not enforce and
	// must not emit dry-run. The trigger-fired event still fires.
	_, events := runDryRunScenario(t,
		"surf-dryrun-enf", "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		enforcementStateResourceRules(failmode.EnforcementStateEnforced, failmode.PermittedModeClosed, failmode.OutcomeEscalate),
	)
	if got := len(dryRunEvents(events)); got != 0 {
		t.Errorf("must not emit dry-run event when enforcement_state=enforced (D29f scope); got %d", got)
	}
}

func TestDryRun_NoEmissionWhenNoResolvedPolicy(t *testing.T) {
	// No FailModePolicy seeded on the surface — trigger-fired
	// precondition fails, so dry-run must not fire either.
	r := newRepos()
	seedActiveSurface(t, r, "surf-dryrun-nofmp")
	seedAgent(t, r, "agent-1")
	seedProfileForTrigger(t, r, "prof-1", "surf-dryrun-nofmp", "test-policy", authority.FailModeClosed)
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	res, err := newOrchestratorWithEvaluator(t, r, failingEvaluator()).Evaluate(
		context.Background(),
		baseRequest("surf-dryrun-nofmp", "agent-1"),
		rawPayload(t),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	if got := len(dryRunEvents(events)); got != 0 {
		t.Errorf("must not emit dry-run event when no FailModePolicy resolved; got %d", got)
	}
}

func TestDryRun_NoEmissionOnResolverFailure(t *testing.T) {
	// Surface references a missing FailModePolicy → resolver errors,
	// trigger-fired precondition fails, dry-run must not fire either.
	r := newRepos()
	seedActiveSurface(t, r, "surf-dryrun-rerr")
	seedAgent(t, r, "agent-1")
	seedProfileForTrigger(t, r, "prof-1", "surf-dryrun-rerr", "test-policy", authority.FailModeClosed)
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	setSurfaceFailModePolicyID(t, r, "surf-dryrun-rerr", "fmp-missing")

	res, err := newOrchestratorWithEvaluator(t, r, failingEvaluator()).Evaluate(
		context.Background(),
		baseRequest("surf-dryrun-rerr", "agent-1"),
		rawPayload(t),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	if got := len(dryRunEvents(events)); got != 0 {
		t.Errorf("must not emit dry-run event when fail-mode resolution failed; got %d", got)
	}
}

func TestDryRun_NoEmissionWhenEvaluatorAllowed(t *testing.T) {
	// NoOp evaluator returns Allowed=true with no error. Trigger
	// doesn't fire, so dry-run must not fire either.
	_, events := runDryRunScenario(t,
		"surf-dryrun-allow", "test-policy",
		authority.FailModeClosed,
		policy.NoOpPolicyEvaluator{},
		dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeEscalate),
	)
	if got := len(dryRunEvents(events)); got != 0 {
		t.Errorf("must not emit dry-run event when evaluator allowed; got %d", got)
	}
}

func TestDryRun_NoEmissionWhenEvaluatorDeniedWithoutError(t *testing.T) {
	// Denying evaluator returns Allowed=false with no error. Trigger
	// doesn't fire on policy_evaluator_error, so dry-run must not fire.
	_, events := runDryRunScenario(t,
		"surf-dryrun-deny", "test-policy",
		authority.FailModeClosed,
		denyingPolicyEvaluator{},
		dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
	)
	if got := len(dryRunEvents(events)); got != 0 {
		t.Errorf("must not emit dry-run event when evaluator denied without error; got %d", got)
	}
}

func TestDryRun_NoEmissionWhenRuleMissing(t *testing.T) {
	// Hand-construct a FailModePolicy without a `resource` rule.
	// Bypasses the validator via direct repo Create. Trigger fires
	// with rule_status=not_found per D29c-2 defensive path; dry-run
	// must NOT fire — there is no rule to compute from.
	r := newRepos()
	seedActiveSurface(t, r, "surf-dryrun-norule")
	seedAgent(t, r, "agent-1")
	seedProfileForTrigger(t, r, "prof-1", "surf-dryrun-norule", "test-policy", authority.FailModeClosed)
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	now := time.Now().UTC()
	missing := &failmode.FailModePolicy{
		ID:             "fmp-dryrun-norule",
		Version:        1,
		Name:           "Missing resource rule",
		Status:         failmode.FailModePolicyStatusActive,
		EffectiveDate:  now.Add(-time.Hour),
		BusinessOwner:  "owner@example.com",
		TechnicalOwner: "platform-team",
		Rules: []failmode.FailModePolicyRule{
			{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateDryRun, Outcome: failmode.OutcomeEscalate},
			// resource rule intentionally absent
		},
		Origin:    "manual",
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "test",
	}
	if err := r.failModePolicies.Create(context.Background(), missing); err != nil {
		t.Fatalf("Create defect policy: %v", err)
	}
	setSurfaceFailModePolicyID(t, r, "surf-dryrun-norule", "fmp-dryrun-norule")

	res, err := newOrchestratorWithEvaluator(t, r, failingEvaluator()).Evaluate(
		context.Background(),
		baseRequest("surf-dryrun-norule", "agent-1"),
		rawPayload(t),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}

	// Trigger-fired should still emit with rule_status=not_found.
	if len(triggerEvents(events)) != 1 {
		t.Errorf("expected 1 trigger-fired on missing-rule path; got %d", len(triggerEvents(events)))
	}
	// Dry-run must NOT fire.
	if got := len(dryRunEvents(events)); got != 0 {
		t.Errorf("must not emit dry-run event when matching rule is not found; got %d", got)
	}
}

// ---------------------------------------------------------------------------
// Payload — required + forbidden keys
// ---------------------------------------------------------------------------

func TestDryRun_PayloadKeys(t *testing.T) {
	_, events := runDryRunScenario(t,
		"surf-dryrun-payload", "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeEscalate),
	)
	drs := dryRunEvents(events)
	if len(drs) != 1 {
		t.Fatalf("expected exactly 1 dry-run event; got %d", len(drs))
	}
	p := drs[0].Payload

	// Required scalar / string fields.
	required := map[string]any{
		"fail_mode_policy_id":      "fmp-dryrun",
		"fail_mode_policy_version": float64(7),
		"source":                   string(failmode.ResolutionSourceSurface),
		"trigger_condition":        string(failmode.FailModePolicyTriggerPolicyEvaluatorError),
		"correctness_class":        string(failmode.CorrectnessClassResource),
		"permitted_mode":           string(failmode.PermittedModeClosed),
		"enforcement_state":        string(failmode.EnforcementStateDryRun),
		"configured_outcome":       string(failmode.OutcomeEscalate),
		"dry_run_outcome":          string(eval.OutcomeEscalate),
		"dry_run_reason_code":      string(eval.ReasonFailModePolicyEscalated),
		"actual_outcome":           string(eval.OutcomeEscalate),
		"actual_reason_code":       string(eval.ReasonPolicyError),
	}
	for k, want := range required {
		got, present := p[k]
		if !present {
			t.Errorf("required payload key %q missing", k)
			continue
		}
		switch want := want.(type) {
		case float64:
			switch g := got.(type) {
			case float64:
				if g != want {
					t.Errorf("payload %q: want %v, got %v", k, want, g)
				}
			case int:
				if float64(g) != want {
					t.Errorf("payload %q: want %v, got %v", k, want, g)
				}
			default:
				t.Errorf("payload %q: unexpected type %T value %v", k, got, got)
			}
		default:
			if got != want {
				t.Errorf("payload %q: want %v, got %v", k, want, got)
			}
		}
	}

	// divergent is a bool — FailModeClosed + Escalate maps to a
	// dry-run Escalate, which equals the actual Escalate, so divergent
	// must be false here.
	if v, ok := p["divergent"]; !ok {
		t.Errorf("required payload key %q missing", "divergent")
	} else if b, ok := v.(bool); !ok || b != false {
		t.Errorf("divergent: want false, got %v (%T)", v, v)
	}

	// computed_at / evaluation_time — RFC3339Nano strings.
	for _, k := range []string{"computed_at", "evaluation_time"} {
		v, present := p[k]
		if !present {
			t.Errorf("required payload key %q missing", k)
			continue
		}
		s, ok := v.(string)
		if !ok || s == "" {
			t.Errorf("payload %q: must be non-empty string, got %v", k, v)
			continue
		}
		if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
			t.Errorf("payload %q: must be RFC3339Nano, got %q (%v)", k, s, err)
		}
	}

	// Optional contextual fields — must appear in the seeded scenario.
	for _, k := range []string{"surface_id", "surface_version", "authority_profile_id", "agent_id", "policy_reference"} {
		if _, present := p[k]; !present {
			t.Errorf("expected contextual payload key %q present in the seeded scenario", k)
		}
	}

	// Forbidden keys — must never appear on the dry-run event.
	for _, k := range []string{
		"rules",
		"raw_policy_error",
		"error",
		"stack_trace",
		"sql",
		"dsn",
		"allowed",
		"fail_open",
		"fail_soft",
		"decision_changed",
		"enforced",
		"applied",
		"triggered_at", // exclusive to the trigger-fired event
	} {
		if _, present := p[k]; present {
			t.Errorf("forbidden payload key %q must not appear; got %v", k, p[k])
		}
	}
}

// ---------------------------------------------------------------------------
// Outcome mapping — one test per valid D29b dry_run combination
// ---------------------------------------------------------------------------

func payloadValue(events []*audit.AuditEvent, t audit.AuditEventType, key string) any {
	for _, e := range events {
		if e.EventType == t {
			return e.Payload[key]
		}
	}
	return nil
}

func TestDryRun_Mapping_ClosedDeny(t *testing.T) {
	_, events := runDryRunScenario(t,
		"surf-dryrun-map-cd", "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
	)
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "dry_run_outcome"); got != string(eval.OutcomeReject) {
		t.Errorf("dry_run_outcome: want %q, got %v", eval.OutcomeReject, got)
	}
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "dry_run_reason_code"); got != string(eval.ReasonFailModePolicyDenied) {
		t.Errorf("dry_run_reason_code: want %q, got %v", eval.ReasonFailModePolicyDenied, got)
	}
	// Actual outcome under FailModeClosed is Escalate; dry-run Reject
	// is different → divergent=true.
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "divergent"); got != true {
		t.Errorf("divergent: want true (escalate vs reject), got %v", got)
	}
}

func TestDryRun_Mapping_ClosedEscalate(t *testing.T) {
	_, events := runDryRunScenario(t,
		"surf-dryrun-map-ce", "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeEscalate),
	)
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "dry_run_outcome"); got != string(eval.OutcomeEscalate) {
		t.Errorf("dry_run_outcome: want %q, got %v", eval.OutcomeEscalate, got)
	}
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "dry_run_reason_code"); got != string(eval.ReasonFailModePolicyEscalated) {
		t.Errorf("dry_run_reason_code: want %q, got %v", eval.ReasonFailModePolicyEscalated, got)
	}
	// Actual outcome under FailModeClosed is Escalate; dry-run Escalate
	// matches → divergent=false.
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "divergent"); got != false {
		t.Errorf("divergent: want false (escalate vs escalate), got %v", got)
	}
}

func TestDryRun_Mapping_ClosedManualReview(t *testing.T) {
	_, events := runDryRunScenario(t,
		"surf-dryrun-map-cm", "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeManualReview),
	)
	// eval.OutcomeManualReview does not exist; mapping falls back to
	// eval.OutcomeEscalate while preserving ReasonFailModePolicyManualReview.
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "dry_run_outcome"); got != string(eval.OutcomeEscalate) {
		t.Errorf("dry_run_outcome: want %q (manual_review falls back to escalate), got %v", eval.OutcomeEscalate, got)
	}
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "dry_run_reason_code"); got != string(eval.ReasonFailModePolicyManualReview) {
		t.Errorf("dry_run_reason_code: want %q, got %v", eval.ReasonFailModePolicyManualReview, got)
	}
	// Actual = Escalate, dry-run = Escalate → divergent=false.
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "divergent"); got != false {
		t.Errorf("divergent: want false, got %v", got)
	}
}

func TestDryRun_Mapping_SoftPermitWithEvidence_FailModeOpen(t *testing.T) {
	// Under FailModeOpen, actual = Accept; permit_with_evidence maps
	// dry_run_outcome to actual_outcome (Accept) → divergent=false.
	_, events := runDryRunScenario(t,
		"surf-dryrun-map-sp-open", "test-policy",
		authority.FailModeOpen,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeSoft, failmode.OutcomePermitWithEvidence),
	)
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "dry_run_outcome"); got != string(eval.OutcomeAccept) {
		t.Errorf("dry_run_outcome: want %q (= actual under FailModeOpen), got %v", eval.OutcomeAccept, got)
	}
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "dry_run_reason_code"); got != string(eval.ReasonFailModePolicyPermitWithEvidence) {
		t.Errorf("dry_run_reason_code: want %q, got %v", eval.ReasonFailModePolicyPermitWithEvidence, got)
	}
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "divergent"); got != false {
		t.Errorf("divergent: want false (permit_with_evidence mirrors actual), got %v", got)
	}
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "actual_outcome"); got != string(eval.OutcomeAccept) {
		t.Errorf("actual_outcome: want %q, got %v", eval.OutcomeAccept, got)
	}
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "actual_reason_code"); got != string(eval.ReasonWithinAuthority) {
		t.Errorf("actual_reason_code: want %q, got %v", eval.ReasonWithinAuthority, got)
	}
}

func TestDryRun_Mapping_OpenPermitWithEvidence_FailModeClosed(t *testing.T) {
	// Under FailModeClosed, actual = Escalate; permit_with_evidence
	// maps dry_run_outcome to actual_outcome (Escalate) → divergent=false.
	_, events := runDryRunScenario(t,
		"surf-dryrun-map-op-closed", "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeOpen, failmode.OutcomePermitWithEvidence),
	)
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "dry_run_outcome"); got != string(eval.OutcomeEscalate) {
		t.Errorf("dry_run_outcome: want %q (= actual under FailModeClosed), got %v", eval.OutcomeEscalate, got)
	}
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "divergent"); got != false {
		t.Errorf("divergent: want false (permit_with_evidence mirrors actual), got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Divergence — explicit pin
// ---------------------------------------------------------------------------

func TestDryRun_Divergent_EscalateVsReject(t *testing.T) {
	// Actual outcome = Escalate (FailModeClosed); configured deny
	// maps to dry-run Reject → divergent must be true.
	_, events := runDryRunScenario(t,
		"surf-dryrun-div-er", "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
	)
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "divergent"); got != true {
		t.Errorf("divergent: want true (escalate vs reject), got %v", got)
	}
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "actual_outcome"); got != string(eval.OutcomeEscalate) {
		t.Errorf("actual_outcome: want %q, got %v", eval.OutcomeEscalate, got)
	}
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "dry_run_outcome"); got != string(eval.OutcomeReject) {
		t.Errorf("dry_run_outcome: want %q, got %v", eval.OutcomeReject, got)
	}
}

func TestDryRun_Divergent_FailModeOpenAcceptVsReject(t *testing.T) {
	// Under FailModeOpen, evaluator error → evaluation continues to
	// Accept. Configured deny maps dry-run to Reject → divergent=true.
	res, events := runDryRunScenario(t,
		"surf-dryrun-div-or", "test-policy",
		authority.FailModeOpen,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
	)
	if res.Outcome != eval.OutcomeAccept {
		t.Fatalf("actual outcome must be Accept under FailModeOpen; got %q", res.Outcome)
	}
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "divergent"); got != true {
		t.Errorf("divergent: want true (accept vs reject), got %v", got)
	}
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "actual_outcome"); got != string(eval.OutcomeAccept) {
		t.Errorf("actual_outcome: want %q, got %v", eval.OutcomeAccept, got)
	}
	if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "actual_reason_code"); got != string(eval.ReasonWithinAuthority) {
		t.Errorf("actual_reason_code: want %q, got %v", eval.ReasonWithinAuthority, got)
	}
}

// ---------------------------------------------------------------------------
// authority.FailMode preservation — dry-run must not influence outcome
// ---------------------------------------------------------------------------

func TestDryRun_DoesNotOverrideAuthorityFailModeOpen(t *testing.T) {
	// Configure dry_run + deny under FailModeOpen. Actual outcome
	// must remain Accept / WithinAuthority — the dry-run event is
	// evidence only.
	res, _ := runDryRunScenario(t,
		"surf-dryrun-pres-open", "test-policy",
		authority.FailModeOpen,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
	)
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("dry-run event must not override FailModeOpen; got Outcome=%q", res.Outcome)
	}
	if res.ReasonCode != eval.ReasonWithinAuthority {
		t.Errorf("dry-run event must not change ReasonCode under FailModeOpen; got %q", res.ReasonCode)
	}
}

func TestDryRun_DoesNotOverrideAuthorityFailModeClosed(t *testing.T) {
	// Configure dry_run + deny under FailModeClosed. Actual outcome
	// must remain Escalate / POLICY_ERROR — the dry-run event is
	// evidence only, even though the configured outcome is Deny.
	res, _ := runDryRunScenario(t,
		"surf-dryrun-pres-closed", "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
	)
	if res.Outcome != eval.OutcomeEscalate {
		t.Errorf("dry-run event must not override FailModeClosed escalation; got Outcome=%q", res.Outcome)
	}
	if res.ReasonCode != eval.ReasonPolicyError {
		t.Errorf("dry-run event must not change ReasonCode under FailModeClosed; got %q", res.ReasonCode)
	}
}

// ---------------------------------------------------------------------------
// Reason-code isolation — the critical pin
// ---------------------------------------------------------------------------

// TestDryRun_ReasonCodeIsolation asserts the load-bearing D29e
// invariant: every D29e fixture that emits a dry-run event must have
// an OUTCOME_RECORDED event whose reason_code does NOT begin with
// "FAIL_MODE_POLICY_". The dry-run reason codes are scoped to the
// dry-run event payload only.
func TestDryRun_ReasonCodeIsolation(t *testing.T) {
	scenarios := []struct {
		name   string
		fm     authority.FailMode
		rules  []failmode.FailModePolicyRule
		policy policy.PolicyEvaluator
	}{
		{
			name:   "closed_dryrun_deny_failmode_closed",
			fm:     authority.FailModeClosed,
			rules:  dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
			policy: failingEvaluator(),
		},
		{
			name:   "closed_dryrun_escalate_failmode_closed",
			fm:     authority.FailModeClosed,
			rules:  dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeEscalate),
			policy: failingEvaluator(),
		},
		{
			name:   "closed_dryrun_manual_review_failmode_closed",
			fm:     authority.FailModeClosed,
			rules:  dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeManualReview),
			policy: failingEvaluator(),
		},
		{
			name:   "soft_dryrun_permit_failmode_open",
			fm:     authority.FailModeOpen,
			rules:  dryRunResourceRules(failmode.PermittedModeSoft, failmode.OutcomePermitWithEvidence),
			policy: failingEvaluator(),
		},
		{
			name:   "open_dryrun_permit_failmode_closed",
			fm:     authority.FailModeClosed,
			rules:  dryRunResourceRules(failmode.PermittedModeOpen, failmode.OutcomePermitWithEvidence),
			policy: failingEvaluator(),
		},
		{
			name:   "closed_dryrun_deny_failmode_open",
			fm:     authority.FailModeOpen,
			rules:  dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
			policy: failingEvaluator(),
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			_, events := runDryRunScenario(t,
				"surf-dryrun-iso-"+sc.name, "test-policy",
				sc.fm, sc.policy, sc.rules,
			)
			// Confirm a dry-run event was emitted in this fixture.
			if len(dryRunEvents(events)) != 1 {
				t.Fatalf("expected dry-run event for %s; got %d", sc.name, len(dryRunEvents(events)))
			}
			// Inspect OUTCOME_RECORDED — its reason_code must not
			// begin with "FAIL_MODE_POLICY_".
			rc := payloadValue(events, audit.AuditEventOutcomeRecorded, "reason_code")
			s, ok := rc.(string)
			if !ok {
				t.Fatalf("OUTCOME_RECORDED reason_code missing or non-string: %v (%T)", rc, rc)
			}
			if strings.HasPrefix(s, "FAIL_MODE_POLICY_") {
				t.Errorf("OUTCOME_RECORDED reason_code leaked dry-run constant: %q", s)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Event ordering
// ---------------------------------------------------------------------------

func TestDryRun_EventOrdering(t *testing.T) {
	_, events := runDryRunScenario(t,
		"surf-dryrun-order", "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeEscalate),
	)
	resolvedIdx := eventTypeIndex(events, audit.AuditEventFailModePolicyResolved)
	policyEvalIdx := eventTypeIndex(events, audit.AuditEventPolicyEvaluated)
	triggerIdx := eventTypeIndex(events, audit.AuditEventFailModePolicyTriggerFired)
	dryRunIdx := eventTypeIndex(events, audit.AuditEventFailModePolicyDryRunDecision)
	outcomeIdx := eventTypeIndex(events, audit.AuditEventOutcomeRecorded)

	if resolvedIdx < 0 || policyEvalIdx < 0 || triggerIdx < 0 || dryRunIdx < 0 || outcomeIdx < 0 {
		t.Fatalf("missing required events: resolved=%d policyEval=%d trigger=%d dryRun=%d outcome=%d",
			resolvedIdx, policyEvalIdx, triggerIdx, dryRunIdx, outcomeIdx)
	}
	if !(policyEvalIdx < triggerIdx) {
		t.Errorf("POLICY_EVALUATED (idx=%d) must come before FAIL_MODE_POLICY_TRIGGER_FIRED (idx=%d)",
			policyEvalIdx, triggerIdx)
	}
	if !(triggerIdx < dryRunIdx) {
		t.Errorf("FAIL_MODE_POLICY_TRIGGER_FIRED (idx=%d) must come before FAIL_MODE_POLICY_DRY_RUN_DECISION (idx=%d)",
			triggerIdx, dryRunIdx)
	}
	if !(dryRunIdx < outcomeIdx) {
		t.Errorf("FAIL_MODE_POLICY_DRY_RUN_DECISION (idx=%d) must come before OUTCOME_RECORDED (idx=%d)",
			dryRunIdx, outcomeIdx)
	}
}

// ---------------------------------------------------------------------------
// Runtime invariance — evidence_only vs dry_run differ only by one event
// ---------------------------------------------------------------------------

// stripPerRunFieldsForDryRun strips fields that necessarily differ
// between two runs from an audit-event payload.
func stripPerRunFieldsForDryRun(p map[string]any) map[string]any {
	out := make(map[string]any, len(p))
	for k, v := range p {
		switch k {
		case "envelope_id", "audit_event_id", "request_id",
			"resolved_at", "evaluation_time", "triggered_at", "computed_at",
			"started_at", "ended_at", "created_at", "updated_at", "timestamp":
			continue
		}
		out[k] = v
	}
	return out
}

// TestDryRun_RuntimeInvariance_OnlyDryRunEventDiffers is the strongest
// D29e pin: two evaluations differing only in the enforcement_state of
// the matched rule (evidence_only vs dry_run) must produce
// byte-identical Outcome / ReasonCode / OUTCOME_RECORDED /
// POLICY_EVALUATED / FAIL_MODE_POLICY_RESOLVED / FAIL_MODE_POLICY_TRIGGER_FIRED
// payloads. The audit-event kind set differs by exactly one entry —
// FAIL_MODE_POLICY_DRY_RUN_DECISION appears in the dry_run case only.
func TestDryRun_RuntimeInvariance_OnlyDryRunEventDiffers(t *testing.T) {
	// Same surface ID across both runs so per-fixture identity does
	// not leak into the byte-equality comparison. Each scenario uses
	// a fresh repo set (newRepos) so there is no cross-run state.
	const sharedSurfaceID = "surf-dryrun-inv"
	baselineRes, baselineEvents := runDryRunScenario(t,
		sharedSurfaceID, "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		enforcementStateResourceRules(failmode.EnforcementStateEvidenceOnly, failmode.PermittedModeClosed, failmode.OutcomeEscalate),
	)
	variantRes, variantEvents := runDryRunScenario(t,
		sharedSurfaceID, "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		enforcementStateResourceRules(failmode.EnforcementStateDryRun, failmode.PermittedModeClosed, failmode.OutcomeEscalate),
	)

	// Outcome / ReasonCode invariance.
	if baselineRes.Outcome != variantRes.Outcome {
		t.Errorf("Outcome diverged: baseline=%q variant=%q", baselineRes.Outcome, variantRes.Outcome)
	}
	if baselineRes.ReasonCode != variantRes.ReasonCode {
		t.Errorf("ReasonCode diverged: baseline=%q variant=%q", baselineRes.ReasonCode, variantRes.ReasonCode)
	}

	// OUTCOME_RECORDED payload byte-equality.
	baseOR := stripPerRunFieldsForDryRun(payloadOf(baselineEvents, audit.AuditEventOutcomeRecorded))
	varOR := stripPerRunFieldsForDryRun(payloadOf(variantEvents, audit.AuditEventOutcomeRecorded))
	if !reflect.DeepEqual(baseOR, varOR) {
		t.Errorf("OUTCOME_RECORDED payload diverged:\nbaseline=%v\nvariant=%v", baseOR, varOR)
	}

	// POLICY_EVALUATED payload byte-equality.
	basePE := stripPerRunFieldsForDryRun(payloadOf(baselineEvents, audit.AuditEventPolicyEvaluated))
	varPE := stripPerRunFieldsForDryRun(payloadOf(variantEvents, audit.AuditEventPolicyEvaluated))
	if !reflect.DeepEqual(basePE, varPE) {
		t.Errorf("POLICY_EVALUATED payload diverged:\nbaseline=%v\nvariant=%v", basePE, varPE)
	}

	// FAIL_MODE_POLICY_RESOLVED payload byte-equality.
	baseR := stripPerRunFieldsForDryRun(payloadOf(baselineEvents, audit.AuditEventFailModePolicyResolved))
	varR := stripPerRunFieldsForDryRun(payloadOf(variantEvents, audit.AuditEventFailModePolicyResolved))
	if !reflect.DeepEqual(baseR, varR) {
		t.Errorf("FAIL_MODE_POLICY_RESOLVED payload diverged:\nbaseline=%v\nvariant=%v", baseR, varR)
	}

	// FAIL_MODE_POLICY_TRIGGER_FIRED payload byte-equality. The two
	// fixtures share the same posture / configured outcome and differ
	// only in enforcement_state, so this payload must differ by exactly
	// that one field. Strip enforcement_state before comparing the
	// remainder.
	baseT := stripPerRunFieldsForDryRun(payloadOf(baselineEvents, audit.AuditEventFailModePolicyTriggerFired))
	varT := stripPerRunFieldsForDryRun(payloadOf(variantEvents, audit.AuditEventFailModePolicyTriggerFired))
	if baseT["enforcement_state"] != "evidence_only" || varT["enforcement_state"] != "dry_run" {
		t.Errorf("trigger payload enforcement_state mismatch: baseline=%v variant=%v",
			baseT["enforcement_state"], varT["enforcement_state"])
	}
	delete(baseT, "enforcement_state")
	delete(varT, "enforcement_state")
	if !reflect.DeepEqual(baseT, varT) {
		t.Errorf("FAIL_MODE_POLICY_TRIGGER_FIRED payload diverged outside enforcement_state:\nbaseline=%v\nvariant=%v", baseT, varT)
	}

	// Audit-event kind set: variant has exactly one extra event kind
	// (FAIL_MODE_POLICY_DRY_RUN_DECISION). After removing that, the
	// sets must match.
	baseKinds := map[audit.AuditEventType]int{}
	for _, e := range baselineEvents {
		baseKinds[e.EventType]++
	}
	varKinds := map[audit.AuditEventType]int{}
	for _, e := range variantEvents {
		varKinds[e.EventType]++
	}
	if varKinds[audit.AuditEventFailModePolicyDryRunDecision] != 1 {
		t.Errorf("variant must emit exactly 1 dry-run event; got %d", varKinds[audit.AuditEventFailModePolicyDryRunDecision])
	}
	delete(varKinds, audit.AuditEventFailModePolicyDryRunDecision)
	if _, present := baseKinds[audit.AuditEventFailModePolicyDryRunDecision]; present {
		t.Errorf("baseline (evidence_only) must NOT emit dry-run event")
	}
	if !reflect.DeepEqual(baseKinds, varKinds) {
		t.Errorf("audit-event kind sets diverged outside the dry-run delta:\nbaseline=%v\nvariant_minus_dryrun=%v",
			baseKinds, varKinds)
	}
}

// ---------------------------------------------------------------------------
// HTTP-response invariance — dry-run payload never on the wire
// ---------------------------------------------------------------------------

func TestDryRun_HTTPResponseNoLeakage(t *testing.T) {
	res, _ := runDryRunScenario(t,
		"surf-dryrun-http", "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
	)

	// Build the same wire shape /v1/evaluate produces.
	type evaluateResponseShape struct {
		Outcome         string `json:"outcome"`
		Reason          string `json:"reason"`
		EnvelopeID      string `json:"envelope_id"`
		Explanation     string `json:"explanation,omitempty"`
		PolicyMode      string `json:"policy_mode,omitempty"`
		PolicyReference string `json:"policy_reference,omitempty"`
		PolicySkipped   bool   `json:"policy_skipped,omitempty"`
	}
	body, err := json.Marshal(evaluateResponseShape{
		Outcome:         string(res.Outcome),
		Reason:          string(res.ReasonCode),
		EnvelopeID:      res.EnvelopeID,
		Explanation:     res.Explanation,
		PolicyMode:      res.PolicyMode,
		PolicyReference: res.PolicyReference,
		PolicySkipped:   res.PolicySkipped,
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	bodyStr := string(body)
	for _, sub := range []string{
		"dry_run_outcome",
		"dry_run_reason_code",
		"configured_outcome",
		"divergent",
		"computed_at",
		"FAIL_MODE_POLICY_DRY_RUN_DECISION",
		"FAIL_MODE_POLICY_DENIED",
		"FAIL_MODE_POLICY_ESCALATED",
		"FAIL_MODE_POLICY_PERMIT_WITH_EVIDENCE",
		"FAIL_MODE_POLICY_MANUAL_REVIEW",
	} {
		if strings.Contains(bodyStr, sub) {
			t.Errorf("/v1/evaluate response must not contain %q; body=%s", sub, bodyStr)
		}
	}
}

// Pure-mapping coverage for all four valid configured-outcome shapes
// (deny / escalate / permit_with_evidence / manual_review) is provided
// by the five TestDryRun_Mapping_* integration tests above, each of
// which seeds a rule with the given posture / outcome and asserts the
// dry_run_outcome / dry_run_reason_code / divergent payload values.
// The helper itself is unexported by design; exporting it solely for
// a unit test would broaden the decision-package surface beyond the
// brief's scope.
