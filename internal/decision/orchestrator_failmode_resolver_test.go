package decision_test

// orchestrator_failmode_resolver_test.go — runtime regression tests for
// the D27j-impl-2 effective FailModePolicy resolver wiring inside the
// orchestrator. The resolver runs after structural resolution and before
// agent / authority resolution. It is observability-only in this tranche:
// resolution failures must not change the outcome, must not fail the
// evaluation, and must not appear on the HTTP response or in audit-event
// payloads. These tests pin that posture.

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/failmode"
	"github.com/accept-io/midas/internal/surface"
)

// seedFailModePolicyForResolver seeds an active fail-mode policy whose
// effective_date is one hour in the past, so any "now"-based evaluation
// will see it as active.
func seedFailModePolicyForResolver(t *testing.T, r testRepos, id string, version int, status failmode.FailModePolicyStatus) {
	t.Helper()
	now := time.Now().UTC()
	if err := r.failModePolicies.Create(context.Background(), &failmode.FailModePolicy{
		ID:             id,
		Version:        version,
		Name:           "Test " + id,
		Status:         status,
		EffectiveDate:  now.Add(-time.Hour),
		BusinessOwner:  "owner@example.com",
		TechnicalOwner: "platform-team",
		Rules: []failmode.FailModePolicyRule{
			{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed},
			{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed},
			{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable},
			{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: failmode.PermittedModeClosed},
			{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeClosed},
		},
		Origin:    "manual",
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "test",
	}); err != nil {
		t.Fatalf("seed FailModePolicy %s/v%d: %v", id, version, err)
	}
}

// setSurfaceFailModePolicyID overwrites the FailModePolicyID on the
// already-seeded surface (seedActiveSurface does not accept it directly).
// Memory SurfaceRepo stores pointers verbatim, so re-Update with the new
// value persists.
func setSurfaceFailModePolicyID(t *testing.T, r testRepos, surfaceID, fmpID string) {
	t.Helper()
	got, err := r.surfaces.FindLatestByID(context.Background(), surfaceID)
	if err != nil || got == nil {
		t.Fatalf("findLatestByID %s: err=%v got=%v", surfaceID, err, got)
	}
	updated := *got
	updated.FailModePolicyID = fmpID
	if err := r.surfaces.Update(context.Background(), &updated); err != nil {
		t.Fatalf("Update surface fail_mode_policy_id: %v", err)
	}
}

// setBusinessServiceFailModePolicyID overwrites the FailModePolicyID on
// the structural-chain BS row that resolveStructure loads via
// fakeBusinessServiceRepo.
func setBusinessServiceFailModePolicyID(t *testing.T, r testRepos, bsID, fmpID string) {
	t.Helper()
	bs := r.businessServices.services[bsID]
	if bs == nil {
		t.Fatalf("business service %q not seeded", bsID)
	}
	bs.FailModePolicyID = fmpID
}

// TestEvaluate_FailModeResolver_SurfaceOverride pins the surface-override
// path: when the surface declares fail_mode_policy_id and that policy is
// active, the resolver records it in EvaluationResult with
// Source=surface. The outcome is unchanged.
func TestEvaluate_FailModeResolver_SurfaceOverride(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-fmp-surface")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-fmp-surface")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	seedFailModePolicyForResolver(t, r, "fmp-surface", 1, failmode.FailModePolicyStatusActive)
	setSurfaceFailModePolicyID(t, r, "surf-fmp-surface", "fmp-surface")

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-fmp-surface", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("outcome must be unaffected by resolver; got %q", res.Outcome)
	}
	if res.EffectiveFailModePolicy == nil {
		t.Fatal("EffectiveFailModePolicy must be populated when surface declares an override")
	}
	if res.EffectiveFailModePolicy.PolicyID != "fmp-surface" ||
		res.EffectiveFailModePolicy.Version != 1 ||
		res.EffectiveFailModePolicy.Source != failmode.ResolutionSourceSurface {
		t.Errorf("unexpected resolved policy: %+v", res.EffectiveFailModePolicy)
	}
}

// TestEvaluate_FailModeResolver_BusinessServiceFallback pins the
// BS-default fallback: when the surface has no override and the BS
// declares fail_mode_policy_id, the resolver records it with
// Source=business_service.
func TestEvaluate_FailModeResolver_BusinessServiceFallback(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-fmp-bs")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-fmp-bs")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	seedFailModePolicyForResolver(t, r, "fmp-bs-default", 1, failmode.FailModePolicyStatusActive)
	setBusinessServiceFailModePolicyID(t, r, "bs-test", "fmp-bs-default")

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-fmp-bs", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("outcome must be unaffected by resolver; got %q", res.Outcome)
	}
	if res.EffectiveFailModePolicy == nil {
		t.Fatal("EffectiveFailModePolicy must be populated from BS default")
	}
	if res.EffectiveFailModePolicy.PolicyID != "fmp-bs-default" ||
		res.EffectiveFailModePolicy.Source != failmode.ResolutionSourceBusinessService {
		t.Errorf("unexpected resolved policy: %+v", res.EffectiveFailModePolicy)
	}
}

