package decision_test

// orchestrator_failmode_audit_test.go — D27j-impl-3 audit-chain regression
// for the FAIL_MODE_POLICY_RESOLVED runtime event. Pins:
//
//   - emission conditions: Surface override and BusinessService default
//     each emit exactly one event; no policy / resolver failure / no
//     repo wired emit zero events
//   - payload shape: required keys present, source value correct, no
//     forbidden keys (rules / permitted modes / outcome fields)
//   - ordering: emitted strictly after SURFACE_RESOLVED and strictly
//     before AGENT_RESOLVED (and therefore before POLICY_EVALUATED)
//   - hash-chain participation: contiguous sequence numbers,
//     prev_hash linkage, audit.VerifyAuditIntegrity passes,
//     Integrity.AuditEventIDs includes (or excludes) the event ID
//   - payload stability: POLICY_EVALUATED payload byte-equal whether
//     or not a FailModePolicy was resolved
//   - rule non-consultation: persisting a policy with nil Rules and
//     emitting the event does not panic and emits the same payload

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/failmode"
)

// fmpEvents filters the audit list to FAIL_MODE_POLICY_RESOLVED events.
func fmpEvents(events []*audit.AuditEvent) []*audit.AuditEvent {
	out := make([]*audit.AuditEvent, 0, 1)
	for _, e := range events {
		if e.EventType == audit.AuditEventFailModePolicyResolved {
			out = append(out, e)
		}
	}
	return out
}

// indexOfEventType returns the index of the first audit event with the
// given EventType, or -1 when not present.
func indexOfEventType(events []*audit.AuditEvent, et audit.AuditEventType) int {
	for i, e := range events {
		if e.EventType == et {
			return i
		}
	}
	return -1
}

// findPolicyEvaluatedPayload returns the POLICY_EVALUATED payload for the
// envelope or nil if the event is missing. Used to compare payload byte
// stability across configurations.
func findPolicyEvaluatedPayload(events []*audit.AuditEvent) map[string]any {
	for _, e := range events {
		if e.EventType == audit.AuditEventPolicyEvaluated {
			return e.Payload
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Emission — surface override
// ---------------------------------------------------------------------------

func TestAudit_FailModePolicyResolved_EmittedOnSurfaceOverride(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-audit-surface")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-audit-surface")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	seedFailModePolicyForResolver(t, r, "fmp-surface", 7, failmode.FailModePolicyStatusActive)
	setSurfaceFailModePolicyID(t, r, "surf-audit-surface", "fmp-surface")

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-audit-surface", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("outcome must be unchanged by event emission; got %q", res.Outcome)
	}

	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	got := fmpEvents(events)
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 FAIL_MODE_POLICY_RESOLVED event, got %d", len(got))
	}

	p := got[0].Payload
	if p["fail_mode_policy_id"] != "fmp-surface" {
		t.Errorf("fail_mode_policy_id: want fmp-surface, got %v", p["fail_mode_policy_id"])
	}
	switch v := p["fail_mode_policy_version"].(type) {
	case int:
		if v != 7 {
			t.Errorf("fail_mode_policy_version: want 7, got %d", v)
		}
	case int64:
		if v != 7 {
			t.Errorf("fail_mode_policy_version: want 7, got %d", v)
		}
	case float64:
		if v != 7 {
			t.Errorf("fail_mode_policy_version: want 7, got %v", v)
		}
	default:
		t.Errorf("fail_mode_policy_version: unexpected type %T value %v", p["fail_mode_policy_version"], p["fail_mode_policy_version"])
	}
	if p["source"] != string(failmode.ResolutionSourceSurface) {
		t.Errorf("source: want %q, got %v", failmode.ResolutionSourceSurface, p["source"])
	}
	if p["resolved_at"] == nil || p["resolved_at"] == "" {
		t.Errorf("resolved_at must be set; got %v", p["resolved_at"])
	}
	if p["evaluation_time"] == nil || p["evaluation_time"] == "" {
		t.Errorf("evaluation_time must be set; got %v", p["evaluation_time"])
	}
	if p["surface_id"] != "surf-audit-surface" {
		t.Errorf("surface_id: want surf-audit-surface, got %v", p["surface_id"])
	}
	// Forbidden keys — guard the closed-only invariant at payload level.
	for _, k := range []string{"rules", "permitted_mode", "degraded", "allowed", "fail_open", "fail_soft", "outcome", "reason_code"} {
		if _, present := p[k]; present {
			t.Errorf("forbidden payload key %q present in FAIL_MODE_POLICY_RESOLVED event", k)
		}
	}
}

