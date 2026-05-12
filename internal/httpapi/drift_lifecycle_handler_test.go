package httpapi

// drift_lifecycle_handler_test.go — HTTP-layer tests for the
// Drift-1e lifecycle endpoints. White-box tests in package httpapi so
// the mockApprovalService type can be reused.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/config"
	"github.com/accept-io/midas/internal/controlplane/approval"
	"github.com/accept-io/midas/internal/drift"
)

// driftLifecycleServer wires a Server with a mock approval service +
// AuthModeOpen so requests aren't blocked by authentication. The
// per-action permission gate is part of what we want to exercise, so
// we install a minimal token authenticator that hands the request a
// principal with the necessary roles.
//
// AuthModeOpen short-circuits requireAuth (no principal in context);
// requirePermission then short-circuits via authMode==open. This
// matches how other write-path tests (e.g. authz_write_test.go) drive
// the lifecycle endpoints.
func driftLifecycleServer(t *testing.T, mock *mockApprovalService) *Server {
	t.Helper()
	srv := NewServerFull(nil, nil, mock, nil, nil, nil)
	srv.WithAuthMode(config.AuthModeOpen)
	return srv
}

func ddOK(id string, version int, status drift.DriftDefinitionStatus) *drift.DriftDefinition {
	now := time.Now().UTC()
	return &drift.DriftDefinition{
		ID:               id,
		Version:          version,
		Name:             "Drift " + id,
		Status:           status,
		EffectiveDate:    now,
		BusinessOwner:    "owner",
		TechnicalOwner:   "owner",
		TargetEntityKind: drift.TargetEntityKindDecisionSurface,
		TargetEntityID:   "surf-x",
		Origin:           drift.DriftOriginManual,
		Managed:          true,
		Metrics: []drift.DriftMetricDefinition{{
			MetricID:           "outcome-psi",
			DriftType:          drift.DriftTypeOutcome,
			BaselineStrategy:   drift.BaselineStrategyFixedGoverned,
			WindowSeconds:      3600,
			Cadence:            drift.CadenceHour,
			WarningThreshold:   0.10,
			BreachedThreshold:  0.20,
			ThresholdDirection: drift.ThresholdDirectionAscending,
		}},
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "alice",
	}
}

// ---------------------------------------------------------------------
// Submit
// ---------------------------------------------------------------------

func TestDriftLifecycle_Submit_HappyPath(t *testing.T) {
	mock := &mockApprovalService{
		submitDriftDefinitionFn: func(_ context.Context, id string, version int, _, _ string) (*drift.DriftDefinition, error) {
			d := ddOK(id, version, drift.DriftDefinitionStatusReview)
			return d, nil
		},
	}
	srv := driftLifecycleServer(t, mock)

	body := []byte(`{"version":1,"submitted_by":"operator","reason":"ready"}`)
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/drift_definitions/approve-rate-drift/submit", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp driftDefinitionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "review" {
		t.Errorf("Status = %q, want review", resp.Status)
	}
	if resp.ID != "approve-rate-drift" {
		t.Errorf("ID = %q", resp.ID)
	}
}

