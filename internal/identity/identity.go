package identity

import "strings"

// Principal represents a caller identity in standalone MIDAS mode.
// In enterprise mode, this can later be populated from JWT/OIDC claims.
type Principal struct {
	ID    string   // e.g. "user:alice"
	Name  string   // human-readable display name
	Roles []string // e.g. ["approver"], ["admin"]
}

const (
	RoleAdmin    = "admin"
	RoleApprover = "approver"
	RoleOperator = "operator"
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
