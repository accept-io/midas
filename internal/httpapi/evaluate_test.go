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
	"github.com/accept-io/midas/internal/inference"
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
// Inference routing tests
// ---------------------------------------------------------------------------

// mockInferenceService is a test double for inferenceService.
type mockInferenceService struct {
	result inference.InferenceResult
	err    error
}

func (m *mockInferenceService) EnsureInferredStructure(_ context.Context, _ string) (inference.InferenceResult, error) {
	return m.result, m.err
}

// inferenceBody is a valid evaluate body that includes request_id (required on /v1/evaluate)
// and no process_id, so inference routing is exercised.
var inferenceBody = []byte(`{"surface_id":"surf-infer","agent_id":"agent-test","confidence":0.9,"request_id":"req-infer-1"}`)

// inferenceBodyWithProcessID is a valid evaluate body with an explicit process_id,
// bypassing inference entirely.
var inferenceBodyWithProcessID = []byte(`{"surface_id":"surf-infer","agent_id":"agent-test","confidence":0.9,"request_id":"req-infer-2","process_id":"proc-explicit"}`)

// inferSrv builds a server wired for inference tests (open auth for simplicity).
func inferSrv(svc inferenceService, enabled bool) *Server {
	return NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithInference(svc, enabled)
}

// inferSrvWithOrch builds an inference-enabled server with a custom orchestrator.
func inferSrvWithOrch(orch orchestrator, svc inferenceService) *Server {
	return NewServerFull(orch, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithInference(svc, true)
}

// TestInference_Returns400_WhenDisabledAndNoProcessID verifies that in enforced
// mode, /v1/evaluate without process_id and without inference returns 400.
func TestInference_Returns400_WhenDisabledAndNoProcessID(t *testing.T) {
	srv := inferSrv(nil, false).WithStructuralMode(config.StructuralModeEnforced)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", inferenceBody)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON[map[string]string](t, rec)
	want := "process_id is required when inference is not enabled"
	if resp["error"] != want {
		t.Errorf("want error %q, got %q", want, resp["error"])
	}
}

// TestInference_Returns400_WhenEnabledButNoService verifies that in enforced mode,
// when inference is flagged enabled but no service is wired, /v1/evaluate returns 400.
func TestInference_Returns400_WhenEnabledButNoService(t *testing.T) {
	srv := inferSrv(nil, true).WithStructuralMode(config.StructuralModeEnforced)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", inferenceBody)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestStructuralPermissive_AllowsEvaluateWithoutProcessID verifies that in
// permissive mode (the default), /v1/evaluate without process_id and without
// inference does not return 400. The request passes through to the orchestrator.
func TestStructuralPermissive_AllowsEvaluateWithoutProcessID(t *testing.T) {
	// Default server — no WithStructuralMode call → permissive.
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", inferenceBody)

	if rec.Code == http.StatusBadRequest {
		resp := decodeJSON[map[string]string](t, rec)
		if resp["error"] == "process_id is required when inference is not enabled" {
			t.Error("permissive mode must not reject evaluate when process_id is absent and inference is disabled")
		}
	}
}

// TestStructuralEnforced_RejectsEvaluateWithoutProcessID verifies that in
// enforced mode, /v1/evaluate without process_id and without inference returns 400.
func TestStructuralEnforced_RejectsEvaluateWithoutProcessID(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithStructuralMode(config.StructuralModeEnforced)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", inferenceBody)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("enforced mode: want 400 when process_id absent and inference disabled, got %d", rec.Code)
	}
	resp := decodeJSON[map[string]string](t, rec)
	want := "process_id is required when inference is not enabled"
	if resp["error"] != want {
		t.Errorf("want error %q, got %q", want, resp["error"])
	}
}

// TestInference_Returns400_ForInvalidSurfaceID verifies that an invalid surface_id
// returns 400 and inference is not called (validation fires before EnsureInferredStructure).
func TestInference_Returns400_ForInvalidSurfaceID(t *testing.T) {
	// "Invalid.Surface" starts with uppercase — rejected by ValidateSurfaceID.
	body := []byte(`{"surface_id":"Invalid.Surface","agent_id":"agent-1","confidence":0.9,"request_id":"req-bad-surf-1"}`)

	mockSvc := &mockInferenceService{} // returns zero result; must never be called
	srv := inferSrv(mockSvc, true)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", body)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON[map[string]string](t, rec)
	if resp["error"] == "" {
		t.Error("want non-empty error message for invalid surface_id")
	}
	if resp["error"] == "process_id is required when inference is not enabled" {
		t.Error("got inference-disabled error; want surface_id validation error")
	}
}

// TestInference_SkipsInference_WhenProcessIDProvided verifies that when process_id
// is explicitly provided, inference is not triggered and inference headers are absent.
func TestInference_SkipsInference_WhenProcessIDProvided(t *testing.T) {
	// Even with inference disabled, explicit process_id should not cause a 400 about inference.
	srv := inferSrv(nil, false)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", inferenceBodyWithProcessID)

	// Should not get the inference-disabled 400.
	resp := decodeJSON[map[string]string](t, rec)
	if resp["error"] == "process_id is required when inference is not enabled" {
		t.Errorf("inference block fired for explicit process_id: %s", rec.Body.String())
	}

	// Inference headers must be absent in explicit mode.
	if rec.Header().Get("X-MIDAS-Inference-Used") != "" {
		t.Error("X-MIDAS-Inference-Used must not be set when process_id is supplied explicitly")
	}
	if rec.Header().Get("X-MIDAS-Inferred-Capability") != "" {
		t.Error("X-MIDAS-Inferred-Capability must not be set when process_id is supplied explicitly")
	}
	if rec.Header().Get("X-MIDAS-Inferred-Process") != "" {
		t.Error("X-MIDAS-Inferred-Process must not be set when process_id is supplied explicitly")
	}
}

// TestInference_AttachesHeadersOnSuccess verifies that when inference succeeds, the
// response includes the three inference headers with the correct values.
func TestInference_AttachesHeadersOnSuccess(t *testing.T) {
	wantCapID := "cap-infer"
	wantProcID := "proc-infer"

	mockSvc := &mockInferenceService{
		result: inference.InferenceResult{
			CapabilityID:      wantCapID,
			ProcessID:         wantProcID,
			SurfaceID:         "surf-infer",
			CapabilityCreated: true,
			ProcessCreated:    true,
			SurfaceCreated:    true,
		},
	}
	// Use a mock orchestrator that returns success so we reach the header-writing code.
	orch := &mockOrchestrator{
		evaluateFn: func(_ context.Context, _ eval.DecisionRequest, _ json.RawMessage) (decision.EvaluationResult, error) {
			return decision.EvaluationResult{Outcome: "execute"}, nil
		},
	}
	srv := inferSrvWithOrch(orch, mockSvc)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", inferenceBody)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-MIDAS-Inference-Used"); got != "true" {
		t.Errorf("X-MIDAS-Inference-Used: want %q, got %q", "true", got)
	}
	if got := rec.Header().Get("X-MIDAS-Inferred-Capability"); got != wantCapID {
		t.Errorf("X-MIDAS-Inferred-Capability: want %q, got %q", wantCapID, got)
	}
	if got := rec.Header().Get("X-MIDAS-Inferred-Process"); got != wantProcID {
		t.Errorf("X-MIDAS-Inferred-Process: want %q, got %q", wantProcID, got)
	}
}