// ---------------------------------------------------------------------------
// Emission — BusinessService default
// ---------------------------------------------------------------------------

func TestAudit_FailModePolicyResolved_EmittedOnBusinessServiceDefault(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-audit-bs")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-audit-bs")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	seedFailModePolicyForResolver(t, r, "fmp-bs-default", 3, failmode.FailModePolicyStatusActive)
	setBusinessServiceFailModePolicyID(t, r, "bs-test", "fmp-bs-default")

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-audit-bs", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	got := fmpEvents(events)
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 FAIL_MODE_POLICY_RESOLVED event, got %d", len(got))
	}
	if got[0].Payload["source"] != string(failmode.ResolutionSourceBusinessService) {
		t.Errorf("source: want %q, got %v", failmode.ResolutionSourceBusinessService, got[0].Payload["source"])
	}
	if got[0].Payload["business_service_id"] != "bs-test" {
		t.Errorf("business_service_id: want bs-test, got %v", got[0].Payload["business_service_id"])
	}
}

// ---------------------------------------------------------------------------
// Absence — no policy / resolver failure
// ---------------------------------------------------------------------------

func TestAudit_FailModePolicyResolved_NotEmittedWhenNoPolicy(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-audit-no-fmp")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-audit-no-fmp")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-audit-no-fmp", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	if matches := fmpEvents(events); len(matches) != 0 {
		t.Errorf("expected zero FAIL_MODE_POLICY_RESOLVED events when no policy configured; got %d", len(matches))
	}
	// Integrity.AuditEventIDs must not contain a FAIL_MODE_POLICY_RESOLVED
	// event id; achieved by ensuring no such event exists.
	env, err := r.envelopes.GetByID(context.Background(), res.EnvelopeID)
	if err != nil || env == nil {
		t.Fatalf("GetByID: env=%v err=%v", env, err)
	}
	for _, id := range env.Integrity.AuditEventIDs {
		for _, ev := range events {
			if ev.ID == id && ev.EventType == audit.AuditEventFailModePolicyResolved {
				t.Errorf("Integrity.AuditEventIDs unexpectedly references FAIL_MODE_POLICY_RESOLVED event %q", id)
			}
		}
	}
}

