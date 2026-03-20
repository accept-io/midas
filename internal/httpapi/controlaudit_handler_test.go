package httpapi

// controlaudit_handler_test.go — tests for GET /v1/controlplane/audit.
// Lives in the httpapi package (white-box) so it can reuse package-private
// helpers and wire format types directly.

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/controlaudit"
)

// ---------------------------------------------------------------------------
// Mock controlAuditService
// ---------------------------------------------------------------------------

type mockControlAuditService struct {
	listAuditFn func(ctx context.Context, f controlaudit.ListFilter) ([]*controlaudit.ControlAuditRecord, error)
}

func (m *mockControlAuditService) ListAudit(ctx context.Context, f controlaudit.ListFilter) ([]*controlaudit.ControlAuditRecord, error) {
	if m.listAuditFn != nil {
		return m.listAuditFn(ctx, f)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func makeAuditRecord(actor, kind, id string, action controlaudit.Action) *controlaudit.ControlAuditRecord {
	return &controlaudit.ControlAuditRecord{
		ID:           "rec-" + id,
		OccurredAt:   time.Now().UTC(),
		Actor:        actor,
		Action:       action,
		ResourceKind: kind,
		ResourceID:   id,
		Summary:      fmt.Sprintf("%s %s %s", actor, action, id),
	}
}

func newAuditServer(svc controlAuditService) *Server {
	return NewServerFull(&mockOrchestrator{}, nil, nil, nil, svc)
}

// ---------------------------------------------------------------------------
// GET /v1/controlplane/audit
// ---------------------------------------------------------------------------

func TestListControlAudit_Success(t *testing.T) {
	rec1 := makeAuditRecord("alice", controlaudit.ResourceKindSurface, "surf-1", controlaudit.ActionSurfaceCreated)
	rec2 := makeAuditRecord("bob", controlaudit.ResourceKindAgent, "agent-1", controlaudit.ActionAgentCreated)

	svc := &mockControlAuditService{
		listAuditFn: func(_ context.Context, _ controlaudit.ListFilter) ([]*controlaudit.ControlAuditRecord, error) {
			return []*controlaudit.ControlAuditRecord{rec1, rec2}, nil
		},
	}

	srv := newAuditServer(svc)
	resp := performRequest(t, srv, http.MethodGet, "/v1/controlplane/audit", nil)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	result := decodeJSON[controlAuditListResponse](t, resp)
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result.Entries))
	}
	if result.Entries[0].Actor != "alice" {
		t.Errorf("entries[0].actor: want alice, got %q", result.Entries[0].Actor)
	}
	if result.Entries[1].Actor != "bob" {
		t.Errorf("entries[1].actor: want bob, got %q", result.Entries[1].Actor)
	}
}

func TestListControlAudit_EmptyResult(t *testing.T) {
	svc := &mockControlAuditService{
		listAuditFn: func(_ context.Context, _ controlaudit.ListFilter) ([]*controlaudit.ControlAuditRecord, error) {
			return nil, nil
		},
	}

	srv := newAuditServer(svc)
	resp := performRequest(t, srv, http.MethodGet, "/v1/controlplane/audit", nil)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	result := decodeJSON[controlAuditListResponse](t, resp)
	if len(result.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(result.Entries))
	}
}

func TestListControlAudit_NilService_Returns501(t *testing.T) {
	srv := newAuditServer(nil)
	resp := performRequest(t, srv, http.MethodGet, "/v1/controlplane/audit", nil)

	if resp.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", resp.Code)
	}
}

func TestListControlAudit_MethodNotAllowed(t *testing.T) {
	svc := &mockControlAuditService{}
	srv := newAuditServer(svc)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		resp := performRequest(t, srv, method, "/v1/controlplane/audit", nil)
		if resp.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: expected 405, got %d", method, resp.Code)
		}
	}
}

func TestListControlAudit_InvalidLimit_Returns400(t *testing.T) {
	svc := &mockControlAuditService{}
	srv := newAuditServer(svc)

	for _, bad := range []string{"abc", "-1", "0", "3.5", ""} {
		if bad == "" {
			continue // empty limit is valid (uses default)
		}
		resp := performRequest(t, srv, http.MethodGet, "/v1/controlplane/audit?limit="+bad, nil)
		if resp.Code != http.StatusBadRequest {
			t.Errorf("limit=%q: expected 400, got %d", bad, resp.Code)
		}
	}
}

