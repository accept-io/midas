package decision_test

// orchestrator_failmode_authority_test.go — D29j pins.
//
// Asserts the FailModePolicy trigger wiring for the
// authority_resolution_failure trigger condition, covering all three
// deterministic authority-chain Reject outcomes:
//
//   - NO_ACTIVE_GRANT                 (agent has no grants at all)
//   - PROFILE_NOT_FOUND               (grants exist, no active profile)
//   - GRANT_PROFILE_SURFACE_MISMATCH  (active profile, wrong surface)
//
// Coverage:
//
//   - For each of the three failure cases × each of the four
//     enforcement states (no-policy, evidence_only, dry_run, enforced):
//     the correct combination of FAIL_MODE_POLICY_* events is emitted
//     and OUTCOME_RECORDED carries the correct outcome / reason_code.
//   - Trigger payload pin: required fields, RFC3339Nano timestamps,
//     forbidden keys, and the authority-failure-specific forbidden
//     fields (authority_profile_id, policy_reference — profile is not
//     resolved on this path so these must not appear).
//   - Event ordering pin: FAIL_MODE_POLICY_RESOLVED comes before
//     FAIL_MODE_POLICY_TRIGGER_FIRED comes before
//     (DRY_RUN_DECISION | ENFORCED) comes before OUTCOME_RECORDED.
//     POLICY_EVALUATED and AUTHORITY_CHAIN_RESOLVED must NOT appear.
//   - Mutual exclusivity pin: dry_run and enforced events never coexist.
//   - Reason-code isolation conditional invariant: OUTCOME_RECORDED's
//     reason_code begins with "FAIL_MODE_POLICY_" iff
//     FAIL_MODE_POLICY_ENFORCED is emitted in the same evaluation.
//   - HTTP-response invariance pin: /v1/evaluate response shape does
//     not leak any FAIL_MODE_POLICY_ENFORCED internal payload field
//     names onto the wire; reason_code may legitimately carry
//     FAIL_MODE_POLICY_* values on the enforced path (the single
//     allowed exposure per D29f).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/failmode"
)

// ---------------------------------------------------------------------------
// Authority-failure fixtures — three deterministic Reject paths
// ---------------------------------------------------------------------------

// authorityFailureCase describes a single deterministic Reject path
// from resolveAuthorityChain. The seedFn must produce that exact
// reason code; wantReason is the assertion target for tests.
type authorityFailureCase struct {
	name       string
	seedFn     func(t *testing.T, r testRepos, surfaceID, agentID string)
	wantReason eval.ReasonCode
}

// authorityFailureCases enumerates the D29j coverage matrix. Each case
// arranges the repos so that resolveAuthorityChain returns
// (eval.OutcomeReject, wantReason).
func authorityFailureCases() []authorityFailureCase {
	return []authorityFailureCase{
		{
			name: "no_active_grant",
			// Agent exists but has zero grants.
			seedFn: func(t *testing.T, r testRepos, surfaceID, agentID string) {
				t.Helper()
				seedActiveSurface(t, r, surfaceID)
				seedAgent(t, r, agentID)
				// no seedActiveGrant
			},
			wantReason: eval.ReasonNoActiveGrant,
		},
		{
			name: "profile_not_found",
			// Active grant points to a profile ID that was never
			// created. profiles.FindActiveAt returns (nil, nil), the
			// for-loop's foundProfile stays false, and the resolver
			// returns PROFILE_NOT_FOUND.
			seedFn: func(t *testing.T, r testRepos, surfaceID, agentID string) {
				t.Helper()
				seedActiveSurface(t, r, surfaceID)
				seedAgent(t, r, agentID)
				seedActiveGrant(t, r, "grant-1", agentID, "prof-missing")
			},
			wantReason: eval.ReasonProfileNotFound,
		},
		{
			name: "grant_profile_surface_mismatch",
			// Active grant points to an active profile, but that
			// profile is scoped to a different surface. The resolver
			// sets foundProfile=true (active+exists) but skips because
			// of surface mismatch, then returns
			// GRANT_PROFILE_SURFACE_MISMATCH.
			seedFn: func(t *testing.T, r testRepos, surfaceID, agentID string) {
				t.Helper()
				seedActiveSurface(t, r, surfaceID)
				// The mismatched profile must reference a real surface
				// so that nothing trips on missing surface; we use a
				// distinct ID and seed it.
				seedActiveSurface(t, r, surfaceID+"-other")
				seedAgent(t, r, agentID)
				seedProfile(t, r, "prof-other", surfaceID+"-other")
				seedActiveGrant(t, r, "grant-1", agentID, "prof-other")
			},
			wantReason: eval.ReasonGrantProfileSurfaceMismatch,
		},
	}
}