// TestEvaluate_FailModeResolver_NoConfig pins the empty-result posture:
// when no level configures a policy the resolver returns a None source
// and the orchestrator leaves EffectiveFailModePolicy nil. No error,
// outcome unchanged.
func TestEvaluate_FailModeResolver_NoConfig(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-fmp-none")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-fmp-none")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-fmp-none", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("outcome must be Accept when no policy configured; got %q", res.Outcome)
	}
	if res.EffectiveFailModePolicy != nil {
		t.Errorf("EffectiveFailModePolicy must be nil when no policy configured at any level; got %+v",
			res.EffectiveFailModePolicy)
	}
}

// TestEvaluate_FailModeResolver_FailureDoesNotChangeOutcome pins the
// **D27j-impl-2 temporary error posture**: a non-empty surface override
// that does not resolve to an active policy logs a warning but does NOT
// fail the evaluation and does NOT change the outcome. The runtime
// resolver is observability-only in this tranche; the apply-time
// validator (`checkFailModePolicyReference`) is the authoritative gate
// that prevents this state from ever being reachable in normal approved
// configuration. A future tranche will tighten this when the resolved
// policy is plumbed into outcome computation.
func TestEvaluate_FailModeResolver_FailureDoesNotChangeOutcome(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-fmp-broken")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-fmp-broken")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	// Reference a policy that does NOT exist — resolver returns
	// ErrFailModePolicyResolutionFailed.
	setSurfaceFailModePolicyID(t, r, "surf-fmp-broken", "fmp-does-not-exist")

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-fmp-broken", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate must not error on resolver failure (D27j-impl-2 posture); got %v", err)
	}
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("outcome must be unchanged on resolver failure; got %q", res.Outcome)
	}
	if res.EffectiveFailModePolicy != nil {
		t.Errorf("EffectiveFailModePolicy must be nil when resolution fails; got %+v",
			res.EffectiveFailModePolicy)
	}
}

// TestEvaluate_FailModeResolver_FailModeKeysScopedToResolvedEvent pins the
// D27j-impl-3 scoping rule: only the FAIL_MODE_POLICY_RESOLVED event may
// carry FailModePolicy keys (fail_mode_policy_id, fail_mode_policy_version,
// source). No other audit-event payload — including POLICY_EVALUATED —
// may carry these keys, because the resolver remains observability-only
// and must not leak into outcome-shaping events. This replaces the
// pre-D27j-impl-3 global "no audit payload contains fail_mode_policy_id"
// invariant, which was correct under -2 but is exactly what -3 changes.
func TestEvaluate_FailModeResolver_FailModeKeysScopedToResolvedEvent(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-fmp-audit")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-fmp-audit")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	seedFailModePolicyForResolver(t, r, "fmp-audit", 1, failmode.FailModePolicyStatusActive)
	setSurfaceFailModePolicyID(t, r, "surf-fmp-audit", "fmp-audit")

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-fmp-audit", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	scopedKeys := map[string]struct{}{
		"fail_mode_policy_id":      {},
		"fail_mode_policy_version": {},
		"source":                   {},
	}
	for _, e := range events {
		if e.EventType == audit.AuditEventFailModePolicyResolved {
			continue
		}
		for k := range e.Payload {
			if _, scoped := scopedKeys[k]; scoped {
				t.Errorf("event %q payload contains key %q reserved for FAIL_MODE_POLICY_RESOLVED only",
					e.EventType, k)
			}
		}
	}
	// Sanity: the resolver result actually populated.
	if res.EffectiveFailModePolicy == nil || res.EffectiveFailModePolicy.PolicyID != "fmp-audit" {
		t.Errorf("expected EffectiveFailModePolicy to be populated; got %+v", res.EffectiveFailModePolicy)
	}
	_ = audit.AuditEventEvaluationStarted // keep the import live across reorganisations
}

// TestEvaluate_FailModeResolver_NotInResolvedEnvelope pins that the
// resolver does not modify Resolved.Structure.BusinessService (the
// audit-chain payload — modifying it would be a contract break). The
// resolver result is read off the EvaluationResult Go struct only.
func TestEvaluate_FailModeResolver_NotInResolvedEnvelope(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-fmp-resolved")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-fmp-resolved")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	seedFailModePolicyForResolver(t, r, "fmp-resolved", 1, failmode.FailModePolicyStatusActive)
	setSurfaceFailModePolicyID(t, r, "surf-fmp-resolved", "fmp-resolved")

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-fmp-resolved", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	got, err := r.envelopes.GetByID(context.Background(), res.EnvelopeID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: env=%v err=%v", got, err)
	}
	// Resolved structure carries the BS snapshot; the snapshot has no
	// FailModePolicy* fields. This is a compile-time guarantee enforced
	// by the envelope.BusinessServiceSnapshot type definition; the
	// runtime check below is a belt-and-braces invariant.
	bs := got.Resolved.Structure.BusinessService
	if bs.ID != "bs-test" {
		t.Errorf("expected bs-test snapshot; got %+v", bs)
	}
}

// _ keeps the surface import live in case future tests need typed
// surface lifecycle helpers.
var _ = surface.SurfaceStatusActive
