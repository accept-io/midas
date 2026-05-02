package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/config"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

// evaluateAuthenticator returns a test authenticator covering the three
// distinct role outcomes for /v1/evaluate: operator (allowed), admin (allowed),
// and reviewer (forbidden).
func evaluateAuthenticator() auth.Authenticator {
	return auth.NewStaticTokenAuthenticator(map[string]*identity.Principal{
		"tok-eval-op":       {ID: "svc:caller", Provider: identity.ProviderStatic, Roles: []string{identity.RolePlatformOperator}},
		"tok-eval-admin":    {ID: "user:admin", Provider: identity.ProviderStatic, Roles: []string{identity.RolePlatformAdmin}},
		"tok-eval-reviewer": {ID: "user:reviewer", Provider: identity.ProviderStatic, Roles: []string{identity.RoleGovernanceReviewer}},
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

// ---------------------------------------------------------------------------
// Structural-mode routing tests
// ---------------------------------------------------------------------------

// noProcessIDBody is a valid evaluate body that includes request_id (required
// on /v1/evaluate) and omits process_id, used by structural-mode tests.
var noProcessIDBody = []byte(`{"surface_id":"surf-no-proc","agent_id":"agent-test","confidence":0.9,"request_id":"req-no-proc-1"}`)

// TestStructuralPermissive_AllowsEvaluateWithoutProcessID verifies that in
// permissive mode (the default), /v1/evaluate without process_id does not
// return 400. The request passes through to the orchestrator.
func TestStructuralPermissive_AllowsEvaluateWithoutProcessID(t *testing.T) {
	// Default server — no WithStructuralMode call → permissive.
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", noProcessIDBody)

	if rec.Code == http.StatusBadRequest {
		resp := decodeJSON[map[string]string](t, rec)
		if resp["error"] == "process_id is required" {
			t.Error("permissive mode must not reject evaluate when process_id is absent")
		}
	}
}

// TestStructuralEnforced_RejectsEvaluateWithoutProcessID verifies that in
// enforced mode, /v1/evaluate without process_id returns 400.
func TestStructuralEnforced_RejectsEvaluateWithoutProcessID(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithStructuralMode(config.StructuralModeEnforced)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", noProcessIDBody)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("enforced mode: want 400 when process_id absent, got %d", rec.Code)
	}
	resp := decodeJSON[map[string]string](t, rec)
	want := "process_id is required"
	if resp["error"] != want {
		t.Errorf("want error %q, got %q", want, resp["error"])
	}
}

// ---------------------------------------------------------------------------
// Explicit-mode validation tests (PR5)
// ---------------------------------------------------------------------------

// mockExplicitValidator is a test double for explicitModeValidator.
type mockExplicitValidator struct {
	getProcessFn     func(ctx context.Context, id string) (*process.Process, error)
	findLatestSurfFn func(ctx context.Context, id string) (*surface.DecisionSurface, error)
}

func (m *mockExplicitValidator) GetProcess(ctx context.Context, id string) (*process.Process, error) {
	if m.getProcessFn != nil {
		return m.getProcessFn(ctx, id)
	}
	return nil, nil
}

func (m *mockExplicitValidator) FindLatestSurface(ctx context.Context, id string) (*surface.DecisionSurface, error) {
	if m.findLatestSurfFn != nil {
		return m.findLatestSurfFn(ctx, id)
	}
	return nil, nil
}

// explicitSrv builds a server wired for explicit-mode validation tests.
func explicitSrv(validator explicitModeValidator) *Server {
	return NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplicitValidator(validator)
}

// explicitBody constructs a JSON evaluate body with an explicit process_id and request_id.
func explicitBody(surfaceID, processID string) []byte {
	return []byte(`{"surface_id":"` + surfaceID + `","process_id":"` + processID + `","agent_id":"agent-1","confidence":0.9,"request_id":"req-explicit-1"}`)
}

// TestExplicit_Returns400_WhenProcessNotFound verifies that an explicit request
// with a process_id that does not exist returns 400 before evaluation proceeds.
func TestExplicit_Returns400_WhenProcessNotFound(t *testing.T) {
	validator := &mockExplicitValidator{
		getProcessFn: func(_ context.Context, _ string) (*process.Process, error) {
			return nil, nil // not found
		},
	}
	rec := performRequest(t, explicitSrv(validator), http.MethodPost, "/v1/evaluate",
		explicitBody("loan.approve", "nonexistent-proc"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON[map[string]string](t, rec)
	if resp["error"] == "" {
		t.Error("want non-empty error for missing process")
	}
}

// TestExplicit_Returns400_WhenSurfaceNotFound verifies that an explicit request
// where the surface does not exist returns 400.
func TestExplicit_Returns400_WhenSurfaceNotFound(t *testing.T) {
	validator := &mockExplicitValidator{
		getProcessFn: func(_ context.Context, id string) (*process.Process, error) {
			return &process.Process{ID: id}, nil // process exists
		},
		findLatestSurfFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, nil // surface not found
		},
	}
	rec := performRequest(t, explicitSrv(validator), http.MethodPost, "/v1/evaluate",
		explicitBody("nonexistent.surface", "loan-proc"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON[map[string]string](t, rec)
	if resp["error"] == "" {
		t.Error("want non-empty error for missing surface")
	}
}

// TestExplicit_Returns400_WhenSurfaceProcessMismatch verifies that when the surface
// exists but belongs to a different process, the request returns 400.
func TestExplicit_Returns400_WhenSurfaceProcessMismatch(t *testing.T) {
	validator := &mockExplicitValidator{
		getProcessFn: func(_ context.Context, id string) (*process.Process, error) {
			return &process.Process{ID: id}, nil // both processes "exist"
		},
		findLatestSurfFn: func(_ context.Context, id string) (*surface.DecisionSurface, error) {
			return &surface.DecisionSurface{ID: id, ProcessID: "loan-proc"}, nil // linked to loan-proc
		},
	}
	rec := performRequest(t, explicitSrv(validator), http.MethodPost, "/v1/evaluate",
		explicitBody("loan.approve", "claims-proc")) // mismatch: surface belongs to loan-proc

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON[map[string]string](t, rec)
	if resp["error"] == "" {
		t.Error("want non-empty error for surface/process mismatch")
	}
}

// TestExplicit_ProceedsToEvaluation_WhenStructureValid verifies that when explicit
// validation passes, evaluation proceeds normally and returns 200.
func TestExplicit_ProceedsToEvaluation_WhenStructureValid(t *testing.T) {
	validator := &mockExplicitValidator{
		getProcessFn: func(_ context.Context, id string) (*process.Process, error) {
			return &process.Process{ID: id}, nil
		},
		findLatestSurfFn: func(_ context.Context, id string) (*surface.DecisionSurface, error) {
			return &surface.DecisionSurface{ID: id, ProcessID: "loan-proc"}, nil
		},
	}
	orch := &mockOrchestrator{
		evaluateFn: func(_ context.Context, _ eval.DecisionRequest, _ json.RawMessage) (decision.EvaluationResult, error) {
			return decision.EvaluationResult{Outcome: "execute"}, nil
		},
	}
	srv := NewServerFull(orch, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplicitValidator(validator)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate",
		explicitBody("loan.approve", "loan-proc"))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
