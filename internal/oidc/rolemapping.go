package oidc

import (
	"sort"

	"github.com/accept-io/midas/internal/identity"
)

// MapExternalRoles maps external group identifiers to MIDAS canonical roles
// using explicit mappings only. Unknown groups are silently ignored.
// Output is deduplicated, deterministically sorted, and passed through
// identity.NormalizeRoles to ensure canonical form.
func MapExternalRoles(external []string, mappings []RoleMapping) []string {
	// Matching is exact (case-sensitive). Providers return group names or domain
	// values verbatim; role_mappings in config must use the exact same casing.
	// This applies equally to Entra group names and Google hosted-domain (hd) values.
	lookup := make(map[string]string, len(mappings))
	for _, m := range mappings {
		if m.External != "" && m.Internal != "" {
			lookup[m.External] = m.Internal
		}
	}

	seen := make(map[string]struct{}, len(external))
	out := make([]string, 0, len(external))
	for _, g := range external {
		role, ok := lookup[g]
		if !ok {
			continue
		}
		if _, dup := seen[role]; dup {
			continue
		}
		seen[role] = struct{}{}
		out = append(out, role)
	}

	sort.Strings(out)
	return identity.NormalizeRoles(out)
}
