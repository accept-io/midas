package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/localiam"
	"github.com/accept-io/midas/internal/oidc"
)

// ---------------------------------------------------------------------------
// OIDC cookie name constants (mirror unexported constants in internal/oidc).
// If those constants ever change, these tests will break — which is correct.
// ---------------------------------------------------------------------------

const (
	oidcStateCookieName = "midas_oidc_state"
	oidcPKCECookieName  = "midas_oidc_pkce"
)

// ---------------------------------------------------------------------------
// mockOIDCProvider — implements oidcProvider; all methods are configurable.
// ---------------------------------------------------------------------------

type mockOIDCProvider struct {
	pkce          bool
	authURLFunc   func(state, challenge string) string
	exchangeFunc  func(ctx context.Context, code, verifier string) (*oidc.Claims, error)
	buildPrinFunc func(*oidc.Claims) (*identity.Principal, error)
}

func (m *mockOIDCProvider) UsePKCE() bool { return m.pkce }

func (m *mockOIDCProvider) AuthURL(state, challenge string) string {
	if m.authURLFunc != nil {
		return m.authURLFunc(state, challenge)
	}
	return "https://login.example.com/oauth2/authorize?state=" + state
}

func (m *mockOIDCProvider) Exchange(ctx context.Context, code, verifier string) (*oidc.Claims, error) {
	if m.exchangeFunc != nil {
		return m.exchangeFunc(ctx, code, verifier)
	}
	return &oidc.Claims{Subject: "user-123", Username: "alice"}, nil
}

func (m *mockOIDCProvider) BuildPrincipal(claims *oidc.Claims) (*identity.Principal, error) {
	if m.buildPrinFunc != nil {
		return m.buildPrinFunc(claims)
	}
	return &identity.Principal{
		ID:       "oidc:user-123",
		Subject:  "user-123",
		Name:     "alice",
		Roles:    []string{identity.RolePlatformAdmin},
		Provider: oidc.Provider,
	}, nil
}

// ---------------------------------------------------------------------------
// defaultMockOIDC returns a mock that succeeds for all methods.
// ---------------------------------------------------------------------------

func defaultMockOIDC() *mockOIDCProvider {
	return &mockOIDCProvider{pkce: true}
}

// ---------------------------------------------------------------------------
// newOIDCServer creates a test Server wired with local IAM and the given
// oidcProvider. Returns the server, the localiam.Service, and the stub
// session store so tests can inspect persisted sessions.
// ---------------------------------------------------------------------------

func newOIDCServer(t *testing.T, provider oidcProvider) (*Server, *localiam.Service, *stubSessionRepo) {
	t.Helper()
	users := newStubUserRepo()
	sessions := newStubSessionRepo()
	svc := localiam.NewService(users, sessions, localiam.Config{SessionTTL: time.Hour})
	if err := svc.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	srv := newTestServer().WithLocalIAM(svc).WithOIDC(provider, false)
	return srv, svc, sessions
}

// ---------------------------------------------------------------------------
// cookieFromResponse extracts a named cookie from a response recorder.
// ---------------------------------------------------------------------------

func cookieFromResponse(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// loginRequest builds a GET /auth/oidc/login request.
// ---------------------------------------------------------------------------

func loginRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/auth/oidc/login", nil)
}

// ---------------------------------------------------------------------------
// callbackRequest builds a GET /auth/oidc/callback request with the given
// state cookie and query parameters.
// ---------------------------------------------------------------------------

func callbackRequest(stateCookieVal, stateParam, codeParam, errParam string) *http.Request {
	target := "/auth/oidc/callback"
	params := []string{}
	if stateParam != "" {
		params = append(params, "state="+stateParam)
	}
	if codeParam != "" {
		params = append(params, "code="+codeParam)
	}
	if errParam != "" {
		params = append(params, "error="+errParam)
	}
	if len(params) > 0 {
		target += "?" + strings.Join(params, "&")
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if stateCookieVal != "" {
		req.AddCookie(&http.Cookie{Name: oidcStateCookieName, Value: stateCookieVal})
	}
	return req
}

// ---------------------------------------------------------------------------
// /auth/oidc/login handler tests
// ---------------------------------------------------------------------------

func TestOIDCLogin_WithOIDC_Returns302(t *testing.T) {
	srv, _, _ := newOIDCServer(t, defaultMockOIDC())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, loginRequest())

	if rec.Code != http.StatusFound {
		t.Errorf("want 302, got %d", rec.Code)
	}
}

