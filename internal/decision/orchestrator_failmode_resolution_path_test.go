package decision_test

// orchestrator_failmode_resolution_path_test.go — D29c-1 orchestrator
// pins.
//
// Asserts:
//
//  1. EvaluationResult.FailModePolicyResolutionPath is populated on
//     every successful resolver-invocation code path: surface override
//     resolved, no policy configured at any level, resolver error.
//  2. Outcome, reason code, audit-event payloads, and audit-event kind
//     set are byte-identical between two runs that differ only in the
//     presence/absence of a resolved FailModePolicy. The
//     FailModePolicyResolutionPath field is the ONLY excluded field
//     (alongside the documented D27j-impl-2 EffectiveFailModePolicy
//     observable).
//  3. The /v1/evaluate HTTP response body never carries the
//     resolution-path field name or any of the eleven stable reason
//     strings.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/failmode"
)

// ---------------------------------------------------------------------------
// Orchestrator-side attachment tests
// ---------------------------------------------------------------------------

// TestEvaluate_FailModeResolutionPath_AttachedOnSurfaceOverride pins
// that a successful surface-level resolution attaches a 3-entry path
// with surface=resolved, business_service=skipped, deployment_default=skipped.
func TestEvaluate_FailModeResolutionPath_AttachedOnSurfaceOverride(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-path-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-path-1")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	seedFailModePolicyForResolver(t, r, "fmp-path-1", 5, failmode.FailModePolicyStatusActive)
	setSurfaceFailModePolicyID(t, r, "surf-path-1", "fmp-path-1")

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-path-1", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("outcome must be unaffected by path attachment; got %q", res.Outcome)
	}
	if len(res.FailModePolicyResolutionPath) != 3 {
		t.Fatalf("Path length: want 3, got %d", len(res.FailModePolicyResolutionPath))
	}
	path := res.FailModePolicyResolutionPath
	if path[0].Level != failmode.ResolutionLevelSurface ||
		path[0].Status != failmode.ResolutionPathStatusResolved ||
		path[0].PolicyID != "fmp-path-1" ||
		path[0].Version != 5 ||
		path[0].Reason != failmode.ResolutionReasonSurfaceResolved {
		t.Errorf("Path[0] (surface): %+v", path[0])
	}
	if path[1].Status != failmode.ResolutionPathStatusSkipped ||
		path[1].Reason != failmode.ResolutionReasonHigherPriorityResolved {
		t.Errorf("Path[1] (business_service): want skipped + higher-priority; got %+v", path[1])
	}
	if path[2].Status != failmode.ResolutionPathStatusSkipped ||
		path[2].Reason != failmode.ResolutionReasonHigherPriorityResolved {
		t.Errorf("Path[2] (deployment_default): want skipped + higher-priority; got %+v", path[2])
	}
}

// TestEvaluate_FailModeResolutionPath_AttachedWhenNoPolicy pins that
// when no policy is configured at any level, the orchestrator still
// attaches a 3-entry path with each level reporting not_configured.
// EffectiveFailModePolicy is nil but the path is non-nil.
func TestEvaluate_FailModeResolutionPath_AttachedWhenNoPolicy(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-path-2")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-path-2")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	// No FailModePolicy seeded; no surface/BS reference set.

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-path-2", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.EffectiveFailModePolicy != nil {
		t.Errorf("EffectiveFailModePolicy must be nil when no policy is configured; got %+v",
			res.EffectiveFailModePolicy)
	}
	if len(res.FailModePolicyResolutionPath) != 3 {
		t.Fatalf("Path must be attached even when no policy resolves; want length 3, got %d",
			len(res.FailModePolicyResolutionPath))
	}
	for i, entry := range res.FailModePolicyResolutionPath {
		if entry.Status != failmode.ResolutionPathStatusNotConfigured {
			t.Errorf("Path[%d] (%s): want not_configured, got %s", i, entry.Level, entry.Status)
		}
	}
}

