package auth

import (
	"errors"
	"net/http"

	"github.com/accept-io/midas/internal/identity"
)

// chainAuthenticator runs a fixed ordered list of Authenticators against each
// request. It is the concrete type returned by Chain — kept unexported so the
// only construction path is via the Chain helper, which also enforces the
// nil-skip and empty-list semantics documented below.
type chainAuthenticator struct {
	members []Authenticator
}

// Compile-time assertion: chainAuthenticator satisfies the Authenticator
// interface. The interface itself is intentionally unchanged (single
// Authenticate method) — the chain is a thin composition over it.
var _ Authenticator = (*chainAuthenticator)(nil)

// Chain composes multiple Authenticators into a single Authenticator that
// tries each in order. The semantics are:
//
//  1. Authenticators are tried in the order supplied.
//  2. The first authenticator that returns a non-nil principal and nil error
//     wins; its principal is returned and no further authenticators run.
//  3. An authenticator returning ErrNoCredentials means "I found no
//     credentials of my scheme on this request" — the chain moves on to the
//     next authenticator.
//  4. Any other non-nil error is returned immediately. A present-but-invalid
//     credential of one scheme MUST NOT silently fall through to a different
//     scheme: that would be a confused-deputy risk (caller explicitly
//     presented a bearer token; chain must not silently substitute a session
//     cookie). This is a security invariant, not a performance optimisation.
//  5. ErrNoCredentials is returned only when every member returned
//     ErrNoCredentials (or every member was nil — see below).
//  6. An empty chain returns ErrNoCredentials immediately.
//  7. Nil entries in the list are skipped (not panicked on). A chain
//     consisting only of nil entries behaves like an empty chain.
//
// Order matters. Place the cheapest, most-specific authenticator first
// (typically static bearer token), and place fallback schemes (e.g. Local
// IAM session cookie) after it. For the security invariant this is also the
// right order: a present bearer token is evaluated as a bearer token and
// either accepted or rejected — never silently bypassed by a session cookie.
//
// The interface contract is preserved: chainAuthenticator implements
// Authenticator with the exact same single-method shape.
func Chain(authenticators ...Authenticator) Authenticator {
	// Filter out nil entries up front so the per-request hot path is a
	// straight loop with no inner nil check. This also collapses the
	// "all nils" case to "empty chain" for free.
	filtered := make([]Authenticator, 0, len(authenticators))
	for _, a := range authenticators {
		if a == nil {
			continue
		}
		filtered = append(filtered, a)
	}
	return &chainAuthenticator{members: filtered}
}

// Authenticate runs each member authenticator in order, applying the chain
// rules documented on Chain.
func (c *chainAuthenticator) Authenticate(r *http.Request) (*identity.Principal, error) {
	if len(c.members) == 0 {
		return nil, ErrNoCredentials
	}
	for _, a := range c.members {
		p, err := a.Authenticate(r)
		if err == nil {
			return p, nil
		}
		if errors.Is(err, ErrNoCredentials) {
			// This authenticator found no credentials of its scheme on
			// the request; move on to the next member.
			continue
		}
		// Present-but-invalid (or any other non-ErrNoCredentials
		// failure): stop immediately. Do NOT fall through to other
		// schemes — see Chain doc point 4.
		return nil, err
	}
	// Every member returned ErrNoCredentials.
	return nil, ErrNoCredentials
}
