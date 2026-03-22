package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlplane/approval"
)

// ---------------------------------------------------------------------------
// Mock grant lifecycle service
// ---------------------------------------------------------------------------

type mockGrantLifecycleService struct {
	suspendGrantFn   func(ctx context.Context, grantID, suspendedBy, reason string) (*authority.AuthorityGrant, error)
	revokeGrantFn    func(ctx context.Context, grantID, revokedBy, reason string) (*authority.AuthorityGrant, error)
	reinstateGrantFn func(ctx context.Context, grantID, reinstatedBy string) (*authority.AuthorityGrant, error)
}

func (m *mockGrantLifecycleService) SuspendGrant(ctx context.Context, grantID, suspendedBy, reason string) (*authority.AuthorityGrant, error) {
	if m.suspendGrantFn != nil {
		return m.suspendGrantFn(ctx, grantID, suspendedBy, reason)
	}
	return nil, errors.New("suspendGrant not implemented")
}

func (m *mockGrantLifecycleService) RevokeGrant(ctx context.Context, grantID, revokedBy, reason string) (*authority.AuthorityGrant, error) {
	if m.revokeGrantFn != nil {
		return m.revokeGrantFn(ctx, grantID, revokedBy, reason)
	}
	return nil, errors.New("revokeGrant not implemented")
}

func (m *mockGrantLifecycleService) ReinstateGrant(ctx context.Context, grantID, reinstatedBy string) (*authority.AuthorityGrant, error) {
	if m.reinstateGrantFn != nil {
		return m.reinstateGrantFn(ctx, grantID, reinstatedBy)
	}
	return nil, errors.New("reinstateGrant not implemented")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func grantServer(grantSvc grantLifecycleService) *Server {
	return NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, grantSvc)
}

func postGrant(t *testing.T, srv *Server, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func decodeGrantResponse(t *testing.T, rec *httptest.ResponseRecorder) grantLifecycleResponse {
	t.Helper()
	var resp grantLifecycleResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode grant response: %v", err)
	}
	return resp
}

// ---------------------------------------------------------------------------
// Suspend
// ---------------------------------------------------------------------------

func TestSuspendGrant_Success(t *testing.T) {
	svc := &mockGrantLifecycleService{
		suspendGrantFn: func(_ context.Context, grantID, suspendedBy, reason string) (*authority.AuthorityGrant, error) {
			return &authority.AuthorityGrant{
				ID:      grantID,
				AgentID: "agent-1",
				Status:  authority.GrantStatusSuspended,
			}, nil
		},
	}
	srv := grantServer(svc)

	rec := postGrant(t, srv, "/v1/controlplane/grants/g1/suspend",
		`{"suspended_by":"ops-user","reason":"investigation"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeGrantResponse(t, rec)
	if resp.Status != "suspended" {
		t.Errorf("expected status suspended, got %s", resp.Status)
	}
}

func TestSuspendGrant_NotFound_Returns404(t *testing.T) {
	svc := &mockGrantLifecycleService{
		suspendGrantFn: func(_ context.Context, _, _, _ string) (*authority.AuthorityGrant, error) {
			return nil, approval.ErrGrantNotFound
		},
	}
	srv := grantServer(svc)

	rec := postGrant(t, srv, "/v1/controlplane/grants/g1/suspend",
		`{"suspended_by":"ops-user","reason":"test"}`)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestSuspendGrant_NotActive_Returns409(t *testing.T) {
	svc := &mockGrantLifecycleService{
		suspendGrantFn: func(_ context.Context, _, _, _ string) (*authority.AuthorityGrant, error) {
			return nil, approval.ErrGrantNotActive
		},
	}
	srv := grantServer(svc)

	rec := postGrant(t, srv, "/v1/controlplane/grants/g1/suspend",
		`{"suspended_by":"ops-user","reason":"test"}`)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
}

func TestSuspendGrant_NilService_Returns501(t *testing.T) {
	srv := grantServer(nil)

	rec := postGrant(t, srv, "/v1/controlplane/grants/g1/suspend",
		`{"suspended_by":"ops-user","reason":"test"}`)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rec.Code)
	}
}

func TestSuspendGrant_MethodNotAllowed(t *testing.T) {
	srv := grantServer(nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/controlplane/grants/g1/suspend", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestSuspendGrant_MissingActor_Returns400(t *testing.T) {
	svc := &mockGrantLifecycleService{}
	srv := grantServer(svc)

	rec := postGrant(t, srv, "/v1/controlplane/grants/g1/suspend",
		`{"suspended_by":"","reason":"test"}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Revoke
// ---------------------------------------------------------------------------

func TestRevokeGrant_Success(t *testing.T) {
	svc := &mockGrantLifecycleService{
		revokeGrantFn: func(_ context.Context, grantID, _, _ string) (*authority.AuthorityGrant, error) {
			return &authority.AuthorityGrant{
				ID:      grantID,
				AgentID: "agent-1",
				Status:  authority.GrantStatusRevoked,
			}, nil
		},
	}
	srv := grantServer(svc)

	rec := postGrant(t, srv, "/v1/controlplane/grants/g1/revoke",
		`{"revoked_by":"admin-1","reason":"policy violation"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeGrantResponse(t, rec)
	if resp.Status != "revoked" {
		t.Errorf("expected status revoked, got %s", resp.Status)
	}
}

func TestRevokeGrant_NotFound_Returns404(t *testing.T) {
	svc := &mockGrantLifecycleService{
		revokeGrantFn: func(_ context.Context, _, _, _ string) (*authority.AuthorityGrant, error) {
			return nil, approval.ErrGrantNotFound
		},
	}
	srv := grantServer(svc)

	rec := postGrant(t, srv, "/v1/controlplane/grants/g1/revoke",
		`{"revoked_by":"admin-1","reason":"test"}`)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestRevokeGrant_NilService_Returns501(t *testing.T) {
	srv := grantServer(nil)

	rec := postGrant(t, srv, "/v1/controlplane/grants/g1/revoke",
		`{"revoked_by":"admin-1","reason":"test"}`)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Reinstate
// ---------------------------------------------------------------------------

func TestReinstateGrant_Success(t *testing.T) {
	svc := &mockGrantLifecycleService{
		reinstateGrantFn: func(_ context.Context, grantID, _ string) (*authority.AuthorityGrant, error) {
			return &authority.AuthorityGrant{
				ID:      grantID,
				AgentID: "agent-1",
				Status:  authority.GrantStatusActive,
			}, nil
		},
	}
	srv := grantServer(svc)

	rec := postGrant(t, srv, "/v1/controlplane/grants/g1/reinstate",
		`{"reinstated_by":"admin-1"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeGrantResponse(t, rec)
	if resp.Status != "active" {
		t.Errorf("expected status active, got %s", resp.Status)
	}
}

func TestReinstateGrant_NotSuspended_Returns409(t *testing.T) {
	svc := &mockGrantLifecycleService{
		reinstateGrantFn: func(_ context.Context, _, _ string) (*authority.AuthorityGrant, error) {
			return nil, approval.ErrGrantNotSuspended
		},
	}
	srv := grantServer(svc)

	rec := postGrant(t, srv, "/v1/controlplane/grants/g1/reinstate",
		`{"reinstated_by":"admin-1"}`)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
}

func TestReinstateGrant_NilService_Returns501(t *testing.T) {
	srv := grantServer(nil)

	rec := postGrant(t, srv, "/v1/controlplane/grants/g1/reinstate",
		`{"reinstated_by":"admin-1"}`)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rec.Code)
	}
}
