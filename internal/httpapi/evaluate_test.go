package httpapi

import (
	"context"
	"encoding/json"
	"errors"
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

// ---------------------------------------------------------------------------
// D27i-c chunk 2 — class-aware HTTP error mapping
//
// The class-aware arm in mapDomainError fires only at the two evaluate call
// sites (handleEvaluateWith / handleSimulateWith) AND only for errors that
// are categorised wrappers — i.e. errors whose chain exposes a
// Category() decision.FailureCategory method. In production that interface
// is implemented exclusively by *decision.categorizedError (constructed by
// the unexported decision.wrapFailure). chunk2CatErr below is a test-only
// stand-in that satisfies the same structural interface and the standard
// errors.Is/As wrap protocol, without depending on the unexported type.
//
// Limit on category coverage: decision.ClassifyFailure recognises only the
// concrete *decision.categorizedError via its first-priority match. For
// chunk2CatErr the classifier falls through to its string-heuristic fallback,
// so chunk2CatErr.Error() must align with the heuristic to elicit the
// desired category. The chunk-1 heuristic naturally produces only three
// categories: policy_evaluation (when the message contains "policy"),
// authority_resolution (authority/grant/profile/surface/agent), and unknown
// (default). The four wrapper-only categories (envelope_persistence,
// audit_append, invalid_transition, resolve_review) cannot be reproduced
// from the test side without exporting wrapFailure or duplicating the
// chunk-1 mapping table; both are out of scope. The tests below cover what
// can be covered cleanly: the resource→503 pin (test obligation #1), the
// scope-axis negative pins, and the input-class preservation pin.
// ---------------------------------------------------------------------------

// chunk2CatErr is the test-only categorised error type used by the chunk-2
// HTTP-error-mapping tests. It implements the same Error / Unwrap / Category
// surface that *decision.categorizedError exposes in production, so it is
// detected by mapDomainError's structural errors.As probe and, when its
// inner error is a known sentinel, propagates through errors.Is for the
// existing typed-sentinel cases above the class-aware arm.
type chunk2CatErr struct {
	cat   decision.FailureCategory
	inner error
}

func (e *chunk2CatErr) Error() string                      { return e.inner.Error() }
func (e *chunk2CatErr) Unwrap() error                      { return e.inner }
func (e *chunk2CatErr) Category() decision.FailureCategory { return e.cat }

// TestMapDomainError_ClassAware_Resource_Returns503 verifies test obligation
// #1: a wrapped categorised resource failure at an evaluate site returns 503
// with the {error, failure_kind, correctness_class} body. The fake's message
// contains "policy" so decision.ClassifyFailure's heuristic matches and
// returns the same (FailureCategoryPolicyEvaluation, FailureClassResource)
// pair that a real *decision.categorizedError with this category would
// produce on the first-priority match.
func TestMapDomainError_ClassAware_Resource_Returns503(t *testing.T) {
	wrapped := &chunk2CatErr{
		cat:   decision.FailureCategoryPolicyEvaluation,
		inner: errors.New("policy evaluator unavailable"),
	}

	status, body := mapDomainErrorForTest(wrapped, true)

	if status != http.StatusServiceUnavailable {
		t.Errorf("want 503, got %d", status)
	}
	if got := body["error"]; got != "policy evaluator unavailable" {
		t.Errorf(`want error %q, got %q`, "policy evaluator unavailable", got)
	}
	if got := body["failure_kind"]; got != "policy_evaluation" {
		t.Errorf(`want failure_kind %q, got %q`, "policy_evaluation", got)
	}
	if got := body["correctness_class"]; got != "resource" {
		t.Errorf(`want correctness_class %q, got %q`, "resource", got)
	}
	if len(body) != 3 {
		t.Errorf("want exactly 3 body keys, got %d: %v", len(body), body)
	}
}

// TestMapDomainError_ClassAware_StatusByClass covers the heuristic-reproducible
// status-by-class transitions: resource → 503, consistency → 500 (via the
// authority_resolution heuristic), and the unknown-default → 500. The four
// wrapper-only categories (envelope_persistence, audit_append, invalid_transition,
// resolve_review) cannot be reproduced from the test side under the chunk-2
// no-decision-export rule; per the file-level comment, they are deliberately
// out of scope here. Production behaviour for those categories is exercised
// indirectly by the orchestrator's own integration tests.
func TestMapDomainError_ClassAware_StatusByClass(t *testing.T) {
	cases := []struct {
		name       string
		message    string
		category   decision.FailureCategory
		wantStatus int
		wantKind   string
		wantClass  string
	}{
		{
			name:       "policy_message_to_resource_503",
			message:    "policy evaluator unavailable",
			category:   decision.FailureCategoryPolicyEvaluation,
			wantStatus: http.StatusServiceUnavailable,
			wantKind:   "policy_evaluation",
			wantClass:  "resource",
		},
		{
			name:       "authority_message_to_consistency_500",
			message:    "authority resolution failed: grant not found",
			category:   decision.FailureCategoryAuthorityResolution,
			wantStatus: http.StatusInternalServerError,
			wantKind:   "authority_resolution",
			wantClass:  "consistency",
		},
		{
			name:       "unknown_message_to_consistency_500",
			message:    "boom",
			category:   decision.FailureCategoryUnknown,
			wantStatus: http.StatusInternalServerError,
			wantKind:   "unknown",
			wantClass:  "consistency",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := &chunk2CatErr{cat: tc.category, inner: errors.New(tc.message)}
			status, body := mapDomainErrorForTest(wrapped, true)

			if status != tc.wantStatus {
				t.Errorf("status: want %d, got %d", tc.wantStatus, status)
			}
			if got := body["failure_kind"]; got != tc.wantKind {
				t.Errorf("failure_kind: want %q, got %q", tc.wantKind, got)
			}
			if got := body["correctness_class"]; got != tc.wantClass {
				t.Errorf("correctness_class: want %q, got %q", tc.wantClass, got)
			}
			if len(body) != 3 {
				t.Errorf("body keys: want exactly 3, got %d: %v", len(body), body)
			}
		})
	}
}