func TestAudit_FailModePolicyResolved_NotEmittedOnResolverFailure(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-audit-broken")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-audit-broken")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	// Surface points at a policy that does not exist — resolver returns
	// ErrFailModePolicyResolutionFailed and the orchestrator logs and
	// continues per the D27j-impl-2 posture.
	setSurfaceFailModePolicyID(t, r, "surf-audit-broken", "fmp-does-not-exist")

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-audit-broken", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate must not error on resolver failure (D27j-impl-3 preserves -2 posture); got %v", err)
	}
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("outcome must be unchanged on resolver failure; got %q", res.Outcome)
	}

	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	if got := fmpEvents(events); len(got) != 0 {
		t.Errorf("expected zero FAIL_MODE_POLICY_RESOLVED events on resolver failure; got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// Ordering
// ---------------------------------------------------------------------------

// TestAudit_FailModePolicyResolved_OrderedAfterSurfaceResolvedBeforePolicyEvaluated
// pins the deterministic event ordering: FAIL_MODE_POLICY_RESOLVED appears
// after SURFACE_RESOLVED and before AGENT_RESOLVED, which guarantees it
// appears before POLICY_EVALUATED (which is emitted after the consequence
// step in the happy path).
func TestAudit_FailModePolicyResolved_OrderedAfterSurfaceResolvedBeforePolicyEvaluated(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-audit-order")
	seedAgent(t, r, "agent-1")
	seedProfileWithPolicy(t, r, "prof-1", "surf-audit-order", "policy.allow.all")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	seedFailModePolicyForResolver(t, r, "fmp-order", 1, failmode.FailModePolicyStatusActive)
	setSurfaceFailModePolicyID(t, r, "surf-audit-order", "fmp-order")

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-audit-order", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}

	surfIdx := indexOfEventType(events, audit.AuditEventSurfaceResolved)
	fmpIdx := indexOfEventType(events, audit.AuditEventFailModePolicyResolved)
	agentIdx := indexOfEventType(events, audit.AuditEventAgentResolved)
	policyIdx := indexOfEventType(events, audit.AuditEventPolicyEvaluated)

	if surfIdx < 0 || fmpIdx < 0 || agentIdx < 0 || policyIdx < 0 {
		t.Fatalf("expected all of SURFACE_RESOLVED/FAIL_MODE_POLICY_RESOLVED/AGENT_RESOLVED/POLICY_EVALUATED present; got indices %d/%d/%d/%d",
			surfIdx, fmpIdx, agentIdx, policyIdx)
	}
	if !(surfIdx < fmpIdx) {
		t.Errorf("FAIL_MODE_POLICY_RESOLVED (idx=%d) must come AFTER SURFACE_RESOLVED (idx=%d)", fmpIdx, surfIdx)
	}
	if !(fmpIdx < agentIdx) {
		t.Errorf("FAIL_MODE_POLICY_RESOLVED (idx=%d) must come BEFORE AGENT_RESOLVED (idx=%d)", fmpIdx, agentIdx)
	}
	if !(fmpIdx < policyIdx) {
		t.Errorf("FAIL_MODE_POLICY_RESOLVED (idx=%d) must come BEFORE POLICY_EVALUATED (idx=%d)", fmpIdx, policyIdx)
	}
}

// ---------------------------------------------------------------------------
// Hash-chain participation
// ---------------------------------------------------------------------------

// TestAudit_FailModePolicyResolved_ParticipatesInHashChain verifies that the
// new event is queued through the standard observation path (sequence
// numbers contiguous, prev_hash chains correctly), and that the envelope's
// Integrity.AuditEventIDs includes the new event ID. We rely on
// audit.VerifyAuditIntegrity for end-to-end chain verification.
func TestAudit_FailModePolicyResolved_ParticipatesInHashChain(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-audit-chain")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-audit-chain")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	seedFailModePolicyForResolver(t, r, "fmp-chain", 2, failmode.FailModePolicyStatusActive)
	setSurfaceFailModePolicyID(t, r, "surf-audit-chain", "fmp-chain")

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-audit-chain", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	got := fmpEvents(events)
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 FAIL_MODE_POLICY_RESOLVED event, got %d", len(got))
	}
	fmpEv := got[0]
	if fmpEv.EventHash == "" {
		t.Errorf("FAIL_MODE_POLICY_RESOLVED event has empty event_hash — not part of hash chain")
	}
	if fmpEv.SequenceNo == 0 {
		t.Errorf("FAIL_MODE_POLICY_RESOLVED event has zero sequence_no — not appended through standard path")
	}

	// Sequence-no contiguity and prev-hash linkage across the whole chain.
	for i := 1; i < len(events); i++ {
		if events[i].SequenceNo != events[i-1].SequenceNo+1 {
			t.Errorf("sequence_no gap at index %d: prev=%d curr=%d",
				i, events[i-1].SequenceNo, events[i].SequenceNo)
		}
		if events[i].PrevHash != events[i-1].EventHash {
			t.Errorf("hash-chain break at index %d (event_type=%q): prev_hash=%q expected %q",
				i, events[i].EventType, events[i].PrevHash, events[i-1].EventHash)
		}
	}

	// End-to-end verification.
	if err := audit.VerifyAuditIntegrity(context.Background(), r.envelopes, r.audit); err != nil {
		t.Errorf("VerifyAuditIntegrity must pass with FAIL_MODE_POLICY_RESOLVED in the chain; got %v", err)
	}

	// Integrity.AuditEventIDs includes the new event ID.
	env, err := r.envelopes.GetByID(context.Background(), res.EnvelopeID)
	if err != nil || env == nil {
		t.Fatalf("GetByID: env=%v err=%v", env, err)
	}
	found := false
	for _, id := range env.Integrity.AuditEventIDs {
		if id == fmpEv.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Integrity.AuditEventIDs must include FAIL_MODE_POLICY_RESOLVED event id %q; got %v",
			fmpEv.ID, env.Integrity.AuditEventIDs)
	}
}

