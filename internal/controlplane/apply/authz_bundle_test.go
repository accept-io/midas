package apply_test

// authz_bundle_test.go — tests the per-document permission check inside
// buildApplyPlan. The check is the fine-grained layer of the two-tier
// authorization model for POST /v1/controlplane/apply.
//
// These tests construct a KindAuthorizer directly (rather than through
// internal/httpapi) so the apply planner is tested in isolation.

import (
	"context"
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/store/memory"
)

// allowKindsAuthorizer returns an apply.KindAuthorizer that permits writes
// to any Kind in the allowed set, and denies all others with a synthetic
// <kind>:write permission string that matches what the real authz package
// would emit.
func allowKindsAuthorizer(allowed ...string) apply.KindAuthorizer {
	set := make(map[string]struct{}, len(allowed))
	for _, k := range allowed {
		set[k] = struct{}{}
	}
	return func(kind string) (bool, string) {
		if _, ok := set[kind]; ok {
			return true, ""
		}
		return false, strings.ToLower(kind) + ":write"
	}
}

// denyAllAuthorizer denies every Kind — used to prove that a principal
// holding controlplane:apply but no <kind>:write permissions cannot
// persist any document.
func denyAllAuthorizer(kind string) (bool, string) {
	return false, strings.ToLower(kind) + ":write"
}

// TestBundleAuthz_AllowedKindsPersistDeniedMarkedInvalid exercises the
// mixed case: a bundle contains several Kinds, and the caller is
// authorized for some but not others. Each authorized document plans as
// create; each denied document is marked invalid with a validation error
// quoting the missing permission. No authorized document is executed,
// because any invalid entry in the plan blocks execution (existing
// ApplyPlan semantics — see buildApplyPlan).
func TestBundleAuthz_AllowedKindsPersistDeniedMarkedInvalid(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities:     repos.Capabilities,
		Processes:        repos.Processes,
		Surfaces:         repos.Surfaces,
		Agents:           repos.Agents,
		Profiles:         repos.Profiles,
		Grants:           repos.Grants,
		BusinessServices: repos.BusinessServices,
	})

	// Caller authorized for Capability+Process+Surface but NOT Agent.
	ctx := apply.WithKindAuthorizer(
		context.Background(),
		allowKindsAuthorizer(types.KindCapability, types.KindProcess, types.KindSurface, types.KindProfile, types.KindGrant),
	)

	plan := svc.Plan(ctx, sixKindBundle())

	foundInvalidAgent := false
	for _, e := range plan.Entries {
		if e.Kind == types.KindAgent {
			if e.Action != apply.ApplyActionInvalid {
				t.Errorf("Agent document must be invalid under denied authorizer, got %s", e.Action)
			}
			foundInvalidAgent = true
			// Message must name the missing permission string.
			if !anyErrContains(e.ValidationErrors, "agent:write") {
				t.Errorf("invalid entry for Agent must name agent:write; got %+v", e.ValidationErrors)
			}
		}
	}
	if !foundInvalidAgent {
		t.Fatal("expected an Agent entry in the plan")
	}

	// Execute the plan — nothing should persist because one invalid entry
	// blocks the whole apply (existing ApplyPlan semantics).
	result := svc.Apply(ctx, sixKindBundle(), "test-actor")
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created (one invalid doc blocks bundle), got %d", result.CreatedCount())
	}

	// Confirm nothing was actually persisted.
	if c, _ := repos.Capabilities.GetByID(context.Background(), "cap-struct-test"); c != nil {
		t.Error("capability must not persist when bundle contains an unauthorized doc")
	}
	if a, _ := repos.Agents.GetByID(context.Background(), "agent-struct-test"); a != nil {
		t.Error("denied agent must not persist")
	}
}