// runAuthorityScenario seeds the failure fixture, optionally attaches a
// FailModePolicy with the supplied rules to the request surface, and
// runs Evaluate against a NoOp evaluator (the policy step is never
// reached on the authority-failure path, so the evaluator choice is
// immaterial).
func runAuthorityScenario(
	t *testing.T,
	surfaceID string,
	c authorityFailureCase,
	rules []failmode.FailModePolicyRule,
	policyID string,
	policyVersion int,
) (decision.EvaluationResult, []*audit.AuditEvent) {
	t.Helper()
	r := newRepos()
	c.seedFn(t, r, surfaceID, "agent-1")
	if rules != nil {
		seedFailModePolicyWithRules(t, r, policyID, policyVersion, rules)
		setSurfaceFailModePolicyID(t, r, surfaceID, policyID)
	}
	res, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		baseRequest(surfaceID, "agent-1"),
		rawPayload(t),
	)
	if err != nil {
		t.Fatalf("Evaluate (%s): %v", c.name, err)
	}
	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID (%s): %v", c.name, err)
	}
	return res, events
}

// authorityResourceRules returns five-rule coverage with the
// resource-class rule carrying the supplied posture / enforcement /
// outcome triple. Other classes use the safe closed+evidence_only+
// escalate defaults.
func authorityResourceRules(
	posture failmode.PermittedMode,
	state failmode.EnforcementState,
	outcome failmode.Outcome,
) []failmode.FailModePolicyRule {
	return []failmode.FailModePolicyRule{
		{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: posture, EnforcementState: state, Outcome: outcome},
		{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
	}
}

func authorityTriggerEvents(events []*audit.AuditEvent) []*audit.AuditEvent {
	out := make([]*audit.AuditEvent, 0, 1)
	for _, e := range events {
		if e.EventType == audit.AuditEventFailModePolicyTriggerFired {
			out = append(out, e)
		}
	}
	return out
}

func authorityEnforcedEvents(events []*audit.AuditEvent) []*audit.AuditEvent {
	out := make([]*audit.AuditEvent, 0, 1)
	for _, e := range events {
		if e.EventType == audit.AuditEventFailModePolicyEnforced {
			out = append(out, e)
		}
	}
	return out
}

func authorityDryRunEvents(events []*audit.AuditEvent) []*audit.AuditEvent {
	out := make([]*audit.AuditEvent, 0, 1)
	for _, e := range events {
		if e.EventType == audit.AuditEventFailModePolicyDryRunDecision {
			out = append(out, e)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// No FailModePolicy resolved — authority Reject preserved exactly
// ---------------------------------------------------------------------------

func TestAuthorityFailure_NoPolicy_PreservesRejectAndEmitsNoFailModeEvents(t *testing.T) {
	for _, c := range authorityFailureCases() {
		t.Run(c.name, func(t *testing.T) {
			res, events := runAuthorityScenario(t,
				"surf-auth-nop-"+c.name, c, nil, "", 0,
			)
			if res.Outcome != eval.OutcomeReject {
				t.Errorf("Outcome: want Reject, got %q", res.Outcome)
			}
			if res.ReasonCode != c.wantReason {
				t.Errorf("ReasonCode: want %q, got %q", c.wantReason, res.ReasonCode)
			}
			if got := len(authorityTriggerEvents(events)); got != 0 {
				t.Errorf("must not emit trigger event when no policy resolved; got %d", got)
			}
			if got := len(authorityDryRunEvents(events)); got != 0 {
				t.Errorf("must not emit dry-run event when no policy resolved; got %d", got)
			}
			if got := len(authorityEnforcedEvents(events)); got != 0 {
				t.Errorf("must not emit enforced event when no policy resolved; got %d", got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// evidence_only — trigger fires; outcome unchanged
// ---------------------------------------------------------------------------

func TestAuthorityFailure_EvidenceOnly_FiresTriggerOnlyAndPreservesReject(t *testing.T) {
	for _, c := range authorityFailureCases() {
		t.Run(c.name, func(t *testing.T) {
			res, events := runAuthorityScenario(t,
				"surf-auth-eo-"+c.name, c,
				authorityResourceRules(
					failmode.PermittedModeClosed,
					failmode.EnforcementStateEvidenceOnly,
					failmode.OutcomeEscalate,
				),
				"fmp-auth-eo-"+c.name, 1,
			)
			if res.Outcome != eval.OutcomeReject {
				t.Errorf("Outcome: want Reject (preserved), got %q", res.Outcome)
			}
			if res.ReasonCode != c.wantReason {
				t.Errorf("ReasonCode: want %q (preserved), got %q", c.wantReason, res.ReasonCode)
			}
			if got := len(authorityTriggerEvents(events)); got != 1 {
				t.Errorf("expected exactly 1 trigger event; got %d", got)
			}
			if got := len(authorityDryRunEvents(events)); got != 0 {
				t.Errorf("must not emit dry-run under evidence_only; got %d", got)
			}
			if got := len(authorityEnforcedEvents(events)); got != 0 {
				t.Errorf("must not emit enforced under evidence_only; got %d", got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// dry_run — trigger + dry-run fire; outcome unchanged
// ---------------------------------------------------------------------------

func TestAuthorityFailure_DryRun_FiresEvidenceAndPreservesReject(t *testing.T) {
	for _, c := range authorityFailureCases() {
		t.Run(c.name, func(t *testing.T) {
			res, events := runAuthorityScenario(t,
				"surf-auth-dr-"+c.name, c,
				authorityResourceRules(
					failmode.PermittedModeClosed,
					failmode.EnforcementStateDryRun,
					failmode.OutcomeDeny,
				),
				"fmp-auth-dr-"+c.name, 2,
			)
			// Reject preserved despite dry-run deny.
			if res.Outcome != eval.OutcomeReject {
				t.Errorf("Outcome: want Reject (preserved), got %q", res.Outcome)
			}
			if res.ReasonCode != c.wantReason {
				t.Errorf("ReasonCode: want %q (preserved), got %q", c.wantReason, res.ReasonCode)
			}
			if got := len(authorityTriggerEvents(events)); got != 1 {
				t.Errorf("expected trigger event; got %d", got)
			}
			if got := len(authorityDryRunEvents(events)); got != 1 {
				t.Errorf("expected exactly 1 dry-run event; got %d", got)
			}
			if got := len(authorityEnforcedEvents(events)); got != 0 {
				t.Errorf("must not emit enforced under dry_run; got %d", got)
			}

			// dry_run_outcome / dry_run_reason_code pin: the rule
			// configures deny → maps to Reject + FAIL_MODE_POLICY_DENIED.
			if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "dry_run_outcome"); got != string(eval.OutcomeReject) {
				t.Errorf("dry_run_outcome: want %q, got %v", eval.OutcomeReject, got)
			}
			if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "dry_run_reason_code"); got != string(eval.ReasonFailModePolicyDenied) {
				t.Errorf("dry_run_reason_code: want %q, got %v", eval.ReasonFailModePolicyDenied, got)
			}
			// actual_outcome / actual_reason_code pin the preserved Reject.
			if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "actual_outcome"); got != string(eval.OutcomeReject) {
				t.Errorf("actual_outcome: want %q, got %v", eval.OutcomeReject, got)
			}
			if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "actual_reason_code"); got != string(c.wantReason) {
				t.Errorf("actual_reason_code: want %q, got %v", c.wantReason, got)
			}
			// trigger_condition pin on both events.
			if got := payloadValue(events, audit.AuditEventFailModePolicyTriggerFired, "trigger_condition"); got != string(failmode.FailModePolicyTriggerAuthorityResolutionFailure) {
				t.Errorf("trigger.trigger_condition: want %q, got %v",
					failmode.FailModePolicyTriggerAuthorityResolutionFailure, got)
			}
			if got := payloadValue(events, audit.AuditEventFailModePolicyDryRunDecision, "trigger_condition"); got != string(failmode.FailModePolicyTriggerAuthorityResolutionFailure) {
				t.Errorf("dry_run.trigger_condition: want %q, got %v",
					failmode.FailModePolicyTriggerAuthorityResolutionFailure, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// enforced — outcome overridden per rule
// ---------------------------------------------------------------------------

// TestAuthorityFailure_Enforced_DenyKeepsReject pins that enforced
// deny under authority resolution failure changes the reason_code to
// FAIL_MODE_POLICY_DENIED while keeping Outcome = Reject (the rule's
// configured outcome happens to match the authority-chain outcome).
func TestAuthorityFailure_Enforced_DenyKeepsReject(t *testing.T) {
	for _, c := range authorityFailureCases() {
		t.Run(c.name, func(t *testing.T) {
			res, events := runAuthorityScenario(t,
				"surf-auth-en-deny-"+c.name, c,
				authorityResourceRules(
					failmode.PermittedModeClosed,
					failmode.EnforcementStateEnforced,
					failmode.OutcomeDeny,
				),
				"fmp-auth-en-deny-"+c.name, 3,
			)
			if res.Outcome != eval.OutcomeReject {
				t.Errorf("Outcome: want Reject (enforced deny), got %q", res.Outcome)
			}
			if res.ReasonCode != eval.ReasonFailModePolicyDenied {
				t.Errorf("ReasonCode: want %q, got %q",
					eval.ReasonFailModePolicyDenied, res.ReasonCode)
			}
			if got := len(authorityEnforcedEvents(events)); got != 1 {
				t.Errorf("expected exactly 1 enforced event; got %d", got)
			}
			if got := len(authorityDryRunEvents(events)); got != 0 {
				t.Errorf("must not emit dry-run on enforced path; got %d", got)
			}
			// previous_outcome / previous_reason_code capture the
			// authority-chain Reject as the counterfactual.
			if got := payloadValue(events, audit.AuditEventFailModePolicyEnforced, "previous_outcome"); got != string(eval.OutcomeReject) {
				t.Errorf("previous_outcome: want %q, got %v", eval.OutcomeReject, got)
			}
			if got := payloadValue(events, audit.AuditEventFailModePolicyEnforced, "previous_reason_code"); got != string(c.wantReason) {
				t.Errorf("previous_reason_code: want %q, got %v", c.wantReason, got)
			}
		})
	}
}

// TestAuthorityFailure_Enforced_EscalateChangesOutcome pins that
// enforced escalate flips the runtime decision from Reject to
// Escalate — the only enforcement combination where the operator-
// visible Outcome value changes on this path.
func TestAuthorityFailure_Enforced_EscalateChangesOutcome(t *testing.T) {
	for _, c := range authorityFailureCases() {
		t.Run(c.name, func(t *testing.T) {
			res, events := runAuthorityScenario(t,
				"surf-auth-en-esc-"+c.name, c,
				authorityResourceRules(
					failmode.PermittedModeClosed,
					failmode.EnforcementStateEnforced,
					failmode.OutcomeEscalate,
				),
				"fmp-auth-en-esc-"+c.name, 4,
			)
			if res.Outcome != eval.OutcomeEscalate {
				t.Errorf("Outcome: want Escalate (enforced escalate), got %q", res.Outcome)
			}
			if res.ReasonCode != eval.ReasonFailModePolicyEscalated {
				t.Errorf("ReasonCode: want %q, got %q",
					eval.ReasonFailModePolicyEscalated, res.ReasonCode)
			}
			if got := len(authorityEnforcedEvents(events)); got != 1 {
				t.Errorf("expected enforced event; got %d", got)
			}
		})
	}
}

// TestAuthorityFailure_Enforced_PermitWithEvidenceOverridesToAccept
// pins the operator-surprise case: enforced permit_with_evidence on
// the authority-failure path flips Reject to Accept regardless of
// which deterministic authority failure produced the original Reject.
func TestAuthorityFailure_Enforced_PermitWithEvidenceOverridesToAccept(t *testing.T) {
	for _, c := range authorityFailureCases() {
		t.Run(c.name, func(t *testing.T) {
			res, events := runAuthorityScenario(t,
				"surf-auth-en-perm-"+c.name, c,
				authorityResourceRules(
					failmode.PermittedModeSoft,
					failmode.EnforcementStateEnforced,
					failmode.OutcomePermitWithEvidence,
				),
				"fmp-auth-en-perm-"+c.name, 5,
			)
			if res.Outcome != eval.OutcomeAccept {
				t.Errorf("Outcome: want Accept (enforced permit_with_evidence overrides authority Reject), got %q",
					res.Outcome)
			}
			if res.ReasonCode != eval.ReasonFailModePolicyPermitWithEvidence {
				t.Errorf("ReasonCode: want %q, got %q",
					eval.ReasonFailModePolicyPermitWithEvidence, res.ReasonCode)
			}
			if got := len(authorityEnforcedEvents(events)); got != 1 {
				t.Errorf("expected enforced event; got %d", got)
			}
		})
	}
}

// TestAuthorityFailure_Enforced_ManualReviewMapsToEscalate pins that
// manual_review maps to eval.OutcomeEscalate while preserving the
// FAIL_MODE_POLICY_MANUAL_REVIEW reason code.
func TestAuthorityFailure_Enforced_ManualReviewMapsToEscalate(t *testing.T) {
	for _, c := range authorityFailureCases() {
		t.Run(c.name, func(t *testing.T) {
			res, events := runAuthorityScenario(t,
				"surf-auth-en-mr-"+c.name, c,
				authorityResourceRules(
					failmode.PermittedModeClosed,
					failmode.EnforcementStateEnforced,
					failmode.OutcomeManualReview,
				),
				"fmp-auth-en-mr-"+c.name, 6,
			)
			if res.Outcome != eval.OutcomeEscalate {
				t.Errorf("Outcome: want Escalate (manual_review), got %q", res.Outcome)
			}
			if res.ReasonCode != eval.ReasonFailModePolicyManualReview {
				t.Errorf("ReasonCode: want %q, got %q",
					eval.ReasonFailModePolicyManualReview, res.ReasonCode)
			}
			if got := len(authorityEnforcedEvents(events)); got != 1 {
				t.Errorf("expected enforced event; got %d", got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Trigger payload pin — required + forbidden keys
// ---------------------------------------------------------------------------

// TestAuthorityFailure_TriggerPayloadKeys pins the trigger-fired event
// payload for the authority-failure path. Required keys mirror D29c-2
// plus the authority-failure trigger condition. Profile-derived
// payload fields (authority_profile_id, policy_reference) MUST be
// absent — the profile is not resolved on this path.
func TestAuthorityFailure_TriggerPayloadKeys(t *testing.T) {
	c := authorityFailureCases()[0] // no_active_grant suffices for payload shape.
	_, events := runAuthorityScenario(t,
		"surf-auth-trigger-payload", c,
		authorityResourceRules(
			failmode.PermittedModeClosed,
			failmode.EnforcementStateEvidenceOnly,
			failmode.OutcomeEscalate,
		),
		"fmp-auth-trigger-payload", 7,
	)
	tr := authorityTriggerEvents(events)
	if len(tr) != 1 {
		t.Fatalf("expected exactly 1 trigger event; got %d", len(tr))
	}
	p := tr[0].Payload

	required := map[string]any{
		"fail_mode_policy_id":      "fmp-auth-trigger-payload",
		"fail_mode_policy_version": float64(7),
		"source":                   string(failmode.ResolutionSourceSurface),
		"trigger_condition":        string(failmode.FailModePolicyTriggerAuthorityResolutionFailure),
		"correctness_class":        string(failmode.CorrectnessClassResource),
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

	// permitted_mode / enforcement_state / outcome — present when a
	// rule was found (the seeded policy has a resource rule).
	for _, k := range []string{"permitted_mode", "enforcement_state", "outcome"} {
		v, present := p[k]
		if !present {
			t.Errorf("required payload key %q missing", k)
			continue
		}
		if s, ok := v.(string); !ok || s == "" {
			t.Errorf("payload %q: must be non-empty string, got %v", k, v)
		}
	}

	// triggered_at / evaluation_time — RFC3339Nano strings.
	for _, k := range []string{"triggered_at", "evaluation_time"} {
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

	// Contextual fields that must appear: agent and surface
	// resolved before the authority chain.
	for _, k := range []string{"surface_id", "surface_version", "agent_id"} {
		if _, present := p[k]; !present {
			t.Errorf("expected contextual payload key %q present", k)
		}
	}
	if got := p["agent_id"]; got != "agent-1" {
		t.Errorf("agent_id: want %q, got %v", "agent-1", got)
	}

	// Forbidden keys — including the authority-failure-specific
	// ones: profile is nil on this path so authority_profile_id and
	// policy_reference must not appear.
	for _, k := range []string{
		"rules",
		"raw_policy_error",
		"error",
		"stack_trace",
		"sql",
		"dsn",
		"degraded",
		"allowed",
		"fail_open",
		"fail_soft",
		"decision_changed",
		"dry_run",
		"enforced",
		"authority_profile_id",
		"policy_reference",
	} {
		if _, present := p[k]; present {
			t.Errorf("forbidden payload key %q must not appear; got %v", k, p[k])
		}
	}
}

// ---------------------------------------------------------------------------
// Event ordering — RESOLVED → TRIGGER_FIRED → (DRY_RUN | ENFORCED) →
// OUTCOME_RECORDED. POLICY_EVALUATED and AUTHORITY_CHAIN_RESOLVED must
// NOT appear.
// ---------------------------------------------------------------------------

func TestAuthorityFailure_EventOrdering_EvidenceOnly(t *testing.T) {
	c := authorityFailureCases()[0]
	_, events := runAuthorityScenario(t,
		"surf-auth-order-eo", c,
		authorityResourceRules(
			failmode.PermittedModeClosed,
			failmode.EnforcementStateEvidenceOnly,
			failmode.OutcomeEscalate,
		),
		"fmp-auth-order-eo", 8,
	)
	resolvedIdx := eventTypeIndex(events, audit.AuditEventFailModePolicyResolved)
	triggerIdx := eventTypeIndex(events, audit.AuditEventFailModePolicyTriggerFired)
	outcomeIdx := eventTypeIndex(events, audit.AuditEventOutcomeRecorded)
	if resolvedIdx < 0 || triggerIdx < 0 || outcomeIdx < 0 {
		t.Fatalf("missing required events: resolved=%d trigger=%d outcome=%d",
			resolvedIdx, triggerIdx, outcomeIdx)
	}
	if !(resolvedIdx < triggerIdx) {
		t.Errorf("FAIL_MODE_POLICY_RESOLVED (%d) must precede TRIGGER_FIRED (%d)",
			resolvedIdx, triggerIdx)
	}
	if !(triggerIdx < outcomeIdx) {
		t.Errorf("TRIGGER_FIRED (%d) must precede OUTCOME_RECORDED (%d)",
			triggerIdx, outcomeIdx)
	}
	// Must NOT appear on authority-failure path.
	if idx := eventTypeIndex(events, audit.AuditEventPolicyEvaluated); idx >= 0 {
		t.Errorf("POLICY_EVALUATED must not appear on authority-failure path; found at %d", idx)
	}
	if idx := eventTypeIndex(events, audit.AuditEventAuthorityChainResolved); idx >= 0 {
		t.Errorf("AUTHORITY_CHAIN_RESOLVED must not appear on authority-failure path; found at %d", idx)
	}
}

func TestAuthorityFailure_EventOrdering_Enforced(t *testing.T) {
	c := authorityFailureCases()[0]
	_, events := runAuthorityScenario(t,
		"surf-auth-order-en", c,
		authorityResourceRules(
			failmode.PermittedModeClosed,
			failmode.EnforcementStateEnforced,
			failmode.OutcomeDeny,
		),
		"fmp-auth-order-en", 9,
	)
	resolvedIdx := eventTypeIndex(events, audit.AuditEventFailModePolicyResolved)
	triggerIdx := eventTypeIndex(events, audit.AuditEventFailModePolicyTriggerFired)
	enforcedIdx := eventTypeIndex(events, audit.AuditEventFailModePolicyEnforced)
	outcomeIdx := eventTypeIndex(events, audit.AuditEventOutcomeRecorded)
	if resolvedIdx < 0 || triggerIdx < 0 || enforcedIdx < 0 || outcomeIdx < 0 {
		t.Fatalf("missing required events: resolved=%d trigger=%d enforced=%d outcome=%d",
			resolvedIdx, triggerIdx, enforcedIdx, outcomeIdx)
	}
	if !(resolvedIdx < triggerIdx && triggerIdx < enforcedIdx && enforcedIdx < outcomeIdx) {
		t.Errorf("event ordering violated: resolved=%d trigger=%d enforced=%d outcome=%d",
			resolvedIdx, triggerIdx, enforcedIdx, outcomeIdx)
	}
	if idx := eventTypeIndex(events, audit.AuditEventFailModePolicyDryRunDecision); idx >= 0 {
		t.Errorf("DRY_RUN_DECISION must not coexist with ENFORCED; found at %d", idx)
	}
	if idx := eventTypeIndex(events, audit.AuditEventPolicyEvaluated); idx >= 0 {
		t.Errorf("POLICY_EVALUATED must not appear on authority-failure path; found at %d", idx)
	}
	if idx := eventTypeIndex(events, audit.AuditEventAuthorityChainResolved); idx >= 0 {
		t.Errorf("AUTHORITY_CHAIN_RESOLVED must not appear on authority-failure path; found at %d", idx)
	}
}

// ---------------------------------------------------------------------------
// Mutual exclusivity — enforced and dry-run never coexist
// ---------------------------------------------------------------------------

func TestAuthorityFailure_DryRunAndEnforcedMutuallyExclusive(t *testing.T) {
	c := authorityFailureCases()[0]

	// Enforced fixture: enforced present, dry-run absent.
	_, enforcedEv := runAuthorityScenario(t,
		"surf-auth-mut-e", c,
		authorityResourceRules(
			failmode.PermittedModeClosed,
			failmode.EnforcementStateEnforced,
			failmode.OutcomeDeny,
		),
		"fmp-auth-mut-e", 10,
	)
	if len(authorityEnforcedEvents(enforcedEv)) != 1 {
		t.Errorf("enforced fixture must emit enforced; got %d", len(authorityEnforcedEvents(enforcedEv)))
	}
	if len(authorityDryRunEvents(enforcedEv)) != 0 {
		t.Errorf("enforced fixture must NOT emit dry-run; got %d", len(authorityDryRunEvents(enforcedEv)))
	}

	// Dry-run fixture: dry-run present, enforced absent.
	_, dryRunEv := runAuthorityScenario(t,
		"surf-auth-mut-d", c,
		authorityResourceRules(
			failmode.PermittedModeClosed,
			failmode.EnforcementStateDryRun,
			failmode.OutcomeDeny,
		),
		"fmp-auth-mut-d", 11,
	)
	if len(authorityDryRunEvents(dryRunEv)) != 1 {
		t.Errorf("dry-run fixture must emit dry-run; got %d", len(authorityDryRunEvents(dryRunEv)))
	}
	if len(authorityEnforcedEvents(dryRunEv)) != 0 {
		t.Errorf("dry-run fixture must NOT emit enforced; got %d", len(authorityEnforcedEvents(dryRunEv)))
	}
}

// ---------------------------------------------------------------------------
// Reason-code isolation — conditional invariant
// ---------------------------------------------------------------------------

// TestAuthorityFailure_ReasonCodeLeak_AllowedOnlyWhenEnforcedEmitted
// pins that OUTCOME_RECORDED.reason_code begins with "FAIL_MODE_POLICY_"
// IFF FAIL_MODE_POLICY_ENFORCED is emitted in the same evaluation.
// On evidence_only / dry_run / no-policy paths, the reason_code must
// be one of the authority-chain values and must NOT carry the
// FAIL_MODE_POLICY_ prefix.
func TestAuthorityFailure_ReasonCodeLeak_AllowedOnlyWhenEnforcedEmitted(t *testing.T) {
	scenarios := []struct {
		name      string
		rules     []failmode.FailModePolicyRule
		wantLeak  bool
		wantReason eval.ReasonCode
	}{
		{
			name:       "evidence_only_no_leak",
			rules:      authorityResourceRules(failmode.PermittedModeClosed, failmode.EnforcementStateEvidenceOnly, failmode.OutcomeEscalate),
			wantLeak:   false,
			wantReason: eval.ReasonNoActiveGrant,
		},
		{
			name:       "dry_run_no_leak",
			rules:      authorityResourceRules(failmode.PermittedModeClosed, failmode.EnforcementStateDryRun, failmode.OutcomeDeny),
			wantLeak:   false,
			wantReason: eval.ReasonNoActiveGrant,
		},
		{
			name:       "enforced_deny_leak",
			rules:      authorityResourceRules(failmode.PermittedModeClosed, failmode.EnforcementStateEnforced, failmode.OutcomeDeny),
			wantLeak:   true,
			wantReason: eval.ReasonFailModePolicyDenied,
		},
		{
			name:       "enforced_escalate_leak",
			rules:      authorityResourceRules(failmode.PermittedModeClosed, failmode.EnforcementStateEnforced, failmode.OutcomeEscalate),
			wantLeak:   true,
			wantReason: eval.ReasonFailModePolicyEscalated,
		},
		{
			name:       "enforced_permit_leak",
			rules:      authorityResourceRules(failmode.PermittedModeSoft, failmode.EnforcementStateEnforced, failmode.OutcomePermitWithEvidence),
			wantLeak:   true,
			wantReason: eval.ReasonFailModePolicyPermitWithEvidence,
		},
	}
	c := authorityFailureCases()[0] // no_active_grant
	for i, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			res, events := runAuthorityScenario(t,
				"surf-auth-leak-"+sc.name, c, sc.rules,
				"fmp-auth-leak-"+sc.name, 20+i,
			)
			rc := payloadValue(events, audit.AuditEventOutcomeRecorded, "reason_code")
			s, ok := rc.(string)
			if !ok {
				t.Fatalf("OUTCOME_RECORDED reason_code missing or non-string: %v (%T)", rc, rc)
			}
			hasPrefix := strings.HasPrefix(s, "FAIL_MODE_POLICY_")
			if hasPrefix != sc.wantLeak {
				t.Errorf("reason_code FAIL_MODE_POLICY_ prefix: want %v, got %v (reason=%q)",
					sc.wantLeak, hasPrefix, s)
			}
			if eval.ReasonCode(s) != sc.wantReason {
				t.Errorf("reason_code: want %q, got %q", sc.wantReason, s)
			}

			// Sanity: leak case must emit enforced; non-leak must not.
			gotEnforced := len(authorityEnforcedEvents(events))
			if sc.wantLeak && gotEnforced != 1 {
				t.Errorf("leak case must emit enforced; got %d", gotEnforced)
			}
			if !sc.wantLeak && gotEnforced != 0 {
				t.Errorf("non-leak case must NOT emit enforced; got %d", gotEnforced)
			}

			// And on the leak path, OUTCOME_RECORDED's reason_code
			// must equal the enforced event's enforced_reason_code.
			if sc.wantLeak {
				enfReason := payloadValue(events, audit.AuditEventFailModePolicyEnforced, "enforced_reason_code")
				if enfReason != rc {
					t.Errorf("OUTCOME_RECORDED reason_code (%v) must equal enforced_reason_code (%v)",
						rc, enfReason)
				}
			}

			// Also re-verify the EvaluationResult mirrors the
			// OUTCOME_RECORDED audit row.
			if string(res.ReasonCode) != s {
				t.Errorf("EvaluationResult.ReasonCode (%q) must match OUTCOME_RECORDED.reason_code (%q)",
					res.ReasonCode, s)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HTTP-response invariance — no internal payload field names leak
// ---------------------------------------------------------------------------

// TestAuthorityFailure_HTTPResponseShape_NoLeakedFieldNames pins that
// the /v1/evaluate response body never carries FailModePolicy
// internal payload field names. reason_code may legitimately carry
// FAIL_MODE_POLICY_* values when enforcement applies (the single
// allowed exposure per D29f); all other internal field names must
// remain absent on the wire.
func TestAuthorityFailure_HTTPResponseShape_NoLeakedFieldNames(t *testing.T) {
	c := authorityFailureCases()[0]
	res, _ := runAuthorityScenario(t,
		"surf-auth-http", c,
		authorityResourceRules(
			failmode.PermittedModeClosed,
			failmode.EnforcementStateEnforced,
			failmode.OutcomeDeny,
		),
		"fmp-auth-http", 30,
	)
	if res.ReasonCode != eval.ReasonFailModePolicyDenied {
		t.Errorf("ReasonCode: want %q, got %q",
			eval.ReasonFailModePolicyDenied, res.ReasonCode)
	}

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
	for _, sub := range []string{
		"FAIL_MODE_POLICY_ENFORCED",
		"FAIL_MODE_POLICY_TRIGGER_FIRED",
		"FAIL_MODE_POLICY_RESOLVED",
		"enforced_outcome",
		"enforced_reason_code",
		"previous_outcome",
		"previous_reason_code",
		"configured_outcome",
		"enforcement_state",
		"permitted_mode",
		"trigger_condition",
		"applied_at",
		"triggered_at",
		"correctness_class",
		"fail_mode_policy_id",
		"fail_mode_policy_version",
		"authority_resolution_failure",
	} {
		if strings.Contains(bodyStr, sub) {
			t.Errorf("/v1/evaluate response must not contain %q; body=%s", sub, bodyStr)
		}
	}
}
