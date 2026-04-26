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
	"github.com/accept-io/midas/internal/controlplane/parser"
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
		Capabilities:                repos.Capabilities,
		Processes:                   repos.Processes,
		Surfaces:                    repos.Surfaces,
		Agents:                      repos.Agents,
		Profiles:                    repos.Profiles,
		Grants:                      repos.Grants,
		BusinessServices:            repos.BusinessServices,
		BusinessServiceCapabilities: repos.BusinessServiceCapabilities,
	})

	// Caller authorized for Capability+Process+Surface but NOT Agent.
	ctx := apply.WithKindAuthorizer(
		context.Background(),
		allowKindsAuthorizer(types.KindCapability, types.KindBusinessServiceCapability, types.KindProcess, types.KindSurface, types.KindProfile, types.KindGrant),
	)

	plan := svc.Plan(ctx, eightKindBundle())

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
	result := svc.Apply(ctx, eightKindBundle(), "test-actor")
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
		Capabilities:                repos.Capabilities,
		Processes:                   repos.Processes,
		Surfaces:                    repos.Surfaces,
		Agents:                      repos.Agents,
		Profiles:                    repos.Profiles,
		Grants:                      repos.Grants,
		BusinessServices:            repos.BusinessServices,
		BusinessServiceCapabilities: repos.BusinessServiceCapabilities,
	})

	ctx := apply.WithKindAuthorizer(context.Background(), apply.KindAuthorizer(denyAllAuthorizer))
	plan := svc.Plan(ctx, eightKindBundle())

	if len(plan.Entries) != 8 {
		t.Fatalf("expected 8 entries, got %d", len(plan.Entries))
	}
	for _, e := range plan.Entries {
		if e.Action != apply.ApplyActionInvalid {
			t.Errorf("%s entry must be invalid under deny-all authorizer; got %s", e.Kind, e.Action)
		}
	}

	result := svc.Apply(ctx, eightKindBundle(), "test-actor")
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
		Capabilities:                repos.Capabilities,
		Processes:                   repos.Processes,
		Surfaces:                    repos.Surfaces,
		Agents:                      repos.Agents,
		Profiles:                    repos.Profiles,
		Grants:                      repos.Grants,
		BusinessServices:            repos.BusinessServices,
		BusinessServiceCapabilities: repos.BusinessServiceCapabilities,
	})

	ctx := apply.WithKindAuthorizer(
		context.Background(),
		apply.KindAuthorizer(func(string) (bool, string) { return true, "" }),
	)
	result := svc.Apply(ctx, eightKindBundle(), "test-actor")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 8 {
		t.Fatalf("expected 8 created, got %d", result.CreatedCount())
	}
}

// TestBundleAuthz_NoAuthorizer_Unenforced proves that the planner does
// not default-deny when no KindAuthorizer is present in context. This
// preserves the contract for direct Service callers (tests, library use)
// and matches the open-mode HTTP path which skips principal extraction.
func TestBundleAuthz_NoAuthorizer_Unenforced(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities:                repos.Capabilities,
		Processes:                   repos.Processes,
		Surfaces:                    repos.Surfaces,
		Agents:                      repos.Agents,
		Profiles:                    repos.Profiles,
		Grants:                      repos.Grants,
		BusinessServices:            repos.BusinessServices,
		BusinessServiceCapabilities: repos.BusinessServiceCapabilities,
	})

	// No WithKindAuthorizer call on the context.
	result := svc.Apply(context.Background(), eightKindBundle(), "test-actor")
	if result.CreatedCount() != 8 {
		t.Fatalf("expected 8 created when no authorizer is set, got %d", result.CreatedCount())
	}
}

