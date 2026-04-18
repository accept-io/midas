// Package authz implements the scoped-permission authorization model for
// MIDAS control-plane write operations.
//
// The model has three elements:
//
//  1. Permission — an opaque scoped string of the form "resource:action"
//     (for example "surface:approve", "controlplane:apply", "grant:revoke").
//     Permissions are the canonical identifier for authorization decisions.
//
//  2. Role bundle — a named set of permissions. Each of the five canonical
//     MIDAS roles expands to a fixed bundle defined in this package.
//
//  3. Decision — HasPermission resolves a principal's normalised roles to the
//     union of their bundles and tests membership.
//
// The package has no I/O, no global state beyond these constant definitions,
// and no dependencies on internal/httpapi/ or internal/controlplane/. It
// deliberately does not know about HTTP requests, YAML documents, or the
// wire shape of apply bundles — those concerns live at the call sites.
//
// This package is the sole source of truth for the mapping from canonical
// role to permission set. Role names are expected in canonical form
// (post-identity.NormalizeRoles); unknown roles expand to the empty set.
package authz

import (
	"strings"

	"github.com/accept-io/midas/internal/identity"
)

// Permission is a scoped authorization string of the form "resource:action".
// Permissions are the canonical identifier for authorization decisions; they
// are not mapped to integer IDs or parallel enums.
type Permission string

// Control-plane endpoint-scoped permissions.
// These gate whether a caller may invoke a specific control-plane endpoint.
// The per-document write permissions below are checked in addition to
// PermControlplaneApply for bundle contents on POST /v1/controlplane/apply.
const (
	PermControlplaneApply   Permission = "controlplane:apply"
	PermControlplanePlan    Permission = "controlplane:plan"
	PermControlplanePromote Permission = "controlplane:promote"
	PermControlplaneCleanup Permission = "controlplane:cleanup"
)

// Per-Kind bundle-scoped write permissions. These are checked per document
// inside the apply planner; a caller holding PermControlplaneApply but
// missing the required *:write for a document Kind will have that document
// marked invalid in the returned plan (and therefore never persisted).
const (
	PermCapabilityWrite             Permission = "capability:write"
	PermProcessWrite                Permission = "process:write"
	PermBusinessServiceWrite        Permission = "businessservice:write"
	PermSurfaceWrite                Permission = "surface:write"
	PermProfileWrite                Permission = "profile:write"
	PermAgentWrite                  Permission = "agent:write"
	PermGrantWrite                  Permission = "grant:write"
	PermProcessCapabilityWrite      Permission = "processcapability:write"
	PermProcessBusinessServiceWrite Permission = "processbusinessservice:write"
)

// Lifecycle-action permissions. Each sub-action of surface/profile/grant
// lifecycle dispatchers maps 1:1 to one of these permissions, preserving the
// maker–checker separation (approve vs deprecate, suspend vs revoke vs
// reinstate) that the prior role-gated model already enforced.
const (
	PermSurfaceApprove   Permission = "surface:approve"
	PermSurfaceDeprecate Permission = "surface:deprecate"
	PermProfileApprove   Permission = "profile:approve"
	PermProfileDeprecate Permission = "profile:deprecate"
	PermGrantSuspend     Permission = "grant:suspend"
	PermGrantRevoke      Permission = "grant:revoke"
	PermGrantReinstate   Permission = "grant:reinstate"
)

// allControlPlaneWritePermissions is the canonical 20-permission set that
// platform.admin expands to. Declared as a slice so tests can iterate the
// full matrix; exposed as a copy via AllControlPlaneWritePermissions.
//
// Order is stable by category: endpoints, per-Kind writes, lifecycle actions.
// Adding a new permission requires updating this slice AND the platform.admin
// bundle in roleBundles; see authz_test.go for coverage that enforces parity.
var allControlPlaneWritePermissions = []Permission{
	PermControlplaneApply,
	PermControlplanePlan,
	PermControlplanePromote,
	PermControlplaneCleanup,

	PermCapabilityWrite,
	PermProcessWrite,
	PermBusinessServiceWrite,
	PermSurfaceWrite,
	PermProfileWrite,
	PermAgentWrite,
	PermGrantWrite,
	PermProcessCapabilityWrite,
	PermProcessBusinessServiceWrite,

	PermSurfaceApprove,
	PermSurfaceDeprecate,
	PermProfileApprove,
	PermProfileDeprecate,
	PermGrantSuspend,
	PermGrantRevoke,
	PermGrantReinstate,
}

