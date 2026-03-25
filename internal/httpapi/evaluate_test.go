package httpapi

import (
	"net/http"
	"testing"

	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/config"
	"github.com/accept-io/midas/internal/identity"
)

// evaluateAuthenticator returns a test authenticator covering the three
// distinct role outcomes for /v1/evaluate: operator (allowed), admin (allowed),
// and reviewer (forbidden).
func evaluateAuthenticator() auth.Authenticator {
	return auth.NewStaticTokenAuthenticator(map[string]*identity.Principal{
		"tok-eval-op":       {ID: "svc:caller", Provider: identity.ProviderStatic, Roles: []string{identity.RoleOperator}},
		"tok-eval-admin":    {ID: "user:admin", Provider: identity.ProviderStatic, Roles: []string{identity.RoleAdmin}},
		"tok-eval-reviewer": {ID: "user:reviewer", Provider: identity.ProviderStatic, Roles: []string{identity.RoleReviewer}},
	})
}

// validEvaluateBody is the minimum valid JSON body for POST /v1/evaluate.
var validEvaluateBody = []byte(`{"surface_id":"surf-test","agent_id":"agent-test","confidence":0.9}`)

// evalSrv returns a server wired with evaluateAuthenticator for auth tests.
// AuthModeRequired is set explicitly so that role and token enforcement are active.
func evalSrv() *Server {
	return NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeRequired).
		WithAuthenticator(evaluateAuthenticator())
}

// ---------------------------------------------------------------------------
// /v1/evaluate — auth enforcement regression tests
//
// These tests verify that requireAuth + requireRole are correctly wired to the
// /v1/evaluate route. They are the regression guard for the class of bug where
// the route is registered without middleware, silently allowing unauthenticated
// callers to reach business logic.
// ---------------------------------------------------------------------------

// TestEvaluate_RequiresAuth_WhenConfigured verifies that POST /v1/evaluate
// rejects a request with no Authorization header when auth is configured.
func TestEvaluate_RequiresAuth_WhenConfigured(t *testing.T) {
	rec := performRequest(t, evalSrv(), http.MethodPost, "/v1/evaluate", validEvaluateBody)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON[map[string]string](t, rec)
	if resp["error"] != "unauthorized" {
		t.Errorf(`want error "unauthorized", got %q`, resp["error"])
	}
}

// TestEvaluate_RejectsInvalidToken verifies that an unrecognised bearer token
// is rejected with 401.
func TestEvaluate_RejectsInvalidToken(t *testing.T) {
	rec := performRequestWithHeaders(t, evalSrv(), http.MethodPost, "/v1/evaluate", validEvaluateBody,
		map[string]string{"Authorization": "Bearer not-a-real-token"})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON[map[string]string](t, rec)
	if resp["error"] != "unauthorized" {
		t.Errorf(`want error "unauthorized", got %q`, resp["error"])
	}
}

// TestEvaluate_RejectsReviewerRole verifies that a valid token with only the
// reviewer role is rejected with 403 — reviewers may not submit evaluations.
func TestEvaluate_RejectsReviewerRole(t *testing.T) {
	rec := performRequestWithHeaders(t, evalSrv(), http.MethodPost, "/v1/evaluate", validEvaluateBody,
		map[string]string{"Authorization": "Bearer tok-eval-reviewer"})

	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON[map[string]string](t, rec)
	if resp["error"] != "forbidden" {
		t.Errorf(`want error "forbidden", got %q`, resp["error"])
	}
}

// TestEvaluate_AllowsOperatorRole verifies that a valid operator-role token is
// accepted and the request reaches business logic.
func TestEvaluate_AllowsOperatorRole(t *testing.T) {
	rec := performRequestWithHeaders(t, evalSrv(), http.MethodPost, "/v1/evaluate", validEvaluateBody,
		map[string]string{"Authorization": "Bearer tok-eval-op"})

	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Fatalf("want request to reach handler (operator role), got %d: %s", rec.Code, rec.Body.String())
	}
	// A business-logic response (e.g. SURFACE_NOT_FOUND) confirms auth passed.
	resp := decodeJSON[map[string]string](t, rec)
	if resp["error"] == "unauthorized" || resp["error"] == "forbidden" {
		t.Errorf("want business response, got auth error %q", resp["error"])
	}
}

// TestEvaluate_AllowsAdminRole verifies that a valid admin-role token is also
// accepted — admins may call evaluate without needing a separate operator token.
func TestEvaluate_AllowsAdminRole(t *testing.T) {
	rec := performRequestWithHeaders(t, evalSrv(), http.MethodPost, "/v1/evaluate", validEvaluateBody,
		map[string]string{"Authorization": "Bearer tok-eval-admin"})

	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Fatalf("want request to reach handler (admin role), got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON[map[string]string](t, rec)
	if resp["error"] == "unauthorized" || resp["error"] == "forbidden" {
		t.Errorf("want business response, got auth error %q", resp["error"])
	}
}

// TestEvaluate_OpenMode_AllowsUnauthenticated verifies that when auth mode is
// open (dev/memory mode), /v1/evaluate is accessible without a bearer token.
func TestEvaluate_OpenMode_AllowsUnauthenticated(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", validEvaluateBody)

	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Errorf("open mode: want request to pass through, got %d: %s", rec.Code, rec.Body.String())
	}
}
