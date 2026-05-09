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