// AllControlPlaneWritePermissions returns a defensive copy of the full
// control-plane write-permission set. Callers (typically tests) may iterate
// the result freely.
func AllControlPlaneWritePermissions() []Permission {
	out := make([]Permission, len(allControlPlaneWritePermissions))
	copy(out, allControlPlaneWritePermissions)
	return out
}

// roleBundles maps each canonical MIDAS role to its permission bundle.
// Role names must match identity.Role* constants exactly; lookup uses
// post-NormalizeRoles canonical strings. Roles not present in this map
// (including unknown strings) expand to the empty set.
//
// platform.admin is populated explicitly from allControlPlaneWritePermissions;
// it is not a wildcard and the enforcement path does not special-case it.
// Removing a permission from the platform.admin entry below removes that
// capability for every principal holding the admin role.
var roleBundles = map[string][]Permission{
	identity.RolePlatformAdmin: append([]Permission(nil), allControlPlaneWritePermissions...),

	// Maker–checker: approver can approve, cannot deprecate, cannot write.
	identity.RoleGovernanceApprover: {
		PermSurfaceApprove,
		PermProfileApprove,
	},

	// Operator, viewer, and reviewer hold no control-plane write permissions
	// under this model. Their existing role-based gates on read-path,
	// data-plane evaluate, /v1/reviews, and Explorer sandbox routes are
	// unchanged and intentionally out of scope for this package.
	identity.RolePlatformOperator:   {},
	identity.RolePlatformViewer:     {},
	identity.RoleGovernanceReviewer: {},
}

// PermissionsForRole returns a copy of the permission bundle assigned to
// role. Unknown roles return nil. role is expected to be in canonical form
// (post-identity.NormalizeRoles).
func PermissionsForRole(role string) []Permission {
	perms, ok := roleBundles[role]
	if !ok {
		return nil
	}
	out := make([]Permission, len(perms))
	copy(out, perms)
	return out
}

// HasPermission reports whether the principal holds the required permission
// through any of its roles. Resolution is the set union of all the
// principal's role bundles; unknown roles contribute the empty set.
//
// Returns false when the principal is nil or has no roles.
//
// Role matching intentionally mirrors identity.Principal.HasRole semantics:
// surrounding whitespace is trimmed and comparison is case-insensitive.
// This preserves the effective behaviour of the prior role-gated model for
// edge cases such as tokens carrying uppercased canonical role names and
// for the deprecated aliases still present in some fixtures. Principal-
// construction paths (UserToPrincipal, MapExternalRoles, buildAuthenticator)
// already apply identity.NormalizeRoles, so a non-normalised Principal here
// is an exceptional case, not the default path.
//
// HasPermission does not consult any external source and is safe for
// concurrent use.
func HasPermission(p *identity.Principal, required Permission) bool {
	if p == nil {
		return false
	}
	for _, role := range p.Roles {
		bundle := bundleForRole(role)
		for _, granted := range bundle {
			if granted == required {
				return true
			}
		}
	}
	return false
}

// bundleForRole returns the permission bundle for a role string, matching
// identity.HasRole semantics: whitespace is trimmed and comparison is
// case-insensitive. Returns nil for unknown roles.
func bundleForRole(role string) []Permission {
	role = strings.ToLower(strings.TrimSpace(role))
	for canonical, bundle := range roleBundles {
		if strings.ToLower(canonical) == role {
			return bundle
		}
	}
	return nil
}

// KindToWritePermission maps a control-plane document Kind string (as used
// in internal/controlplane/types.Kind* constants) to the per-Kind write
// permission required to create that document via a bundle apply.
//
// Returns the empty string for unknown Kinds. This mirrors the nine
// apply-eligible Kinds currently supported by the control plane.
//
// The string-based lookup is intentional: authz must not import the
// controlplane or httpapi packages (see package doc comment).
func KindToWritePermission(kind string) Permission {
	switch kind {
	case "Capability":
		return PermCapabilityWrite
	case "Process":
		return PermProcessWrite
	case "BusinessService":
		return PermBusinessServiceWrite
	case "Surface":
		return PermSurfaceWrite
	case "Profile":
		return PermProfileWrite
	case "Agent":
		return PermAgentWrite
	case "Grant":
		return PermGrantWrite
	case "ProcessCapability":
		return PermProcessCapabilityWrite
	case "ProcessBusinessService":
		return PermProcessBusinessServiceWrite
	default:
		return ""
	}
}
