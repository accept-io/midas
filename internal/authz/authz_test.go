package authz_test

import (
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/authz"
	"github.com/accept-io/midas/internal/identity"
)

// ---------------------------------------------------------------------------
// T1 — Permission inventory and naming discipline
// ---------------------------------------------------------------------------

// TestAllControlPlaneWritePermissions_Count guards the v1 17-permission
// invariant. The v1 service-led model removed PermProcessCapabilityWrite
// and PermProcessBusinessServiceWrite alongside the obsolete
// ProcessCapability and ProcessBusinessService Kinds (ADR-XXX),
// reintroduced a single PermBusinessServiceCapabilityWrite to gate the
// new BusinessServiceCapability junction Kind, and removed
// PermControlplanePromote and PermControlplaneCleanup with the inference
// subsystem. Adding or removing a permission requires updating this count
// deliberately, which forces a review of the corresponding platform.admin
// bundle.
func TestAllControlPlaneWritePermissions_Count(t *testing.T) {
	got := authz.AllControlPlaneWritePermissions()
	if len(got) != 17 {
		t.Errorf("want 17 control-plane write permissions, got %d", len(got))
	}
}

// TestAllControlPlaneWritePermissions_Unique asserts no permission is
// declared twice. Duplicates would mask a bundle-composition bug.
func TestAllControlPlaneWritePermissions_Unique(t *testing.T) {
	seen := map[authz.Permission]struct{}{}
	for _, p := range authz.AllControlPlaneWritePermissions() {
		if _, dup := seen[p]; dup {
			t.Errorf("duplicate permission in catalogue: %q", p)
		}
		seen[p] = struct{}{}
	}
}

// TestPermissionNaming ensures every permission follows the canonical
// "resource:action" shape. This keeps the wire identifier stable and
// prevents accidental introduction of alternate schemes.
func TestPermissionNaming(t *testing.T) {
	for _, p := range authz.AllControlPlaneWritePermissions() {
		s := string(p)
		if strings.Count(s, ":") != 1 {
			t.Errorf("permission %q does not match resource:action shape", s)
			continue
		}
		parts := strings.Split(s, ":")
		if parts[0] == "" || parts[1] == "" {
			t.Errorf("permission %q has empty resource or action", s)
		}
		if s != strings.ToLower(s) {
			t.Errorf("permission %q must be lower-case", s)
		}
	}
}

// ---------------------------------------------------------------------------
// T2 — Role bundle composition
// ---------------------------------------------------------------------------

// TestPlatformAdmin_BundleIsComplete asserts that platform.admin contains
// every control-plane write permission. This is the load-bearing invariant
// for the seeded bootstrap admin user; omitting any cell would silently
// regress the admin apply path for bundles containing that Kind.
func TestPlatformAdmin_BundleIsComplete(t *testing.T) {
	got := authz.PermissionsForRole(identity.RolePlatformAdmin)
	all := authz.AllControlPlaneWritePermissions()
	if len(got) != len(all) {
		t.Fatalf("platform.admin bundle size: want %d, got %d", len(all), len(got))
	}
	gotSet := toSet(got)
	for _, p := range all {
		if _, ok := gotSet[p]; !ok {
			t.Errorf("platform.admin missing permission %q", p)
		}
	}
}

// TestPlatformAdmin_IsNotWildcard asserts platform.admin is literal, not a
// wildcard. The bundle must resolve through the same lookup every other
// role uses; there must be no special-case bypass.
func TestPlatformAdmin_IsNotWildcard(t *testing.T) {
	p := &identity.Principal{Roles: []string{identity.RolePlatformAdmin}}

	// Sanity: admin allows a known real permission.
	if !authz.HasPermission(p, authz.PermControlplaneApply) {
		t.Fatal("platform.admin must hold controlplane:apply")
	}

	// Fabricated permission must be denied.
	if authz.HasPermission(p, authz.Permission("fabricated:permission")) {
		t.Error("platform.admin allowed unknown permission — wildcard bypass detected")
	}
}

// TestGovernanceApprover_ExactScope fixes the approver bundle to exactly
// the two approve permissions. Widening it (e.g. adding deprecate) would
// regress the maker–checker split the prior role model enforced.
func TestGovernanceApprover_ExactScope(t *testing.T) {
	got := authz.PermissionsForRole(identity.RoleGovernanceApprover)
	want := []authz.Permission{authz.PermSurfaceApprove, authz.PermProfileApprove}
	if len(got) != len(want) {
		t.Fatalf("governance.approver bundle size: want %d, got %d (%v)", len(want), len(got), got)
	}
	gotSet := toSet(got)
	for _, p := range want {
		if _, ok := gotSet[p]; !ok {
			t.Errorf("governance.approver missing %q", p)
		}
	}
	// Explicit denials — one representative permission per category.
	p := &identity.Principal{Roles: []string{identity.RoleGovernanceApprover}}
	for _, denied := range []authz.Permission{
		authz.PermSurfaceDeprecate,
		authz.PermProfileDeprecate,
		authz.PermControlplaneApply,
		authz.PermControlplanePlan,
		authz.PermGrantSuspend,
		authz.PermGrantRevoke,
		authz.PermGrantReinstate,
		authz.PermSurfaceWrite,
		authz.PermProfileWrite,
	} {
		if authz.HasPermission(p, denied) {
			t.Errorf("governance.approver must not hold %q", denied)
		}
	}
}