// TestBundleAuthz_DenyAll_EveryEntryInvalid proves that a caller with
// controlplane:apply but no per-Kind permissions cannot write anything.
// Every document in the bundle is marked invalid; total creates = 0.
func TestBundleAuthz_DenyAll_EveryEntryInvalid(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
		Processes:    repos.Processes,
		Surfaces:     repos.Surfaces,
		Agents:       repos.Agents,
		Profiles:     repos.Profiles,
		Grants:       repos.Grants,
	})

	ctx := apply.WithKindAuthorizer(context.Background(), apply.KindAuthorizer(denyAllAuthorizer))
	plan := svc.Plan(ctx, sixKindBundle())

	if len(plan.Entries) != 6 {
		t.Fatalf("expected 6 entries, got %d", len(plan.Entries))
	}
	for _, e := range plan.Entries {
		if e.Action != apply.ApplyActionInvalid {
			t.Errorf("%s entry must be invalid under deny-all authorizer; got %s", e.Kind, e.Action)
		}
	}

	result := svc.Apply(ctx, sixKindBundle(), "test-actor")
	if result.CreatedCount() != 0 {
		t.Errorf("deny-all must create nothing; got %d", result.CreatedCount())
	}
}

// TestBundleAuthz_AllowAll_BehavesAsBefore proves that when every Kind is
// allowed, the planner's behaviour is indistinguishable from a bundle
// with no authorizer set. This is the equivalent of AuthModeOpen or a
// platform.admin caller.
func TestBundleAuthz_AllowAll_BehavesAsBefore(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
		Processes:    repos.Processes,
		Surfaces:     repos.Surfaces,
		Agents:       repos.Agents,
		Profiles:     repos.Profiles,
		Grants:       repos.Grants,
	})

	ctx := apply.WithKindAuthorizer(
		context.Background(),
		apply.KindAuthorizer(func(string) (bool, string) { return true, "" }),
	)
	result := svc.Apply(ctx, sixKindBundle(), "test-actor")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 6 {
		t.Fatalf("expected 6 created, got %d", result.CreatedCount())
	}
}

// TestBundleAuthz_NoAuthorizer_Unenforced proves that the planner does
// not default-deny when no KindAuthorizer is present in context. This
// preserves the contract for direct Service callers (tests, library use)
// and matches the open-mode HTTP path which skips principal extraction.
func TestBundleAuthz_NoAuthorizer_Unenforced(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
		Processes:    repos.Processes,
		Surfaces:     repos.Surfaces,
		Agents:       repos.Agents,
		Profiles:     repos.Profiles,
		Grants:       repos.Grants,
	})

	// No WithKindAuthorizer call on the context.
	result := svc.Apply(context.Background(), sixKindBundle(), "test-actor")
	if result.CreatedCount() != 6 {
		t.Fatalf("expected 6 created when no authorizer is set, got %d", result.CreatedCount())
	}
}

// TestBundleAuthz_DeniedMessagesName_EachKind_SeparatePermission asserts
// that every denied entry carries its own per-Kind permission string —
// the caller must be able to read the 403-like message and know exactly
// which permission to grant.
func TestBundleAuthz_DeniedMessagesName_EachKind_SeparatePermission(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
		Processes:    repos.Processes,
		Surfaces:     repos.Surfaces,
		Agents:       repos.Agents,
		Profiles:     repos.Profiles,
		Grants:       repos.Grants,
	})

	ctx := apply.WithKindAuthorizer(context.Background(), apply.KindAuthorizer(denyAllAuthorizer))
	plan := svc.Plan(ctx, sixKindBundle())

	wantByKind := map[string]string{
		types.KindCapability: "capability:write",
		types.KindProcess:    "process:write",
		types.KindSurface:    "surface:write",
		types.KindAgent:      "agent:write",
		types.KindProfile:    "profile:write",
		types.KindGrant:      "grant:write",
	}
	for _, e := range plan.Entries {
		want := wantByKind[e.Kind]
		if want == "" {
			continue
		}
		if !anyErrContains(e.ValidationErrors, want) {
			t.Errorf("%s entry denial message must name %q; got %+v", e.Kind, want, e.ValidationErrors)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func anyErrContains(errs []types.ValidationError, sub string) bool {
	for _, e := range errs {
		if strings.Contains(e.Message, sub) {
			return true
		}
	}
	return false
}
