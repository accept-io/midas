package decision_test

// orchestrator_failmode_trigger_test.go — D29c-2 pins.
//
// Asserts:
//
//   - FAIL_MODE_POLICY_TRIGGER_FIRED is emitted ONLY when (a) a
//     FailModePolicy resolved successfully AND (b) the policy evaluator
//     returned an error.
//   - The payload carries exactly the brief-required fields. Forbidden
//     keys are absent.
//   - authority.FailMode behaviour on policy evaluator error is
//     unchanged: FailModeOpen continues, FailModeClosed escalates.
//   - Event ordering: POLICY_EVALUATED → FAIL_MODE_POLICY_TRIGGER_FIRED
//     → OUTCOME_RECORDED.
//   - Outcome / ReasonCode / OUTCOME_RECORDED / POLICY_EVALUATED /
//     FAIL_MODE_POLICY_RESOLVED payloads byte-identical between
//     equivalent runs with and without a resolved policy (accounting
//     for the one expected new event kind).
//   - Missing-rule defensive path: emits rule_status="not_found" with
//     empty rule fields and does not panic.
//   - HTTP-response body never carries trigger-event payload key names.

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
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/memory"
	"github.com/accept-io/midas/internal/value"
)

// ---------------------------------------------------------------------------
// Test infrastructure — failing policy evaluator + orchestrator
// constructor that wires it in + profile seed that controls FailMode.
// ---------------------------------------------------------------------------

type failingPolicyEvaluator struct {
	mode string
	err  error
}

func (f *failingPolicyEvaluator) Evaluate(_ context.Context, _ policy.PolicyInput) (policy.PolicyResult, error) {
	return policy.PolicyResult{}, f.err
}

func (f *failingPolicyEvaluator) PolicyMode() string { return f.mode }

func newOrchestratorWithEvaluator(t *testing.T, r testRepos, evaluator policy.PolicyEvaluator) *decision.Orchestrator {
	t.Helper()
	memStore := memory.NewStoreWithRepositories(&store.Repositories{
		Surfaces:                    r.surfaces,
		Agents:                      r.agents,
		Profiles:                    r.profiles,
		Grants:                      r.grants,
		Envelopes:                   r.envelopes,
		Audit:                       r.audit,
		Processes:                   r.processes,
		BusinessServices:            r.businessServices,
		BusinessServiceCapabilities: r.bscLinks,
		Capabilities:                r.capabilities,
		FailModePolicies:            r.failModePolicies,
	})
	orch, err := decision.NewOrchestrator(memStore, evaluator, nil)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	return orch
}

// seedProfileForTrigger seeds a profile with both PolicyReference and
// a caller-supplied FailMode. The existing seedProfileWithPolicy helper
// hard-codes FailModeClosed; D29c-2 needs both open and closed variants.
// Thresholds match seedProfileWithPolicy so that baseRequest()'s
// confidence (0.9) and medium-risk consequence pass through to the
// policy step — otherwise the consequence threshold would short-circuit
// before evaluatePolicy runs.
func seedProfileForTrigger(t *testing.T, r testRepos, id, surfaceID, policyRef string, fm authority.FailMode) {
	t.Helper()
	err := r.profiles.Create(context.Background(), &authority.AuthorityProfile{
		ID:                  id,
		SurfaceID:           surfaceID,
		Name:                "trigger profile",
		Status:              authority.ProfileStatusActive,
		ConfidenceThreshold: 0.8,
		ConsequenceThreshold: authority.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingHigh,
		},
		PolicyReference: policyRef,
		FailMode:        fm,
		Version:         1,
		EffectiveDate:   time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("seed profile %q: %v", id, err)
	}
}