// TestAudit_FailModePolicyResolved_AbsenceMeansNoIDInIntegrity confirms
// the corollary: when no policy is resolved, no FAIL_MODE_POLICY_RESOLVED
// event ID appears in Integrity.AuditEventIDs.
func TestAudit_FailModePolicyResolved_AbsenceMeansNoIDInIntegrity(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-audit-no-id")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-audit-no-id")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-audit-no-id", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	for _, e := range events {
		if e.EventType == audit.AuditEventFailModePolicyResolved {
			t.Fatalf("no FAIL_MODE_POLICY_RESOLVED expected; found one")
		}
	}

	env, err := r.envelopes.GetByID(context.Background(), res.EnvelopeID)
	if err != nil || env == nil {
		t.Fatalf("GetByID: env=%v err=%v", env, err)
	}
	// Cross-check: no entry in Integrity.AuditEventIDs corresponds to a
	// FAIL_MODE_POLICY_RESOLVED event id (we asserted no such event was
	// listed, so this is a defensive belt-and-braces check).
	for _, id := range env.Integrity.AuditEventIDs {
		for _, ev := range events {
			if ev.ID == id && ev.EventType == audit.AuditEventFailModePolicyResolved {
				t.Errorf("Integrity.AuditEventIDs references FAIL_MODE_POLICY_RESOLVED event %q when none should exist", id)
			}
		}
	}
	// VerifyAuditIntegrity passes for the unconfigured case too.
	if err := audit.VerifyAuditIntegrity(context.Background(), r.envelopes, r.audit); err != nil {
		t.Errorf("VerifyAuditIntegrity must pass when no FailModePolicy is resolved; got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Payload stability — POLICY_EVALUATED unaffected by FailModePolicy
// resolution
// ---------------------------------------------------------------------------

// TestAudit_PolicyEvaluatedPayload_StableAcrossFailModePolicyConfig pins
// that adding a resolved FailModePolicy does not perturb the
// POLICY_EVALUATED payload by even one key. The resolver is a separate
// concern; the policy-evaluator event is owned by appendPolicyEvaluatedEvent
// alone.
func TestAudit_PolicyEvaluatedPayload_StableAcrossFailModePolicyConfig(t *testing.T) {
	// Run 1: profile carries a policy_ref, no FailModePolicy configured.
	rNo := newRepos()
	seedActiveSurface(t, rNo, "surf-pe-no-fmp")
	seedAgent(t, rNo, "agent-1")
	seedProfileWithPolicy(t, rNo, "prof-1", "surf-pe-no-fmp", "policy.allow.all")
	seedActiveGrant(t, rNo, "grant-1", "agent-1", "prof-1")

	resNo, err := newOrchestrator(t, rNo).Evaluate(context.Background(),
		baseRequest("surf-pe-no-fmp", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate (no FMP): %v", err)
	}
	eventsNo, _ := rNo.audit.ListByEnvelopeID(context.Background(), resNo.EnvelopeID)
	payloadNo := findPolicyEvaluatedPayload(eventsNo)
	if payloadNo == nil {
		t.Fatalf("expected POLICY_EVALUATED in run with policy_ref; got none")
	}

	// Run 2: same setup PLUS a configured Surface override that resolves.
	rYes := newRepos()
	seedActiveSurface(t, rYes, "surf-pe-with-fmp")
	seedAgent(t, rYes, "agent-1")
	seedProfileWithPolicy(t, rYes, "prof-1", "surf-pe-with-fmp", "policy.allow.all")
	seedActiveGrant(t, rYes, "grant-1", "agent-1", "prof-1")
	seedFailModePolicyForResolver(t, rYes, "fmp-pe", 1, failmode.FailModePolicyStatusActive)
	setSurfaceFailModePolicyID(t, rYes, "surf-pe-with-fmp", "fmp-pe")

	resYes, err := newOrchestrator(t, rYes).Evaluate(context.Background(),
		baseRequest("surf-pe-with-fmp", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate (with FMP): %v", err)
	}
	eventsYes, _ := rYes.audit.ListByEnvelopeID(context.Background(), resYes.EnvelopeID)
	payloadYes := findPolicyEvaluatedPayload(eventsYes)
	if payloadYes == nil {
		t.Fatalf("expected POLICY_EVALUATED in run with FMP; got none")
	}

	// Same key set, same values.
	if !reflect.DeepEqual(payloadNo, payloadYes) {
		t.Errorf("POLICY_EVALUATED payload changed when a FailModePolicy was resolved.\n  no-FMP:   %v\n  with-FMP: %v",
			payloadNo, payloadYes)
	}
	// Defensive — the payload must not borrow any FailModePolicy keys.
	for _, k := range []string{"fail_mode_policy_id", "fail_mode_policy_version", "source"} {
		if _, present := payloadYes[k]; present {
			t.Errorf("POLICY_EVALUATED payload contains forbidden FailModePolicy key %q", k)
		}
	}
}

// ---------------------------------------------------------------------------
// No-rule consultation
// ---------------------------------------------------------------------------

// TestAudit_FailModePolicyResolved_NoRuleConsultation seeds a
// FailModePolicy with NIL Rules and confirms the orchestrator still
// emits FAIL_MODE_POLICY_RESOLVED with the same payload shape. This
// pins that the helper does not inspect or serialise rules at any
// point — if it did, ranging over a nil slice would still be safe but
// would risk leaking rule keys; the assertion that no rule keys are
// present is the authoritative check.
func TestAudit_FailModePolicyResolved_NoRuleConsultation(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-audit-no-rules")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-audit-no-rules")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	// Hand-roll a policy with nil Rules into the failModePolicies repo —
	// bypasses the seed helper that would populate Rules.
	pastTime := time.Now().UTC().Add(-time.Hour)
	if err := r.failModePolicies.Create(context.Background(), &failmode.FailModePolicy{
		ID:             "fmp-no-rules",
		Version:        1,
		Name:           "FMP no rules",
		Status:         failmode.FailModePolicyStatusActive,
		EffectiveDate:  pastTime,
		BusinessOwner:  "owner@example.com",
		TechnicalOwner: "platform-team",
		Rules:          nil, // <- the invariant under test
		Origin:         "manual",
		Managed:        true,
		CreatedAt:      pastTime,
		UpdatedAt:      pastTime,
		CreatedBy:      "test",
	}); err != nil {
		t.Fatalf("seed nil-rules policy: %v", err)
	}
	setSurfaceFailModePolicyID(t, r, "surf-audit-no-rules", "fmp-no-rules")

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-audit-no-rules", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate must not panic on nil-rules policy; got %v", err)
	}

	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	got := fmpEvents(events)
	if len(got) != 1 {
		t.Fatalf("expected 1 FAIL_MODE_POLICY_RESOLVED event for nil-rules policy; got %d", len(got))
	}
	for _, k := range []string{"rules", "permitted_mode"} {
		if _, present := got[0].Payload[k]; present {
			t.Errorf("FAIL_MODE_POLICY_RESOLVED payload must not carry %q", k)
		}
	}
}

