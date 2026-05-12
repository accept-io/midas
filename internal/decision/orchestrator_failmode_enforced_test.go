package decision_test

// orchestrator_failmode_enforced_test.go — D29f pins.
//
// Asserts:
//
//   - FAIL_MODE_POLICY_ENFORCED is emitted ONLY when all five gate
//     conditions hold (evaluator error, resolved policy, available
//     policy entity, matching rule for correctness_class=resource,
//     enforcement_state=enforced).
//   - When enforcement applies, the orchestrator's actual outcome
//     is the FailModePolicy-derived outcome — authority.FailMode is
//     NOT consulted on the enforced path.
//   - Outcome mapping mirrors D29e via the shared pure helper:
//       deny                  → reject  + FAIL_MODE_POLICY_DENIED
//       escalate              → escalate + FAIL_MODE_POLICY_ESCALATED
//       permit_with_evidence  → accept  + FAIL_MODE_POLICY_PERMIT_WITH_EVIDENCE
//       manual_review         → escalate + FAIL_MODE_POLICY_MANUAL_REVIEW
//     permit_with_evidence's enforced_outcome is the FailModeOpen
//     continuation (accept) regardless of profile FailMode — this is
//     the documented operator-surprise case.
//   - Dry-run and enforced are mutually exclusive: enforced fixtures
//     never emit dry-run; dry-run fixtures never emit enforced.
//   - Authority.FailMode preservation: when no enforced rule applies
//     (evidence_only, dry_run, missing rule, no policy, resolver
//     error), the orchestrator's outcome is byte-identical to
//     pre-D29f behaviour.
//   - Event ordering: POLICY_EVALUATED → FAIL_MODE_POLICY_TRIGGER_FIRED
//     → FAIL_MODE_POLICY_ENFORCED → OUTCOME_RECORDED.
//   - OUTCOME_RECORDED.outcome and reason_code match the enforced
//     event's payload exactly.
//   - Reason-code isolation conditional invariant:
//     OUTCOME_RECORDED.reason_code begins with "FAIL_MODE_POLICY_"
//     iff FAIL_MODE_POLICY_ENFORCED is emitted in the same evaluation.
//   - /v1/evaluate response shape is unchanged — no enforced-internal
//     field names leak onto the wire. reason_code MAY carry a
//     FAIL_MODE_POLICY_* value when enforcement applies; this is the
//     single intentional exposure.