// runTriggerScenario seeds a fresh repo set with the given surface /
// agent / profile / grant fixture and runs Evaluate against an
// orchestrator wired with the supplied policy evaluator. When
// withFailModePolicy is true, an active FailModePolicy is seeded and
// attached to the surface so resolution succeeds.
func runTriggerScenario(
	t *testing.T,
	surfaceID, policyRef string,
	fm authority.FailMode,
	evaluator policy.PolicyEvaluator,
	withFailModePolicy bool,
) (decision.EvaluationResult, []*audit.AuditEvent) {
	t.Helper()
	r := newRepos()
	seedActiveSurface(t, r, surfaceID)
	seedAgent(t, r, "agent-1")
	seedProfileForTrigger(t, r, "prof-1", surfaceID, policyRef, fm)
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	if withFailModePolicy {
		seedFailModePolicyForResolver(t, r, "fmp-trigger", 9, failmode.FailModePolicyStatusActive)
		setSurfaceFailModePolicyID(t, r, surfaceID, "fmp-trigger")
	}
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

func triggerEvents(events []*audit.AuditEvent) []*audit.AuditEvent {
	out := make([]*audit.AuditEvent, 0, 1)
	for _, e := range events {
		if e.EventType == audit.AuditEventFailModePolicyTriggerFired {
			out = append(out, e)
		}
	}
	return out
}

func eventTypeIndex(events []*audit.AuditEvent, t audit.AuditEventType) int {
	for i, e := range events {
		if e.EventType == t {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// Emission scenarios — eight cases from the brief
// ---------------------------------------------------------------------------

func TestTrigger_EmitsOnEvaluatorErrorWithResolvedPolicy_FailOpen(t *testing.T) {
	res, events := runTriggerScenario(t,
		"surf-trig-open", "test-policy",
		authority.FailModeOpen,
		&failingPolicyEvaluator{mode: policy.PolicyModeNoop, err: errors.New("simulated evaluator failure")},
		true,
	)
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("authority FailModeOpen must continue evaluation to accept; got Outcome=%q", res.Outcome)
	}
	if len(triggerEvents(events)) != 1 {
		t.Fatalf("expected exactly 1 FAIL_MODE_POLICY_TRIGGER_FIRED on resolved-policy + evaluator-error path; got %d",
			len(triggerEvents(events)))
	}
}

func TestTrigger_EmitsOnEvaluatorErrorWithResolvedPolicy_FailClosed(t *testing.T) {
	res, events := runTriggerScenario(t,
		"surf-trig-closed", "test-policy",
		authority.FailModeClosed,
		&failingPolicyEvaluator{mode: policy.PolicyModeNoop, err: errors.New("simulated evaluator failure")},
		true,
	)
	if res.Outcome != eval.OutcomeEscalate || res.ReasonCode != eval.ReasonPolicyError {
		t.Errorf("authority FailModeClosed must escalate with ReasonPolicyError; got %q / %q",
			res.Outcome, res.ReasonCode)
	}
	if len(triggerEvents(events)) != 1 {
		t.Fatalf("expected exactly 1 FAIL_MODE_POLICY_TRIGGER_FIRED on resolved-policy + evaluator-error path; got %d",
			len(triggerEvents(events)))
	}
}

func TestTrigger_NoEmissionOnEvaluatorErrorWithoutResolvedPolicy_FailOpen(t *testing.T) {
	_, events := runTriggerScenario(t,
		"surf-trig-no-fmp-open", "test-policy",
		authority.FailModeOpen,
		&failingPolicyEvaluator{mode: policy.PolicyModeNoop, err: errors.New("simulated evaluator failure")},
		false,
	)
	if len(triggerEvents(events)) != 0 {
		t.Errorf("must not emit trigger event when no FailModePolicy resolved; got %d", len(triggerEvents(events)))
	}
}

func TestTrigger_NoEmissionOnEvaluatorErrorWithoutResolvedPolicy_FailClosed(t *testing.T) {
	_, events := runTriggerScenario(t,
		"surf-trig-no-fmp-closed", "test-policy",
		authority.FailModeClosed,
		&failingPolicyEvaluator{mode: policy.PolicyModeNoop, err: errors.New("simulated evaluator failure")},
		false,
	)
	if len(triggerEvents(events)) != 0 {
		t.Errorf("must not emit trigger event when no FailModePolicy resolved; got %d", len(triggerEvents(events)))
	}
}

func TestTrigger_NoEmissionWhenFailModeResolutionFailed(t *testing.T) {
	// Surface references a FailModePolicy that is not seeded → resolver
	// returns error → orchestrator logs warn and continues. The
	// resolved-policy precondition for trigger emission is not met.
	r := newRepos()
	seedActiveSurface(t, r, "surf-trig-resolver-err")
	seedAgent(t, r, "agent-1")
	seedProfileForTrigger(t, r, "prof-1", "surf-trig-resolver-err", "test-policy", authority.FailModeOpen)
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	// Surface references a missing policy.
	setSurfaceFailModePolicyID(t, r, "surf-trig-resolver-err", "fmp-missing")

	res, err := newOrchestratorWithEvaluator(t, r,
		&failingPolicyEvaluator{mode: policy.PolicyModeNoop, err: errors.New("simulated evaluator failure")},
	).Evaluate(context.Background(), baseRequest("surf-trig-resolver-err", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	if len(triggerEvents(events)) != 0 {
		t.Errorf("must not emit trigger event when fail-mode resolution failed; got %d", len(triggerEvents(events)))
	}
}

func TestTrigger_NoEmissionWhenEvaluatorAllowed(t *testing.T) {
	// NoOp evaluator returns Allowed=true with no error. Policy
	// resolved, but no trigger condition fires.
	_, events := runTriggerScenario(t,
		"surf-trig-allowed", "test-policy",
		authority.FailModeClosed,
		policy.NoOpPolicyEvaluator{},
		true,
	)
	if len(triggerEvents(events)) != 0 {
		t.Errorf("must not emit trigger event when evaluator allowed; got %d", len(triggerEvents(events)))
	}
}

// denyingPolicyEvaluator returns Allowed=false with no error. This
// drives the policy-denied branch (eval.OutcomeEscalate /
// eval.ReasonPolicyDeny), which must NOT fire the trigger event.
type denyingPolicyEvaluator struct{}

func (denyingPolicyEvaluator) Evaluate(_ context.Context, _ policy.PolicyInput) (policy.PolicyResult, error) {
	return policy.PolicyResult{Allowed: false}, nil
}
func (denyingPolicyEvaluator) PolicyMode() string { return policy.PolicyModeNoop }

func TestTrigger_NoEmissionWhenEvaluatorDeniedWithoutError(t *testing.T) {
	res, events := runTriggerScenario(t,
		"surf-trig-denied", "test-policy",
		authority.FailModeClosed,
		denyingPolicyEvaluator{},
		true,
	)
	if res.ReasonCode != eval.ReasonPolicyDeny {
		t.Errorf("expected policy-denied path; got Reason=%q", res.ReasonCode)
	}
	if len(triggerEvents(events)) != 0 {
		t.Errorf("must not emit trigger event when evaluator denied without error; got %d", len(triggerEvents(events)))
	}
}

// ---------------------------------------------------------------------------
// Payload pin — required + forbidden keys
// ---------------------------------------------------------------------------

func TestTrigger_PayloadKeys(t *testing.T) {
	res, events := runTriggerScenario(t,
		"surf-trig-payload", "test-policy",
		authority.FailModeClosed,
		&failingPolicyEvaluator{mode: policy.PolicyModeNoop, err: errors.New("simulated evaluator failure")},
		true,
	)
	_ = res
	tr := triggerEvents(events)
	if len(tr) != 1 {
		t.Fatalf("expected exactly 1 trigger event; got %d", len(tr))
	}
	p := tr[0].Payload

	// Required fields.
	required := map[string]any{
		"fail_mode_policy_id":      "fmp-trigger",
		"fail_mode_policy_version": float64(9),
		"source":                   string(failmode.ResolutionSourceSurface),
		"trigger_condition":        string(failmode.FailModePolicyTriggerPolicyEvaluatorError),
		"correctness_class":        string(failmode.CorrectnessClassResource),
	}
	for k, want := range required {
		got, present := p[k]
		if !present {
			t.Errorf("required payload key %q missing", k)
			continue
		}
		// Numeric round-trips: encoded as float64 via map[string]any +
		// JSON-shaped persistence; ints come back as float64. Compare
		// loosely on numeric values.
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

	// permitted_mode / enforcement_state / outcome — rule fields
	// (present when a rule was found, which it is for the seeded
	// closed-only policy with axis defaults).
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

	// Optional contextual fields — present when available on the
	// orchestrator's resolved state.
	for _, k := range []string{"surface_id", "surface_version", "authority_profile_id", "agent_id", "policy_reference"} {
		if _, present := p[k]; !present {
			t.Errorf("expected contextual payload key %q present in the seeded scenario", k)
		}
	}

	// Forbidden keys.
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
	} {
		if _, present := p[k]; present {
			t.Errorf("forbidden payload key %q must not appear; got %v", k, p[k])
		}
	}

	// rule_status must be absent when a rule was found (it is a
	// defensive marker added only on the missing-rule path).
	if _, present := p["rule_status"]; present {
		t.Errorf("rule_status must be absent on the rule-found path; got %v", p["rule_status"])
	}
}

// ---------------------------------------------------------------------------
// Event ordering
// ---------------------------------------------------------------------------

func TestTrigger_EventOrdering(t *testing.T) {
	_, events := runTriggerScenario(t,
		"surf-trig-order", "test-policy",
		authority.FailModeClosed,
		&failingPolicyEvaluator{mode: policy.PolicyModeNoop, err: errors.New("simulated evaluator failure")},
		true,
	)
	resolvedIdx := eventTypeIndex(events, audit.AuditEventFailModePolicyResolved)
	policyEvalIdx := eventTypeIndex(events, audit.AuditEventPolicyEvaluated)
	triggerIdx := eventTypeIndex(events, audit.AuditEventFailModePolicyTriggerFired)
	outcomeIdx := eventTypeIndex(events, audit.AuditEventOutcomeRecorded)

	if resolvedIdx < 0 || policyEvalIdx < 0 || triggerIdx < 0 || outcomeIdx < 0 {
		t.Fatalf("missing required events: resolved=%d policyEval=%d trigger=%d outcome=%d",
			resolvedIdx, policyEvalIdx, triggerIdx, outcomeIdx)
	}
	if !(resolvedIdx < triggerIdx) {
		t.Errorf("FAIL_MODE_POLICY_RESOLVED (idx=%d) must come before FAIL_MODE_POLICY_TRIGGER_FIRED (idx=%d)",
			resolvedIdx, triggerIdx)
	}
	if !(policyEvalIdx < triggerIdx) {
		t.Errorf("POLICY_EVALUATED (idx=%d) must come before FAIL_MODE_POLICY_TRIGGER_FIRED (idx=%d)",
			policyEvalIdx, triggerIdx)
	}
	if !(triggerIdx < outcomeIdx) {
		t.Errorf("FAIL_MODE_POLICY_TRIGGER_FIRED (idx=%d) must come before OUTCOME_RECORDED (idx=%d)",
			triggerIdx, outcomeIdx)
	}
}

// ---------------------------------------------------------------------------
// Authority FailMode preservation
// ---------------------------------------------------------------------------

func TestTrigger_DoesNotOverrideAuthorityFailModeOpen(t *testing.T) {
	res, _ := runTriggerScenario(t,
		"surf-trig-pres-open", "test-policy",
		authority.FailModeOpen,
		&failingPolicyEvaluator{mode: policy.PolicyModeNoop, err: errors.New("simulated evaluator failure")},
		true,
	)
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("trigger event must not override FailModeOpen; got Outcome=%q", res.Outcome)
	}
	if res.ReasonCode != eval.ReasonWithinAuthority {
		t.Errorf("trigger event must not change ReasonCode under FailModeOpen; got %q", res.ReasonCode)
	}
}

func TestTrigger_DoesNotOverrideAuthorityFailModeClosed(t *testing.T) {
	res, _ := runTriggerScenario(t,
		"surf-trig-pres-closed", "test-policy",
		authority.FailModeClosed,
		&failingPolicyEvaluator{mode: policy.PolicyModeNoop, err: errors.New("simulated evaluator failure")},
		true,
	)
	if res.Outcome != eval.OutcomeEscalate {
		t.Errorf("trigger event must not override FailModeClosed escalation; got Outcome=%q", res.Outcome)
	}
	if res.ReasonCode != eval.ReasonPolicyError {
		t.Errorf("trigger event must not change ReasonCode under FailModeClosed; got %q", res.ReasonCode)
	}
}

// ---------------------------------------------------------------------------
// Outcome invariance — the brief's strongest pin
// ---------------------------------------------------------------------------

// stripPerRunFields removes per-run identifying fields from an audit-
// event payload so two runs can be compared byte-for-byte.
func stripPerRunFieldsForTriggerInvariance(p map[string]any) map[string]any {
	out := make(map[string]any, len(p))
	for k, v := range p {
		switch k {
		case "envelope_id", "audit_event_id", "request_id",
			"resolved_at", "evaluation_time", "triggered_at",
			"started_at", "ended_at", "created_at", "updated_at", "timestamp":
			continue
		}
		out[k] = v
	}
	return out
}

func payloadOf(events []*audit.AuditEvent, t audit.AuditEventType) map[string]any {
	for _, e := range events {
		if e.EventType == t {
			return e.Payload
		}
	}
	return nil
}

// TestTrigger_RuntimeInvariance_OnlyTriggerEventDiffers pins the
// strongest single guarantee of D29c-2: two evaluations differing only
// in the presence/absence of a resolved FailModePolicy (with the same
// failing policy evaluator) produce byte-identical outcomes, reason
// codes, OUTCOME_RECORDED payloads, and POLICY_EVALUATED payloads.
// The audit-event kind set differs by exactly the two FailModePolicy
// observational events (FAIL_MODE_POLICY_RESOLVED + the new
// FAIL_MODE_POLICY_TRIGGER_FIRED) which appear only in the variant.
func TestTrigger_RuntimeInvariance_OnlyTriggerEventDiffers(t *testing.T) {
	evalErr := errors.New("simulated evaluator failure")
	baselineRes, baselineEvents := runTriggerScenario(t,
		"surf-trig-inv-baseline", "test-policy",
		authority.FailModeClosed,
		&failingPolicyEvaluator{mode: policy.PolicyModeNoop, err: evalErr},
		false, // no FailModePolicy resolved
	)
	variantRes, variantEvents := runTriggerScenario(t,
		"surf-trig-inv-variant", "test-policy",
		authority.FailModeClosed,
		&failingPolicyEvaluator{mode: policy.PolicyModeNoop, err: evalErr},
		true, // resolved policy → both fail-mode events present
	)

	// Outcome / ReasonCode invariance.
	if baselineRes.Outcome != variantRes.Outcome {
		t.Errorf("Outcome diverged: baseline=%q variant=%q", baselineRes.Outcome, variantRes.Outcome)
	}
	if baselineRes.ReasonCode != variantRes.ReasonCode {
		t.Errorf("ReasonCode diverged: baseline=%q variant=%q", baselineRes.ReasonCode, variantRes.ReasonCode)
	}

	// POLICY_EVALUATED payload invariance.
	basePE := stripPerRunFieldsForTriggerInvariance(payloadOf(baselineEvents, audit.AuditEventPolicyEvaluated))
	varPE := stripPerRunFieldsForTriggerInvariance(payloadOf(variantEvents, audit.AuditEventPolicyEvaluated))
	if !reflect.DeepEqual(basePE, varPE) {
		t.Errorf("POLICY_EVALUATED payload diverged:\nbaseline=%v\nvariant=%v", basePE, varPE)
	}

	// OUTCOME_RECORDED payload invariance.
	baseOR := stripPerRunFieldsForTriggerInvariance(payloadOf(baselineEvents, audit.AuditEventOutcomeRecorded))
	varOR := stripPerRunFieldsForTriggerInvariance(payloadOf(variantEvents, audit.AuditEventOutcomeRecorded))
	if !reflect.DeepEqual(baseOR, varOR) {
		t.Errorf("OUTCOME_RECORDED payload diverged:\nbaseline=%v\nvariant=%v", baseOR, varOR)
	}

	// Audit-event kind set: variant has exactly two extra event kinds
	// (FAIL_MODE_POLICY_RESOLVED + FAIL_MODE_POLICY_TRIGGER_FIRED).
	// After removing those, the sets must match.
	baseKinds := map[audit.AuditEventType]int{}
	for _, e := range baselineEvents {
		baseKinds[e.EventType]++
	}
	varKinds := map[audit.AuditEventType]int{}
	for _, e := range variantEvents {
		varKinds[e.EventType]++
	}
	delete(varKinds, audit.AuditEventFailModePolicyResolved)
	delete(varKinds, audit.AuditEventFailModePolicyTriggerFired)
	if !reflect.DeepEqual(baseKinds, varKinds) {
		t.Errorf("audit-event kind sets diverged outside the expected two fail-mode events:\nbaseline=%v\nvariant_minus_fmp=%v",
			baseKinds, varKinds)
	}

	// Forbidden future event kinds must NOT appear in either run.
	for _, forbidden := range []audit.AuditEventType{
		"FAIL_MODE_POLICY_DRY_RUN_DECISION",
		"FAIL_MODE_POLICY_ENFORCED",
	} {
		for _, e := range baselineEvents {
			if e.EventType == forbidden {
				t.Errorf("baseline must not emit %q", forbidden)
			}
		}
		for _, e := range variantEvents {
			if e.EventType == forbidden {
				t.Errorf("variant must not emit %q", forbidden)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Missing-rule defensive path
// ---------------------------------------------------------------------------

// TestTrigger_MissingRuleDefensivePath constructs a FailModePolicy with
// no `resource` rule, persists it directly via the failmode repo
// (bypassing the validator), and confirms the orchestrator emits the
// trigger event with rule_status="not_found" and empty rule fields —
// without panicking.
func TestTrigger_MissingRuleDefensivePath(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-trig-defect")
	seedAgent(t, r, "agent-1")
	seedProfileForTrigger(t, r, "prof-1", "surf-trig-defect", "test-policy", authority.FailModeOpen)
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	// Hand-construct a FailModePolicy with the resource rule missing.
	// Bypasses the validator (which requires exhaustive coverage) by
	// calling Create directly on the repo.
	now := time.Now().UTC()
	bogus := &failmode.FailModePolicy{
		ID:             "fmp-defect",
		Version:        1,
		Name:           "Defect",
		Status:         failmode.FailModePolicyStatusActive,
		EffectiveDate:  now.Add(-time.Hour),
		BusinessOwner:  "owner@example.com",
		TechnicalOwner: "platform-team",
		Rules: []failmode.FailModePolicyRule{
			{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity,
				PermittedMode:    failmode.PermittedModeClosed,
				EnforcementState: failmode.EnforcementStateEvidenceOnly,
				Outcome:          failmode.OutcomeEscalate},
			// Other rules intentionally omitted.
		},
		Origin:    "manual",
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "test",
	}
	if err := r.failModePolicies.Create(context.Background(), bogus); err != nil {
		t.Fatalf("Create defect policy: %v", err)
	}
	setSurfaceFailModePolicyID(t, r, "surf-trig-defect", "fmp-defect")

	res, err := newOrchestratorWithEvaluator(t, r,
		&failingPolicyEvaluator{mode: policy.PolicyModeNoop, err: errors.New("simulated evaluator failure")},
	).Evaluate(context.Background(), baseRequest("surf-trig-defect", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	tr := triggerEvents(events)
	if len(tr) != 1 {
		t.Fatalf("expected exactly 1 trigger event on missing-rule defensive path; got %d", len(tr))
	}
	p := tr[0].Payload
	if p["rule_status"] != "not_found" {
		t.Errorf("rule_status: want \"not_found\", got %v", p["rule_status"])
	}
	if p["permitted_mode"] != "" {
		t.Errorf("permitted_mode must be empty on missing-rule path; got %v", p["permitted_mode"])
	}
	if p["enforcement_state"] != "" {
		t.Errorf("enforcement_state must be empty on missing-rule path; got %v", p["enforcement_state"])
	}
	if p["outcome"] != "" {
		t.Errorf("outcome must be empty on missing-rule path; got %v", p["outcome"])
	}
}

// ---------------------------------------------------------------------------
// HTTP-response invariance — payload key names never on the wire
// ---------------------------------------------------------------------------

func TestTrigger_HTTPResponseNoLeakage(t *testing.T) {
	res, _ := runTriggerScenario(t,
		"surf-trig-http", "test-policy",
		authority.FailModeClosed,
		&failingPolicyEvaluator{mode: policy.PolicyModeNoop, err: errors.New("simulated evaluator failure")},
		true,
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
		"trigger_condition",
		"TriggerFired",
		"FAIL_MODE_POLICY_TRIGGER_FIRED",
		"permitted_mode",
		"enforcement_state",
		"correctness_class",
		"triggered_at",
		"rule_status",
		"fail_mode_policy_id",
		"fail_mode_policy_version",
	} {
		if strings.Contains(bodyStr, sub) {
			t.Errorf("/v1/evaluate response must not contain %q; body=%s", sub, bodyStr)
		}
	}
}