func TestListControlAudit_LimitExceedsMax_Returns400(t *testing.T) {
	svc := &mockControlAuditService{}
	srv := newAuditServer(svc)

	resp := performRequest(t, srv, http.MethodGet,
		fmt.Sprintf("/v1/controlplane/audit?limit=%d", controlaudit.MaxListLimit+1), nil)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}
	errResp := decodeError(t, resp)
	if errResp["error"] == "" {
		t.Error("expected non-empty error message")
	}
}

func TestListControlAudit_ValidLimit_PassedToService(t *testing.T) {
	var capturedFilter controlaudit.ListFilter
	svc := &mockControlAuditService{
		listAuditFn: func(_ context.Context, f controlaudit.ListFilter) ([]*controlaudit.ControlAuditRecord, error) {
			capturedFilter = f
			return nil, nil
		},
	}

	srv := newAuditServer(svc)
	resp := performRequest(t, srv, http.MethodGet, "/v1/controlplane/audit?limit=10", nil)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if capturedFilter.Limit != 10 {
		t.Errorf("expected limit 10, got %d", capturedFilter.Limit)
	}
}

func TestListControlAudit_FilterParams_PassedToService(t *testing.T) {
	var capturedFilter controlaudit.ListFilter
	svc := &mockControlAuditService{
		listAuditFn: func(_ context.Context, f controlaudit.ListFilter) ([]*controlaudit.ControlAuditRecord, error) {
			capturedFilter = f
			return nil, nil
		},
	}

	srv := newAuditServer(svc)
	resp := performRequest(t, srv, http.MethodGet,
		"/v1/controlplane/audit?resource_kind=surface&resource_id=surf-1&actor=alice&action=surface.created", nil)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if capturedFilter.ResourceKind != "surface" {
		t.Errorf("ResourceKind: want surface, got %q", capturedFilter.ResourceKind)
	}
	if capturedFilter.ResourceID != "surf-1" {
		t.Errorf("ResourceID: want surf-1, got %q", capturedFilter.ResourceID)
	}
	if capturedFilter.Actor != "alice" {
		t.Errorf("Actor: want alice, got %q", capturedFilter.Actor)
	}
	if capturedFilter.Action != controlaudit.ActionSurfaceCreated {
		t.Errorf("Action: want %q, got %q", controlaudit.ActionSurfaceCreated, capturedFilter.Action)
	}
}

func TestListControlAudit_ServiceError_Returns500(t *testing.T) {
	svc := &mockControlAuditService{
		listAuditFn: func(_ context.Context, _ controlaudit.ListFilter) ([]*controlaudit.ControlAuditRecord, error) {
			return nil, fmt.Errorf("db connection lost")
		},
	}

	srv := newAuditServer(svc)
	resp := performRequest(t, srv, http.MethodGet, "/v1/controlplane/audit", nil)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestListControlAudit_MetadataFieldsPresent(t *testing.T) {
	v := 2
	rec := &controlaudit.ControlAuditRecord{
		ID:              "rec-depr-1",
		OccurredAt:      time.Now().UTC(),
		Actor:           "ops",
		Action:          controlaudit.ActionSurfaceDeprecated,
		ResourceKind:    controlaudit.ResourceKindSurface,
		ResourceID:      "surf-depr",
		ResourceVersion: &v,
		Summary:         "surface deprecated",
		Metadata: &controlaudit.Metadata{
			DeprecationReason:  "replaced by v3",
			SuccessorSurfaceID: "surf-v3",
		},
	}

	svc := &mockControlAuditService{
		listAuditFn: func(_ context.Context, _ controlaudit.ListFilter) ([]*controlaudit.ControlAuditRecord, error) {
			return []*controlaudit.ControlAuditRecord{rec}, nil
		},
	}

	srv := newAuditServer(svc)
	resp := performRequest(t, srv, http.MethodGet, "/v1/controlplane/audit", nil)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	result := decodeJSON[controlAuditListResponse](t, resp)
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
	entry := result.Entries[0]
	if entry.ResourceVersion == nil || *entry.ResourceVersion != 2 {
		t.Errorf("ResourceVersion: want 2, got %v", entry.ResourceVersion)
	}
	if entry.Metadata == nil {
		t.Fatal("expected non-nil metadata")
	}
	if entry.Metadata["deprecation_reason"] != "replaced by v3" {
		t.Errorf("metadata.deprecation_reason: want 'replaced by v3', got %q", entry.Metadata["deprecation_reason"])
	}
	if entry.Metadata["successor_surface_id"] != "surf-v3" {
		t.Errorf("metadata.successor_surface_id: want surf-v3, got %q", entry.Metadata["successor_surface_id"])
	}
}
