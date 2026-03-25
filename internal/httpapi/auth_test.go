package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/config"
	cpTypes "github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestServer() *Server {
	return NewServerFull(nil, nil, nil, nil, nil, nil)
}

func aliceAuthenticator() auth.Authenticator {
	return auth.NewStaticTokenAuthenticator(map[string]*identity.Principal{
		"tok-alice": {
			ID:       "user:alice",
			Provider: identity.ProviderStatic,
			Roles:    []string{identity.RoleAdmin},
		},
	})
}

// ---------------------------------------------------------------------------
// PrincipalFromContext
// ---------------------------------------------------------------------------

func TestPrincipalFromContext_NoPrincipal(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := PrincipalFromContext(r.Context()); got != nil {
		t.Errorf("want nil, got %+v", got)
	}
}

// ---------------------------------------------------------------------------
// actorFromContext
// ---------------------------------------------------------------------------

func TestActorFromContext_NoPrincipal_ReturnsFallback(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	got := actorFromContext(r.Context(), "body-actor")
	if got != "body-actor" {
		t.Errorf("want body-actor, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// requireAuth — no authenticator configured (fail closed)
// ---------------------------------------------------------------------------

// TestRequireAuth_NoAuthenticator_FailsClosed verifies that requireAuth returns
// 401 when no authenticator is configured and the auth mode is not open.
// A server that has not been given an authenticator must not silently allow
// access — callers must explicitly use AuthModeOpen for unauthenticated access.
func TestRequireAuth_NoAuthenticator_FailsClosed(t *testing.T) {
	srv := newTestServer() // no WithAuthMode, no WithAuthenticator

	called := false
	handler := srv.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	if called {
		t.Error("inner handler must not be called when no authenticator is configured")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401 (fail closed), got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// requireAuth — authenticator configured
// ---------------------------------------------------------------------------

func TestRequireAuth_ValidToken_StoresPrincipalInContext(t *testing.T) {
	srv := newTestServer().WithAuthenticator(aliceAuthenticator())

	var capturedPrincipal *identity.Principal
	handler := srv.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		capturedPrincipal = PrincipalFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.Header.Set("Authorization", "Bearer tok-alice")
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if capturedPrincipal == nil {
		t.Fatal("principal not stored in context")
	}
	if capturedPrincipal.ID != "user:alice" {
		t.Errorf("want user:alice, got %q", capturedPrincipal.ID)
	}
}

func TestRequireAuth_MissingToken_Returns401(t *testing.T) {
	srv := newTestServer().WithAuthenticator(aliceAuthenticator())

	called := false
	handler := srv.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	if called {
		t.Error("inner handler must not be called on auth failure")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestRequireAuth_UnknownToken_Returns401(t *testing.T) {
	srv := newTestServer().WithAuthenticator(aliceAuthenticator())

	handler := srv.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// WithAuthenticator — post-construction wiring
// ---------------------------------------------------------------------------

func TestWithAuthenticator_AfterConstruction(t *testing.T) {
	srv := newTestServer()
	if srv.authenticator != nil {
		t.Fatal("authenticator should be nil before WithAuthenticator")
	}

	returned := srv.WithAuthenticator(aliceAuthenticator())
	if returned != srv {
		t.Error("WithAuthenticator should return the same server instance")
	}
	if srv.authenticator == nil {
		t.Error("authenticator should be set after WithAuthenticator")
	}
}

// ---------------------------------------------------------------------------
// actorFromContext — principal present
// ---------------------------------------------------------------------------

func TestActorFromContext_WithPrincipal_ReturnsID(t *testing.T) {
	srv := newTestServer().WithAuthenticator(aliceAuthenticator())

	var capturedActor string
	handler := srv.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		capturedActor = actorFromContext(r.Context(), "body-actor")
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.Header.Set("Authorization", "Bearer tok-alice")
	w := httptest.NewRecorder()
	handler(w, r)

	if capturedActor != "user:alice" {
		t.Errorf("want user:alice, got %q", capturedActor)
	}
}

// ---------------------------------------------------------------------------
// Handler-level integration tests: auth enforcement and principal propagation
// ---------------------------------------------------------------------------

// TestHandlerAuth_Reviews_NoToken_Returns401 verifies that POST /v1/reviews
// rejects unauthenticated requests when an authenticator is configured.
func TestHandlerAuth_Reviews_NoToken_Returns401(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthenticator(aliceAuthenticator())

	body := marshalJSON(t, map[string]any{
		"envelope_id": "env-abc",
		"decision":    "approve",
		"reviewer":    "imposter",
	})
	rec := performRequest(t, srv, http.MethodPost, "/v1/reviews", body)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestHandlerAuth_Reviews_TokenOverridesBodyReviewer verifies that when a
// valid bearer token is present, the authenticated principal's ID is used as
// ReviewerID — the body-supplied "reviewer" field is ignored.
func TestHandlerAuth_Reviews_TokenOverridesBodyReviewer(t *testing.T) {
	var capturedReviewerID string

	mock := &mockOrchestrator{
		resolveEscalationFn: func(_ context.Context, res decision.EscalationResolution) (*envelope.Envelope, error) {
			capturedReviewerID = res.ReviewerID
			return nil, nil // nil envelope handled gracefully by the handler
		},
	}

	srv := NewServerFull(mock, nil, nil, nil, nil, nil).
		WithAuthenticator(aliceAuthenticator())

	body := marshalJSON(t, map[string]any{
		"envelope_id": "env-abc",
		"decision":    "approve",
		"reviewer":    "imposter", // should be overridden by the token principal
	})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/reviews", body,
		map[string]string{"Authorization": "Bearer tok-alice"})

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if capturedReviewerID != "user:alice" {
		t.Errorf("want ReviewerID=user:alice (from token), got %q", capturedReviewerID)
	}
}

// TestHandlerAuth_ApproveProfile_TokenOverridesBodyActor verifies that when a
// valid bearer token is present on a governance handler that already uses
// actorFromContext, the authenticated principal's ID is used as the actor —
// the body-supplied actor field is ignored.
func TestHandlerAuth_ApproveProfile_TokenOverridesBodyActor(t *testing.T) {
	var capturedApprovedBy string

	mockApproval := &mockApprovalService{
		approveProfileFn: func(_ context.Context, _ string, _ int, approvedBy string) (*authority.AuthorityProfile, error) {
			capturedApprovedBy = approvedBy
			return &authority.AuthorityProfile{
				ID:         "prof-1",
				Version:    1,
				Status:     authority.ProfileStatusActive,
				ApprovedBy: approvedBy,
			}, nil
		},
	}

	srv := NewServerFull(&mockOrchestrator{}, nil, mockApproval, nil, nil, nil).
		WithAuthenticator(aliceAuthenticator())

	body := marshalJSON(t, map[string]any{
		"version":     1,
		"approved_by": "imposter", // should be overridden by the token principal
	})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/profiles/prof-1/approve", body,
		map[string]string{"Authorization": "Bearer tok-alice"})

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if capturedApprovedBy != "user:alice" {
		t.Errorf("want approvedBy=user:alice (from token), got %q", capturedApprovedBy)
	}
}

// ---------------------------------------------------------------------------
// RBAC test helpers
// ---------------------------------------------------------------------------

// rbacAuthenticator returns an authenticator with one token per role scenario.
func rbacAuthenticator() auth.Authenticator {
	return auth.NewStaticTokenAuthenticator(map[string]*identity.Principal{
		"tok-admin":        {ID: "user:admin", Provider: identity.ProviderStatic, Roles: []string{identity.RoleAdmin}},
		"tok-approver":     {ID: "user:approver", Provider: identity.ProviderStatic, Roles: []string{identity.RoleApprover}},
		"tok-reviewer":     {ID: "user:reviewer", Provider: identity.ProviderStatic, Roles: []string{identity.RoleReviewer}},
		"tok-operator":     {ID: "user:operator", Provider: identity.ProviderStatic, Roles: []string{identity.RoleOperator}},
		"tok-multi":        {ID: "user:multi", Provider: identity.ProviderStatic, Roles: []string{identity.RoleOperator, identity.RoleApprover}},
		"tok-unknown-role": {ID: "user:unknown", Provider: identity.ProviderStatic, Roles: []string{"some_unknown_role"}},
		"tok-empty-roles":  {ID: "user:empty", Provider: identity.ProviderStatic, Roles: []string{}},
		"tok-upper-admin":  {ID: "user:upperadmin", Provider: identity.ProviderStatic, Roles: []string{"ADMIN"}},
	})
}

// approvalSrvRBAC returns a server wired with the RBAC authenticator and a
// minimal approval service mock. Used by surface-approve RBAC tests.
func approvalSrvRBAC(t *testing.T) *Server {
	t.Helper()
	mockApproval := &mockApprovalService{
		approveSurfaceFn: func(_ context.Context, surfaceID string, _ identity.Principal, approver identity.Principal) (*surface.DecisionSurface, error) {
			return &surface.DecisionSurface{
				ID:     surfaceID,
				Status: surface.SurfaceStatusActive,
			}, nil
		},
	}
	return NewServerFull(&mockOrchestrator{}, nil, mockApproval, nil, nil, nil).
		WithAuthenticator(rbacAuthenticator())
}

// reviewSrvRBAC returns a server wired with the RBAC authenticator and a mock
// orchestrator that resolves escalations successfully. Used by /v1/reviews RBAC tests.
func reviewSrvRBAC(t *testing.T) *Server {
	t.Helper()
	mock := &mockOrchestrator{
		resolveEscalationFn: func(_ context.Context, _ decision.EscalationResolution) (*envelope.Envelope, error) {
			return nil, nil
		},
	}
	return NewServerFull(mock, nil, nil, nil, nil, nil).
		WithAuthenticator(rbacAuthenticator())
}

// applySrvRBAC returns a server wired with the RBAC authenticator and a mock
// control plane. Used by /v1/controlplane/apply RBAC tests.
func applySrvRBAC(t *testing.T) *Server {
	t.Helper()
	mockCP := &mockControlPlane{
		applyBundleFn: func(_ context.Context, _ []byte, _ string) (*cpTypes.ApplyResult, error) {
			return &cpTypes.ApplyResult{}, nil
		},
	}
	return NewServerWithControlPlane(&mockOrchestrator{}, mockCP).
		WithAuthenticator(rbacAuthenticator())
}

// ---------------------------------------------------------------------------
// RBAC tests: surface approve endpoint
// ---------------------------------------------------------------------------

// 1. approval route: no token => 401
func TestRBAC_ApproveSurface_NoToken_Returns401(t *testing.T) {
	srv := approvalSrvRBAC(t)
	body := marshalJSON(t, map[string]any{"submitted_by": "user:sub"})
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/approve", body)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

// 2. approval route: operator token => 403
func TestRBAC_ApproveSurface_OperatorToken_Returns403(t *testing.T) {
	srv := approvalSrvRBAC(t)
	body := marshalJSON(t, map[string]any{"submitted_by": "user:sub"})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/approve", body,
		map[string]string{"Authorization": "Bearer tok-operator"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

// 3. approval route: approver token => success
func TestRBAC_ApproveSurface_ApproverToken_Succeeds(t *testing.T) {
	srv := approvalSrvRBAC(t)
	body := marshalJSON(t, map[string]any{"submitted_by": "user:sub"})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/approve", body,
		map[string]string{"Authorization": "Bearer tok-approver"})
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// 4. approval route: admin token => success
func TestRBAC_ApproveSurface_AdminToken_Succeeds(t *testing.T) {
	srv := approvalSrvRBAC(t)
	body := marshalJSON(t, map[string]any{"submitted_by": "user:sub"})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/approve", body,
		map[string]string{"Authorization": "Bearer tok-admin"})
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// RBAC tests: POST /v1/reviews
// ---------------------------------------------------------------------------

// 5. POST /v1/reviews: operator token => 403
func TestRBAC_Reviews_OperatorToken_Returns403(t *testing.T) {
	srv := reviewSrvRBAC(t)
	body := marshalJSON(t, map[string]any{"envelope_id": "env-1", "decision": "approve"})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/reviews", body,
		map[string]string{"Authorization": "Bearer tok-operator"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

// 6. POST /v1/reviews: reviewer token => success
func TestRBAC_Reviews_ReviewerToken_Succeeds(t *testing.T) {
	srv := reviewSrvRBAC(t)
	body := marshalJSON(t, map[string]any{"envelope_id": "env-1", "decision": "approve"})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/reviews", body,
		map[string]string{"Authorization": "Bearer tok-reviewer"})
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// RBAC tests: POST /v1/controlplane/apply
// ---------------------------------------------------------------------------

// 7. POST /v1/controlplane/apply: operator token => 403
func TestRBAC_Apply_OperatorToken_Returns403(t *testing.T) {
	srv := applySrvRBAC(t)
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/apply",
		[]byte(`kind: Surface`),
		map[string]string{"Authorization": "Bearer tok-operator", "Content-Type": "application/yaml"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

// 8. POST /v1/controlplane/apply: admin token => allowed path reached (non-403)
func TestRBAC_Apply_AdminToken_ReachesHandler(t *testing.T) {
	srv := applySrvRBAC(t)
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/apply",
		[]byte(`kind: Surface`),
		map[string]string{"Authorization": "Bearer tok-admin", "Content-Type": "application/yaml"})
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Errorf("admin should reach handler, got %d: %s", rec.Code, rec.Body.String())
	}
}

// 9. principal override: authorized reviewer's identity overrides body reviewer field.
// (Covered by TestHandlerAuth_Reviews_TokenOverridesBodyReviewer using aliceAuthenticator.)

// ---------------------------------------------------------------------------
// RBAC tests: edge cases
// ---------------------------------------------------------------------------

// 10. unknown role => 403
func TestRBAC_UnknownRole_Returns403(t *testing.T) {
	srv := approvalSrvRBAC(t)
	body := marshalJSON(t, map[string]any{"submitted_by": "user:sub"})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/approve", body,
		map[string]string{"Authorization": "Bearer tok-unknown-role"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

// 11. multi-role principal including allowed role => success
// tok-multi has [operator, approver]; approver is sufficient for surface approve.
func TestRBAC_MultiRole_AllowedRolePresent_Succeeds(t *testing.T) {
	srv := approvalSrvRBAC(t)
	body := marshalJSON(t, map[string]any{"submitted_by": "user:sub"})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/approve", body,
		map[string]string{"Authorization": "Bearer tok-multi"})
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// 12. empty roles => 403
func TestRBAC_EmptyRoles_Returns403(t *testing.T) {
	srv := approvalSrvRBAC(t)
	body := marshalJSON(t, map[string]any{"submitted_by": "user:sub"})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/approve", body,
		map[string]string{"Authorization": "Bearer tok-empty-roles"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

// 13. role normalization: "ADMIN" (uppercase) matches RoleAdmin check.
func TestRBAC_RoleNormalization_UppercaseRoleMatches(t *testing.T) {
	srv := approvalSrvRBAC(t)
	body := marshalJSON(t, map[string]any{"submitted_by": "user:sub"})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/approve", body,
		map[string]string{"Authorization": "Bearer tok-upper-admin"})
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 (uppercase ADMIN normalized), got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// /v1/evaluate — auth enforcement regression tests
//
// These tests guard against the class of bug where requireAuth is accidentally
// omitted from the evaluate route, allowing unauthenticated access to the data
// plane when an authenticator is configured.
// ---------------------------------------------------------------------------

// TestHandlerAuth_Evaluate_NoToken_Returns401 verifies that POST /v1/evaluate
// rejects requests with no Authorization header when auth is configured.
func TestHandlerAuth_Evaluate_NoToken_Returns401(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthenticator(rbacAuthenticator())

	body := marshalJSON(t, map[string]any{
		"surface_id": "surf-1",
		"agent_id":   "agent-1",
		"confidence": 0.9,
	})
	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", body)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 (no token), got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestHandlerAuth_Evaluate_InvalidToken_Returns401 verifies that POST /v1/evaluate
// rejects requests carrying an unrecognised bearer token.
func TestHandlerAuth_Evaluate_InvalidToken_Returns401(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthenticator(rbacAuthenticator())

	body := marshalJSON(t, map[string]any{
		"surface_id": "surf-1",
		"agent_id":   "agent-1",
		"confidence": 0.9,
	})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/evaluate", body,
		map[string]string{"Authorization": "Bearer not-a-real-token"})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 (bad token), got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestHandlerAuth_Evaluate_OperatorToken_ReachesHandler verifies that a valid
// operator-role token is accepted and the request reaches the evaluate handler.
func TestHandlerAuth_Evaluate_OperatorToken_ReachesHandler(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthenticator(rbacAuthenticator())

	body := marshalJSON(t, map[string]any{
		"surface_id": "surf-1",
		"agent_id":   "agent-1",
		"confidence": 0.9,
	})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/evaluate", body,
		map[string]string{
			"Authorization": "Bearer tok-operator",
			"Content-Type":  "application/json",
		})

	// The mock orchestrator returns a zero-value result; any non-401/403 response
	// means the request passed auth and reached the handler.
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Errorf("want request to reach handler (operator role), got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestHandlerAuth_Evaluate_OpenMode_PassesThrough verifies that when auth mode
// is explicitly set to open (the default for memory/dev deployments), the
// evaluate endpoint is accessible without a token.
func TestHandlerAuth_Evaluate_OpenMode_PassesThrough(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen) // explicit open mode — simulates memory/dev config

	body := marshalJSON(t, map[string]any{
		"surface_id": "surf-1",
		"agent_id":   "agent-1",
		"confidence": 0.9,
	})
	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", body)

	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Errorf("want request to pass through without auth (no authenticator), got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Fix 3: requireRole must fail closed — open mode does not grant role access
// ---------------------------------------------------------------------------

// withPrincipal returns a copy of r with the given principal injected into the
// request context, simulating what requireAuth does on a successful auth check.
func withPrincipal(r *http.Request, p *identity.Principal) *http.Request {
	ctx := context.WithValue(r.Context(), principalContextKey, p)
	return r.WithContext(ctx)
}

// TestRequireAuth_OpenMode_AllowsUnauthenticated verifies that requireAuth in
// explicit open mode passes through without requiring a token.
func TestRequireAuth_OpenMode_AllowsUnauthenticated(t *testing.T) {
	srv := newTestServer().WithAuthMode(config.AuthModeOpen)

	called := false
	handler := srv.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	// No Authorization header.
	w := httptest.NewRecorder()
	handler(w, r)

	if !called {
		t.Error("inner handler should be called in open mode without a token")
	}
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

// TestRequireRole_OpenMode_PassesThrough verifies that in open mode requireRole
// is a no-op: requests without a principal reach the handler. This makes
// dev/memory deployments fully functional without bearer tokens.
func TestRequireRole_OpenMode_PassesThrough(t *testing.T) {
	srv := newTestServer().WithAuthMode(config.AuthModeOpen)

	called := false
	handler := srv.requireRole(identity.RoleAdmin)(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest(http.MethodPost, "/test", nil)
	// No principal in context — open mode, requireAuth did not set one.
	w := httptest.NewRecorder()
	handler(w, r)

	if !called {
		t.Error("inner handler should be called in open mode without a principal")
	}
	if w.Code != http.StatusOK {
		t.Errorf("want 200 in open mode, got %d", w.Code)
	}
}

// TestRequireRole_OpenMode_AllowsWithPrincipalAndRole verifies that requireRole
// succeeds in open mode when a principal with the required role is already in
// context (e.g. from a test helper or a future auth plugin).
func TestRequireRole_OpenMode_AllowsWithPrincipalAndRole(t *testing.T) {
	srv := newTestServer().WithAuthMode(config.AuthModeOpen)

	called := false
	handler := srv.requireRole(identity.RoleAdmin)(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest(http.MethodPost, "/test", nil)
	r = withPrincipal(r, &identity.Principal{
		ID:    "user:admin",
		Roles: []string{identity.RoleAdmin},
	})
	w := httptest.NewRecorder()
	handler(w, r)

	if !called {
		t.Error("inner handler should be called when principal has required role")
	}
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

// TestRequireAuth_RequiredMode_NoAuthenticator_FailsClosed verifies that a
// server configured with AuthModeRequired but no authenticator returns 401
// rather than silently allowing access.
func TestRequireAuth_RequiredMode_NoAuthenticator_FailsClosed(t *testing.T) {
	srv := newTestServer().WithAuthMode(config.AuthModeRequired) // no WithAuthenticator

	called := false
	handler := srv.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	if called {
		t.Error("inner handler must not be called: required mode with no authenticator must fail closed")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

// TestRequireAuth_RequiredMode_RejectsUnauthenticated verifies that requireAuth
// in AuthModeRequired rejects requests with no Authorization header.
func TestRequireAuth_RequiredMode_RejectsUnauthenticated(t *testing.T) {
	srv := newTestServer().
		WithAuthMode(config.AuthModeRequired).
		WithAuthenticator(aliceAuthenticator())

	called := false
	handler := srv.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	if called {
		t.Error("inner handler must not be called without a token in required mode")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

// TestRequireRole_RequiredMode_RejectsNoPrincipal verifies that requireRole
// in AuthModeRequired returns 401 when no principal is in context.
func TestRequireRole_RequiredMode_RejectsNoPrincipal(t *testing.T) {
	srv := newTestServer().
		WithAuthMode(config.AuthModeRequired).
		WithAuthenticator(aliceAuthenticator())

	called := false
	handler := srv.requireRole(identity.RoleAdmin)(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	r := httptest.NewRequest(http.MethodPost, "/test", nil)
	// No principal in context.
	w := httptest.NewRecorder()
	handler(w, r)

	if called {
		t.Error("inner handler must not be called when no principal in required mode")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

// TestRequireRole_WrongRole_Returns403 verifies that a principal present in
// context but lacking the required role receives 403, not 401.
func TestRequireRole_WrongRole_Returns403(t *testing.T) {
	srv := newTestServer().WithAuthMode(config.AuthModeRequired).WithAuthenticator(aliceAuthenticator())

	called := false
	handler := srv.requireRole(identity.RoleAdmin)(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	r := httptest.NewRequest(http.MethodPost, "/test", nil)
	r = withPrincipal(r, &identity.Principal{
		ID:    "user:operator",
		Roles: []string{identity.RoleOperator},
	})
	w := httptest.NewRecorder()
	handler(w, r)

	if called {
		t.Error("inner handler must not be called with wrong role")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", w.Code)
	}
}
