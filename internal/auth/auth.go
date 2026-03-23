package auth

import (
	"errors"
	"net/http"

	"github.com/accept-io/midas/internal/identity"
)

// Authenticator verifies inbound HTTP requests and returns a verified Principal.
// Implementations may read a bearer token, a JWT, or any other credential scheme.
// Returning ErrNoCredentials signals "no credentials presented" (distinct from
// "credentials presented but invalid") — both map to 401 in the middleware, but
// the distinction is logged for observability.
type Authenticator interface {
	Authenticate(r *http.Request) (*identity.Principal, error)
}

// ErrNoCredentials is returned when the request carries no recognisable credentials
// (e.g. missing Authorization header). Use errors.Is to test for this sentinel.
var ErrNoCredentials = errors.New("auth: no credentials provided")
