package main

import (
	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/localiam"
)

// composeAuthenticator decides which Authenticator (or composition of
// Authenticators) the HTTP server should install given the runtime config.
//
// It is the single home of the wiring rule that Local IAM session cookies
// are accepted on /v1/* alongside static bearer tokens. Extracted out of
// main() so it can be unit-tested directly.
//
// Inputs:
//   - staticAuth: the StaticTokenAuthenticator built from MIDAS_AUTH_TOKENS;
//     nil when no tokens are configured.
//   - iamSvc:     the constructed Local IAM service when MIDAS_LOCAL_IAM_ENABLED
//     is true; nil when Local IAM is disabled.
//
// Returns:
//   - the static authenticator alone when only static tokens are configured.
//   - the Local IAM session authenticator alone when only Local IAM is enabled.
//   - an auth.Chain[static, session] when both are configured; ordering is
//     load-bearing — see auth.Chain doc for the security rationale (a present
//     bearer token must be evaluated as a bearer token, never silently
//     bypassed by a session cookie).
//   - nil when neither is configured. main() must surface this nil to
//     server.WithAuthenticator unchanged: under AuthModeRequired the server's
//     fail-closed branch (auth.go:484-487) handles it, and under
//     AuthModeOpen the authenticator is never consulted.
//
// Both arguments may be nil. The function never panics on nil.
func composeAuthenticator(staticAuth auth.Authenticator, iamSvc *localiam.Service) auth.Authenticator {
	var sessionAuth auth.Authenticator
	if iamSvc != nil {
		// Wrap the service as an Authenticator. NewSessionAuthenticator
		// returns a non-nil pointer; the helper itself never errors.
		sessionAuth = localiam.NewSessionAuthenticator(iamSvc)
	}

	switch {
	case staticAuth != nil && sessionAuth != nil:
		// Both schemes available. Static first so a present bearer
		// token is never silently bypassed by a session cookie.
		return auth.Chain(staticAuth, sessionAuth)
	case staticAuth != nil:
		// Static tokens only. Return the bare authenticator (not a
		// length-1 Chain) so the production path is identical to what
		// it was before this phase — no behaviour change for
		// deployments that don't enable Local IAM.
		return staticAuth
	case sessionAuth != nil:
		// Local IAM only. Same posture: return the bare authenticator
		// rather than a length-1 Chain.
		return sessionAuth
	default:
		// Neither scheme configured. Caller decides what to do with
		// the nil; the server's existing required-mode fail-closed
		// handling already covers it.
		return nil
	}
}
