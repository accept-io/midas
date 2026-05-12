package failmode_test

// resolve_test.go — exercises the effective-policy resolver for
// D27j-impl-2. Uses an instrumented fake finder that records every
// FindActiveAt call and fails the test if ListVersions or any other
// non-bounded operation is invoked. The fake also returns nil-Rules
// policies so any code that incorrectly inspects rules would panic on
// nil dereference — pinning the "no rule consultation" invariant.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/failmode"
	"github.com/accept-io/midas/internal/surface"
)

// fakeActiveFinder records every FindActiveAt call. It returns the
// configured policy when (id, at) lands inside a registered active
// window; nil otherwise. Tests inject custom policies via Set.
type fakeActiveFinder struct {
	calls    []finderCall
	policies map[string]*failmode.FailModePolicy
}

type finderCall struct {
	id string
	at time.Time
}

func newFakeActiveFinder() *fakeActiveFinder {
	return &fakeActiveFinder{policies: map[string]*failmode.FailModePolicy{}}
}

func (f *fakeActiveFinder) FindActiveAt(_ context.Context, id string, at time.Time) (*failmode.FailModePolicy, error) {
	f.calls = append(f.calls, finderCall{id: id, at: at})
	p, ok := f.policies[id]
	if !ok {
		return nil, nil
	}
	// Policies registered with Set are treated as active at any time.
	cp := *p
	// Deliberately leave Rules nil — any caller that inspects Rules
	// crashes the test. Pins the "no rule consultation" invariant.
	cp.Rules = nil
	return &cp, nil
}

func (f *fakeActiveFinder) Set(id string, version int) {
	f.policies[id] = &failmode.FailModePolicy{
		ID:      id,
		Version: version,
		Status:  failmode.FailModePolicyStatusActive,
	}
}

// TestResolve_SurfaceOverrideWins
func TestResolve_SurfaceOverrideWins(t *testing.T) {
	finder := newFakeActiveFinder()
	finder.Set("surface-policy", 3)
	finder.Set("bs-policy", 1)
	finder.Set("deployment-default", 1)

	sur := &surface.DecisionSurface{ID: "surf-1", FailModePolicyID: "surface-policy"}
	bs := &businessservice.BusinessService{ID: "bs-1", FailModePolicyID: "bs-policy"}
	at := time.Now().UTC()

	got, err := failmode.Resolve(context.Background(), finder, sur, bs, at, "deployment-default")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.PolicyID != "surface-policy" {
		t.Errorf("PolicyID: want surface-policy, got %q", got.PolicyID)
	}
	if got.Version != 3 {
		t.Errorf("Version: want 3, got %d", got.Version)
	}
	if got.Source != failmode.ResolutionSourceSurface {
		t.Errorf("Source: want surface, got %q", got.Source)
	}
	if len(finder.calls) != 1 {
		t.Errorf("FindActiveAt calls: want exactly 1 (surface only), got %d", len(finder.calls))
	}
}

func TestResolve_BusinessServiceFallbackUsedWhenSurfaceEmpty(t *testing.T) {
	finder := newFakeActiveFinder()
	finder.Set("bs-policy", 5)
	finder.Set("deployment-default", 1)

	sur := &surface.DecisionSurface{ID: "surf-1"} // no override
	bs := &businessservice.BusinessService{ID: "bs-1", FailModePolicyID: "bs-policy"}
	at := time.Now().UTC()

	got, err := failmode.Resolve(context.Background(), finder, sur, bs, at, "deployment-default")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.PolicyID != "bs-policy" {
		t.Errorf("PolicyID: want bs-policy, got %q", got.PolicyID)
	}
	if got.Version != 5 {
		t.Errorf("Version: want 5, got %d", got.Version)
	}
	if got.Source != failmode.ResolutionSourceBusinessService {
		t.Errorf("Source: want business_service, got %q", got.Source)
	}
}

func TestResolve_DeploymentDefaultUsedWhenBothEmpty(t *testing.T) {
	finder := newFakeActiveFinder()
	finder.Set("deployment-default", 7)

	sur := &surface.DecisionSurface{ID: "surf-1"}
	bs := &businessservice.BusinessService{ID: "bs-1"}
	at := time.Now().UTC()

	got, err := failmode.Resolve(context.Background(), finder, sur, bs, at, "deployment-default")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.PolicyID != "deployment-default" {
		t.Errorf("PolicyID: want deployment-default, got %q", got.PolicyID)
	}
	if got.Version != 7 {
		t.Errorf("Version: want 7, got %d", got.Version)
	}
	if got.Source != failmode.ResolutionSourceDeploymentDefault {
		t.Errorf("Source: want deployment_default, got %q", got.Source)
	}
}

