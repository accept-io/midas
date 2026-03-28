package identity

import (
	"sort"
	"strings"
)

// Principal represents a verified caller identity.
// The Provider field indicates which authentication mechanism populated this
// struct, enabling handlers to apply provider-specific logic without type switches.
type Principal struct {
	ID       string         // e.g. "user:alice" or an OIDC sub claim
	Name     string         // human-readable display name
	Roles    []string       // e.g. ["approver"], ["admin"]
	Provider string         // identifies the auth provider; use Provider* constants
	Subject  string         // raw subject claim from the provider (e.g. OIDC sub)
	Claims   map[string]any // arbitrary provider-specific claims (e.g. JWT payload)
}

// Provider constants identify which authentication mechanism issued a Principal.
// Handlers and audit code can branch on these without importing provider packages.
const (
	ProviderStatic = "static" // StaticTokenAuthenticator — token-to-principal map
	ProviderEntra  = "entra"  // Microsoft Entra ID (Azure AD) — future
	ProviderPing   = "ping"   // Ping Identity — future
)

// Platform domain roles govern access to MIDAS operations.
const (
	RolePlatformAdmin    = "platform.admin"
	RolePlatformOperator = "platform.operator"
	RolePlatformViewer   = "platform.viewer"
)

// Governance domain roles govern workflow participation (approval, review).
const (
	RoleGovernanceApprover = "governance.approver"
	RoleGovernanceReviewer = "governance.reviewer"
)

// Deprecated: use RolePlatformAdmin instead.
const RoleAdmin = "admin"

// Deprecated: use RolePlatformOperator instead.
const RoleOperator = "operator"

// Deprecated: use RoleGovernanceApprover instead.
const RoleApprover = "approver"

// Deprecated: use RoleGovernanceReviewer instead.
const RoleReviewer = "reviewer"

// legacyRoleMap maps legacy role strings (lowercased) to their canonical equivalents.
var legacyRoleMap = map[string]string{
	"admin":    RolePlatformAdmin,
	"operator": RolePlatformOperator,
	"approver": RoleGovernanceApprover,
	"reviewer": RoleGovernanceReviewer,
}

// NormalizeRoles maps legacy role strings to their canonical equivalents,
// deduplicates, and returns a deterministic sorted slice.
// Unknown or already-canonical roles are preserved as-is.
// Normalization is case-insensitive for legacy role lookup.
func NormalizeRoles(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, r := range in {
		canonical, ok := legacyRoleMap[strings.ToLower(r)]
		if ok {
			r = canonical
		}
		if _, dup := seen[r]; dup {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// HasRole returns true if the principal has the given role.
// Comparison is case-insensitive and trims surrounding whitespace.
func (p Principal) HasRole(role string) bool {
	for _, r := range p.Roles {
		if strings.EqualFold(strings.TrimSpace(r), role) {
			return true
		}
	}
	return false
}

// HasAnyRole returns true if the principal holds at least one of the given roles.
// Comparison is case-insensitive, matching the semantics of HasRole.
func (p *Principal) HasAnyRole(roles ...string) bool {
	for _, role := range roles {
		if p.HasRole(role) {
			return true
		}
	}
	return false
}
