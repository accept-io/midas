package httpapi

// auth_chain_v1_test.go — required-mode authentication tests for /v1/*
// when the chain composition (Phase 7) is in effect.
//
// Each test wires the server with WithAuthMode(Required) and a chained
// authenticator that accepts BOTH a static bearer token AND a Local IAM
// session cookie. The chain is constructed inline as
// auth.Chain(staticAuth, localiam.NewSessionAuthenticator(iamSvc)) — the
// production path uses the same composition through composeAuthenticator
// (cmd/midas/auth_compose.go), which is unit-tested separately.
//
// /v1/capabilities is exercised because it has the lightest dependency
// surface (only the StructuralService with a capability reader) while
// still passing through requireAuth + requireRole, which is the gate the
// fix targets.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/config"
	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/localiam"
	"github.com/accept-io/midas/internal/store/memory"
)

// authChainTestSetup wires a server with required-mode + the chained
// authenticator + a structural service backed by an in-memory capability
// repo. Returns the server, the iam service (so tests can drive logins),
// and the bearer token configured for the static authenticator.
type authChainTestSetup struct {
	srv         *Server
	iamSvc      *localiam.Service
	sessions    *stubSessionRepo
	bearerToken string
	bearerID    string
}

func newAuthChainTestServer(t *testing.T) *authChainTestSetup {
	t.Helper()

	// Static-token authenticator with one valid token.
	const bearerToken = "tok-static-operator"
	const bearerID = "svc:test-static"
	staticAuth := auth.NewStaticTokenAuthenticator(map[string]*identity.Principal{
		bearerToken: {
			ID:       bearerID,
			Subject:  bearerID,
			Provider: identity.ProviderStatic,
			Roles:    []string{identity.RolePlatformOperator},
		},
	})

	// Local IAM service with the standard demo user (platform.operator role)
	// pre-seeded so tests can log in as demo/demo.
	users := newStubUserRepo()
	sessions := newStubSessionRepo()
	iamSvc := localiam.NewService(users, sessions, localiam.Config{
		SessionTTL: time.Hour,
	})
	if err := iamSvc.SeedDemoUser(context.Background()); err != nil {
		t.Fatalf("seed demo user: %v", err)
	}

	// Structural service with at least one capability so the list
	// endpoint returns a deterministic shape on success.
	capRepo := memory.NewCapabilityRepo()
	now := time.Now().UTC()
	if err := capRepo.Create(context.Background(), &capability.Capability{
		ID:        "cap-auth-chain-test",
		Name:      "Auth Chain Test Capability",
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed capability: %v", err)
	}
	structural := NewStructuralService(capRepo, memory.NewProcessRepo(), memory.NewSurfaceRepo())

	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithStructural(structural).
		WithLocalIAM(iamSvc).
		WithAuthenticator(auth.Chain(staticAuth, localiam.NewSessionAuthenticator(iamSvc))).
		WithAuthMode(config.AuthModeRequired)

	return &authChainTestSetup{
		srv:         srv,
		iamSvc:      iamSvc,
		sessions:    sessions,
		bearerToken: bearerToken,
		bearerID:    bearerID,
	}
}

func (s *authChainTestSetup) loginDemo(t *testing.T) string {
	t.Helper()
	rec := doLogin(t, s.srv, "demo", "demo")
	if rec.Code != http.StatusOK {
		t.Fatalf("demo/demo login failed: %d %s", rec.Code, rec.Body.String())
	}
	cookie := sessionCookie(rec)
	if cookie == "" {
		t.Fatal("no session cookie returned by login")
	}
	return cookie
}

// requestV1 sends a GET to a /v1/* path with the supplied auth headers /
// cookies. Either token or cookie may be empty; both empty exercises the
// "no credentials" path.
func (s *authChainTestSetup) requestV1(t *testing.T, path, bearerToken, sessionCookie string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	if sessionCookie != "" {
		req.AddCookie(&http.Cookie{Name: localiam.SessionCookieName, Value: sessionCookie})
	}
	rec := httptest.NewRecorder()
	s.srv.ServeHTTP(rec, req)
	return rec
}

// TestRequireAuth_LocalIAMSession_AcceptsCookieOnV1Routes is the headline
// fix: a user signed in via Local IAM can access /v1/capabilities under
// AuthModeRequired. Before Phase 7 this returned 401 because requireAuth
// only consulted the static-token authenticator.
func TestRequireAuth_LocalIAMSession_AcceptsCookieOnV1Routes(t *testing.T) {
	setup := newAuthChainTestServer(t)
	cookie := setup.loginDemo(t)

	rec := setup.requestV1(t, "/v1/capabilities", "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 with session cookie, got %d: %s", rec.Code, rec.Body.String())
	}
	// Verify the response shape so we know we actually reached the
	// handler — not just any 200 on the way there.
	var caps []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&caps); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(caps) != 1 || caps[0]["id"] != "cap-auth-chain-test" {
		t.Errorf("want one seeded capability, got %+v", caps)
	}
}