// TestMapDomainError_RawPolicyError_NotClassAware_At_EvaluateSite verifies the
// chunk-2 rule that the class-aware arm fires only for categorised wrappers.
// A raw error whose message contains "policy" does NOT become 503 even at an
// evaluate site — the heuristic in classifyFailure cannot, on its own, change
// a status code. The error falls through to the existing default 500 with the
// existing {error}-only body.
func TestMapDomainError_RawPolicyError_NotClassAware_At_EvaluateSite(t *testing.T) {
	raw := errors.New("policy evaluator unreachable")

	status, body := mapDomainErrorForTest(raw, true)

	if status != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", status)
	}
	if got := body["error"]; got != "policy evaluator unreachable" {
		t.Errorf(`want error %q, got %q`, "policy evaluator unreachable", got)
	}
	if _, ok := body["failure_kind"]; ok {
		t.Errorf("raw error must not carry failure_kind, got body: %v", body)
	}
	if _, ok := body["correctness_class"]; ok {
		t.Errorf("raw error must not carry correctness_class, got body: %v", body)
	}
	if len(body) != 1 {
		t.Errorf("want exactly 1 body key, got %d: %v", len(body), body)
	}
}

// TestMapDomainError_RawError_AtNonEvaluateSite_NoClassFields verifies that
// read/list callers (classAware=false) keep the existing {error}-only 500 body
// even for raw generic errors. This is the negative-scope assertion: the
// class-aware arm does not fire outside the two evaluate sites.
func TestMapDomainError_RawError_AtNonEvaluateSite_NoClassFields(t *testing.T) {
	raw := errors.New("repository unavailable")

	status, body := mapDomainErrorForTest(raw, false)

	if status != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", status)
	}
	if got := body["error"]; got != "repository unavailable" {
		t.Errorf(`want error %q, got %q`, "repository unavailable", got)
	}
	if _, ok := body["failure_kind"]; ok {
		t.Errorf("non-evaluate site must not carry failure_kind, got body: %v", body)
	}
	if _, ok := body["correctness_class"]; ok {
		t.Errorf("non-evaluate site must not carry correctness_class, got body: %v", body)
	}
	if len(body) != 1 {
		t.Errorf("want exactly 1 body key, got %d: %v", len(body), body)
	}
}

