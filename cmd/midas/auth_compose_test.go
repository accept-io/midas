package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/localiam"
	"github.com/accept-io/midas/internal/store/memory"
)

// newTestIAMService constructs a real *localiam.Service backed by the
// in-memory repos used in production-mode dev. It is the cheapest way to
// get a non-nil iamSvc that composeAuthenticator can wrap.
func newTestIAMService(t *testing.T) *localiam.Service {
	t.Helper()
	users := memory.NewLocalUserRepo()
	sessions := memory.NewLocalSessionRepo()
	return localiam.NewService(users, sessions, localiam.Config{
		Enabled:    true,
		SessionTTL: time.Hour,
	})
}

// fakeStaticAuth mimics auth.StaticTokenAuthenticator's contract closely
// enough for wiring assertions: it returns a principal only when the
// request carries an Authorization header; otherwise it returns
// ErrNoCredentials. This matches the real authenticator's behaviour and
// keeps the chain semantics under test honest (a bare static authenticator
// must surface ErrNoCredentials to advance the chain — it must not fabricate
// a principal for unauthenticated requests).
type fakeStaticAuth struct{ id string }

func (f *fakeStaticAuth) Authenticate(r *http.Request) (*identity.Principal, error) {
	if r.Header.Get("Authorization") == "" {
		return nil, auth.ErrNoCredentials
	}
	return &identity.Principal{ID: f.id, Provider: identity.ProviderStatic}, nil
}

// TestComposeAuthenticator_BothConfigured pins the chain composition rule:
// when BOTH static tokens AND Local IAM are configured, the wiring composes
// a Chain with the static authenticator first and the session authenticator
// second. We assert this by sending a Bearer token (only the static
// authenticator handles it) and observing that the static authenticator's
// principal is returned.
func TestComposeAuthenticator_BothConfigured(t *testing.T) {
	staticAuth := &fakeStaticAuth{id: "user:fromstatic"}
	iamSvc := newTestIAMService(t)

	composed := composeAuthenticator(staticAuth, iamSvc)
	if composed == nil {
		t.Fatal("want non-nil composed authenticator when both schemes are configured")
	}

	// Send a Bearer-only request. The static authenticator returns a
	// principal; the chain stops there and the session authenticator is
	// never consulted (and could not have produced this principal — its
	// principal would carry Provider=localiam, not static).
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer anything")
	p, err := composed.Authenticate(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil || p.ID != "user:fromstatic" {
		t.Errorf("want static-authenticator principal user:fromstatic, got %+v", p)
	}

	// Cookie-only request: static returns ErrNoCredentials (no Bearer
	// header), the chain advances to the session authenticator, which
	// finds no cookie and also returns ErrNoCredentials. The chain
	// surfaces ErrNoCredentials, proving the second member was reached.
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := composed.Authenticate(r2); !errors.Is(err, auth.ErrNoCredentials) {
		t.Errorf("want ErrNoCredentials when neither scheme is present, got %v", err)
	}
}

// TestComposeAuthenticator_StaticOnly pins the case where Local IAM is
// disabled (iamSvc nil): the static authenticator is returned unwrapped,
// not as a Chain of length 1. Equality is checked by interface identity.
func TestComposeAuthenticator_StaticOnly(t *testing.T) {
	staticAuth := &fakeStaticAuth{id: "user:fromstatic"}

	composed := composeAuthenticator(staticAuth, nil)
	if composed == nil {
		t.Fatal("want non-nil authenticator when static tokens are configured")
	}
	// Concrete identity check: same pointer, not a wrapper. Using == on
	// interface values works because both sides hold the same
	// (*fakeStaticAuth) concrete value.
	if composed != auth.Authenticator(staticAuth) {
		t.Errorf("want bare staticAuth (no Chain wrapper) when iamSvc is nil; got %T", composed)
	}
}

// TestComposeAuthenticator_SessionOnly pins the case where MIDAS_AUTH_TOKENS
// is unset (staticAuth nil) but Local IAM is enabled. The session
// authenticator is returned unwrapped. We assert by sending a request with
// no credentials and checking that the result is ErrNoCredentials produced
// by SessionAuthenticator's no-cookie branch (i.e. the wiring actually
// invokes the session authenticator).
func TestComposeAuthenticator_SessionOnly(t *testing.T) {
	iamSvc := newTestIAMService(t)

	composed := composeAuthenticator(nil, iamSvc)
	if composed == nil {
		t.Fatal("want non-nil authenticator when Local IAM is enabled")
	}
	// The bare session authenticator must be a *localiam.SessionAuthenticator,
	// NOT a Chain. We probe by type assertion.
	if _, ok := composed.(*localiam.SessionAuthenticator); !ok {
		t.Errorf("want bare *localiam.SessionAuthenticator (no Chain wrapper) when staticAuth is nil; got %T", composed)
	}

	// Functional probe: a request with neither header nor cookie must
	// return ErrNoCredentials (the session authenticator's no-cookie
	// branch).
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := composed.Authenticate(r); !errors.Is(err, auth.ErrNoCredentials) {
		t.Errorf("want ErrNoCredentials for cookie-less request, got %v", err)
	}
}

// TestComposeAuthenticator_NeitherConfigured pins the empty case: both
// arguments nil produces nil. main() relies on this to skip
// WithAuthenticator entirely so the server's existing fail-closed behaviour
// under AuthModeRequired is preserved unchanged.
func TestComposeAuthenticator_NeitherConfigured(t *testing.T) {
	got := composeAuthenticator(nil, nil)
	if got != nil {
		t.Errorf("want nil authenticator when neither scheme is configured, got %T", got)
	}
}
