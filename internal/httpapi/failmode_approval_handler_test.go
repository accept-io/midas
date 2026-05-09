package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/controlplane/approval"
	"github.com/accept-io/midas/internal/failmode"
)

// ---------------------------------------------------------------------------
// FailModePolicy approval handler tests (D27j-impl-1c)
// ---------------------------------------------------------------------------

func TestApproveFailModePolicy_Success(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockApprovalService{
		approveFailModePolicyFn: func(_ context.Context, policyID string, version int, approvedBy string) (*failmode.FailModePolicy, error) {
			return &failmode.FailModePolicy{
				ID:         policyID,
				Version:    version,
				Status:     failmode.FailModePolicyStatusActive,
				ApprovedBy: approvedBy,
				ApprovedAt: &now,
			}, nil
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	body, _ := json.Marshal(map[string]any{"version": 1, "approved_by": "operator"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/fail_mode_policies/default-fmp/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp approveFailModePolicyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.PolicyID != "default-fmp" || resp.Version != 1 || resp.Status != "active" || resp.ApprovedBy != "operator" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestApproveFailModePolicy_NotFound_Returns404(t *testing.T) {
	mock := &mockApprovalService{
		approveFailModePolicyFn: func(_ context.Context, _ string, _ int, _ string) (*failmode.FailModePolicy, error) {
			return nil, approval.ErrFailModePolicyNotFound
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	body, _ := json.Marshal(map[string]any{"version": 1, "approved_by": "operator"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/fail_mode_policies/missing/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestApproveFailModePolicy_WrongStatus_Returns409(t *testing.T) {
	mock := &mockApprovalService{
		approveFailModePolicyFn: func(_ context.Context, _ string, _ int, _ string) (*failmode.FailModePolicy, error) {
			return nil, approval.ErrFailModePolicyNotInReview
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	body, _ := json.Marshal(map[string]any{"version": 1, "approved_by": "operator"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/fail_mode_policies/default-fmp/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestApproveFailModePolicy_BadRequestBody_Returns400(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/fail_mode_policies/default-fmp/approve",
		bytes.NewReader([]byte(`{"unknown_field":"x"}`)))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field (strict-decode), got %d: %s", w.Code, w.Body.String())
	}
}

func TestApproveFailModePolicy_MissingVersion_Returns400(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	body, _ := json.Marshal(map[string]any{"version": 0, "approved_by": "operator"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/fail_mode_policies/default-fmp/approve",
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for version<1, got %d: %s", w.Code, w.Body.String())
	}
}

func TestApproveFailModePolicy_MissingApprovedBy_Returns400(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	body, _ := json.Marshal(map[string]any{"version": 1, "approved_by": ""})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/fail_mode_policies/default-fmp/approve",
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty approved_by, got %d: %s", w.Code, w.Body.String())
	}
}

func TestApproveFailModePolicy_GET_Returns405(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	req := httptest.NewRequest(http.MethodGet, "/v1/controlplane/fail_mode_policies/default-fmp/approve", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET, got %d", w.Code)
	}
}

func TestFailModePolicy_UnknownAction_Returns404(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	body, _ := json.Marshal(map[string]any{"version": 1})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/fail_mode_policies/default-fmp/retire",
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown action, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Deprecation handler tests
// ---------------------------------------------------------------------------

func TestDeprecateFailModePolicy_Success(t *testing.T) {
	mock := &mockApprovalService{
		deprecateFailModePolicyFn: func(_ context.Context, policyID string, version int, deprecatedBy string, reason string) (*failmode.FailModePolicy, error) {
			if reason != "Superseded" {
				t.Errorf("reason: want %q, got %q", "Superseded", reason)
			}
			if deprecatedBy != "operator" {
				t.Errorf("deprecatedBy: want %q, got %q", "operator", deprecatedBy)
			}
			return &failmode.FailModePolicy{
				ID:      policyID,
				Version: version,
				Status:  failmode.FailModePolicyStatusDeprecated,
			}, nil
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	body, _ := json.Marshal(map[string]any{"version": 1, "deprecated_by": "operator", "reason": "Superseded"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/fail_mode_policies/default-fmp/deprecate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp deprecateFailModePolicyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.PolicyID != "default-fmp" || resp.Version != 1 || resp.Status != "deprecated" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestDeprecateFailModePolicy_NotFound_Returns404(t *testing.T) {
	mock := &mockApprovalService{
		deprecateFailModePolicyFn: func(_ context.Context, _ string, _ int, _ string, _ string) (*failmode.FailModePolicy, error) {
			return nil, approval.ErrFailModePolicyNotFound
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	body, _ := json.Marshal(map[string]any{"version": 1, "deprecated_by": "operator", "reason": "x"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/fail_mode_policies/missing/deprecate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeprecateFailModePolicy_WrongStatus_Returns409(t *testing.T) {
	mock := &mockApprovalService{
		deprecateFailModePolicyFn: func(_ context.Context, _ string, _ int, _ string, _ string) (*failmode.FailModePolicy, error) {
			return nil, approval.ErrFailModePolicyNotActive
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	body, _ := json.Marshal(map[string]any{"version": 1, "deprecated_by": "operator", "reason": "x"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/fail_mode_policies/default-fmp/deprecate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeprecateFailModePolicy_BadRequestBody_Returns400(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/fail_mode_policies/default-fmp/deprecate",
		bytes.NewReader([]byte(`{"unknown_field":"x"}`)))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeprecateFailModePolicy_EmptyReasonAccepted(t *testing.T) {
	mock := &mockApprovalService{
		deprecateFailModePolicyFn: func(_ context.Context, policyID string, version int, deprecatedBy string, reason string) (*failmode.FailModePolicy, error) {
			if reason != "" {
				t.Errorf("reason: want empty, got %q", reason)
			}
			return &failmode.FailModePolicy{
				ID:      policyID,
				Version: version,
				Status:  failmode.FailModePolicyStatusDeprecated,
			}, nil
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	body, _ := json.Marshal(map[string]any{"version": 1, "deprecated_by": "operator", "reason": ""})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/fail_mode_policies/default-fmp/deprecate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with empty reason, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeprecateFailModePolicy_GET_Returns405(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	req := httptest.NewRequest(http.MethodGet, "/v1/controlplane/fail_mode_policies/default-fmp/deprecate", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET, got %d", w.Code)
	}
}
