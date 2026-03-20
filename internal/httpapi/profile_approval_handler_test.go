package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlplane/approval"
)

func TestApproveProfile_Success(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockApprovalService{
		approveProfileFn: func(_ context.Context, profileID string, version int, approvedBy string) (*authority.AuthorityProfile, error) {
			return &authority.AuthorityProfile{
				ID:         profileID,
				Version:    version,
				Status:     authority.ProfileStatusActive,
				ApprovedBy: approvedBy,
				ApprovedAt: &now,
			}, nil
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	body, _ := json.Marshal(map[string]any{"version": 1, "approved_by": "operator"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/profiles/prof-1/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp approveProfileResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ProfileID != "prof-1" || resp.Version != 1 || resp.Status != "active" || resp.ApprovedBy != "operator" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestApproveProfile_NotFound_Returns404(t *testing.T) {
	mock := &mockApprovalService{
		approveProfileFn: func(_ context.Context, _ string, _ int, _ string) (*authority.AuthorityProfile, error) {
			return nil, errProfileNotFound()
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	body, _ := json.Marshal(map[string]any{"version": 1, "approved_by": "operator"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/profiles/prof-x/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestApproveProfile_WrongStatus_Returns409(t *testing.T) {
	mock := &mockApprovalService{
		approveProfileFn: func(_ context.Context, _ string, _ int, _ string) (*authority.AuthorityProfile, error) {
			return nil, errProfileNotInReview()
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	body, _ := json.Marshal(map[string]any{"version": 1, "approved_by": "operator"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/profiles/prof-1/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestApproveProfile_NilService_Returns501(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, nil)
	body, _ := json.Marshal(map[string]any{"version": 1, "approved_by": "operator"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/profiles/prof-1/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", w.Code)
	}
}

func TestApproveProfile_MethodNotAllowed(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	req := httptest.NewRequest(http.MethodGet, "/v1/controlplane/profiles/prof-1/approve", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestApproveProfile_MissingApprovedBy_Returns400(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	body, _ := json.Marshal(map[string]any{"version": 1, "approved_by": ""})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/profiles/prof-1/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeprecateProfile_Success(t *testing.T) {
	mock := &mockApprovalService{
		deprecateProfileFn: func(_ context.Context, profileID string, version int, _ string) (*authority.AuthorityProfile, error) {
			return &authority.AuthorityProfile{
				ID:      profileID,
				Version: version,
				Status:  authority.ProfileStatusDeprecated,
			}, nil
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	body, _ := json.Marshal(map[string]any{"version": 1, "deprecated_by": "operator"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/profiles/prof-1/deprecate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp deprecateProfileResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "deprecated" {
		t.Errorf("expected status deprecated, got %s", resp.Status)
	}
}

func TestDeprecateProfile_WrongStatus_Returns409(t *testing.T) {
	mock := &mockApprovalService{
		deprecateProfileFn: func(_ context.Context, _ string, _ int, _ string) (*authority.AuthorityProfile, error) {
			return nil, errProfileNotActive()
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	body, _ := json.Marshal(map[string]any{"version": 1, "deprecated_by": "operator"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/profiles/prof-1/deprecate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeprecateProfile_NilService_Returns501(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, nil)
	body, _ := json.Marshal(map[string]any{"version": 1, "deprecated_by": "operator"})
	req := httptest.NewRequest(http.MethodPost, "/v1/controlplane/profiles/prof-1/deprecate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", w.Code)
	}
}

func TestDeprecateProfile_MethodNotAllowed(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	req := httptest.NewRequest(http.MethodGet, "/v1/controlplane/profiles/prof-1/deprecate", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// helpers to produce the typed approval errors in test context
func errProfileNotFound() error    { return approval.ErrProfileNotFound }
func errProfileNotInReview() error { return approval.ErrProfileNotInReview }
func errProfileNotActive() error   { return approval.ErrProfileNotActive }
