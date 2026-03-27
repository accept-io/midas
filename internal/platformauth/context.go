package platformauth

import (
	"context"

	"github.com/accept-io/midas/internal/identity"
)

// contextKey is an unexported type for context keys owned by this package.
// Using a package-local type prevents collisions with keys from other packages.
type contextKey int

const principalKey contextKey = iota

// WithPrincipal returns a copy of ctx with p stored under the package-local key.
func WithPrincipal(ctx context.Context, p *identity.Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFromContext retrieves the *identity.Principal stored by WithPrincipal.
// The second return value is false when no principal is present or the stored
// value is nil.
func PrincipalFromContext(ctx context.Context) (*identity.Principal, bool) {
	p, ok := ctx.Value(principalKey).(*identity.Principal)
	return p, ok && p != nil
}

// MustPrincipalFromContext is like PrincipalFromContext but panics when no
// principal is present. Use only inside middleware chains that guarantee a
// principal has already been stored (e.g. after RequireAuthenticated).
func MustPrincipalFromContext(ctx context.Context) *identity.Principal {
	p, ok := PrincipalFromContext(ctx)
	if !ok {
		panic("platformauth: no principal in context; ensure Authenticate and RequireAuthenticated middleware are applied")
	}
	return p
}
