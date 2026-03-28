package platformauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/identity"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func alicePrincipal() *identity.Principal {
	return &identity.Principal{
		ID:       "user:alice",
		Provider: identity.ProviderStatic,
		Roles:    []string{identity.RolePlatformAdmin},
	}
}

// staticAuthenticator is a minimal auth.Authenticator for testing.
type staticAuthenticator struct {
	principal *identity.Principal
	err       error
}

func (a *staticAuthenticator) Authenticate(_ *http.Request) (*identity.Principal, error) {
	return a.principal, a.err
}

// ok is a trivial handler that records it was called and responds 200.
func ok(called *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if called != nil {
			*called = true
		}
		w.WriteHeader(http.StatusOK)
	})
}

func responseBody(rec *httptest.ResponseRecorder) map[string]string {
	var m map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&m)
	return m
}

// ---------------------------------------------------------------------------
// WithPrincipal / PrincipalFromContext / MustPrincipalFromContext
// ---------------------------------------------------------------------------

func TestWithPrincipal_StoresAndRetrieves(t *testing.T) {
	p := alicePrincipal()
	ctx := WithPrincipal(context.Background(), p)

	got, ok := PrincipalFromContext(ctx)
	if !ok {
		t.Fatal("want ok=true, got false")
	}
	if got != p {
		t.Errorf("want same pointer, got different")
	}
}

func TestPrincipalFromContext_EmptyContext_ReturnsFalse(t *testing.T) {
	got, ok := PrincipalFromContext(context.Background())
	if ok {
		t.Error("want ok=false for empty context")
	}
	if got != nil {
		t.Errorf("want nil principal, got %+v", got)
	}
}

func TestPrincipalFromContext_NilPrincipal_ReturnsFalse(t *testing.T) {
	ctx := context.WithValue(context.Background(), principalKey, (*identity.Principal)(nil))
	got, ok := PrincipalFromContext(ctx)
	if ok {
		t.Error("want ok=false when nil principal stored")
	}
	if got != nil {
		t.Errorf("want nil, got %+v", got)
	}
}

func TestMustPrincipalFromContext_Present_ReturnsPrincipal(t *testing.T) {
	p := alicePrincipal()
	ctx := WithPrincipal(context.Background(), p)

	got := MustPrincipalFromContext(ctx)
	if got != p {
		t.Error("want same pointer")
	}
}

func TestMustPrincipalFromContext_Absent_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("want panic when no principal in context")
		}
	}()
	MustPrincipalFromContext(context.Background())
}

// ---------------------------------------------------------------------------
// Authenticate middleware
// ---------------------------------------------------------------------------

func TestAuthenticate_NilAuthenticator_PassesThrough(t *testing.T) {
	called := false
	mw := Authenticate(nil)(ok(&called))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, r)

	if !called {
		t.Error("inner handler should be called when authenticator is nil")
	}
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func TestAuthenticate_ValidPrincipal_StoresInContext(t *testing.T) {
	p := alicePrincipal()
	a := &staticAuthenticator{principal: p}

	var stored *identity.Principal
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stored, _ = PrincipalFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	mw := Authenticate(a)(handler)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	mw.ServeHTTP(httptest.NewRecorder(), r)

	if stored != p {
		t.Errorf("want principal stored in context, got %+v", stored)
	}
}

func TestAuthenticate_Error_PassesThroughWithoutPrincipal(t *testing.T) {
	a := &staticAuthenticator{err: errors.New("bad token")}

	var stored *identity.Principal
	var ok bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stored, ok = PrincipalFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	mw := Authenticate(a)(handler)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 (extraction only, no enforcement), got %d", w.Code)
	}
	if ok || stored != nil {
		t.Error("want no principal stored when authenticator errors")
	}
}

func TestAuthenticate_NilPrincipalNilError_PassesThroughWithoutPrincipal(t *testing.T) {
	a := &staticAuthenticator{principal: nil, err: nil}

	var hasPrincipal bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hasPrincipal = PrincipalFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	mw := Authenticate(a)(handler)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	mw.ServeHTTP(httptest.NewRecorder(), r)

	if hasPrincipal {
		t.Error("want no principal when authenticator returns (nil, nil)")
	}
}

func TestAuthenticate_DoesNotOverwriteExistingPrincipal(t *testing.T) {
	existing := &identity.Principal{ID: "user:existing", Roles: []string{identity.RolePlatformAdmin}}
	incoming := &identity.Principal{ID: "user:incoming", Roles: []string{identity.RolePlatformOperator}}

	a := &staticAuthenticator{principal: incoming}

	var stored *identity.Principal
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stored, _ = PrincipalFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	mw := Authenticate(a)(handler)

	// Pre-load the existing principal into the request context.
	ctx := WithPrincipal(context.Background(), existing)
	r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	mw.ServeHTTP(httptest.NewRecorder(), r)

	if stored != existing {
		t.Errorf("want existing principal preserved, got %+v", stored)
	}
}

