package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/escalation"
	"github.com/accept-io/midas/internal/store/memory"
)

func newEscalationTargetHandlerServer(t *testing.T, seeds []*escalation.EscalationTarget) *Server {
	t.Helper()
	repo := memory.NewEscalationTargetRepo()
	for _, tgt := range seeds {
		if err := repo.Create(context.Background(), tgt); err != nil {
			t.Fatalf("seed escalation target %q v%d: %v", tgt.ID, tgt.Version, err)
		}
	}
	svc := NewEscalationTargetReadService(repo)
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithEscalationTargetReadService(svc)
	return srv
}

func makeTestEscalationTarget(id string, version int, status escalation.Status) *escalation.EscalationTarget {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	return &escalation.EscalationTarget{
		ID:             id,
		Version:        version,
		Name:           "target " + id,
		Description:    "test",
		Kind:           escalation.KindRole,
		Handle:         "governance.approver",
		Status:         status,
		EffectiveDate:  now,
		BusinessOwner:  "biz",
		TechnicalOwner: "tech",
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "alice",
	}
}

func TestEscalationTargetHandler_List_HappyPath(t *testing.T) {
	srv := newEscalationTargetHandlerServer(t, []*escalation.EscalationTarget{
		makeTestEscalationTarget("et-b", 1, escalation.StatusActive),
		makeTestEscalationTarget("et-a", 1, escalation.StatusActive),
	})
	rec := performRequest(t, srv, http.MethodGet, "/v1/escalation_targets", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp escalationTargetListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.EscalationTargets) != 2 {
		t.Fatalf("len = %d, want 2", len(resp.EscalationTargets))
	}
	// Repo sorts by id ascending.
	if resp.EscalationTargets[0].ID != "et-a" || resp.EscalationTargets[1].ID != "et-b" {
		t.Errorf("List order: %+v", resp.EscalationTargets)
	}
}

func TestEscalationTargetHandler_List_Empty_Returns200WithEmptyArray(t *testing.T) {
	srv := newEscalationTargetHandlerServer(t, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/escalation_targets", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp escalationTargetListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.EscalationTargets == nil {
		t.Errorf("EscalationTargets array must be non-null (empty list expected)")
	}
}

func TestEscalationTargetHandler_GetLatest_HappyPath(t *testing.T) {
	srv := newEscalationTargetHandlerServer(t, []*escalation.EscalationTarget{
		makeTestEscalationTarget("et-1", 1, escalation.StatusDeprecated),
		makeTestEscalationTarget("et-1", 2, escalation.StatusActive),
	})
	rec := performRequest(t, srv, http.MethodGet, "/v1/escalation_targets/et-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp escalationTargetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Version != 2 {
		t.Errorf("Version: want latest (2), got %d", resp.Version)
	}
	if resp.Kind != "role" || resp.Handle != "governance.approver" {
		t.Errorf("Kind/Handle: %+v", resp)
	}
}

func TestEscalationTargetHandler_Get_UnknownID_Returns404(t *testing.T) {
	srv := newEscalationTargetHandlerServer(t, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/escalation_targets/et-missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

func TestEscalationTargetHandler_ListVersions_HappyPath(t *testing.T) {
	srv := newEscalationTargetHandlerServer(t, []*escalation.EscalationTarget{
		makeTestEscalationTarget("et-1", 1, escalation.StatusActive),
		makeTestEscalationTarget("et-1", 2, escalation.StatusActive),
		makeTestEscalationTarget("et-1", 3, escalation.StatusActive),
	})
	rec := performRequest(t, srv, http.MethodGet, "/v1/escalation_targets/et-1/versions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp escalationTargetListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.EscalationTargets) != 3 {
		t.Fatalf("len = %d, want 3", len(resp.EscalationTargets))
	}
	for i, want := range []int{3, 2, 1} {
		if resp.EscalationTargets[i].Version != want {
			t.Errorf("[%d].Version: want %d, got %d", i, want, resp.EscalationTargets[i].Version)
		}
	}
}

func TestEscalationTargetHandler_ListVersions_UnknownID_Returns404(t *testing.T) {
	srv := newEscalationTargetHandlerServer(t, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/escalation_targets/et-missing/versions", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

func TestEscalationTargetHandler_GetVersion_HappyPath(t *testing.T) {
	srv := newEscalationTargetHandlerServer(t, []*escalation.EscalationTarget{
		makeTestEscalationTarget("et-1", 1, escalation.StatusActive),
		makeTestEscalationTarget("et-1", 2, escalation.StatusReview),
	})
	rec := performRequest(t, srv, http.MethodGet, "/v1/escalation_targets/et-1/versions/2", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp escalationTargetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Version != 2 || resp.Status != string(escalation.StatusReview) {
		t.Errorf("unexpected: %+v", resp)
	}
}

func TestEscalationTargetHandler_GetVersion_NotFound(t *testing.T) {
	srv := newEscalationTargetHandlerServer(t, []*escalation.EscalationTarget{
		makeTestEscalationTarget("et-1", 1, escalation.StatusActive),
	})
	rec := performRequest(t, srv, http.MethodGet, "/v1/escalation_targets/et-1/versions/99", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

func TestEscalationTargetHandler_GetVersion_BadVersion(t *testing.T) {
	srv := newEscalationTargetHandlerServer(t, []*escalation.EscalationTarget{
		makeTestEscalationTarget("et-1", 1, escalation.StatusActive),
	})
	for _, bad := range []string{"abc", "-1", "0"} {
		rec := performRequest(t, srv, http.MethodGet, "/v1/escalation_targets/et-1/versions/"+bad, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("bad version %q: status = %d, want 400", bad, rec.Code)
		}
	}
}

func TestEscalationTargetHandler_MethodNotAllowed(t *testing.T) {
	srv := newEscalationTargetHandlerServer(t, []*escalation.EscalationTarget{
		makeTestEscalationTarget("et-1", 1, escalation.StatusActive),
	})
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := performRequest(t, srv, m, "/v1/escalation_targets/et-1", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s status = %d, want 405", m, rec.Code)
		}
		rec = performRequest(t, srv, m, "/v1/escalation_targets", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s (list) status = %d, want 405", m, rec.Code)
		}
	}
}

func TestEscalationTargetHandler_NoReader_Returns501(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	// No WithEscalationTargetReadService call.
	for _, path := range []string{
		"/v1/escalation_targets",
		"/v1/escalation_targets/et-1",
		"/v1/escalation_targets/et-1/versions",
		"/v1/escalation_targets/et-1/versions/1",
	} {
		rec := performRequest(t, srv, http.MethodGet, path, nil)
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("%s: status = %d, want 501", path, rec.Code)
		}
	}
}

func TestEscalationTargetHandler_NilReader_Returns501(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithEscalationTargetReadService(NewEscalationTargetReadService(nil))
	rec := performRequest(t, srv, http.MethodGet, "/v1/escalation_targets", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}

func TestEscalationTargetHandler_EmptyIDDispatch_Returns404(t *testing.T) {
	srv := newEscalationTargetHandlerServer(t, []*escalation.EscalationTarget{
		makeTestEscalationTarget("et-1", 1, escalation.StatusActive),
	})
	rec := performRequest(t, srv, http.MethodGet, "/v1/escalation_targets/", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestEscalationTargetHandler_UnknownSubpath_Returns404(t *testing.T) {
	srv := newEscalationTargetHandlerServer(t, []*escalation.EscalationTarget{
		makeTestEscalationTarget("et-1", 1, escalation.StatusActive),
	})
	rec := performRequest(t, srv, http.MethodGet, "/v1/escalation_targets/et-1/unknown", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
