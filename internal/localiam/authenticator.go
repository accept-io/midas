package localiam

import (
	"fmt"
	"net/http"

	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/identity"
)

// SessionAuthenticator implements auth.Authenticator by resolving the session
// cookie to an *identity.Principal. This is the bridge that allows the
// platformauth middleware to use localiam sessions with the existing
// auth.Authenticator abstraction.
type SessionAuthenticator struct {
	service *Service
}

// NewSessionAuthenticator wraps a Service as an auth.Authenticator.
func NewSessionAuthenticator(service *Service) *SessionAuthenticator {
	return &SessionAuthenticator{service: service}
}

// Authenticate reads the session cookie from the request and resolves it to a
// Principal. Returns auth.ErrNoCredentials when no cookie is present.
func (a *SessionAuthenticator) Authenticate(r *http.Request) (*identity.Principal, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, auth.ErrNoCredentials
	}

	_, principal, _, err := a.service.ResolveSession(r.Context(), cookie.Value)
	if err != nil {
		return nil, fmt.Errorf("localiam: session resolution failed: %w", err)
	}

	return principal, nil
}

// Compile-time assertion that *SessionAuthenticator implements auth.Authenticator.
var _ auth.Authenticator = (*SessionAuthenticator)(nil)