func TestResolve_EmptyResultWhenNoneConfigured(t *testing.T) {
	finder := newFakeActiveFinder()

	sur := &surface.DecisionSurface{ID: "surf-1"}
	bs := &businessservice.BusinessService{ID: "bs-1"}
	at := time.Now().UTC()

	got, err := failmode.Resolve(context.Background(), finder, sur, bs, at, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.PolicyID != "" || got.Version != 0 {
		t.Errorf("expected empty result, got %+v", got)
	}
	if got.Source != failmode.ResolutionSourceNone {
		t.Errorf("Source: want empty (none), got %q", got.Source)
	}
	if len(finder.calls) != 0 {
		t.Errorf("FindActiveAt calls: want 0, got %d", len(finder.calls))
	}
}

func TestResolve_SurfaceRefMissing_ReturnsError_NoFallback(t *testing.T) {
	finder := newFakeActiveFinder()
	finder.Set("bs-policy", 1)
	// surface-policy NOT registered; FindActiveAt returns nil.

	sur := &surface.DecisionSurface{ID: "surf-1", FailModePolicyID: "surface-policy"}
	bs := &businessservice.BusinessService{ID: "bs-1", FailModePolicyID: "bs-policy"}
	at := time.Now().UTC()

	_, err := failmode.Resolve(context.Background(), finder, sur, bs, at, "")
	if err == nil {
		t.Fatal("expected resolution error; got nil")
	}
	if !errors.Is(err, failmode.ErrFailModePolicyResolutionFailed) {
		t.Errorf("error chain: want ErrFailModePolicyResolutionFailed; got %v", err)
	}
	// Confirms only the surface lookup ran — no fallback.
	if len(finder.calls) != 1 || finder.calls[0].id != "surface-policy" {
		t.Errorf("FindActiveAt calls: want exactly 1 (surface) with id=surface-policy; got %+v", finder.calls)
	}
}

func TestResolve_BusinessServiceRefNotActive_ReturnsError_NoFallback(t *testing.T) {
	finder := newFakeActiveFinder()
	// bs-policy NOT registered.
	finder.Set("deployment-default", 1)

	sur := &surface.DecisionSurface{ID: "surf-1"}
	bs := &businessservice.BusinessService{ID: "bs-1", FailModePolicyID: "bs-policy"}
	at := time.Now().UTC()

	_, err := failmode.Resolve(context.Background(), finder, sur, bs, at, "deployment-default")
	if err == nil {
		t.Fatal("expected resolution error; got nil")
	}
	if !errors.Is(err, failmode.ErrFailModePolicyResolutionFailed) {
		t.Errorf("error chain: want ErrFailModePolicyResolutionFailed; got %v", err)
	}
	// No fallback to deployment-default.
	for _, c := range finder.calls {
		if c.id == "deployment-default" {
			t.Errorf("must not fall back to deployment-default after BusinessService failure")
		}
	}
}

func TestResolve_DeploymentDefaultNotActive_ReturnsError(t *testing.T) {
	finder := newFakeActiveFinder()
	// deployment-default NOT registered.

	sur := &surface.DecisionSurface{ID: "surf-1"}
	bs := &businessservice.BusinessService{ID: "bs-1"}
	at := time.Now().UTC()

	_, err := failmode.Resolve(context.Background(), finder, sur, bs, at, "deployment-default")
	if err == nil {
		t.Fatal("expected resolution error; got nil")
	}
	if !errors.Is(err, failmode.ErrFailModePolicyResolutionFailed) {
		t.Errorf("error chain: want ErrFailModePolicyResolutionFailed; got %v", err)
	}
}

func TestResolve_NeverInspectsRules(t *testing.T) {
	// fakeActiveFinder.FindActiveAt deliberately returns policies with
	// nil Rules. If Resolve ever inspected r.Rules, it would either panic
	// or silently produce nonsense. This test passing is the proof that
	// the resolver does not consult rules.
	finder := newFakeActiveFinder()
	finder.Set("surface-policy", 1)

	sur := &surface.DecisionSurface{ID: "surf-1", FailModePolicyID: "surface-policy"}
	at := time.Now().UTC()

	got, err := failmode.Resolve(context.Background(), finder, sur, nil, at, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.PolicyID != "surface-policy" {
		t.Errorf("PolicyID: want surface-policy, got %q", got.PolicyID)
	}
}

// TestResolve_NeverCallsListVersions: the fake finder only implements
// FindActiveAt. If Resolve required ListVersions or any other method,
// the resolver would not compile against the activeFailModePolicyFinder
// interface — proven by the package compiling. This test additionally
// verifies the full resolver path with all three sources empty results
// in zero finder calls.
func TestResolve_NeverCallsListVersions(t *testing.T) {
	finder := newFakeActiveFinder()
	sur := &surface.DecisionSurface{}
	bs := &businessservice.BusinessService{}
	at := time.Now().UTC()

	got, err := failmode.Resolve(context.Background(), finder, sur, bs, at, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Source != failmode.ResolutionSourceNone {
		t.Errorf("Source: want None, got %q", got.Source)
	}
	if len(finder.calls) != 0 {
		t.Errorf("calls: want 0 (no configuration), got %d", len(finder.calls))
	}
}

func TestResolve_NilFinderReturnsError(t *testing.T) {
	sur := &surface.DecisionSurface{ID: "surf-1", FailModePolicyID: "p"}
	at := time.Now().UTC()

	_, err := failmode.Resolve(context.Background(), nil, sur, nil, at, "")
	if err == nil {
		t.Fatal("expected error for nil finder")
	}
}

func TestResolve_ZeroEvaluationTimeReturnsError(t *testing.T) {
	finder := newFakeActiveFinder()
	finder.Set("surface-policy", 1)
	sur := &surface.DecisionSurface{ID: "surf-1", FailModePolicyID: "surface-policy"}

	_, err := failmode.Resolve(context.Background(), finder, sur, nil, time.Time{}, "")
	if err == nil {
		t.Fatal("expected error for zero evaluation time")
	}
}

// TestResolve_NilSurfaceAndBusinessService_StillResolvesDeploymentDefault
// pins the defensive nil-handling: callers that pass nil surface/bs
// should still receive deployment-default resolution (or empty).
func TestResolve_NilSurfaceAndBusinessService_StillResolvesDeploymentDefault(t *testing.T) {
	finder := newFakeActiveFinder()
	finder.Set("deployment-default", 1)

	at := time.Now().UTC()
	got, err := failmode.Resolve(context.Background(), finder, nil, nil, at, "deployment-default")
	if err != nil {
		t.Fatalf("Resolve with nil surface and bs should still resolve deployment default; got %v", err)
	}
	if got.Source != failmode.ResolutionSourceDeploymentDefault {
		t.Errorf("Source: want deployment_default, got %q", got.Source)
	}
}

// ---------------------------------------------------------------------------
// D29c-1 — ResolveWithPath path-shape tests
//
// Every test below asserts the fixed-length-3 path produced by
// ResolveWithPath. The path's ordering is invariant: index 0 is
// surface, index 1 is business_service, index 2 is deployment_default.
// Path-entry Reason strings come from the failmode reason-string
// constants — tests reference the constants rather than literals so a
// future reason-vocabulary change updates both implementation and
// tests in lockstep.
// ---------------------------------------------------------------------------

// assertPathEntry pins every field of a single path entry. Splits the
// long argument list across multiple lines so per-test calls remain
// readable.
func assertPathEntry(t *testing.T, entry failmode.ResolutionPathEntry,
	wantLevel failmode.ResolutionLevel,
	wantStatus failmode.ResolutionPathStatus,
	wantSource failmode.ResolutionSource,
	wantRef, wantPolicyID, wantReason string,
	wantVersion int,
) {
	t.Helper()
	if entry.Level != wantLevel {
		t.Errorf("Level: want %q, got %q", wantLevel, entry.Level)
	}
	if entry.Status != wantStatus {
		t.Errorf("Status (level=%q): want %q, got %q", wantLevel, wantStatus, entry.Status)
	}
	if entry.Source != wantSource {
		t.Errorf("Source (level=%q): want %q, got %q", wantLevel, wantSource, entry.Source)
	}
	if entry.ReferenceID != wantRef {
		t.Errorf("ReferenceID (level=%q): want %q, got %q", wantLevel, wantRef, entry.ReferenceID)
	}
	if entry.PolicyID != wantPolicyID {
		t.Errorf("PolicyID (level=%q): want %q, got %q", wantLevel, wantPolicyID, entry.PolicyID)
	}
	if entry.Version != wantVersion {
		t.Errorf("Version (level=%q): want %d, got %d", wantLevel, wantVersion, entry.Version)
	}
	if entry.Reason != wantReason {
		t.Errorf("Reason (level=%q): want %q, got %q", wantLevel, wantReason, entry.Reason)
	}
}

func TestResolveWithPath_SurfaceOverrideWins(t *testing.T) {
	finder := newFakeActiveFinder()
	finder.Set("policy-surface", 7)
	sur := &surface.DecisionSurface{ID: "surf-1", FailModePolicyID: "policy-surface"}
	bs := &businessservice.BusinessService{ID: "bs-1", FailModePolicyID: "policy-bs"}

	at := time.Now().UTC()
	result, err := failmode.ResolveWithPath(context.Background(), finder, sur, bs, at, "policy-deployment")
	if err != nil {
		t.Fatalf("ResolveWithPath: unexpected error %v", err)
	}
	if len(result.Path) != 3 {
		t.Fatalf("Path length: want 3, got %d", len(result.Path))
	}
	if result.Effective.Source != failmode.ResolutionSourceSurface {
		t.Errorf("Effective.Source: want surface, got %q", result.Effective.Source)
	}
	if result.Effective.PolicyID != "policy-surface" || result.Effective.Version != 7 {
		t.Errorf("Effective: want (policy-surface,7), got (%q,%d)", result.Effective.PolicyID, result.Effective.Version)
	}
	assertPathEntry(t, result.Path[0], failmode.ResolutionLevelSurface,
		failmode.ResolutionPathStatusResolved, failmode.ResolutionSourceSurface,
		"policy-surface", "policy-surface", failmode.ResolutionReasonSurfaceResolved, 7)
	assertPathEntry(t, result.Path[1], failmode.ResolutionLevelBusinessService,
		failmode.ResolutionPathStatusSkipped, failmode.ResolutionSource(""),
		"", "", failmode.ResolutionReasonHigherPriorityResolved, 0)
	assertPathEntry(t, result.Path[2], failmode.ResolutionLevelDeploymentDefault,
		failmode.ResolutionPathStatusSkipped, failmode.ResolutionSource(""),
		"", "", failmode.ResolutionReasonHigherPriorityResolved, 0)
}

func TestResolveWithPath_BusinessServiceDefaultWins(t *testing.T) {
	finder := newFakeActiveFinder()
	finder.Set("policy-bs", 3)
	sur := &surface.DecisionSurface{ID: "surf-1"} // no FailModePolicyID
	bs := &businessservice.BusinessService{ID: "bs-1", FailModePolicyID: "policy-bs"}

	at := time.Now().UTC()
	result, err := failmode.ResolveWithPath(context.Background(), finder, sur, bs, at, "")
	if err != nil {
		t.Fatalf("ResolveWithPath: unexpected error %v", err)
	}
	if result.Effective.Source != failmode.ResolutionSourceBusinessService {
		t.Errorf("Effective.Source: want business_service, got %q", result.Effective.Source)
	}
	assertPathEntry(t, result.Path[0], failmode.ResolutionLevelSurface,
		failmode.ResolutionPathStatusNotConfigured, failmode.ResolutionSource(""),
		"", "", failmode.ResolutionReasonSurfaceNotConfigured, 0)
	assertPathEntry(t, result.Path[1], failmode.ResolutionLevelBusinessService,
		failmode.ResolutionPathStatusResolved, failmode.ResolutionSourceBusinessService,
		"policy-bs", "policy-bs", failmode.ResolutionReasonBSResolved, 3)
	assertPathEntry(t, result.Path[2], failmode.ResolutionLevelDeploymentDefault,
		failmode.ResolutionPathStatusSkipped, failmode.ResolutionSource(""),
		"", "", failmode.ResolutionReasonHigherPriorityResolved, 0)
}

func TestResolveWithPath_DeploymentDefaultWins(t *testing.T) {
	finder := newFakeActiveFinder()
	finder.Set("policy-deployment", 11)
	sur := &surface.DecisionSurface{ID: "surf-1"}
	bs := &businessservice.BusinessService{ID: "bs-1"}

	at := time.Now().UTC()
	result, err := failmode.ResolveWithPath(context.Background(), finder, sur, bs, at, "policy-deployment")
	if err != nil {
		t.Fatalf("ResolveWithPath: unexpected error %v", err)
	}
	if result.Effective.Source != failmode.ResolutionSourceDeploymentDefault {
		t.Errorf("Effective.Source: want deployment_default, got %q", result.Effective.Source)
	}
	assertPathEntry(t, result.Path[0], failmode.ResolutionLevelSurface,
		failmode.ResolutionPathStatusNotConfigured, failmode.ResolutionSource(""),
		"", "", failmode.ResolutionReasonSurfaceNotConfigured, 0)
	assertPathEntry(t, result.Path[1], failmode.ResolutionLevelBusinessService,
		failmode.ResolutionPathStatusNotConfigured, failmode.ResolutionSource(""),
		"", "", failmode.ResolutionReasonBSNotConfigured, 0)
	assertPathEntry(t, result.Path[2], failmode.ResolutionLevelDeploymentDefault,
		failmode.ResolutionPathStatusResolved, failmode.ResolutionSourceDeploymentDefault,
		"policy-deployment", "policy-deployment", failmode.ResolutionReasonDeploymentDefaultResolved, 11)
}

func TestResolveWithPath_NoPolicyAtAnyLevel(t *testing.T) {
	finder := newFakeActiveFinder()
	sur := &surface.DecisionSurface{ID: "surf-1"}
	bs := &businessservice.BusinessService{ID: "bs-1"}

	at := time.Now().UTC()
	result, err := failmode.ResolveWithPath(context.Background(), finder, sur, bs, at, "")
	if err != nil {
		t.Fatalf("ResolveWithPath: unexpected error %v", err)
	}
	if result.Effective.Source != failmode.ResolutionSourceNone {
		t.Errorf("Effective.Source: want None, got %q", result.Effective.Source)
	}
	assertPathEntry(t, result.Path[0], failmode.ResolutionLevelSurface,
		failmode.ResolutionPathStatusNotConfigured, failmode.ResolutionSource(""),
		"", "", failmode.ResolutionReasonSurfaceNotConfigured, 0)
	assertPathEntry(t, result.Path[1], failmode.ResolutionLevelBusinessService,
		failmode.ResolutionPathStatusNotConfigured, failmode.ResolutionSource(""),
		"", "", failmode.ResolutionReasonBSNotConfigured, 0)
	assertPathEntry(t, result.Path[2], failmode.ResolutionLevelDeploymentDefault,
		failmode.ResolutionPathStatusNotConfigured, failmode.ResolutionSource(""),
		"", "", failmode.ResolutionReasonDeploymentDefaultEmpty, 0)
}

func TestResolveWithPath_SurfaceExplicitReferenceFails(t *testing.T) {
	finder := newFakeActiveFinder()
	// Do NOT Set policy-surface; FindActiveAt returns nil, nil for
	// unknown IDs. That nil-result is treated as a lookup failure.
	sur := &surface.DecisionSurface{ID: "surf-1", FailModePolicyID: "policy-surface-missing"}
	bs := &businessservice.BusinessService{ID: "bs-1", FailModePolicyID: "policy-bs"}

	at := time.Now().UTC()
	result, err := failmode.ResolveWithPath(context.Background(), finder, sur, bs, at, "policy-deployment")
	if err == nil {
		t.Fatal("ResolveWithPath: expected error on surface explicit-reference failure")
	}
	if !errors.Is(err, failmode.ErrFailModePolicyResolutionFailed) {
		t.Errorf("error: want ErrFailModePolicyResolutionFailed wrapper; got %v", err)
	}
	if result.Effective.Source != failmode.ResolutionSourceNone {
		t.Errorf("Effective.Source: want None on failure, got %q", result.Effective.Source)
	}
	if len(result.Path) != 3 {
		t.Fatalf("Path length: want 3 even on failure, got %d", len(result.Path))
	}
	assertPathEntry(t, result.Path[0], failmode.ResolutionLevelSurface,
		failmode.ResolutionPathStatusFailed, failmode.ResolutionSource(""),
		"policy-surface-missing", "", failmode.ResolutionReasonSurfaceLookupFailed, 0)
	assertPathEntry(t, result.Path[1], failmode.ResolutionLevelBusinessService,
		failmode.ResolutionPathStatusSkipped, failmode.ResolutionSource(""),
		"", "", failmode.ResolutionReasonEarlierLevelFailed, 0)
	assertPathEntry(t, result.Path[2], failmode.ResolutionLevelDeploymentDefault,
		failmode.ResolutionPathStatusSkipped, failmode.ResolutionSource(""),
		"", "", failmode.ResolutionReasonEarlierLevelFailed, 0)
	// Pin no-fallthrough: BS would resolve if consulted (its policy is
	// in the finder) but the failure must short-circuit.
	if result.Effective.PolicyID != "" {
		t.Errorf("no-fallthrough invariant: Effective.PolicyID must be empty when surface fails; got %q",
			result.Effective.PolicyID)
	}
}

func TestResolveWithPath_BusinessServiceExplicitReferenceFails(t *testing.T) {
	finder := newFakeActiveFinder()
	sur := &surface.DecisionSurface{ID: "surf-1"} // no override
	bs := &businessservice.BusinessService{ID: "bs-1", FailModePolicyID: "policy-bs-missing"}

	at := time.Now().UTC()
	result, err := failmode.ResolveWithPath(context.Background(), finder, sur, bs, at, "policy-deployment")
	if err == nil {
		t.Fatal("ResolveWithPath: expected error on BS explicit-reference failure")
	}
	if !errors.Is(err, failmode.ErrFailModePolicyResolutionFailed) {
		t.Errorf("error: want ErrFailModePolicyResolutionFailed wrapper; got %v", err)
	}
	assertPathEntry(t, result.Path[0], failmode.ResolutionLevelSurface,
		failmode.ResolutionPathStatusNotConfigured, failmode.ResolutionSource(""),
		"", "", failmode.ResolutionReasonSurfaceNotConfigured, 0)
	assertPathEntry(t, result.Path[1], failmode.ResolutionLevelBusinessService,
		failmode.ResolutionPathStatusFailed, failmode.ResolutionSource(""),
		"policy-bs-missing", "", failmode.ResolutionReasonBSLookupFailed, 0)
	assertPathEntry(t, result.Path[2], failmode.ResolutionLevelDeploymentDefault,
		failmode.ResolutionPathStatusSkipped, failmode.ResolutionSource(""),
		"", "", failmode.ResolutionReasonEarlierLevelFailed, 0)
}

func TestResolveWithPath_DeploymentDefaultFails(t *testing.T) {
	finder := newFakeActiveFinder()
	sur := &surface.DecisionSurface{ID: "surf-1"}
	bs := &businessservice.BusinessService{ID: "bs-1"}

	at := time.Now().UTC()
	result, err := failmode.ResolveWithPath(context.Background(), finder, sur, bs, at, "policy-deployment-missing")
	if err == nil {
		t.Fatal("ResolveWithPath: expected error on deployment-default failure")
	}
	if !errors.Is(err, failmode.ErrFailModePolicyResolutionFailed) {
		t.Errorf("error: want ErrFailModePolicyResolutionFailed wrapper; got %v", err)
	}
	assertPathEntry(t, result.Path[0], failmode.ResolutionLevelSurface,
		failmode.ResolutionPathStatusNotConfigured, failmode.ResolutionSource(""),
		"", "", failmode.ResolutionReasonSurfaceNotConfigured, 0)
	assertPathEntry(t, result.Path[1], failmode.ResolutionLevelBusinessService,
		failmode.ResolutionPathStatusNotConfigured, failmode.ResolutionSource(""),
		"", "", failmode.ResolutionReasonBSNotConfigured, 0)
	assertPathEntry(t, result.Path[2], failmode.ResolutionLevelDeploymentDefault,
		failmode.ResolutionPathStatusFailed, failmode.ResolutionSource(""),
		"policy-deployment-missing", "", failmode.ResolutionReasonDeploymentDefaultFailed, 0)
}

// TestResolveWithPath_ReasonsAreNonSensitive pins that the path's
// Reason field never carries raw error text from the repository. The
// underlying FindActiveAt error is wrapped into the returned error
// (which the orchestrator logs via slog.Warn) but the path itself only
// carries the stable vocabulary.
func TestResolveWithPath_ReasonsAreNonSensitive(t *testing.T) {
	finder := &errorReturningFinder{err: errors.New("sensitive: dsn=postgres://user:pass@host/db")}
	sur := &surface.DecisionSurface{ID: "surf-1", FailModePolicyID: "policy-x"}

	at := time.Now().UTC()
	result, err := failmode.ResolveWithPath(context.Background(), finder, sur, nil, at, "")
	if err == nil {
		t.Fatal("expected error from finder")
	}
	if result.Path[0].Reason != failmode.ResolutionReasonSurfaceLookupFailed {
		t.Errorf("Reason: want stable %q, got %q",
			failmode.ResolutionReasonSurfaceLookupFailed, result.Path[0].Reason)
	}
	for i, entry := range result.Path {
		if strings.Contains(entry.Reason, "dsn=") {
			t.Errorf("Path[%d].Reason leaks repository internal error text: %q", i, entry.Reason)
		}
		if strings.Contains(entry.Reason, "sensitive") {
			t.Errorf("Path[%d].Reason leaks raw repository error: %q", i, entry.Reason)
		}
	}
}

// errorReturningFinder always returns the configured error. Used to
// exercise the path's reason-string vocabulary in the face of
// repository errors.
type errorReturningFinder struct {
	err error
}

func (f *errorReturningFinder) FindActiveAt(_ context.Context, _ string, _ time.Time) (*failmode.FailModePolicy, error) {
	return nil, f.err
}

// ---------------------------------------------------------------------------
// D29c-2 — ResolutionResult.Policy is populated on success
// ---------------------------------------------------------------------------

// fakeFinderWithRules returns a policy whose Rules slice carries the
// supplied entries. Mirrors fakeActiveFinder but preserves Rules
// (which fakeActiveFinder deliberately nils out to pin the no-rule-
// consultation invariant). The trigger-event tests in
// internal/decision need the resolver to surface the full policy.
type fakeFinderWithRules struct {
	policies map[string]*failmode.FailModePolicy
}

func newFakeFinderWithRules() *fakeFinderWithRules {
	return &fakeFinderWithRules{policies: map[string]*failmode.FailModePolicy{}}
}

func (f *fakeFinderWithRules) FindActiveAt(_ context.Context, id string, _ time.Time) (*failmode.FailModePolicy, error) {
	if p, ok := f.policies[id]; ok {
		return p, nil
	}
	return nil, nil
}

func (f *fakeFinderWithRules) Set(p *failmode.FailModePolicy) {
	f.policies[p.ID] = p
}

func newDefaultClosedRules() []failmode.FailModePolicyRule {
	return []failmode.FailModePolicyRule{
		{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed,
			EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed,
			EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable,
			EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: failmode.PermittedModeClosed,
			EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate,
			Reason: "policy evaluator unreachable"},
		{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeClosed,
			EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
	}
}

// TestResolveWithPath_AttachesPolicyOnSuccess pins that
// ResolutionResult.Policy carries the full domain entity (with Rules)
// when resolution succeeds at any level. Without this, the orchestrator
// would need a second FindActiveAt call to inspect rules for the
// trigger-event payload.
func TestResolveWithPath_AttachesPolicyOnSuccess(t *testing.T) {
	finder := newFakeFinderWithRules()
	finder.Set(&failmode.FailModePolicy{
		ID:      "fmp-x",
		Version: 2,
		Status:  failmode.FailModePolicyStatusActive,
		Rules:   newDefaultClosedRules(),
	})

	at := time.Now().UTC()

	// Surface override.
	result, err := failmode.ResolveWithPath(context.Background(), finder,
		&surface.DecisionSurface{ID: "surf-1", FailModePolicyID: "fmp-x"},
		nil, at, "")
	if err != nil {
		t.Fatalf("ResolveWithPath (surface): %v", err)
	}
	if result.Policy == nil {
		t.Fatal("Policy must be non-nil when surface resolves")
	}
	if result.Policy.ID != "fmp-x" || result.Policy.Version != 2 {
		t.Errorf("Policy identity: want fmp-x/v2, got %s/v%d", result.Policy.ID, result.Policy.Version)
	}
	if len(result.Policy.Rules) != 5 {
		t.Errorf("Policy.Rules length: want 5, got %d", len(result.Policy.Rules))
	}

	// Business service default.
	result, err = failmode.ResolveWithPath(context.Background(), finder,
		&surface.DecisionSurface{ID: "surf-1"},
		&businessservice.BusinessService{ID: "bs-1", FailModePolicyID: "fmp-x"},
		at, "")
	if err != nil {
		t.Fatalf("ResolveWithPath (bs): %v", err)
	}
	if result.Policy == nil || result.Policy.ID != "fmp-x" {
		t.Errorf("Policy must be non-nil when business service resolves; got %+v", result.Policy)
	}

	// Deployment default.
	result, err = failmode.ResolveWithPath(context.Background(), finder, nil, nil, at, "fmp-x")
	if err != nil {
		t.Fatalf("ResolveWithPath (deployment default): %v", err)
	}
	if result.Policy == nil || result.Policy.ID != "fmp-x" {
		t.Errorf("Policy must be non-nil when deployment default resolves; got %+v", result.Policy)
	}
}

func TestResolveWithPath_PolicyNilOnNoResolutionAndOnError(t *testing.T) {
	finder := newFakeFinderWithRules()

	at := time.Now().UTC()

	// No policy configured at any level.
	result, err := failmode.ResolveWithPath(context.Background(), finder,
		&surface.DecisionSurface{ID: "surf-1"},
		&businessservice.BusinessService{ID: "bs-1"},
		at, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Policy != nil {
		t.Errorf("Policy must be nil when no policy resolves; got %+v", result.Policy)
	}

	// Surface explicit reference fails (policy not in finder).
	result, _ = failmode.ResolveWithPath(context.Background(), finder,
		&surface.DecisionSurface{ID: "surf-1", FailModePolicyID: "fmp-missing"},
		nil, at, "")
	if result.Policy != nil {
		t.Errorf("Policy must be nil when resolution fails; got %+v", result.Policy)
	}
}

// ---------------------------------------------------------------------------
// D29c-2 — SelectRuleForClass
// ---------------------------------------------------------------------------

func TestSelectRuleForClass_FoundReturnsDefaultedRule(t *testing.T) {
	// Construct a policy whose resource rule has zero-value axis fields
	// to confirm ApplyRuleAxisDefaults is applied.
	rules := newDefaultClosedRules()
	for i := range rules {
		if rules[i].CorrectnessClass == failmode.CorrectnessClassResource {
			rules[i].EnforcementState = ""
			rules[i].Outcome = ""
		}
	}
	p := &failmode.FailModePolicy{ID: "x", Version: 1, Rules: rules}

	got, found := failmode.SelectRuleForClass(p, failmode.CorrectnessClassResource)
	if !found {
		t.Fatal("expected to find resource rule")
	}
	if got.PermittedMode != failmode.PermittedModeClosed {
		t.Errorf("PermittedMode: want closed, got %q", got.PermittedMode)
	}
	if got.EnforcementState != failmode.EnforcementStateEvidenceOnly {
		t.Errorf("EnforcementState default: want evidence_only, got %q", got.EnforcementState)
	}
	if got.Outcome != failmode.OutcomeEscalate {
		t.Errorf("Outcome default for closed: want escalate, got %q", got.Outcome)
	}
}

func TestSelectRuleForClass_NotFoundReturnsZero(t *testing.T) {
	// Policy with no resource rule (corrupted / hand-constructed).
	p := &failmode.FailModePolicy{
		ID:      "x",
		Version: 1,
		Rules: []failmode.FailModePolicyRule{
			{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed,
				EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		},
	}

	got, found := failmode.SelectRuleForClass(p, failmode.CorrectnessClassResource)
	if found {
		t.Errorf("expected not-found for missing resource rule; got %+v", got)
	}
	if got != (failmode.FailModePolicyRule{}) {
		t.Errorf("expected zero rule, got %+v", got)
	}
}

func TestSelectRuleForClass_NilPolicyReturnsNotFound(t *testing.T) {
	got, found := failmode.SelectRuleForClass(nil, failmode.CorrectnessClassResource)
	if found {
		t.Error("expected not-found on nil policy")
	}
	if got != (failmode.FailModePolicyRule{}) {
		t.Errorf("expected zero rule on nil policy, got %+v", got)
	}
}