// TestRequireAuth_StaticToken_StillWorksUnderChain pins that the existing
// static-token path is preserved when the chain is in effect — token-bearing
// service callers are unaffected.
func TestRequireAuth_StaticToken_StillWorksUnderChain(t *testing.T) {
	setup := newAuthChainTestServer(t)

	rec := setup.requestV1(t, "/v1/capabilities", setup.bearerToken, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 with valid bearer token, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestRequireAuth_NeitherTokenNorCookie_Returns401 pins the no-credentials
// case under chained auth: requireAuth's "no Authorization header AND no
// cookie" path must surface 401.
func TestRequireAuth_NeitherTokenNorCookie_Returns401(t *testing.T) {
	setup := newAuthChainTestServer(t)

	rec := setup.requestV1(t, "/v1/capabilities", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 with no credentials, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestRequireAuth_InvalidBearerWithValidCookie_Returns401 is the security
// invariant: a present-but-invalid bearer token MUST be rejected, never
// silently bypassed by a valid session cookie. This guards against
// confused-deputy attacks where a caller presents one credential and the
// server quietly substitutes another.
func TestRequireAuth_InvalidBearerWithValidCookie_Returns401(t *testing.T) {
	setup := newAuthChainTestServer(t)
	cookie := setup.loginDemo(t)

	rec := setup.requestV1(t, "/v1/capabilities", "definitely-not-a-real-token", cookie)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 — invalid bearer must NOT fall through to session cookie; got %d: %s",
			rec.Code, rec.Body.String())
	}
	// Belt-and-braces: assert the cookie alone IS valid (so the failure
	// is genuinely caused by the bad bearer header, not by a bad
	// cookie).
	rec2 := setup.requestV1(t, "/v1/capabilities", "", cookie)
	if rec2.Code != http.StatusOK {
		t.Fatalf("control assertion: cookie alone should be 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

// TestRequireAuth_ExpiredSessionCookie_Returns401 pins that an expired
// session cookie is treated as a present-but-invalid credential and yields
// 401. Importantly the session authenticator MUST NOT downgrade an expired
// session to ErrNoCredentials, because the chain's "no creds" semantics
// are about credential SCHEME presence, not session validity.
func TestRequireAuth_ExpiredSessionCookie_Returns401(t *testing.T) {
	setup := newAuthChainTestServer(t)
	cookie := setup.loginDemo(t)

	// Force the session to expire by reaching into the stub repo and
	// rewinding ExpiresAt. The session authenticator's ResolveSession
	// then returns ErrSessionNotFound, which the SessionAuthenticator
	// wraps into a non-ErrNoCredentials error.
	past := time.Now().UTC().Add(-time.Hour)
	for _, sess := range setup.sessions.items {
		sess.ExpiresAt = past
	}

	rec := setup.requestV1(t, "/v1/capabilities", "", cookie)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 for expired session cookie, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestRequireAuth_GarbageSessionCookie_Returns401 pins that a syntactically
// valid but unknown session id (e.g. tampered or stale across server
// restarts) yields 401, not silent fallthrough. Same security invariant as
// the invalid-bearer case but for the cookie scheme.
func TestRequireAuth_GarbageSessionCookie_Returns401(t *testing.T) {
	setup := newAuthChainTestServer(t)

	rec := setup.requestV1(t, "/v1/capabilities", "", "garbage-session-id-not-in-store")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 for unknown session cookie, got %d: %s", rec.Code, rec.Body.String())
	}
	// And the response body should not echo the cookie value.
	if strings.Contains(rec.Body.String(), "garbage-session-id-not-in-store") {
		t.Error("response body should not echo the bad session id")
	}
}