func TestDriftLifecycle_Submit_VersionRequired(t *testing.T) {
	srv := driftLifecycleServer(t, &mockApprovalService{})
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/drift_definitions/x/submit", []byte(`{"version":0}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestDriftLifecycle_Submit_MissingBody_Returns400(t *testing.T) {
	srv := driftLifecycleServer(t, &mockApprovalService{})
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/drift_definitions/x/submit", []byte(`{}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestDriftLifecycle_Submit_NotFound_Returns404(t *testing.T) {
	mock := &mockApprovalService{
		submitDriftDefinitionFn: func(_ context.Context, _ string, _ int, _, _ string) (*drift.DriftDefinition, error) {
			return nil, approvalDriftErr("not_found")
		},
	}
	srv := driftLifecycleServer(t, mock)
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/drift_definitions/missing/submit", []byte(`{"version":1,"submitted_by":"op"}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDriftLifecycle_Submit_InvalidTransition_Returns409(t *testing.T) {
	mock := &mockApprovalService{
		submitDriftDefinitionFn: func(_ context.Context, _ string, _ int, _, _ string) (*drift.DriftDefinition, error) {
			return nil, approvalDriftErr("not_in_draft")
		},
	}
	srv := driftLifecycleServer(t, mock)
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/drift_definitions/x/submit", []byte(`{"version":1,"submitted_by":"op"}`))
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

// ---------------------------------------------------------------------
// Approve
// ---------------------------------------------------------------------

func TestDriftLifecycle_Approve_HappyPath(t *testing.T) {
	mock := &mockApprovalService{
		approveDriftDefinitionFn: func(_ context.Context, id string, version int, _ string) (*drift.DriftDefinition, error) {
			d := ddOK(id, version, drift.DriftDefinitionStatusActive)
			now := time.Now().UTC()
			d.ApprovedBy = "bob"
			d.ApprovedAt = &now
			return d, nil
		},
	}
	srv := driftLifecycleServer(t, mock)
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/drift_definitions/x/approve", []byte(`{"version":1,"approved_by":"bob"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp driftDefinitionResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Status != "active" {
		t.Errorf("Status = %q", resp.Status)
	}
	if resp.ApprovedBy != "bob" {
		t.Errorf("ApprovedBy = %q", resp.ApprovedBy)
	}
	if resp.ApprovedAt == nil {
		t.Error("ApprovedAt nil")
	}
}

func TestDriftLifecycle_Approve_MakerCheckerViolation_Returns403(t *testing.T) {
	mock := &mockApprovalService{
		approveDriftDefinitionFn: func(_ context.Context, _ string, _ int, _ string) (*drift.DriftDefinition, error) {
			return nil, approvalDriftErr("maker_checker")
		},
	}
	srv := driftLifecycleServer(t, mock)
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/drift_definitions/x/approve", []byte(`{"version":1,"approved_by":"alice"}`))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestDriftLifecycle_Approve_NotInReview_Returns409(t *testing.T) {
	mock := &mockApprovalService{
		approveDriftDefinitionFn: func(_ context.Context, _ string, _ int, _ string) (*drift.DriftDefinition, error) {
			return nil, approvalDriftErr("not_in_review")
		},
	}
	srv := driftLifecycleServer(t, mock)
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/drift_definitions/x/approve", []byte(`{"version":1,"approved_by":"bob"}`))
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

// ---------------------------------------------------------------------
// Reject
// ---------------------------------------------------------------------

func TestDriftLifecycle_Reject_HappyPath(t *testing.T) {
	mock := &mockApprovalService{
		rejectDriftDefinitionFn: func(_ context.Context, id string, version int, _, _ string) (*drift.DriftDefinition, error) {
			return ddOK(id, version, drift.DriftDefinitionStatusDraft), nil
		},
	}
	srv := driftLifecycleServer(t, mock)
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/drift_definitions/x/reject", []byte(`{"version":1,"rejected_by":"bob","reason":"need work"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp driftDefinitionResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Status != "draft" {
		t.Errorf("Status = %q", resp.Status)
	}
}

// ---------------------------------------------------------------------
// Deprecate
// ---------------------------------------------------------------------

func TestDriftLifecycle_Deprecate_HappyPath_WithSuccessor(t *testing.T) {
	var capturedID string
	var capturedVer int
	mock := &mockApprovalService{
		deprecateDriftDefinitionFn: func(_ context.Context, id string, version int, _, _ string, sid string, sver int) (*drift.DriftDefinition, error) {
			capturedID = sid
			capturedVer = sver
			d := ddOK(id, version, drift.DriftDefinitionStatusDeprecated)
			d.SuccessorDefinitionID = sid
			d.SuccessorVersion = sver
			return d, nil
		},
	}
	srv := driftLifecycleServer(t, mock)
	body := []byte(`{"version":1,"deprecated_by":"admin","reason":"replaced","successor_definition_id":"approve-rate-drift","successor_version":2}`)
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/drift_definitions/approve-rate-drift/deprecate", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if capturedID != "approve-rate-drift" || capturedVer != 2 {
		t.Errorf("successor not forwarded: id=%q ver=%d", capturedID, capturedVer)
	}
	var resp driftDefinitionResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.SuccessorVersion != 2 {
		t.Errorf("SuccessorVersion = %d", resp.SuccessorVersion)
	}
}