// TestMapDomainError_WrappedError_AtNonEvaluateSite_NoClassFields verifies the
// final corner of the (call-site × error-type) matrix: even a categorised
// wrapper, when surfaced at a non-evaluate caller (classAware=false), keeps
// the existing {error}-only body. The class-aware arm is gated on the call
// site, not on the error type alone.
func TestMapDomainError_WrappedError_AtNonEvaluateSite_NoClassFields(t *testing.T) {
	wrapped := &chunk2CatErr{
		cat:   decision.FailureCategoryAuthorityResolution,
		inner: errors.New("authority resolution failed"),
	}

	status, body := mapDomainErrorForTest(wrapped, false)

	if status != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", status)
	}
	if _, ok := body["failure_kind"]; ok {
		t.Errorf("non-evaluate site must not carry failure_kind, got body: %v", body)
	}
	if _, ok := body["correctness_class"]; ok {
		t.Errorf("non-evaluate site must not carry correctness_class, got body: %v", body)
	}
	if len(body) != 1 {
		t.Errorf("want exactly 1 body key, got %d: %v", len(body), body)
	}
}

// TestMapDomainError_InputClass_StatusAndBodyPreserved verifies that the
// idempotency-conflict path keeps its existing 409 status and {error}-only
// body even when surfaced at an evaluate site. The wrapper carries
// FailureCategoryIdempotencyConflict and unwraps to ErrScopedRequestConflict;
// the typed-sentinel case `errors.Is(err, decision.ErrScopedRequestConflict)`
// fires before the class-aware arm runs, so 409 + {error} is preserved.
func TestMapDomainError_InputClass_StatusAndBodyPreserved(t *testing.T) {
	wrapped := &chunk2CatErr{
		cat:   decision.FailureCategoryIdempotencyConflict,
		inner: decision.ErrScopedRequestConflict,
	}

	status, body := mapDomainErrorForTest(wrapped, true)

	if status != http.StatusConflict {
		t.Errorf("want 409, got %d", status)
	}
	if got := body["error"]; got != decision.ErrScopedRequestConflict.Error() {
		t.Errorf(`want error %q, got %q`, decision.ErrScopedRequestConflict.Error(), got)
	}
	if _, ok := body["failure_kind"]; ok {
		t.Errorf("input-class path must not carry failure_kind, got body: %v", body)
	}
	if _, ok := body["correctness_class"]; ok {
		t.Errorf("input-class path must not carry correctness_class, got body: %v", body)
	}
	if len(body) != 1 {
		t.Errorf("want exactly 1 body key, got %d: %v", len(body), body)
	}
}

// chunk2EvaluateBody is a complete /v1/evaluate body including the required
// request_id. Used by chunk-2 HTTP-level tests that need the request to reach
// the orchestrator (rather than being short-circuited by request-id validation).
var chunk2EvaluateBody = []byte(`{"surface_id":"surf-chunk2","agent_id":"agent-chunk2","confidence":0.9,"request_id":"req-chunk2-1"}`)

// TestEvaluate_SuccessResponse_NoClassFields verifies that a successful
// evaluation through /v1/evaluate returns 200 with no failure_kind or
// correctness_class fields. Chunk 2 must not change success-response shape.
func TestEvaluate_SuccessResponse_NoClassFields(t *testing.T) {
	orch := &mockOrchestrator{
		evaluateFn: func(_ context.Context, _ eval.DecisionRequest, _ json.RawMessage) (decision.EvaluationResult, error) {
			return decision.EvaluationResult{
				Outcome:    "execute",
				ReasonCode: "ALLOW",
				EnvelopeID: "env-success-1",
			}, nil
		},
	}
	srv := NewServerFull(orch, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", chunk2EvaluateBody)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := raw["failure_kind"]; ok {
		t.Errorf("success response must not carry failure_kind, got body: %v", raw)
	}
	if _, ok := raw["correctness_class"]; ok {
		t.Errorf("success response must not carry correctness_class, got body: %v", raw)
	}
}