// TestBundleAuthz_DeniedMessagesName_EachKind_SeparatePermission asserts
// that every denied entry carries its own per-Kind permission string —
// the caller must be able to read the 403-like message and know exactly
// which permission to grant.
func TestBundleAuthz_DeniedMessagesName_EachKind_SeparatePermission(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities:                repos.Capabilities,
		Processes:                   repos.Processes,
		Surfaces:                    repos.Surfaces,
		Agents:                      repos.Agents,
		Profiles:                    repos.Profiles,
		Grants:                      repos.Grants,
		BusinessServices:            repos.BusinessServices,
		BusinessServiceCapabilities: repos.BusinessServiceCapabilities,
	})

	ctx := apply.WithKindAuthorizer(context.Background(), apply.KindAuthorizer(denyAllAuthorizer))
	plan := svc.Plan(ctx, eightKindBundle())

	wantByKind := map[string]string{
		types.KindBusinessService:           "businessservice:write",
		types.KindCapability:                "capability:write",
		types.KindBusinessServiceCapability: "businessservicecapability:write",
		types.KindProcess:                   "process:write",
		types.KindSurface:                   "surface:write",
		types.KindAgent:                     "agent:write",
		types.KindProfile:                   "profile:write",
		types.KindGrant:                     "grant:write",
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

// eightKindBundle builds a structurally-valid bundle covering the eight
// apply-eligible kinds in the v1 service-led model: BusinessService,
// Capability, BusinessServiceCapability, Process, Surface, Agent, Profile,
// Grant. Used by per-Kind authorization tests in this file.
func eightKindBundle() []parser.ParsedDocument {
	return []parser.ParsedDocument{
		{
			Kind: types.KindBusinessService,
			ID:   "bs-struct-test",
			Doc: types.BusinessServiceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindBusinessService,
				Metadata:   types.DocumentMetadata{ID: "bs-struct-test", Name: "Struct Test BS"},
				Spec:       types.BusinessServiceSpec{ServiceType: "internal", Status: "active"},
			},
		},
		{
			Kind: types.KindCapability,
			ID:   "cap-struct-test",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-struct-test", Name: "Struct Test Cap"},
				Spec:       types.CapabilitySpec{Status: "active"},
			},
		},
		{
			Kind: types.KindBusinessServiceCapability,
			ID:   "bsc-struct-test",
			Doc: types.BusinessServiceCapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindBusinessServiceCapability,
				Metadata:   types.DocumentMetadata{ID: "bsc-struct-test"},
				Spec: types.BusinessServiceCapabilitySpec{
					BusinessServiceID: "bs-struct-test",
					CapabilityID:      "cap-struct-test",
				},
			},
		},
		{
			Kind: types.KindProcess,
			ID:   "proc-struct-test",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-struct-test", Name: "Struct Test Process"},
				Spec:       types.ProcessSpec{BusinessServiceID: "bs-struct-test", Status: "active"},
			},
		},
		{
			Kind: types.KindSurface,
			ID:   "surf-struct-test",
			Doc: types.SurfaceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindSurface,
				Metadata:   types.DocumentMetadata{ID: "surf-struct-test", Name: "Struct Test Surface"},
				Spec: types.SurfaceSpec{
					Category:  "test",
					RiskTier:  "low",
					Status:    "active",
					ProcessID: "proc-struct-test",
				},
			},
		},
		{
			Kind: types.KindAgent,
			ID:   "agent-struct-test",
			Doc: types.AgentDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindAgent,
				Metadata:   types.DocumentMetadata{ID: "agent-struct-test", Name: "Struct Test Agent"},
				Spec:       types.AgentSpec{Type: "automation", Status: "active"},
			},
		},
		{
			Kind: types.KindProfile,
			ID:   "prof-struct-test",
			Doc: types.ProfileDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProfile,
				Metadata:   types.DocumentMetadata{ID: "prof-struct-test", Name: "Struct Test Profile"},
				Spec: types.ProfileSpec{
					SurfaceID: "surf-struct-test",
					Authority: types.ProfileAuthority{
						DecisionConfidenceThreshold: 0.5,
						ConsequenceThreshold:        types.ConsequenceThreshold{Type: "monetary", Amount: 100, Currency: "USD"},
					},
					Policy:    types.ProfilePolicy{Reference: "rego://test/struct_v1", FailMode: "open"},
					Lifecycle: types.ProfileLifecycle{Status: "active"},
				},
			},
		},
		{
			Kind: types.KindGrant,
			ID:   "grant-struct-test",
			Doc: types.GrantDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindGrant,
				Metadata:   types.DocumentMetadata{ID: "grant-struct-test", Name: "Struct Test Grant"},
				Spec: types.GrantSpec{
					AgentID:       "agent-struct-test",
					ProfileID:     "prof-struct-test",
					Status:        "active",
					GrantedBy:     "team-struct",
					GrantedAt:     "2026-01-01T00:00:00Z",
					EffectiveFrom: "2026-01-01T00:00:00Z",
				},
			},
		},
	}
}