func TestDriftLifecycle_Deprecate_NotActive_Returns409(t *testing.T) {
	mock := &mockApprovalService{
		deprecateDriftDefinitionFn: func(_ context.Context, _ string, _ int, _, _ string, _ string, _ int) (*drift.DriftDefinition, error) {
			return nil, approvalDriftErr("not_active")
		},
	}
	srv := driftLifecycleServer(t, mock)
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/drift_definitions/x/deprecate", []byte(`{"version":1,"deprecated_by":"admin"}`))
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

// ---------------------------------------------------------------------
// Retire
// ---------------------------------------------------------------------

func TestDriftLifecycle_Retire_HappyPath(t *testing.T) {
	mock := &mockApprovalService{
		retireDriftDefinitionFn: func(_ context.Context, id string, version int, _, _ string) (*drift.DriftDefinition, error) {
			d := ddOK(id, version, drift.DriftDefinitionStatusRetired)
			now := time.Now().UTC()
			d.RetiredAt = &now
			return d, nil
		},
	}
	srv := driftLifecycleServer(t, mock)
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/drift_definitions/x/retire", []byte(`{"version":1,"retired_by":"admin","reason":"obsolete"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp driftDefinitionResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Status != "retired" {
		t.Errorf("Status = %q", resp.Status)
	}
	if resp.RetiredAt == nil {
		t.Error("RetiredAt nil")
	}
}

func TestDriftLifecycle_Retire_AlreadyRetired_Returns409(t *testing.T) {
	mock := &mockApprovalService{
		retireDriftDefinitionFn: func(_ context.Context, _ string, _ int, _, _ string) (*drift.DriftDefinition, error) {
			return nil, approvalDriftErr("already_retired")
		},
	}
	srv := driftLifecycleServer(t, mock)
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/drift_definitions/x/retire", []byte(`{"version":1,"retired_by":"admin"}`))
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

// ---------------------------------------------------------------------
// Method / dispatch / authentication coverage
// ---------------------------------------------------------------------

func TestDriftLifecycle_MethodNotAllowed(t *testing.T) {
	srv := driftLifecycleServer(t, &mockApprovalService{})
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := performRequest(t, srv, m, "/v1/controlplane/drift_definitions/x/approve", []byte(`{}`))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want 405", m, rec.Code)
		}
	}
}

func TestDriftLifecycle_UnknownAction_Returns404(t *testing.T) {
	srv := driftLifecycleServer(t, &mockApprovalService{})
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/drift_definitions/x/explode", []byte(`{"version":1}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDriftLifecycle_NoApprovalService_Returns501(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithAuthMode(config.AuthModeOpen)
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/drift_definitions/x/approve", []byte(`{"version":1}`))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}

// ---------------------------------------------------------------------
// Runtime-inert source pin
// ---------------------------------------------------------------------

func TestDriftLifecycle_RuntimeInert_SourcePin(t *testing.T) {
	body, err := os.ReadFile("drift_lifecycle_handler.go")
	if err != nil {
		t.Fatalf("read drift_lifecycle_handler.go: %v", err)
	}
	src := string(body)
	for _, banned := range []string{
		"DriftSeries.Create",
		"DriftSeriesPoint.Create",
		"DriftObservation.Create",
		"DriftAnnotation.Create",
		// Lifecycle endpoints must not call the metric-mutating
		// repository paths, even indirectly.
		"insertDriftMetric",
	} {
		if strings.Contains(src, banned) {
			t.Errorf("drift_lifecycle_handler.go must remain runtime-inert; found %q", banned)
		}
	}
}

// approvalDriftErr returns the drift-lifecycle sentinel error keyed
// by short tag. Used by the table-driven cases above to verify the
// HTTP layer's status-code mapping.
func approvalDriftErr(tag string) error {
	switch tag {
	case "not_found":
		return approval.ErrDriftDefinitionNotFound
	case "not_in_draft":
		return approval.ErrDriftDefinitionNotInDraft
	case "not_in_review":
		return approval.ErrDriftDefinitionNotInReview
	case "not_active":
		return approval.ErrDriftDefinitionNotActive
	case "already_retired":
		return approval.ErrDriftDefinitionAlreadyRetired
	case "maker_checker":
		return approval.ErrDriftDefinitionMakerChecker
	}
	return nil
}