// TestEvaluate_HTTP_ClassMappedFailure_503 drives a wrapped resource-class
// failure end-to-end through the /v1/evaluate handler and asserts the wire
// status and body. This is the integration-level pin for the unit-level
// TestMapDomainError_ClassAware_Resource_Returns503.
func TestEvaluate_HTTP_ClassMappedFailure_503(t *testing.T) {
	orch := &mockOrchestrator{
		evaluateFn: func(_ context.Context, _ eval.DecisionRequest, _ json.RawMessage) (decision.EvaluationResult, error) {
			return decision.EvaluationResult{}, &chunk2CatErr{
				cat:   decision.FailureCategoryPolicyEvaluation,
				inner: errors.New("policy evaluator unavailable"),
			}
		},
	}
	srv := NewServerFull(orch, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", chunk2EvaluateBody)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON[map[string]string](t, rec)
	if got := body["failure_kind"]; got != "policy_evaluation" {
		t.Errorf(`want failure_kind "policy_evaluation", got %q`, got)
	}
	if got := body["correctness_class"]; got != "resource" {
		t.Errorf(`want correctness_class "resource", got %q`, got)
	}
}

// mapDomainErrorForTest is a thin helper around the package-private
// mapDomainError so the chunk-2 tests above can pin its behaviour without
// going through the full HTTP handler stack on every assertion. Both axes —
// call-site classAware and error-type categorisation — are exercised by
// passing the appropriate flag.
func mapDomainErrorForTest(err error, classAware bool) (int, map[string]string) {
	return mapDomainError(err, entityEvaluation, classAware)
}

// ---------------------------------------------------------------------------
// D27i-c chunk 3 — mandatory audit_status marker on success responses
//
// The marker has different values per route:
//   - /v1/evaluate         → "recorded"
//   - /explorer            → "explorer_recorded"
//   - /explorer/simulate   → field absent entirely (the existing simulated:true
//                            field continues to carry the simulate signal)
//
// Failure responses on any path do not carry audit_status; they use the
// chunk-2 class-aware body or the existing {error}-only shape.
//
// The marker is structurally absent on simulate (rather than omitempty-driven)
// because /explorer/simulate writes evaluateResponse, which has no AuditStatus
// field. /v1/evaluate and /explorer write evaluateSuccessResponse which embeds
// evaluateResponse and adds the field.
// ---------------------------------------------------------------------------

// chunk3SuccessOrch returns a mockOrchestrator whose Evaluate yields a fixed
// successful EvaluationResult. Used by /v1/evaluate chunk-3 tests so they
// don't depend on Explorer seed data.
func chunk3SuccessOrch() *mockOrchestrator {
	return &mockOrchestrator{
		evaluateFn: func(_ context.Context, _ eval.DecisionRequest, _ json.RawMessage) (decision.EvaluationResult, error) {
			return decision.EvaluationResult{
				Outcome:    eval.OutcomeAccept,
				ReasonCode: eval.ReasonWithinAuthority,
				EnvelopeID: "env-chunk3-success-1",
			}, nil
		},
	}
}

// TestEvaluate_SuccessResponse_HasAuditStatusRecorded pins that a successful
// /v1/evaluate response carries audit_status:"recorded". Decoding into
// map[string]any verifies the key is structurally present (the no-omitempty
// rule); a sibling assertion on the literal value confirms the constant.
func TestEvaluate_SuccessResponse_HasAuditStatusRecorded(t *testing.T) {
	srv := NewServerFull(chunk3SuccessOrch(), nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", chunk2EvaluateBody)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	got, ok := raw["audit_status"]
	if !ok {
		t.Fatalf("audit_status key missing on /v1/evaluate success response: %v", raw)
	}
	if got != "recorded" {
		t.Errorf(`audit_status: want "recorded", got %v`, got)
	}
}

// TestExplorerEvaluate_SuccessResponse_HasAuditStatusExplorerRecorded pins
// that a successful /explorer response carries audit_status:"explorer_recorded".
// The Explorer route runs against its isolated in-memory orchestrator (built
// by WithExplorerEnabled), so the request body mirrors the seed shape used by
// TestExplorerEvaluate_UsesIsolatedMemoryStore.
func TestExplorerEvaluate_SuccessResponse_HasAuditStatusExplorerRecorded(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	body := []byte(`{
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "risk_rating", "risk_rating": "low"}
	}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	got, ok := raw["audit_status"]
	if !ok {
		t.Fatalf("audit_status key missing on /explorer success response: %v", raw)
	}
	if got != "explorer_recorded" {
		t.Errorf(`audit_status: want "explorer_recorded", got %v`, got)
	}
}

// TestExplorerSimulate_SuccessResponse_HasNoAuditStatus pins the structural
// absence of audit_status on /explorer/simulate. The existing simulated:true
// field continues to carry the simulate signal. If a future refactor moves
// simulate onto evaluateSuccessResponse this test fails loudly.
func TestExplorerSimulate_SuccessResponse_HasNoAuditStatus(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	body := []byte(`{
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "risk_rating", "risk_rating": "low"}
	}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer/simulate", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := raw["audit_status"]; ok {
		t.Errorf("audit_status must be absent on /explorer/simulate, got %v", raw)
	}
	if simulated, ok := raw["simulated"].(bool); !ok || !simulated {
		t.Errorf(`simulated:true must still be present on /explorer/simulate, got %v`, raw["simulated"])
	}
}

// TestEvaluate_FailureResponse_HasNoAuditStatus pins that a class-aware
// chunk-2 failure body on /v1/evaluate carries no audit_status field. The
// marker is for governed success outcomes only. Failure responses use the
// chunk-2 {error, failure_kind, correctness_class} shape exclusively.
func TestEvaluate_FailureResponse_HasNoAuditStatus(t *testing.T) {
	orch := &mockOrchestrator{
		evaluateFn: func(_ context.Context, _ eval.DecisionRequest, _ json.RawMessage) (decision.EvaluationResult, error) {
			return decision.EvaluationResult{}, &chunk2CatErr{
				cat:   decision.FailureCategoryPolicyEvaluation,
				inner: errors.New("policy evaluator unavailable"),
			}
		},
	}
	srv := NewServerFull(orch, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", chunk2EvaluateBody)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d: %s", rec.Code, rec.Body.String())
	}

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := raw["audit_status"]; ok {
		t.Errorf("audit_status must be absent on failure responses, got %v", raw)
	}
	// Sanity: confirm we are looking at the chunk-2 class-aware body, not a
	// success-shape regression that just happens to omit the marker.
	if raw["failure_kind"] != "policy_evaluation" {
		t.Errorf("failure_kind: want %q, got %v", "policy_evaluation", raw["failure_kind"])
	}
	if raw["correctness_class"] != "resource" {
		t.Errorf("correctness_class: want %q, got %v", "resource", raw["correctness_class"])
	}
}

