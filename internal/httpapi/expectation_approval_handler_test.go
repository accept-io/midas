package httpapi

// expectation_approval_handler_test.go — HTTP-level tests for #57's
// POST /v1/controlplane/expectations/{id}/approve. Mirrors
// profile_approval_handler_test.go in shape: drives the handler
// through NewServerWithServices with a mockApprovalService that
// produces the desired return values, asserts on response code and
// body shape.
//
// Auth/role enforcement at the middleware layer is tested separately
// in authz_write_test.go (the matrix-style table at the top of that
// file already includes a row for governanceexpectation-approve added
// alongside this file).

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/controlplane/approval"
	"github.com/accept-io/midas/internal/governanceexpectation"
)

// helpers to produce the typed approval errors in test context.
func errExpectationNotFound() error    { return approval.ErrGovernanceExpectationNotFound }
func errExpectationNotInReview() error { return approval.ErrGovernanceExpectationNotInReview }

// makeApprovedExpectation returns a fixture expectation pre-populated to
// the post-approval state. Helper to keep happy-path tests terse.
func makeApprovedExpectation(id string, version int, approvedBy string) *governanceexpectation.GovernanceExpectation {
	now := time.Now().UTC()
	return &governanceexpectation.GovernanceExpectation{
		ID:         id,
		Version:    version,
		Status:     governanceexpectation.ExpectationStatusActive,
		ApprovedBy: approvedBy,
		ApprovedAt: &now,
	}
}

// ---------------------------------------------------------------------------
// H1 — valid request returns 200.
// H2 — response body shape matches approveExpectationResponse.
// ---------------------------------------------------------------------------

func TestApproveExpectationHandler_ValidRequest_Returns200(t *testing.T) {
	mock := &mockApprovalService{
		approveGovernanceExpectationFn: func(_ context.Context, expectationID string, version int, approvedBy string) (*governanceexpectation.GovernanceExpectation, error) {
			return makeApprovedExpectation(expectationID, version, approvedBy), nil
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	body, _ := json.Marshal(map[string]any{"version": 1, "approved_by": "operator"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/expectations/expect-1/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp approveExpectationResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ExpectationID != "expect-1" {
		t.Errorf("ExpectationID: got %q, want expect-1", resp.ExpectationID)
	}
	if resp.Version != 1 {
		t.Errorf("Version: got %d, want 1", resp.Version)
	}
	if resp.Status != "active" {
		t.Errorf("Status: got %q, want active", resp.Status)
	}
	if resp.ApprovedBy != "operator" {
		t.Errorf("ApprovedBy: got %q, want operator", resp.ApprovedBy)
	}
}

// ---------------------------------------------------------------------------
// H3 — version < 1 returns 400.
// H4 — version 0 (omitted in body) returns 400.
// ---------------------------------------------------------------------------

func TestApproveExpectationHandler_MissingVersion_Returns400(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	body, _ := json.Marshal(map[string]any{"approved_by": "operator"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/expectations/expect-1/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestApproveExpectationHandler_NegativeVersion_Returns400(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	body, _ := json.Marshal(map[string]any{"version": -3, "approved_by": "operator"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/expectations/expect-1/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// H5 — malformed JSON returns 400.
// ---------------------------------------------------------------------------

func TestApproveExpectationHandler_MalformedJSON_Returns400(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/expectations/expect-1/approve",
		bytes.NewReader([]byte(`{not even json`)))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// H6 — unknown fields in the request body are rejected with 400 by
// strict-decode.
// ---------------------------------------------------------------------------

func TestApproveExpectationHandler_UnknownField_Returns400(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	body, _ := json.Marshal(map[string]any{
		"version":     1,
		"approved_by": "operator",
		"unknown_key": "should-be-rejected",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/expectations/expect-1/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// H7 — unknown expectation returns 404.
// ---------------------------------------------------------------------------

func TestApproveExpectationHandler_UnknownExpectation_Returns404(t *testing.T) {
	mock := &mockApprovalService{
		approveGovernanceExpectationFn: func(_ context.Context, _ string, _ int, _ string) (*governanceexpectation.GovernanceExpectation, error) {
			return nil, errExpectationNotFound()
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	body, _ := json.Marshal(map[string]any{"version": 1, "approved_by": "operator"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/expectations/expect-x/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// H8 — non-review state returns 409.
// ---------------------------------------------------------------------------

func TestApproveExpectationHandler_NotInReview_Returns409(t *testing.T) {
	mock := &mockApprovalService{
		approveGovernanceExpectationFn: func(_ context.Context, _ string, _ int, _ string) (*governanceexpectation.GovernanceExpectation, error) {
			return nil, errExpectationNotInReview()
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	body, _ := json.Marshal(map[string]any{"version": 1, "approved_by": "operator"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/expectations/expect-1/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// H9 — approval service nil → 501 (parity with profile/surface).
// ---------------------------------------------------------------------------

func TestApproveExpectationHandler_NilService_Returns501(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, nil)
	body, _ := json.Marshal(map[string]any{"version": 1, "approved_by": "operator"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/expectations/expect-1/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// H10 — wrong method returns 405.
// ---------------------------------------------------------------------------

func TestApproveExpectationHandler_MethodNotAllowed(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	req := httptest.NewRequest(http.MethodGet, "/v1/controlplane/expectations/expect-1/approve", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// H11 — missing approved_by returns 400 (after trimming and
// actorFromContext fallback).
// ---------------------------------------------------------------------------

func TestApproveExpectationHandler_MissingApprovedBy_Returns400(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	body, _ := json.Marshal(map[string]any{"version": 1, "approved_by": ""})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/expectations/expect-1/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// H12 — unknown action under the dispatcher returns 404 (forward
// compatibility for the deferred 'deprecate' action).
// ---------------------------------------------------------------------------

func TestApproveExpectationHandler_UnknownAction_Returns404(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	body, _ := json.Marshal(map[string]any{"version": 1})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/expectations/expect-1/deprecate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unsupported action, got %d", w.Code)
	}
}

// (H13 case omitted: the dispatcher's isValidIdentifier check is
// intentionally permissive — Profile/Surface use the same helper and
// don't cover invalid-id-format at this layer either. Path-format
// constraints are enforced upstream at parser/validate time.)