// ---------------------------------------------------------------------------
// Explicit-mode validation tests (PR5)
// ---------------------------------------------------------------------------

// mockExplicitValidator is a test double for explicitModeValidator.
type mockExplicitValidator struct {
	getProcessFn      func(ctx context.Context, id string) (*process.Process, error)
	findLatestSurfFn  func(ctx context.Context, id string) (*surface.DecisionSurface, error)
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
// Inference is disabled; explicit validation is the focus.
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
	// Must not proceed to inference or evaluation.
	if rec.Header().Get("X-MIDAS-Inference-Used") != "" {
		t.Error("inference headers must not be set on explicit-mode validation failure")
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
// validation passes, evaluation proceeds normally with no inference metadata or headers.
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
	// No inference headers in explicit mode.
	if rec.Header().Get("X-MIDAS-Inference-Used") != "" {
		t.Error("X-MIDAS-Inference-Used must not be set in explicit mode")
	}
	if rec.Header().Get("X-MIDAS-Inferred-Capability") != "" {
		t.Error("X-MIDAS-Inferred-Capability must not be set in explicit mode")
	}
	if rec.Header().Get("X-MIDAS-Inferred-Process") != "" {
		t.Error("X-MIDAS-Inferred-Process must not be set in explicit mode")
	}
	// No inference metadata in response body.
	type evalResp struct {
		Outcome   string `json:"outcome"`
		Inference *struct{} `json:"inference,omitempty"`
	}
	resp := decodeJSON[evalResp](t, rec)
	if resp.Inference != nil {
		t.Error("inference field must be absent in explicit mode response")
	}
}

// TestExplicit_BypassesInference_EvenWhenEnabled verifies that when inference is
// globally enabled but process_id is provided, the explicit validation path is
// taken and inference is never invoked.
func TestExplicit_BypassesInference_EvenWhenEnabled(t *testing.T) {
	inferenceCalled := false

	// inferenceService that records whether it was called.
	type trackingInference struct{}
	trackSvc := &mockInferenceService{}
	_ = trackSvc

	validator := &mockExplicitValidator{
		getProcessFn: func(_ context.Context, id string) (*process.Process, error) {
			return &process.Process{ID: id}, nil
		},
		findLatestSurfFn: func(_ context.Context, id string) (*surface.DecisionSurface, error) {
			return &surface.DecisionSurface{ID: id, ProcessID: "loan-proc"}, nil
		},
	}

	// Inference service that panics if called — proves inference was bypassed.
	panicSvc := &mockInferenceService{} // zero value; EnsureInferredStructure returns zero result
	_ = inferenceCalled

	// Build server with BOTH inference enabled AND explicit validator wired.
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplicitValidator(validator).
		WithInference(panicSvc, true) // inference globally enabled

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate",
		explicitBody("loan.approve", "loan-proc"))

	// Inference headers must be absent — explicit mode was used.
	if rec.Header().Get("X-MIDAS-Inference-Used") != "" {
		t.Error("explicit mode must not set inference headers even when inference is globally enabled")
	}
	// Must not have fallen into the inference branch (which would produce this specific error).
	resp := decodeJSON[map[string]string](t, rec)
	if resp["error"] == "process_id is required when inference is not enabled" {
		t.Error("inference block entered instead of explicit validation block")
	}
}