import (
	"context"
	"encoding/json"
	"errors"
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

// enforcedEvents filters audit events down to the enforced kind.
func enforcedEvents(events []*audit.AuditEvent) []*audit.AuditEvent {
	out := make([]*audit.AuditEvent, 0, 1)
	for _, e := range events {
		if e.EventType == audit.AuditEventFailModePolicyEnforced {
			out = append(out, e)
		}
	}
	return out
}

// enforcedResourceRules returns five rules covering all correctness
// classes, with the resource rule carrying the supplied posture /
// configured outcome under EnforcementStateEnforced. Other rules use
// the safe closed+evidence_only+escalate defaults so they satisfy the
// exhaustive-class invariant when the validator is bypassed via direct
// repo Create.
func enforcedResourceRules(posture failmode.PermittedMode, outcome failmode.Outcome) []failmode.FailModePolicyRule {
	return []failmode.FailModePolicyRule{
		{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: posture, EnforcementState: failmode.EnforcementStateEnforced, Outcome: outcome},
		{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
	}
}

// runEnforcedScenario seeds a fresh repo set, attaches a FailModePolicy
// with the supplied rules as the surface override, runs Evaluate with
// the supplied evaluator and authority.FailMode, and returns the
// EvaluationResult plus the full audit-event list. Mirrors
// runDryRunScenario in shape so D29e and D29f tests share idiom.
func runEnforcedScenario(
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
	seedFailModePolicyWithRules(t, r, "fmp-enforced", 11, rules)
	setSurfaceFailModePolicyID(t, r, surfaceID, "fmp-enforced")

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

func enforcedFailingEvaluator() policy.PolicyEvaluator {
	return &failingPolicyEvaluator{mode: policy.PolicyModeNoop, err: errors.New("simulated evaluator failure")}
}

// ---------------------------------------------------------------------------
// Positive enforcement — five valid D29b combinations under enforced
// ---------------------------------------------------------------------------

// TestEnforced_ClosedDeny_OverridesAuthorityFailModeClosed pins the
// operator-surprise case where an enforced deny rule overrides the
// existing FailModeClosed escalate path: actual outcome flips from
// Escalate/POLICY_ERROR to Reject/FAIL_MODE_POLICY_DENIED.
func TestEnforced_ClosedDeny_OverridesAuthorityFailModeClosed(t *testing.T) {
	res, events := runEnforcedScenario(t,
		"surf-enf-cd", "test-policy",
		authority.FailModeClosed,
		enforcedFailingEvaluator(),
		enforcedResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
	)
	if res.Outcome != eval.OutcomeReject {
		t.Errorf("Outcome: want %q (FailModePolicy enforced deny), got %q", eval.OutcomeReject, res.Outcome)
	}
	if res.ReasonCode != eval.ReasonFailModePolicyDenied {
		t.Errorf("ReasonCode: want %q, got %q", eval.ReasonFailModePolicyDenied, res.ReasonCode)
	}
	if len(enforcedEvents(events)) != 1 {
		t.Fatalf("expected exactly 1 enforced event; got %d", len(enforcedEvents(events)))
	}
	if len(dryRunEvents(events)) != 0 {
		t.Errorf("dry-run event must not appear in enforced fixture; got %d", len(dryRunEvents(events)))
	}
}

// TestEnforced_ClosedEscalate_ChangesReasonCode pins the outcome-stable
// case: actual Outcome remains Escalate (matches the pre-D29f
// FailModeClosed dispatch), but the reason_code becomes
// FAIL_MODE_POLICY_ESCALATED. This is the only enforcement
// combination where the operator-visible Outcome value does not
// change.
func TestEnforced_ClosedEscalate_ChangesReasonCode(t *testing.T) {
	res, events := runEnforcedScenario(t,
		"surf-enf-ce", "test-policy",
		authority.FailModeClosed,
		enforcedFailingEvaluator(),
		enforcedResourceRules(failmode.PermittedModeClosed, failmode.OutcomeEscalate),
	)
	if res.Outcome != eval.OutcomeEscalate {
		t.Errorf("Outcome: want %q, got %q", eval.OutcomeEscalate, res.Outcome)
	}
	if res.ReasonCode != eval.ReasonFailModePolicyEscalated {
		t.Errorf("ReasonCode: want %q, got %q", eval.ReasonFailModePolicyEscalated, res.ReasonCode)
	}
	if len(enforcedEvents(events)) != 1 {
		t.Fatalf("expected enforced event; got %d", len(enforcedEvents(events)))
	}
}

// TestEnforced_SoftPermitWithEvidence_OverridesFailModeClosed is the
// SECOND operator-surprise case: enforced permit_with_evidence
// under authority.FailModeClosed produces Accept (the FailModeOpen
// continuation), explicitly overriding the FailModeClosed escalate
// path. The brief documents this as intentional.
func TestEnforced_SoftPermitWithEvidence_OverridesFailModeClosed(t *testing.T) {
	res, events := runEnforcedScenario(t,
		"surf-enf-sp-closed", "test-policy",
		authority.FailModeClosed,
		enforcedFailingEvaluator(),
		enforcedResourceRules(failmode.PermittedModeSoft, failmode.OutcomePermitWithEvidence),
	)
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("Outcome: want %q (enforced permit_with_evidence overrides FailModeClosed), got %q", eval.OutcomeAccept, res.Outcome)
	}
	if res.ReasonCode != eval.ReasonFailModePolicyPermitWithEvidence {
		t.Errorf("ReasonCode: want %q, got %q", eval.ReasonFailModePolicyPermitWithEvidence, res.ReasonCode)
	}
	if len(enforcedEvents(events)) != 1 {
		t.Fatalf("expected enforced event; got %d", len(enforcedEvents(events)))
	}
	// previous_outcome / previous_reason_code capture the
	// counterfactual: what authority.FailMode would have produced.
	prev := payloadValue(events, audit.AuditEventFailModePolicyEnforced, "previous_outcome")
	if prev != string(eval.OutcomeEscalate) {
		t.Errorf("previous_outcome: want %q (FailModeClosed counterfactual), got %v", eval.OutcomeEscalate, prev)
	}
	prevReason := payloadValue(events, audit.AuditEventFailModePolicyEnforced, "previous_reason_code")
	if prevReason != string(eval.ReasonPolicyError) {
		t.Errorf("previous_reason_code: want %q, got %v", eval.ReasonPolicyError, prevReason)
	}
}

// TestEnforced_OpenPermitWithEvidence_UnchangedFromFailModeOpen pins
// the no-surprise case for permit_with_evidence: under
// authority.FailModeOpen the actual outcome was already Accept; with
// enforced permit_with_evidence the outcome is still Accept, but the
// reason_code becomes FAIL_MODE_POLICY_PERMIT_WITH_EVIDENCE (instead
// of WITHIN_AUTHORITY).
func TestEnforced_OpenPermitWithEvidence_UnchangedFromFailModeOpen(t *testing.T) {
	res, events := runEnforcedScenario(t,
		"surf-enf-op-open", "test-policy",
		authority.FailModeOpen,
		enforcedFailingEvaluator(),
		enforcedResourceRules(failmode.PermittedModeOpen, failmode.OutcomePermitWithEvidence),
	)
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("Outcome: want %q, got %q", eval.OutcomeAccept, res.Outcome)
	}
	if res.ReasonCode != eval.ReasonFailModePolicyPermitWithEvidence {
		t.Errorf("ReasonCode: want %q, got %q", eval.ReasonFailModePolicyPermitWithEvidence, res.ReasonCode)
	}
	if len(enforcedEvents(events)) != 1 {
		t.Fatalf("expected enforced event; got %d", len(enforcedEvents(events)))
	}
	// previous_outcome should be Accept here (FailModeOpen
	// counterfactual matches the enforced outcome).
	prev := payloadValue(events, audit.AuditEventFailModePolicyEnforced, "previous_outcome")
	if prev != string(eval.OutcomeAccept) {
		t.Errorf("previous_outcome: want %q, got %v", eval.OutcomeAccept, prev)
	}
}

// TestEnforced_ManualReview_MapsToEscalate pins that manual_review
// maps to eval.OutcomeEscalate (since eval.OutcomeManualReview does
// not exist) while preserving the FAIL_MODE_POLICY_MANUAL_REVIEW
// reason code so operator intent is recorded.
func TestEnforced_ManualReview_MapsToEscalate(t *testing.T) {
	res, events := runEnforcedScenario(t,
		"surf-enf-cm", "test-policy",
		authority.FailModeClosed,
		enforcedFailingEvaluator(),
		enforcedResourceRules(failmode.PermittedModeClosed, failmode.OutcomeManualReview),
	)
	if res.Outcome != eval.OutcomeEscalate {
		t.Errorf("Outcome: want %q (manual_review falls back to escalate), got %q", eval.OutcomeEscalate, res.Outcome)
	}
	if res.ReasonCode != eval.ReasonFailModePolicyManualReview {
		t.Errorf("ReasonCode: want %q, got %q", eval.ReasonFailModePolicyManualReview, res.ReasonCode)
	}
	if len(enforcedEvents(events)) != 1 {
		t.Fatalf("expected enforced event; got %d", len(enforcedEvents(events)))
	}
}

// ---------------------------------------------------------------------------
// Non-enforcement — every gate failure preserves authority.FailMode
// ---------------------------------------------------------------------------

func TestEnforced_NoEnforcementOnEvidenceOnly(t *testing.T) {
	res, events := runEnforcedScenario(t,
		"surf-enf-eo", "test-policy",
		authority.FailModeClosed,
		enforcedFailingEvaluator(),
		// evidence_only resource rule; trigger fires but enforcement does not.
		[]failmode.FailModePolicyRule{
			{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		},
	)
	// Authority FailModeClosed preserved: Escalate / POLICY_ERROR.
	if res.Outcome != eval.OutcomeEscalate {
		t.Errorf("Outcome must preserve FailModeClosed under evidence_only; got %q", res.Outcome)
	}
	if res.ReasonCode != eval.ReasonPolicyError {
		t.Errorf("ReasonCode must preserve POLICY_ERROR under evidence_only; got %q", res.ReasonCode)
	}
	if len(enforcedEvents(events)) != 0 {
		t.Errorf("must not emit enforced event for evidence_only; got %d", len(enforcedEvents(events)))
	}
	// Trigger still fires (D29c-2 preserved).
	if len(triggerEvents(events)) != 1 {
		t.Errorf("trigger event must still fire under evidence_only; got %d", len(triggerEvents(events)))
	}
}

func TestEnforced_NoEnforcementOnDryRun(t *testing.T) {
	res, events := runEnforcedScenario(t,
		"surf-enf-dr", "test-policy",
		authority.FailModeClosed,
		enforcedFailingEvaluator(),
		// dry_run resource rule.
		[]failmode.FailModePolicyRule{
			{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateDryRun, Outcome: failmode.OutcomeDeny},
			{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		},
	)
	// Authority FailModeClosed preserved despite dry_run + deny rule.
	if res.Outcome != eval.OutcomeEscalate {
		t.Errorf("Outcome must preserve FailModeClosed under dry_run; got %q", res.Outcome)
	}
	if res.ReasonCode != eval.ReasonPolicyError {
		t.Errorf("ReasonCode must preserve POLICY_ERROR under dry_run; got %q", res.ReasonCode)
	}
	if len(enforcedEvents(events)) != 0 {
		t.Errorf("must not emit enforced event for dry_run; got %d", len(enforcedEvents(events)))
	}
	// Dry-run event still fires (D29e preserved).
	if len(dryRunEvents(events)) != 1 {
		t.Errorf("dry-run event must still fire under dry_run; got %d", len(dryRunEvents(events)))
	}
}

func TestEnforced_NoEnforcementWhenNoResolvedPolicy(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-enf-nofmp")
	seedAgent(t, r, "agent-1")
	seedProfileForTrigger(t, r, "prof-1", "surf-enf-nofmp", "test-policy", authority.FailModeClosed)
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	res, err := newOrchestratorWithEvaluator(t, r, enforcedFailingEvaluator()).Evaluate(
		context.Background(),
		baseRequest("surf-enf-nofmp", "agent-1"),
		rawPayload(t),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	if res.Outcome != eval.OutcomeEscalate || res.ReasonCode != eval.ReasonPolicyError {
		t.Errorf("Outcome / ReasonCode must preserve FailModeClosed; got %q / %q", res.Outcome, res.ReasonCode)
	}
	if len(enforcedEvents(events)) != 0 {
		t.Errorf("must not emit enforced event when no FailModePolicy resolved; got %d", len(enforcedEvents(events)))
	}
}

func TestEnforced_NoEnforcementOnResolverFailure(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-enf-rerr")
	seedAgent(t, r, "agent-1")
	seedProfileForTrigger(t, r, "prof-1", "surf-enf-rerr", "test-policy", authority.FailModeClosed)
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	setSurfaceFailModePolicyID(t, r, "surf-enf-rerr", "fmp-missing")

	res, err := newOrchestratorWithEvaluator(t, r, enforcedFailingEvaluator()).Evaluate(
		context.Background(),
		baseRequest("surf-enf-rerr", "agent-1"),
		rawPayload(t),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	if res.Outcome != eval.OutcomeEscalate || res.ReasonCode != eval.ReasonPolicyError {
		t.Errorf("Outcome / ReasonCode must preserve FailModeClosed on resolver error; got %q / %q", res.Outcome, res.ReasonCode)
	}
	if len(enforcedEvents(events)) != 0 {
		t.Errorf("must not emit enforced event on resolver error; got %d", len(enforcedEvents(events)))
	}
}

func TestEnforced_NoEnforcementWhenRuleMissing(t *testing.T) {
	// Hand-construct a FailModePolicy without a `resource` rule.
	// Trigger fires with rule_status=not_found per D29c-2 defensive
	// path; enforcement must NOT apply.
	r := newRepos()
	seedActiveSurface(t, r, "surf-enf-norule")
	seedAgent(t, r, "agent-1")
	seedProfileForTrigger(t, r, "prof-1", "surf-enf-norule", "test-policy", authority.FailModeClosed)
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	now := time.Now().UTC()
	missing := &failmode.FailModePolicy{
		ID:             "fmp-enf-norule",
		Version:        1,
		Name:           "Missing resource rule",
		Status:         failmode.FailModePolicyStatusActive,
		EffectiveDate:  now.Add(-time.Hour),
		BusinessOwner:  "owner@example.com",
		TechnicalOwner: "platform-team",
		// Only governance_integrity rule with enforced+deny; resource
		// rule intentionally absent.
		Rules: []failmode.FailModePolicyRule{
			{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEnforced, Outcome: failmode.OutcomeDeny},
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
	setSurfaceFailModePolicyID(t, r, "surf-enf-norule", "fmp-enf-norule")

	res, err := newOrchestratorWithEvaluator(t, r, enforcedFailingEvaluator()).Evaluate(
		context.Background(),
		baseRequest("surf-enf-norule", "agent-1"),
		rawPayload(t),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	if res.Outcome != eval.OutcomeEscalate || res.ReasonCode != eval.ReasonPolicyError {
		t.Errorf("Outcome / ReasonCode must preserve FailModeClosed on missing-rule path; got %q / %q", res.Outcome, res.ReasonCode)
	}
	if len(triggerEvents(events)) != 1 {
		t.Errorf("trigger event must still fire on missing-rule path; got %d", len(triggerEvents(events)))
	}
	if len(enforcedEvents(events)) != 0 {
		t.Errorf("must not emit enforced event when rule is missing; got %d", len(enforcedEvents(events)))
	}
}

func TestEnforced_NoEnforcementWhenEvaluatorAllowed(t *testing.T) {
	// NoOp evaluator returns Allowed=true; trigger does not fire;
	// enforcement cannot apply.
	res, events := runEnforcedScenario(t,
		"surf-enf-allow", "test-policy",
		authority.FailModeClosed,
		policy.NoOpPolicyEvaluator{},
		enforcedResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
	)
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("Outcome must be Accept when evaluator allowed; got %q", res.Outcome)
	}
	if len(enforcedEvents(events)) != 0 {
		t.Errorf("must not emit enforced event when evaluator allowed; got %d", len(enforcedEvents(events)))
	}
}

func TestEnforced_NoEnforcementWhenEvaluatorDeniedWithoutError(t *testing.T) {
	res, events := runEnforcedScenario(t,
		"surf-enf-deny", "test-policy",
		authority.FailModeClosed,
		denyingPolicyEvaluator{},
		enforcedResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
	)
	if res.ReasonCode != eval.ReasonPolicyDeny {
		t.Errorf("ReasonCode must remain POLICY_DENY when evaluator denied; got %q", res.ReasonCode)
	}
	if len(enforcedEvents(events)) != 0 {
		t.Errorf("must not emit enforced event when evaluator denied without error; got %d", len(enforcedEvents(events)))
	}
}

// ---------------------------------------------------------------------------
// Authority.FailMode fallback — preserved exactly when gate is false
// ---------------------------------------------------------------------------

// TestEnforced_FailModeOpenFallback_UnchangedWithoutEnforcedRule
// pins that under FailModeOpen, the legacy Accept / WITHIN_AUTHORITY
// outcome is byte-identical to pre-D29f when the matched rule is
// evidence_only.
func TestEnforced_FailModeOpenFallback_UnchangedWithoutEnforcedRule(t *testing.T) {
	res, _ := runEnforcedScenario(t,
		"surf-enf-fbo-open", "test-policy",
		authority.FailModeOpen,
		enforcedFailingEvaluator(),
		// evidence_only resource rule → fallback path.
		[]failmode.FailModePolicyRule{
			{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		},
	)
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("FailModeOpen fallback must produce Accept; got %q", res.Outcome)
	}
	if res.ReasonCode != eval.ReasonWithinAuthority {
		t.Errorf("FailModeOpen fallback must produce WITHIN_AUTHORITY; got %q", res.ReasonCode)
	}
}

// TestEnforced_FailModeClosedFallback_UnchangedWithoutEnforcedRule
// pins that under FailModeClosed, the legacy Escalate / POLICY_ERROR
// outcome is byte-identical to pre-D29f when no enforced rule
// applies.
func TestEnforced_FailModeClosedFallback_UnchangedWithoutEnforcedRule(t *testing.T) {
	res, _ := runEnforcedScenario(t,
		"surf-enf-fbo-closed", "test-policy",
		authority.FailModeClosed,
		enforcedFailingEvaluator(),
		// evidence_only resource rule → fallback path.
		[]failmode.FailModePolicyRule{
			{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		},
	)
	if res.Outcome != eval.OutcomeEscalate {
		t.Errorf("FailModeClosed fallback must produce Escalate; got %q", res.Outcome)
	}
	if res.ReasonCode != eval.ReasonPolicyError {
		t.Errorf("FailModeClosed fallback must produce POLICY_ERROR; got %q", res.ReasonCode)
	}
}

// ---------------------------------------------------------------------------
// Payload — required + forbidden + contextual keys
// ---------------------------------------------------------------------------

func TestEnforced_PayloadKeys(t *testing.T) {
	_, events := runEnforcedScenario(t,
		"surf-enf-payload", "test-policy",
		authority.FailModeClosed,
		enforcedFailingEvaluator(),
		enforcedResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
	)
	enf := enforcedEvents(events)
	if len(enf) != 1 {
		t.Fatalf("expected exactly 1 enforced event; got %d", len(enf))
	}
	p := enf[0].Payload

	required := map[string]any{
		"fail_mode_policy_id":      "fmp-enforced",
		"fail_mode_policy_version": float64(11),
		"source":                   string(failmode.ResolutionSourceSurface),
		"trigger_condition":        string(failmode.FailModePolicyTriggerPolicyEvaluatorError),
		"correctness_class":        string(failmode.CorrectnessClassResource),
		"permitted_mode":           string(failmode.PermittedModeClosed),
		"enforcement_state":        string(failmode.EnforcementStateEnforced),
		"configured_outcome":       string(failmode.OutcomeDeny),
		"enforced_outcome":         string(eval.OutcomeReject),
		"enforced_reason_code":     string(eval.ReasonFailModePolicyDenied),
		"previous_outcome":         string(eval.OutcomeEscalate),
		"previous_reason_code":     string(eval.ReasonPolicyError),
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

	// applied_at / evaluation_time — RFC3339Nano strings.
	for _, k := range []string{"applied_at", "evaluation_time"} {
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

	// Contextual fields — must appear in the seeded scenario.
	for _, k := range []string{"surface_id", "surface_version", "authority_profile_id", "agent_id", "policy_reference"} {
		if _, present := p[k]; !present {
			t.Errorf("expected contextual payload key %q present in the seeded scenario", k)
		}
	}

	// Forbidden keys — must never appear on the enforced event.
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
		"dry_run",
		"dry_run_outcome",
		"applied",
		"triggered_at",    // exclusive to FAIL_MODE_POLICY_TRIGGER_FIRED
		"computed_at",     // exclusive to FAIL_MODE_POLICY_DRY_RUN_DECISION
		"actual_outcome",  // dry-run terminology
		"actual_reason_code",
		"divergent",
	} {
		if _, present := p[k]; present {
			t.Errorf("forbidden payload key %q must not appear; got %v", k, p[k])
		}
	}
}

// ---------------------------------------------------------------------------
// Event ordering — TRIGGER_FIRED < ENFORCED < OUTCOME_RECORDED
// ---------------------------------------------------------------------------

func TestEnforced_EventOrdering(t *testing.T) {
	_, events := runEnforcedScenario(t,
		"surf-enf-order", "test-policy",
		authority.FailModeClosed,
		enforcedFailingEvaluator(),
		enforcedResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
	)
	resolvedIdx := eventTypeIndex(events, audit.AuditEventFailModePolicyResolved)
	policyEvalIdx := eventTypeIndex(events, audit.AuditEventPolicyEvaluated)
	triggerIdx := eventTypeIndex(events, audit.AuditEventFailModePolicyTriggerFired)
	enforcedIdx := eventTypeIndex(events, audit.AuditEventFailModePolicyEnforced)
	outcomeIdx := eventTypeIndex(events, audit.AuditEventOutcomeRecorded)

	if resolvedIdx < 0 || policyEvalIdx < 0 || triggerIdx < 0 || enforcedIdx < 0 || outcomeIdx < 0 {
		t.Fatalf("missing required events: resolved=%d policyEval=%d trigger=%d enforced=%d outcome=%d",
			resolvedIdx, policyEvalIdx, triggerIdx, enforcedIdx, outcomeIdx)
	}
	if !(policyEvalIdx < triggerIdx) {
		t.Errorf("POLICY_EVALUATED (idx=%d) must come before FAIL_MODE_POLICY_TRIGGER_FIRED (idx=%d)",
			policyEvalIdx, triggerIdx)
	}
	if !(triggerIdx < enforcedIdx) {
		t.Errorf("FAIL_MODE_POLICY_TRIGGER_FIRED (idx=%d) must come before FAIL_MODE_POLICY_ENFORCED (idx=%d)",
			triggerIdx, enforcedIdx)
	}
	if !(enforcedIdx < outcomeIdx) {
		t.Errorf("FAIL_MODE_POLICY_ENFORCED (idx=%d) must come before OUTCOME_RECORDED (idx=%d)",
			enforcedIdx, outcomeIdx)
	}
	// Dry-run event must NOT appear when enforcement applies.
	if dryRunIdx := eventTypeIndex(events, audit.AuditEventFailModePolicyDryRunDecision); dryRunIdx >= 0 {
		t.Errorf("FAIL_MODE_POLICY_DRY_RUN_DECISION must not appear in enforced fixture; found at idx=%d", dryRunIdx)
	}
}

// ---------------------------------------------------------------------------
// Mutually exclusive — enforced and dry-run never coexist
// ---------------------------------------------------------------------------

func TestEnforced_DryRunAndEnforcedMutuallyExclusive(t *testing.T) {
	// Enforced fixture: must emit enforced, must NOT emit dry-run.
	_, enforcedEv := runEnforcedScenario(t,
		"surf-enf-mut-e", "test-policy",
		authority.FailModeClosed,
		enforcedFailingEvaluator(),
		enforcedResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
	)
	if len(enforcedEvents(enforcedEv)) != 1 {
		t.Errorf("enforced fixture must emit enforced; got %d", len(enforcedEvents(enforcedEv)))
	}
	if len(dryRunEvents(enforcedEv)) != 0 {
		t.Errorf("enforced fixture must NOT emit dry-run; got %d", len(dryRunEvents(enforcedEv)))
	}

	// Dry-run fixture: must emit dry-run, must NOT emit enforced.
	_, dryRunEv := runDryRunScenario(t,
		"surf-enf-mut-d", "test-policy",
		authority.FailModeClosed,
		failingEvaluator(),
		dryRunResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
	)
	if len(dryRunEvents(dryRunEv)) != 1 {
		t.Errorf("dry-run fixture must emit dry-run; got %d", len(dryRunEvents(dryRunEv)))
	}
	if len(enforcedEvents(dryRunEv)) != 0 {
		t.Errorf("dry-run fixture must NOT emit enforced; got %d", len(enforcedEvents(dryRunEv)))
	}
}

// ---------------------------------------------------------------------------
// OUTCOME_RECORDED matches enforced payload
// ---------------------------------------------------------------------------

func TestEnforced_OutcomeRecordedMatchesEnforcedPayload(t *testing.T) {
	cases := []struct {
		name        string
		fm          authority.FailMode
		posture     failmode.PermittedMode
		outcome     failmode.Outcome
		wantOutcome eval.Outcome
		wantReason  eval.ReasonCode
	}{
		{"closed_deny_closed", authority.FailModeClosed, failmode.PermittedModeClosed, failmode.OutcomeDeny, eval.OutcomeReject, eval.ReasonFailModePolicyDenied},
		{"closed_escalate_closed", authority.FailModeClosed, failmode.PermittedModeClosed, failmode.OutcomeEscalate, eval.OutcomeEscalate, eval.ReasonFailModePolicyEscalated},
		{"soft_permit_closed", authority.FailModeClosed, failmode.PermittedModeSoft, failmode.OutcomePermitWithEvidence, eval.OutcomeAccept, eval.ReasonFailModePolicyPermitWithEvidence},
		{"open_permit_open", authority.FailModeOpen, failmode.PermittedModeOpen, failmode.OutcomePermitWithEvidence, eval.OutcomeAccept, eval.ReasonFailModePolicyPermitWithEvidence},
		{"closed_manual_review", authority.FailModeClosed, failmode.PermittedModeClosed, failmode.OutcomeManualReview, eval.OutcomeEscalate, eval.ReasonFailModePolicyManualReview},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, events := runEnforcedScenario(t,
				"surf-enf-or-"+c.name, "test-policy",
				c.fm, enforcedFailingEvaluator(),
				enforcedResourceRules(c.posture, c.outcome),
			)
			enfOutcome := payloadValue(events, audit.AuditEventFailModePolicyEnforced, "enforced_outcome")
			enfReason := payloadValue(events, audit.AuditEventFailModePolicyEnforced, "enforced_reason_code")
			orOutcome := payloadValue(events, audit.AuditEventOutcomeRecorded, "outcome")
			orReason := payloadValue(events, audit.AuditEventOutcomeRecorded, "reason_code")
			if enfOutcome != string(c.wantOutcome) {
				t.Errorf("enforced_outcome: want %q, got %v", c.wantOutcome, enfOutcome)
			}
			if enfReason != string(c.wantReason) {
				t.Errorf("enforced_reason_code: want %q, got %v", c.wantReason, enfReason)
			}
			if orOutcome != enfOutcome {
				t.Errorf("OUTCOME_RECORDED.outcome (%v) must equal enforced_outcome (%v)", orOutcome, enfOutcome)
			}
			if orReason != enfReason {
				t.Errorf("OUTCOME_RECORDED.reason_code (%v) must equal enforced_reason_code (%v)", orReason, enfReason)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Reason-code isolation — conditional invariant (sibling to D29e test)
// ---------------------------------------------------------------------------

// TestEnforced_ReasonCodeLeak_AllowedOnlyWhenEnforcedEmitted is the
// D29f conditional invariant. D29e's TestDryRun_ReasonCodeIsolation
// asserts that OUTCOME_RECORDED.reason_code never has a
// FAIL_MODE_POLICY_* prefix for dry-run fixtures. D29f introduces the
// one exception: when FAIL_MODE_POLICY_ENFORCED is emitted in the
// same evaluation, OUTCOME_RECORDED.reason_code MUST begin with
// "FAIL_MODE_POLICY_" and MUST equal the enforced_reason_code from
// the enforced event payload.
func TestEnforced_ReasonCodeLeak_AllowedOnlyWhenEnforcedEmitted(t *testing.T) {
	scenarios := []struct {
		name    string
		fm      authority.FailMode
		posture failmode.PermittedMode
		outcome failmode.Outcome
	}{
		{"closed_enforced_deny_closed", authority.FailModeClosed, failmode.PermittedModeClosed, failmode.OutcomeDeny},
		{"closed_enforced_escalate_closed", authority.FailModeClosed, failmode.PermittedModeClosed, failmode.OutcomeEscalate},
		{"closed_enforced_manual_review_closed", authority.FailModeClosed, failmode.PermittedModeClosed, failmode.OutcomeManualReview},
		{"soft_enforced_permit_closed", authority.FailModeClosed, failmode.PermittedModeSoft, failmode.OutcomePermitWithEvidence},
		{"open_enforced_permit_open", authority.FailModeOpen, failmode.PermittedModeOpen, failmode.OutcomePermitWithEvidence},
		{"closed_enforced_deny_open", authority.FailModeOpen, failmode.PermittedModeClosed, failmode.OutcomeDeny},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			_, events := runEnforcedScenario(t,
				"surf-enf-leak-"+sc.name, "test-policy",
				sc.fm, enforcedFailingEvaluator(),
				enforcedResourceRules(sc.posture, sc.outcome),
			)
			if len(enforcedEvents(events)) != 1 {
				t.Fatalf("expected enforced event for %s; got %d", sc.name, len(enforcedEvents(events)))
			}
			rc := payloadValue(events, audit.AuditEventOutcomeRecorded, "reason_code")
			s, ok := rc.(string)
			if !ok {
				t.Fatalf("OUTCOME_RECORDED reason_code missing or non-string: %v (%T)", rc, rc)
			}
			if !strings.HasPrefix(s, "FAIL_MODE_POLICY_") {
				t.Errorf("OUTCOME_RECORDED reason_code must begin with FAIL_MODE_POLICY_ when enforced; got %q", s)
			}
			// And it must equal the enforced_reason_code from the
			// FAIL_MODE_POLICY_ENFORCED payload.
			enfReason := payloadValue(events, audit.AuditEventFailModePolicyEnforced, "enforced_reason_code")
			if rc != enfReason {
				t.Errorf("OUTCOME_RECORDED reason_code (%v) must equal enforced_reason_code (%v)", rc, enfReason)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HTTP response — no internal enforcement fields leak onto the wire
// ---------------------------------------------------------------------------

func TestEnforced_HTTPResponseNoEnforcedFieldLeakage(t *testing.T) {
	res, _ := runEnforcedScenario(t,
		"surf-enf-http", "test-policy",
		authority.FailModeClosed,
		enforcedFailingEvaluator(),
		enforcedResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
	)
	// reason_code may legitimately carry FAIL_MODE_POLICY_DENIED on
	// the wire here — that is the single allowed exposure per D29f.
	// Internal enforcement field names must NOT appear.
	if res.ReasonCode != eval.ReasonFailModePolicyDenied {
		t.Errorf("ReasonCode must be FAIL_MODE_POLICY_DENIED for enforced deny; got %q", res.ReasonCode)
	}

	// Build the same wire shape /v1/evaluate produces.
	type evaluateResponseShape struct {
		Outcome         string `json:"outcome"`
		Reason          string `json:"reason"`
		EnvelopeID      string `json:"envelope_id,omitempty"`
		Explanation     string `json:"explanation,omitempty"`
		Simulated       bool   `json:"simulated,omitempty"`
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
	// Forbidden substrings — none of these internal field names must
	// appear on the wire.
	for _, sub := range []string{
		"FAIL_MODE_POLICY_ENFORCED",
		"enforced_outcome",
		"enforced_reason_code",
		"previous_outcome",
		"previous_reason_code",
		"configured_outcome",
		"enforcement_state",
		"permitted_mode",
		"trigger_condition",
		"applied_at",
		"correctness_class",
		"fail_mode_policy_id",
		"fail_mode_policy_version",
	} {
		if strings.Contains(bodyStr, sub) {
			t.Errorf("/v1/evaluate response must not contain %q; body=%s", sub, bodyStr)
		}
	}
}

// TestEnforced_HTTPResponseShape_UnchangedFromPreD29f pins that the
// /v1/evaluate response shape (set of fields, field names) is
// identical between an evidence_only fixture (legacy path) and an
// enforced fixture (D29f path). Only the values of outcome and
// reason_code differ; no new field appears.
func TestEnforced_HTTPResponseShape_UnchangedFromPreD29f(t *testing.T) {
	// Legacy path: evidence_only fixture, FailModeClosed →
	// Escalate / POLICY_ERROR.
	legacyRes, _ := runEnforcedScenario(t,
		"surf-enf-shape-legacy", "test-policy",
		authority.FailModeClosed,
		enforcedFailingEvaluator(),
		[]failmode.FailModePolicyRule{
			{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		},
	)
	// Enforced path: same fixture but resource rule is enforced+deny.
	enforcedRes, _ := runEnforcedScenario(t,
		"surf-enf-shape-enforced", "test-policy",
		authority.FailModeClosed,
		enforcedFailingEvaluator(),
		enforcedResourceRules(failmode.PermittedModeClosed, failmode.OutcomeDeny),
	)

	// Serialise both via the same wire shape and confirm both produce
	// JSON objects with exactly the same set of keys.
	type evaluateResponseShape struct {
		Outcome         string `json:"outcome"`
		Reason          string `json:"reason"`
		EnvelopeID      string `json:"envelope_id,omitempty"`
		Explanation     string `json:"explanation,omitempty"`
		Simulated       bool   `json:"simulated,omitempty"`
		PolicyMode      string `json:"policy_mode,omitempty"`
		PolicyReference string `json:"policy_reference,omitempty"`
		PolicySkipped   bool   `json:"policy_skipped,omitempty"`
	}
	marshal := func(r decision.EvaluationResult) map[string]any {
		raw, err := json.Marshal(evaluateResponseShape{
			Outcome:         string(r.Outcome),
			Reason:          string(r.ReasonCode),
			EnvelopeID:      r.EnvelopeID,
			Explanation:     r.Explanation,
			PolicyMode:      r.PolicyMode,
			PolicyReference: r.PolicyReference,
			PolicySkipped:   r.PolicySkipped,
		})
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		return m
	}
	legacyMap := marshal(legacyRes)
	enforcedMap := marshal(enforcedRes)

	// Strip per-run identifier so we compare the field-name sets only.
	for _, k := range []string{"envelope_id"} {
		delete(legacyMap, k)
		delete(enforcedMap, k)
	}
	legacyKeys := make(map[string]struct{}, len(legacyMap))
	for k := range legacyMap {
		legacyKeys[k] = struct{}{}
	}
	enforcedKeys := make(map[string]struct{}, len(enforcedMap))
	for k := range enforcedMap {
		enforcedKeys[k] = struct{}{}
	}
	for k := range legacyKeys {
		if _, ok := enforcedKeys[k]; !ok {
			t.Errorf("legacy response field %q absent from enforced response", k)
		}
	}
	for k := range enforcedKeys {
		if _, ok := legacyKeys[k]; !ok {
			t.Errorf("enforced response added new field %q not in legacy response", k)
		}
	}
}