// TestEvaluate_AuditStatus_PerRouteMarkerDiverges is the cross-path drift
// pin. It defends against a future refactor that consolidates
// handleEvaluateWith's response construction in a way that flattens the
// per-route marker into a single value. /v1/evaluate must emit "recorded";
// /explorer must emit "explorer_recorded"; the values must differ.
func TestEvaluate_AuditStatus_PerRouteMarkerDiverges(t *testing.T) {
	srv := NewServerFull(chunk3SuccessOrch(), nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	evalRec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", chunk2EvaluateBody)
	if evalRec.Code != http.StatusOK {
		t.Fatalf("/v1/evaluate: want 200, got %d: %s", evalRec.Code, evalRec.Body.String())
	}
	var evalBody map[string]any
	if err := json.Unmarshal(evalRec.Body.Bytes(), &evalBody); err != nil {
		t.Fatalf("/v1/evaluate decode: %v", err)
	}

	explorerBody := []byte(`{
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "risk_rating", "risk_rating": "low"}
	}`)
	expRec := performRequest(t, srv, http.MethodPost, "/explorer", explorerBody)
	if expRec.Code != http.StatusOK {
		t.Fatalf("/explorer: want 200, got %d: %s", expRec.Code, expRec.Body.String())
	}
	var expRawBody map[string]any
	if err := json.Unmarshal(expRec.Body.Bytes(), &expRawBody); err != nil {
		t.Fatalf("/explorer decode: %v", err)
	}

	evalMarker := evalBody["audit_status"]
	expMarker := expRawBody["audit_status"]

	if evalMarker != "recorded" {
		t.Errorf(`/v1/evaluate audit_status: want "recorded", got %v`, evalMarker)
	}
	if expMarker != "explorer_recorded" {
		t.Errorf(`/explorer audit_status: want "explorer_recorded", got %v`, expMarker)
	}
	if evalMarker == expMarker {
		t.Errorf("audit_status must diverge per route; both paths emitted %v", evalMarker)
	}
}