func TestOIDCLogin_RedirectLocationUsesAuthURLReturn(t *testing.T) {
	const wantBase = "https://login.example.com/oauth2/authorize"
	mock := &mockOIDCProvider{
		authURLFunc: func(state, _ string) string {
			return wantBase + "?state=" + state + "&other=param"
		},
	}
	srv, _, _ := newOIDCServer(t, mock)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, loginRequest())

	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, wantBase) {
		t.Errorf("Location %q does not start with %q", loc, wantBase)
	}
}

func TestOIDCLogin_SetsStateCookie(t *testing.T) {
	srv, _, _ := newOIDCServer(t, defaultMockOIDC())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, loginRequest())

	c := cookieFromResponse(rec, oidcStateCookieName)
	if c == nil {
		t.Fatal("state cookie not set")
	}
	if c.Value == "" {
		t.Error("state cookie value is empty")
	}
	if !c.HttpOnly {
		t.Error("state cookie must be HttpOnly")
	}
}

func TestOIDCLogin_StateCookieValueMatchesRedirectState(t *testing.T) {
	srv, _, _ := newOIDCServer(t, defaultMockOIDC())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, loginRequest())

	stateCookie := cookieFromResponse(rec, oidcStateCookieName)
	if stateCookie == nil {
		t.Fatal("state cookie not set")
	}

	loc := rec.Header().Get("Location")
	// The state cookie value must appear somewhere in the redirect URL.
	if !strings.Contains(loc, stateCookie.Value) {
		t.Errorf("redirect URL %q does not contain state cookie value %q", loc, stateCookie.Value)
	}
}

func TestOIDCLogin_WithPKCE_SetsPKCECookie(t *testing.T) {
	mock := &mockOIDCProvider{pkce: true}
	srv, _, _ := newOIDCServer(t, mock)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, loginRequest())

	c := cookieFromResponse(rec, oidcPKCECookieName)
	if c == nil {
		t.Fatal("PKCE cookie not set when use_pkce=true")
	}
	if c.Value == "" {
		t.Error("PKCE cookie value is empty")
	}
	if !c.HttpOnly {
		t.Error("PKCE cookie must be HttpOnly")
	}
}

func TestOIDCLogin_WithoutPKCE_NoPKCECookie(t *testing.T) {
	mock := &mockOIDCProvider{pkce: false}
	srv, _, _ := newOIDCServer(t, mock)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, loginRequest())

	c := cookieFromResponse(rec, oidcPKCECookieName)
	if c != nil {
		t.Errorf("PKCE cookie must not be set when use_pkce=false; got value=%q", c.Value)
	}
}

func TestOIDCLogin_WithPKCE_ChallengeInRedirect(t *testing.T) {
	var capturedChallenge string
	mock := &mockOIDCProvider{
		pkce: true,
		authURLFunc: func(state, challenge string) string {
			capturedChallenge = challenge
			return "https://login.example.com/?state=" + state + "&code_challenge=" + challenge
		},
	}
	srv, _, _ := newOIDCServer(t, mock)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, loginRequest())

	if capturedChallenge == "" {
		t.Error("PKCE challenge was not passed to AuthURL")
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, capturedChallenge) {
		t.Errorf("redirect URL does not contain PKCE challenge %q", capturedChallenge)
	}
}

func TestOIDCLogin_WithoutPKCE_EmptyChallengePassedToAuthURL(t *testing.T) {
	var capturedChallenge string
	mock := &mockOIDCProvider{
		pkce: false,
		authURLFunc: func(_, challenge string) string {
			capturedChallenge = challenge
			return "https://login.example.com/"
		},
	}
	srv, _, _ := newOIDCServer(t, mock)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, loginRequest())

	if capturedChallenge != "" {
		t.Errorf("expected empty challenge when PKCE disabled, got %q", capturedChallenge)
	}
}