// TestEmptyBundleRoles asserts that operator, viewer, and reviewer hold
// zero control-plane write permissions. This preserves the demo/demo user's
// behaviour (operator) and keeps the read-only and review-only roles out of
// the new model entirely.
func TestEmptyBundleRoles(t *testing.T) {
	for _, role := range []string{
		identity.RolePlatformOperator,
		identity.RolePlatformViewer,
		identity.RoleGovernanceReviewer,
	} {
		if got := authz.PermissionsForRole(role); len(got) != 0 {
			t.Errorf("role %q must have empty write bundle, got %v", role, got)
		}
	}
}

// TestUnknownRoleYieldsEmptyBundle asserts that unknown role strings silently
// contribute the empty set, never a default-allow.
func TestUnknownRoleYieldsEmptyBundle(t *testing.T) {
	if got := authz.PermissionsForRole("some.unknown.role"); got != nil && len(got) != 0 {
		t.Errorf("unknown role must return empty bundle, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// T3 — HasPermission decision matrix
// ---------------------------------------------------------------------------

// TestHasPermission_NilPrincipal asserts a nil principal always denies.
func TestHasPermission_NilPrincipal(t *testing.T) {
	if authz.HasPermission(nil, authz.PermControlplaneApply) {
		t.Error("nil principal must never hold any permission")
	}
}

// TestHasPermission_NoRoles asserts an empty-roles principal always denies.
func TestHasPermission_NoRoles(t *testing.T) {
	p := &identity.Principal{Roles: []string{}}
	if authz.HasPermission(p, authz.PermControlplaneApply) {
		t.Error("principal with no roles must be denied")
	}
}

// TestHasPermission_Matrix — for every declared permission and every
// canonical role, prove that the caller is allowed iff the permission is in
// the role's bundle. No permission may leak to a neighbouring cell.
func TestHasPermission_Matrix(t *testing.T) {
	roles := []string{
		identity.RolePlatformAdmin,
		identity.RolePlatformOperator,
		identity.RolePlatformViewer,
		identity.RoleGovernanceApprover,
		identity.RoleGovernanceReviewer,
	}
	for _, role := range roles {
		role := role
		expectedSet := toSet(authz.PermissionsForRole(role))
		p := &identity.Principal{Roles: []string{role}}
		for _, perm := range authz.AllControlPlaneWritePermissions() {
			perm := perm
			_, shouldAllow := expectedSet[perm]
			t.Run(role+"/"+string(perm), func(t *testing.T) {
				got := authz.HasPermission(p, perm)
				if got != shouldAllow {
					t.Errorf("HasPermission(role=%q, perm=%q) = %v, want %v", role, perm, got, shouldAllow)
				}
			})
		}
	}
}

// TestHasPermission_MultiRoleUnion asserts that a principal with two roles
// holds the union of both bundles — no more, no less.
func TestHasPermission_MultiRoleUnion(t *testing.T) {
	p := &identity.Principal{Roles: []string{
		identity.RolePlatformOperator,
		identity.RoleGovernanceApprover,
	}}
	// Inherits approver's two permissions.
	for _, allowed := range []authz.Permission{
		authz.PermSurfaceApprove,
		authz.PermProfileApprove,
	} {
		if !authz.HasPermission(p, allowed) {
			t.Errorf("union bundle must include %q", allowed)
		}
	}
	// Operator adds nothing; deprecate/apply/write remain denied.
	for _, denied := range []authz.Permission{
		authz.PermSurfaceDeprecate,
		authz.PermProfileDeprecate,
		authz.PermControlplaneApply,
		authz.PermSurfaceWrite,
		authz.PermGrantRevoke,
	} {
		if authz.HasPermission(p, denied) {
			t.Errorf("union bundle must not include %q (operator contributes nothing)", denied)
		}
	}
}

// TestHasPermission_AdminPlusApprover asserts the admin+approver composition
// documented in README.md continues to work: caller gets the full admin
// bundle since admin is a superset of approver.
func TestHasPermission_AdminPlusApprover(t *testing.T) {
	p := &identity.Principal{Roles: []string{
		identity.RolePlatformAdmin,
		identity.RoleGovernanceApprover,
	}}
	for _, perm := range authz.AllControlPlaneWritePermissions() {
		if !authz.HasPermission(p, perm) {
			t.Errorf("admin+approver must hold %q", perm)
		}
	}
}

// TestHasPermission_AliasNormalizationExpected documents the expected
// interaction with identity.NormalizeRoles. Principal-construction paths
// already call NormalizeRoles; this test proves that a principal whose
// Roles field contains a deprecated alias that has been normalised to its
// canonical form resolves to the correct bundle.
//
// A principal carrying the raw alias "admin" without normalisation will
// NOT resolve — that would be a caller bug and is covered at the
// principal-construction layer, not here.
func TestHasPermission_AliasNormalizationExpected(t *testing.T) {
	// Simulate what UserToPrincipal produces: NormalizeRoles was applied.
	roles := identity.NormalizeRoles([]string{"admin"})
	p := &identity.Principal{Roles: roles}

	if !authz.HasPermission(p, authz.PermControlplaneApply) {
		t.Error("principal with normalised admin alias must resolve to full admin bundle")
	}
}

// TestHasPermission_WhitespaceTolerant asserts that surrounding whitespace
// on role strings does not prevent lookup. This guards against subtle
// breakage from role-claim sources that do not trim.
func TestHasPermission_WhitespaceTolerant(t *testing.T) {
	p := &identity.Principal{Roles: []string{"  " + identity.RolePlatformAdmin + "  "}}
	if !authz.HasPermission(p, authz.PermControlplaneApply) {
		t.Error("whitespace around role string must not defeat lookup")
	}
}

// TestHasPermission_CaseInsensitiveRoleMatch asserts that role lookup is
// case-insensitive, matching identity.Principal.HasRole semantics. This
// preserves the behaviour of the prior requireRole gate for tokens that
// carry uppercased or mixed-case canonical role names (covered by the
// pre-existing TestRBAC_RoleNormalization_UppercaseRoleMatches).
func TestHasPermission_CaseInsensitiveRoleMatch(t *testing.T) {
	for _, role := range []string{"PLATFORM.ADMIN", "Platform.Admin", "platform.admin"} {
		p := &identity.Principal{Roles: []string{role}}
		if !authz.HasPermission(p, authz.PermControlplaneApply) {
			t.Errorf("role %q must resolve to platform.admin bundle (case-insensitive)", role)
		}
	}
}

// ---------------------------------------------------------------------------
// T4 — KindToWritePermission coverage
// ---------------------------------------------------------------------------

// TestKindToWritePermission_AllKnownKinds asserts that every apply-eligible
// Kind maps to a non-empty per-Kind write permission. The string literals
// here must match internal/controlplane/types.Kind* exactly; this test
// catches drift when a new Kind is added or a constant is renamed.
func TestKindToWritePermission_AllKnownKinds(t *testing.T) {
	want := map[string]authz.Permission{
		"Capability":                authz.PermCapabilityWrite,
		"Process":                   authz.PermProcessWrite,
		"BusinessService":           authz.PermBusinessServiceWrite,
		"BusinessServiceCapability": authz.PermBusinessServiceCapabilityWrite,
		"Surface":                   authz.PermSurfaceWrite,
		"Profile":                   authz.PermProfileWrite,
		"Agent":                     authz.PermAgentWrite,
		"Grant":                     authz.PermGrantWrite,
	}
	for kind, wantPerm := range want {
		got := authz.KindToWritePermission(kind)
		if got != wantPerm {
			t.Errorf("KindToWritePermission(%q) = %q, want %q", kind, got, wantPerm)
		}
	}
}

// TestKindToWritePermission_UnknownReturnsEmpty asserts unknown Kinds
// return empty, which callers treat as deny-by-default.
func TestKindToWritePermission_UnknownReturnsEmpty(t *testing.T) {
	for _, kind := range []string{"", "Unknown", "Surface ", "surface", "EXAMPLE"} {
		if got := authz.KindToWritePermission(kind); got != "" {
			t.Errorf("KindToWritePermission(%q) = %q, want empty", kind, got)
		}
	}
}

// ---------------------------------------------------------------------------
// T5 — Bundle mutation isolation
// ---------------------------------------------------------------------------

// TestPermissionsForRole_ReturnsDefensiveCopy asserts that mutating the
// slice returned by PermissionsForRole does not affect subsequent calls.
// This protects the global bundle from accidental caller mutation.
func TestPermissionsForRole_ReturnsDefensiveCopy(t *testing.T) {
	got1 := authz.PermissionsForRole(identity.RolePlatformAdmin)
	if len(got1) == 0 {
		t.Fatal("unexpected empty bundle")
	}
	// Mutate the returned slice.
	for i := range got1 {
		got1[i] = authz.Permission("mutated")
	}
	got2 := authz.PermissionsForRole(identity.RolePlatformAdmin)
	for _, p := range got2 {
		if p == "mutated" {
			t.Error("mutation of returned slice leaked into global bundle")
		}
	}
}

// TestAllControlPlaneWritePermissions_ReturnsDefensiveCopy — same contract
// for the full-matrix accessor.
func TestAllControlPlaneWritePermissions_ReturnsDefensiveCopy(t *testing.T) {
	got1 := authz.AllControlPlaneWritePermissions()
	for i := range got1 {
		got1[i] = authz.Permission("mutated")
	}
	got2 := authz.AllControlPlaneWritePermissions()
	for _, p := range got2 {
		if p == "mutated" {
			t.Error("mutation of returned slice leaked into global catalogue")
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func toSet(perms []authz.Permission) map[authz.Permission]struct{} {
	out := make(map[authz.Permission]struct{}, len(perms))
	for _, p := range perms {
		out[p] = struct{}{}
	}
	return out
}
