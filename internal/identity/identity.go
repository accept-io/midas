package identity

import "strings"

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

const (
	RoleAdmin    = "admin"
	RoleApprover = "approver"
	RoleOperator = "operator"
	RoleReviewer = "reviewer"
)

// HasRole returns true if the principal has the given role.
func (p Principal) HasRole(role string) bool {
	for _, r := range p.Roles {
		if strings.EqualFold(strings.TrimSpace(r), role) {
			return true
		}
	}
	return false
}