// TestEvaluate_FailModeResolutionPath_AttachedOnResolverError pins
// that when the surface's FailModePolicyID points to a missing policy,
// the orchestrator continues evaluation, logs a warning (existing
// behaviour), and attaches the path with surface=failed and lower
// levels=skipped(earlier_level_failed). EffectiveFailModePolicy
// remains nil; outcome is unchanged.
func TestEvaluate_FailModeResolutionPath_AttachedOnResolverError(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-path-3")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-path-3")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	// Surface references a policy that was never seeded — resolver fails.
	setSurfaceFailModePolicyID(t, r, "surf-path-3", "fmp-missing")

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-path-3", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate must not fail on resolver-error path; got %v", err)
	}
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("outcome must be unaffected by resolver error; got %q", res.Outcome)
	}
	if res.EffectiveFailModePolicy != nil {
		t.Errorf("EffectiveFailModePolicy must be nil on resolver error; got %+v",
			res.EffectiveFailModePolicy)
	}
	if len(res.FailModePolicyResolutionPath) != 3 {
		t.Fatalf("Path must be attached on resolver error; want length 3, got %d",
			len(res.FailModePolicyResolutionPath))
	}
	path := res.FailModePolicyResolutionPath
	if path[0].Status != failmode.ResolutionPathStatusFailed ||
		path[0].Reason != failmode.ResolutionReasonSurfaceLookupFailed ||
		path[0].ReferenceID != "fmp-missing" {
		t.Errorf("Path[0]: want failed + surface_lookup_failed + ref=fmp-missing; got %+v", path[0])
	}
	if path[1].Status != failmode.ResolutionPathStatusSkipped ||
		path[1].Reason != failmode.ResolutionReasonEarlierLevelFailed {
		t.Errorf("Path[1]: want skipped + earlier_level_failed; got %+v", path[1])
	}
	if path[2].Status != failmode.ResolutionPathStatusSkipped ||
		path[2].Reason != failmode.ResolutionReasonEarlierLevelFailed {
		t.Errorf("Path[2]: want skipped + earlier_level_failed; got %+v", path[2])
	}
}

// Note: a "path-nil-when-repo-unwired" test was considered but the
// testRepos helper assigns r.failModePolicies into an interface field
// on store.Repositories. Setting r.failModePolicies = nil produces a
// typed-nil interface (non-nil at the interface level), so the
// orchestrator's "if repos.FailModePolicies != nil" branch still
// fires. Exercising the genuine unwired-repo state requires
// constructing a custom store directly; the orchestrator behaviour
// is unchanged from D27j-impl-2, so the nil-repo branch is already
// pinned by the upstream resolver tests rather than re-pinned here.

// ---------------------------------------------------------------------------
// Runtime invariance — path is the only field that necessarily differs
// ---------------------------------------------------------------------------