func TestAuthenticate_ErrNoCredentials_PassesThrough(t *testing.T) {
	a := &staticAuthenticator{err: auth.ErrNoCredentials}

	called := false
	mw := Authenticate(a)(ok(&called))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, r)

	if !called {
		t.Error("inner handler should be called; Authenticate is extraction-only")
	}
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// RequireAuthenticated middleware
// ---------------------------------------------------------------------------

func TestRequireAuthenticated_NoPrincipal_Returns401(t *testing.T) {
	called := false
	mw := RequireAuthenticated(ok(&called))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, r)

	if called {
		t.Error("inner handler must not be called when no principal is present")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
	if body := responseBody(w); body["error"] == "" {
		t.Error("want non-empty error in JSON body")
	}
}

func TestRequireAuthenticated_WithPrincipal_CallsNext(t *testing.T) {
	called := false
	mw := RequireAuthenticated(ok(&called))

	ctx := WithPrincipal(context.Background(), alicePrincipal())
	r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, r)

	if !called {
		t.Error("inner handler should be called when principal is present")
	}
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// RequireRole middleware
// ---------------------------------------------------------------------------

func TestRequireRole_ZeroRoles_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("want panic when RequireRole called with zero roles")
		}
	}()
	RequireRole()
}

func TestRequireRole_NoPrincipal_Returns401(t *testing.T) {
	called := false
	mw := RequireRole(identity.RolePlatformAdmin)(ok(&called))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, r)

	if called {
		t.Error("inner handler must not be called with no principal")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestRequireRole_WrongRole_Returns403(t *testing.T) {
	called := false
	mw := RequireRole(identity.RolePlatformAdmin)(ok(&called))

	operator := &identity.Principal{ID: "user:op", Roles: []string{identity.RolePlatformOperator}}
	ctx := WithPrincipal(context.Background(), operator)
	r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, r)

	if called {
		t.Error("inner handler must not be called with wrong role")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", w.Code)
	}
	if body := responseBody(w); body["error"] == "" {
		t.Error("want non-empty error in JSON body")
	}
}

func TestRequireRole_CorrectRole_CallsNext(t *testing.T) {
	called := false
	mw := RequireRole(identity.RolePlatformAdmin)(ok(&called))

	ctx := WithPrincipal(context.Background(), alicePrincipal())
	r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, r)

	if !called {
		t.Error("inner handler should be called when principal has required role")
	}
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func TestRequireRole_MultiRole_AnyMatch_CallsNext(t *testing.T) {
	called := false
	// Require platform.admin or governance.approver; principal has only platform.operator and governance.approver.
	mw := RequireRole(identity.RolePlatformAdmin, identity.RoleGovernanceApprover)(ok(&called))

	p := &identity.Principal{ID: "user:appr", Roles: []string{identity.RolePlatformOperator, identity.RoleGovernanceApprover}}
	ctx := WithPrincipal(context.Background(), p)
	r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, r)

	if !called {
		t.Error("inner handler should be called when at least one role matches")
	}
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func TestRequireRole_CaseInsensitive_Matches(t *testing.T) {
	called := false
	mw := RequireRole(identity.RolePlatformAdmin)(ok(&called))

	// Role stored with uppercase canonical form — HasAnyRole uses EqualFold.
	p := &identity.Principal{ID: "user:upper", Roles: []string{"PLATFORM.ADMIN"}}
	ctx := WithPrincipal(context.Background(), p)
	r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, r)

	if !called {
		t.Error("role comparison should be case-insensitive")
	}
}

// ---------------------------------------------------------------------------
// Middleware composition: Authenticate → RequireRole
// ---------------------------------------------------------------------------

func TestMiddlewareChain_AuthenticateThenRequireRole(t *testing.T) {
	p := alicePrincipal() // has RolePlatformAdmin
	a := &staticAuthenticator{principal: p}

	called := false
	chain := Authenticate(a)(RequireRole(identity.RolePlatformAdmin)(ok(&called)))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, r)

	if !called {
		t.Error("inner handler should be called for admin principal")
	}
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func TestMiddlewareChain_AuthFailThenRequireRole_Returns401(t *testing.T) {
	a := &staticAuthenticator{err: auth.ErrNoCredentials}

	called := false
	chain := Authenticate(a)(RequireRole(identity.RolePlatformAdmin)(ok(&called)))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, r)

	if called {
		t.Error("inner handler must not be called when auth fails and RequireRole blocks")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}