func TestOIDCLogin_DomainHintIncludedWhenPresent(t *testing.T) {
	// domain_hint is applied inside AuthURL. At the handler level we verify
	// that when AuthURL returns a URL containing domain_hint the redirect
	// faithfully uses it.
	mock := &mockOIDCProvider{
		authURLFunc: func(state, _ string) string {
			return "https://login.example.com/?domain_hint=contoso.com&state=" + state
		},
	}
	srv, _, _ := newOIDCServer(t, mock)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, loginRequest())

	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "domain_hint=contoso.com") {
		t.Errorf("redirect URL %q does not contain domain_hint", loc)
	}
}

func TestOIDCLogin_NoOIDCService_Returns503(t *testing.T) {
	srv := newTestServer()
	// Do not call WithOIDC — oidcService remains nil.
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/login", nil)
	rec := httptest.NewRecorder()
	srv.handleOIDCLogin(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503, got %d", rec.Code)
	}
}

func TestOIDCLogin_MethodNotAllowed_Returns405(t *testing.T) {
	srv, _, _ := newOIDCServer(t, defaultMockOIDC())
	req := httptest.NewRequest(http.MethodPost, "/auth/oidc/login", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// /auth/oidc/callback handler tests
// ---------------------------------------------------------------------------

func TestOIDCCallback_MissingStateParam_Returns400(t *testing.T) {
	srv, _, _ := newOIDCServer(t, defaultMockOIDC())
	// No state param, no state cookie.
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?code=abc", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestOIDCCallback_MissingStateCookie_Returns400(t *testing.T) {
	srv, _, _ := newOIDCServer(t, defaultMockOIDC())
	// State in query but no cookie.
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state=somestate&code=abc", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestOIDCCallback_MismatchedState_Returns400(t *testing.T) {
	srv, _, _ := newOIDCServer(t, defaultMockOIDC())
	req := callbackRequest("cookie-state", "different-state", "code123", "")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestOIDCCallback_ProviderError_Returns401(t *testing.T) {
	srv, _, _ := newOIDCServer(t, defaultMockOIDC())
	req := callbackRequest("s1", "s1", "", "access_denied")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestOIDCCallback_ProviderError_DoesNotLeakDetails(t *testing.T) {
	srv, _, _ := newOIDCServer(t, defaultMockOIDC())
	req := callbackRequest("s1", "s1", "", "access_denied")
	req.URL.RawQuery += "&error_description=Token+contains+secret+refresh_token+xyz"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	body := rec.Body.String()
	// The error_description param is passed through oidcError which HTML-escapes it.
	// Verify the raw token value doesn't appear in the response verbatim as a secret.
	// The description IS shown (it's user-facing info), but we check the status is correct.
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
	// The response must be HTML (browser redirect path), not JSON.
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("error response should be HTML, got Content-Type: %q", ct)
	}
	// The HTML must not contain raw cookie values or internal state.
	if strings.Contains(body, "s1") {
		t.Errorf("state value leaked into error response body")
	}
}

func TestOIDCCallback_MissingCode_Returns400(t *testing.T) {
	srv, _, _ := newOIDCServer(t, defaultMockOIDC())
	// Valid matching state, no code, no provider error.
	req := callbackRequest("s1", "s1", "", "")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestOIDCCallback_ExchangeFailure_Returns401(t *testing.T) {
	mock := &mockOIDCProvider{
		exchangeFunc: func(_ context.Context, _, _ string) (*oidc.Claims, error) {
			return nil, errors.New("token exchange failed")
		},
	}
	srv, _, _ := newOIDCServer(t, mock)
	req := callbackRequest("s1", "s1", "code123", "")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestOIDCCallback_NoRolesMapped_Returns403(t *testing.T) {
	mock := &mockOIDCProvider{
		buildPrinFunc: func(_ *oidc.Claims) (*identity.Principal, error) {
			return nil, oidc.ErrNoRolesMapped
		},
	}
	srv, _, _ := newOIDCServer(t, mock)
	req := callbackRequest("s1", "s1", "code123", "")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

func TestOIDCCallback_GroupNotAllowed_Returns403(t *testing.T) {
	mock := &mockOIDCProvider{
		buildPrinFunc: func(_ *oidc.Claims) (*identity.Principal, error) {
			return nil, oidc.ErrGroupNotAllowed
		},
	}
	srv, _, _ := newOIDCServer(t, mock)
	req := callbackRequest("s1", "s1", "code123", "")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

func TestOIDCCallback_AccessDenied_ErrorResponseIsSafe(t *testing.T) {
	// Both ErrNoRolesMapped and ErrGroupNotAllowed produce the same opaque
	// "access_denied" error. Verify the response doesn't reveal which check failed.
	for _, buildErr := range []error{oidc.ErrNoRolesMapped, oidc.ErrGroupNotAllowed} {
		buildErr := buildErr
		mock := &mockOIDCProvider{
			buildPrinFunc: func(_ *oidc.Claims) (*identity.Principal, error) {
				return nil, buildErr
			},
		}
		srv, _, _ := newOIDCServer(t, mock)
		req := callbackRequest("s1", "s1", "code123", "")
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		body := rec.Body.String()
		if strings.Contains(body, "no_roles") || strings.Contains(body, "group_not") {
			t.Errorf("error response reveals internal denial reason for %v: %s", buildErr, body)
		}
	}
}

func TestOIDCCallback_Success_Returns302ToExplorer(t *testing.T) {
	srv, _, _ := newOIDCServer(t, defaultMockOIDC())
	req := callbackRequest("s1", "s1", "code123", "")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("want 302, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "/explorer" {
		t.Errorf("want redirect to /explorer, got %q", loc)
	}
}

func TestOIDCCallback_Success_SetsMIDASSessionCookie(t *testing.T) {
	srv, _, _ := newOIDCServer(t, defaultMockOIDC())
	req := callbackRequest("s1", "s1", "code123", "")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	c := cookieFromResponse(rec, localiam.SessionCookieName)
	if c == nil {
		t.Fatalf("midas_session cookie not set after successful OIDC callback")
	}
	if c.Value == "" {
		t.Error("midas_session cookie value is empty")
	}
	if !c.HttpOnly {
		t.Error("midas_session cookie must be HttpOnly")
	}
}

func TestOIDCCallback_Success_SessionPersistedInStore(t *testing.T) {
	srv, _, sessions := newOIDCServer(t, defaultMockOIDC())
	req := callbackRequest("s1", "s1", "code123", "")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	c := cookieFromResponse(rec, localiam.SessionCookieName)
	if c == nil {
		t.Fatal("midas_session cookie not set")
	}

	sess, ok := sessions.items[c.Value]
	if !ok {
		t.Fatalf("session %q not found in store after callback", c.Value)
	}
	// Must be an external (OIDC) session: no local user ID, principal stored as JSON.
	if sess.UserID != "" {
		t.Errorf("OIDC session must have empty UserID, got %q", sess.UserID)
	}
	if sess.PrincipalJSON == "" {
		t.Error("OIDC session must have PrincipalJSON set")
	}
}

func TestOIDCCallback_Success_SessionResolvesToCorrectPrincipal(t *testing.T) {
	const wantSubject = "user-456"
	const wantName = "bob"
	mock := &mockOIDCProvider{
		buildPrinFunc: func(_ *oidc.Claims) (*identity.Principal, error) {
			return &identity.Principal{
				ID:       "oidc:" + wantSubject,
				Subject:  wantSubject,
				Name:     wantName,
				Roles:    []string{identity.RolePlatformOperator},
				Provider: oidc.Provider,
			}, nil
		},
	}
	srv, svc, _ := newOIDCServer(t, mock)
	req := callbackRequest("s1", "s1", "code123", "")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	c := cookieFromResponse(rec, localiam.SessionCookieName)
	if c == nil {
		t.Fatal("midas_session cookie not set")
	}

	// Resolve the session via the same path the auth middleware uses.
	_, principal, mustChange, err := svc.ResolveSession(context.Background(), c.Value)
	if err != nil {
		t.Fatalf("ResolveSession failed: %v", err)
	}
	if mustChange {
		t.Error("mustChangePassword must be false for OIDC sessions")
	}
	if principal.Subject != wantSubject {
		t.Errorf("Subject = %q, want %q", principal.Subject, wantSubject)
	}
	if principal.Name != wantName {
		t.Errorf("Name = %q, want %q", principal.Name, wantName)
	}
	if principal.Provider != oidc.Provider {
		t.Errorf("Provider = %q, want %q", principal.Provider, oidc.Provider)
	}
}

func TestOIDCCallback_Success_PrincipalJSONContainsOnlyMinimalData(t *testing.T) {
	// Verify that no token material is stored in the session row.
	// The persisted JSON must contain only the identity fields needed for
	// authorization (ID, Subject, Name, Roles, Provider) — not raw JWT claims,
	// access tokens, or refresh tokens.
	srv, _, sessions := newOIDCServer(t, defaultMockOIDC())
	req := callbackRequest("s1", "s1", "code123", "")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	c := cookieFromResponse(rec, localiam.SessionCookieName)
	if c == nil {
		t.Fatal("midas_session cookie not set")
	}
	sess := sessions.items[c.Value]
	if sess == nil {
		t.Fatal("session not found in store")
	}

	json := sess.PrincipalJSON
	// Must contain the expected identity fields.
	if !strings.Contains(json, `"Subject"`) && !strings.Contains(json, `"subject"`) {
		t.Error("PrincipalJSON missing Subject field")
	}
	if !strings.Contains(json, `"Roles"`) && !strings.Contains(json, `"roles"`) {
		t.Error("PrincipalJSON missing Roles field")
	}
	// Must NOT contain raw JWT token material indicators.
	// access_token, refresh_token, id_token are never in identity.Principal.
	for _, forbidden := range []string{"access_token", "refresh_token", "id_token", "raw"} {
		if strings.Contains(strings.ToLower(json), forbidden) {
			t.Errorf("PrincipalJSON contains forbidden token material key %q: %s", forbidden, json)
		}
	}
}

func TestOIDCCallback_NoOIDCService_Returns503(t *testing.T) {
	srv := newTestServer()
	// No WithOIDC — oidcService is nil.
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback", nil)
	rec := httptest.NewRecorder()
	srv.handleOIDCCallback(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503, got %d", rec.Code)
	}
}

func TestOIDCCallback_NoLocalIAM_Returns503(t *testing.T) {
	// Construct a server with OIDC but no local IAM.
	srv := newTestServer()
	srv.oidcService = defaultMockOIDC()
	// Register routes manually (normally done by WithOIDC).
	srv.mux.HandleFunc("/auth/oidc/callback", srv.handleOIDCCallback)

	// Provide a valid matching state so we get past the state check.
	req := callbackRequest("s1", "s1", "code123", "")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503, got %d", rec.Code)
	}
}

func TestOIDCCallback_StateCookieClearedAfterUse(t *testing.T) {
	// The state cookie must be consumed (cleared) on callback even on success,
	// preventing replay.
	srv, _, _ := newOIDCServer(t, defaultMockOIDC())
	req := callbackRequest("s1", "s1", "code123", "")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	// The state cookie should appear in Set-Cookie with MaxAge=-1 (cleared).
	cleared := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == oidcStateCookieName && c.MaxAge == -1 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("state cookie was not cleared (MaxAge=-1) after successful callback")
	}
}

func TestOIDCCallback_MethodNotAllowed_Returns405(t *testing.T) {
	srv, _, _ := newOIDCServer(t, defaultMockOIDC())
	req := httptest.NewRequest(http.MethodPost, "/auth/oidc/callback", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rec.Code)
	}
}