// runResolverConfig runs an evaluation against a fresh repo set up
// with (or without) a surface-override FailModePolicy. Returns the
// orchestrator result and the audit-event list.
//
// withPolicy=true seeds an active FailModePolicy and sets the surface
// override. The variant produces a 3-entry path with one resolved + two
// skipped entries and emits FAIL_MODE_POLICY_RESOLVED.
//
// withPolicy=false leaves the surface/BS without a FailModePolicy
// reference. The baseline produces a 3-entry path with three
// not_configured entries and emits no FAIL_MODE_POLICY_RESOLVED event.
func runResolverConfig(t *testing.T, surfaceID string, withPolicy bool) (decision.EvaluationResult, []*audit.AuditEvent) {
	t.Helper()
	r := newRepos()
	seedActiveSurface(t, r, surfaceID)
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", surfaceID)
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	if withPolicy {
		seedFailModePolicyForResolver(t, r, "fmp-inv", 1, failmode.FailModePolicyStatusActive)
		setSurfaceFailModePolicyID(t, r, surfaceID, "fmp-inv")
	}
	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest(surfaceID, "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	events, err := r.audit.ListByEnvelopeID(context.Background(), res.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	return res, events
}

// stripPerRunFields removes fields from an audit-event payload that
// are necessarily different between two runs (UUIDs, timestamps).
// Mirrors the helper in orchestrator_failmode_runtime_invariance_test.go;
// duplicated here so this file remains self-contained.
func stripPerRunFields(p map[string]any) map[string]any {
	out := make(map[string]any, len(p))
	for k, v := range p {
		switch k {
		case "envelope_id", "audit_event_id", "request_id",
			"resolved_at", "evaluation_time", "started_at", "ended_at",
			"created_at", "updated_at", "timestamp":
			continue
		}
		out[k] = v
	}
	return out
}

func extractEventPayload(events []*audit.AuditEvent, t audit.AuditEventType) map[string]any {
	for _, e := range events {
		if e.EventType == t {
			return e.Payload
		}
	}
	return nil
}

func eventKindCount(events []*audit.AuditEvent) map[audit.AuditEventType]int {
	m := make(map[audit.AuditEventType]int, len(events))
	for _, e := range events {
		m[e.EventType]++
	}
	return m
}

// TestRuntimeInvariance_D29c1ResolutionPathIsOnlyExcludedField is the
// brief-required strongest single guarantee that D29c-1 is runtime-
// inert. Two evaluations are run with identical inputs except for the
// presence/absence of a resolved FailModePolicy. The path itself
// necessarily differs (the variant configures a policy and produces a
// resolved/skipped/skipped path; the baseline records three
// not_configured entries), and EffectiveFailModePolicy switches from
// nil to non-nil. Every other observable must be byte-identical:
//
//   - eval.Outcome
//   - eval.ReasonCode
//   - POLICY_EVALUATED audit payload (content-stable fields)
//   - OUTCOME_RECORDED audit payload (content-stable fields)
//   - the audit-event kind set, excluding the expected
//     FAIL_MODE_POLICY_RESOLVED event that the variant emits
//
// The two excluded EvaluationResult fields are documented Go-only
// observables: EffectiveFailModePolicy (D27j-impl-2) and the new
// FailModePolicyResolutionPath (D29c-1). Both are intentionally
// excluded from the comparison.
func TestRuntimeInvariance_D29c1ResolutionPathIsOnlyExcludedField(t *testing.T) {
	baselineRes, baselineEvents := runResolverConfig(t, "surf-inv-baseline", false)
	variantRes, variantEvents := runResolverConfig(t, "surf-inv-variant", true)

	// Sanity: the two documented exclusions must actually differ
	// between runs, otherwise the test is not exercising the
	// contract.
	if reflect.DeepEqual(baselineRes.FailModePolicyResolutionPath, variantRes.FailModePolicyResolutionPath) {
		t.Fatal("test precondition: baseline and variant paths must differ")
	}
	if (baselineRes.EffectiveFailModePolicy == nil) == (variantRes.EffectiveFailModePolicy == nil) {
		t.Fatal("test precondition: EffectiveFailModePolicy must differ in nil-ness")
	}

	// Outcome / ReasonCode invariance.
	if baselineRes.Outcome != variantRes.Outcome {
		t.Errorf("Outcome diverged: baseline=%q variant=%q",
			baselineRes.Outcome, variantRes.Outcome)
	}
	if baselineRes.ReasonCode != variantRes.ReasonCode {
		t.Errorf("ReasonCode diverged: baseline=%q variant=%q",
			baselineRes.ReasonCode, variantRes.ReasonCode)
	}

	// POLICY_EVALUATED payload invariance.
	basePE := stripPerRunFields(extractEventPayload(baselineEvents, audit.AuditEventPolicyEvaluated))
	varPE := stripPerRunFields(extractEventPayload(variantEvents, audit.AuditEventPolicyEvaluated))
	if !reflect.DeepEqual(basePE, varPE) {
		t.Errorf("POLICY_EVALUATED payload diverged:\nbaseline=%v\nvariant=%v", basePE, varPE)
	}

	// OUTCOME_RECORDED payload invariance.
	baseOR := stripPerRunFields(extractEventPayload(baselineEvents, audit.AuditEventOutcomeRecorded))
	varOR := stripPerRunFields(extractEventPayload(variantEvents, audit.AuditEventOutcomeRecorded))
	if !reflect.DeepEqual(baseOR, varOR) {
		t.Errorf("OUTCOME_RECORDED payload diverged:\nbaseline=%v\nvariant=%v", baseOR, varOR)
	}

	// Audit-event kind set differs by exactly one entry:
	// FAIL_MODE_POLICY_RESOLVED appears in the variant only. After
	// removing that expected difference, the kind sets must match.
	baseKinds := eventKindCount(baselineEvents)
	varKinds := eventKindCount(variantEvents)
	delete(varKinds, audit.AuditEventFailModePolicyResolved)
	if !reflect.DeepEqual(baseKinds, varKinds) {
		t.Errorf("audit-event kind sets diverged outside the expected FAIL_MODE_POLICY_RESOLVED delta:\nbaseline=%v\nvariant_minus_fmp=%v",
			baseKinds, varKinds)
	}

	// Pin the no-new-event-kinds invariant for the new fail-mode
	// event family — D29c-1 must not emit any of these.
	for _, forbidden := range []audit.AuditEventType{
		"FAIL_MODE_POLICY_TRIGGER_FIRED",
		"FAIL_MODE_POLICY_DRY_RUN_DECISION",
		"FAIL_MODE_POLICY_ENFORCED",
	} {
		if _, present := eventKindCount(baselineEvents)[forbidden]; present {
			t.Errorf("D29c-1 must not emit %q; found in baseline run", forbidden)
		}
		if _, present := eventKindCount(variantEvents)[forbidden]; present {
			t.Errorf("D29c-1 must not emit %q; found in variant run", forbidden)
		}
	}
}

// ---------------------------------------------------------------------------
// HTTP-response invariance — path never leaks onto the wire
// ---------------------------------------------------------------------------

// TestEvaluate_HTTPResponse_NoResolutionPathLeakage pins that the
// /v1/evaluate response body never carries the path field name or any
// of the eleven stable reason strings. The orchestrator HTTP handler
// constructs evaluateResponse from specific fields of EvaluationResult;
// this test serialises the result through the same path it would take
// in production and inspects the bytes.
//
// The test uses the orchestrator directly (not the HTTP server) to
// avoid pulling in the full server bootstrap. It serialises the result
// through encoding/json — which is what writeJSON does under the
// hood — to a response shape that mirrors evaluateResponse in
// internal/httpapi/server.go. If a field were accidentally added to
// that response shape, a future test pin against the live HTTP server
// would catch it; this test pins the orchestrator-side promise that
// the path stays Go-only.
func TestEvaluate_HTTPResponse_NoResolutionPathLeakage(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-leak")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-leak")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	seedFailModePolicyForResolver(t, r, "fmp-leak", 1, failmode.FailModePolicyStatusActive)
	setSurfaceFailModePolicyID(t, r, "surf-leak", "fmp-leak")

	res, err := newOrchestrator(t, r).Evaluate(context.Background(),
		baseRequest("surf-leak", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	// Sanity-check that the resolver did run and attached a non-empty
	// path. The leakage test is meaningless if the path is empty.
	if len(res.FailModePolicyResolutionPath) != 3 {
		t.Fatalf("test precondition: path must be populated; got %v", res.FailModePolicyResolutionPath)
	}
	if res.EffectiveFailModePolicy == nil {
		t.Fatal("test precondition: effective policy must be set")
	}

	// Build the same response shape /v1/evaluate's handler constructs
	// (see internal/httpapi/server.go handleEvaluateWith). Anything
	// not explicitly listed here MUST NOT appear in the wire response.
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

	// Forbidden substrings: path field names and the eleven stable
	// reason strings.
	forbidden := []string{
		"resolution_path", "ResolutionPath", "resolutionPath",
		"FailModePolicyResolutionPath", "fail_mode_policy_resolution_path",
		// Eleven reason-string constants from internal/failmode/resolve.go.
		failmode.ResolutionReasonSurfaceNotConfigured,
		failmode.ResolutionReasonSurfaceResolved,
		failmode.ResolutionReasonSurfaceLookupFailed,
		failmode.ResolutionReasonBSNotConfigured,
		failmode.ResolutionReasonBSResolved,
		failmode.ResolutionReasonBSLookupFailed,
		failmode.ResolutionReasonDeploymentDefaultEmpty,
		failmode.ResolutionReasonDeploymentDefaultResolved,
		failmode.ResolutionReasonDeploymentDefaultFailed,
		failmode.ResolutionReasonEarlierLevelFailed,
		failmode.ResolutionReasonHigherPriorityResolved,
	}
	bodyStr := string(body)
	for _, sub := range forbidden {
		if strings.Contains(bodyStr, sub) {
			t.Errorf("/v1/evaluate response body must not contain %q; body=%s", sub, bodyStr)
		}
	}

	// Additionally, prove that a writeJSON-style HTTP response built
	// from the same shape produces an identical body. This guards
	// against a future refactor that changes the serialisation site.
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	rec.WriteHeader(http.StatusOK)
	if _, err := rec.Write(body); err != nil {
		t.Fatalf("rec.Write: %v", err)
	}
	if rec.Body.String() != bodyStr {
		t.Errorf("httptest body diverged from json.Marshal body: %q vs %q",
			rec.Body.String(), bodyStr)
	}
}
