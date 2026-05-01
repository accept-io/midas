package httpapi

// coverage_handler_test.go — tests for GET /v1/coverage (Issue #56).
// Lives in the httpapi package (white-box) so it can reuse package-private
// helpers and wire format types directly, mirroring controlaudit_handler_test.go.

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/governancecoverage"
)

// ---------------------------------------------------------------------------
// Mock coverageReadService
// ---------------------------------------------------------------------------

type mockCoverageReadService struct {
	listFn func(ctx context.Context, f governancecoverage.CoverageFilter) ([]*governancecoverage.CoverageRecord, error)
}

func (m *mockCoverageReadService) ListCoverage(ctx context.Context, f governancecoverage.CoverageFilter) ([]*governancecoverage.CoverageRecord, error) {
	if m.listFn != nil {
		return m.listFn(ctx, f)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newCoverageServer(svc coverageReadService) *Server {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil)
	if svc != nil {
		srv.WithCoverageReadService(svc)
	}
	return srv
}

func makeCoveredRecord(envelopeID, expID string) *governancecoverage.CoverageRecord {
	t := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	return &governancecoverage.CoverageRecord{
		RequestSource:      "api",
		RequestID:          "req-" + expID,
		EnvelopeID:         envelopeID,
		ProcessID:          "proc-1",
		ExpectationID:      expID,
		ExpectationVersion: 1,
		ConditionType:      "process_id",
		RequiredSurfaceID:  "surf-required",
		ActualSurfaceID:    "surf-required",
		Status:             governancecoverage.CoverageStatusCovered,
		Gap:                false,
		Partial:            false,
		Summary:            map[string]any{"foo": "bar"},
		DetectedAt:         &t,
	}
}

func makeGapRecord(envelopeID, expID string) *governancecoverage.CoverageRecord {
	d := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	g := time.Date(2026, 4, 1, 12, 0, 1, 0, time.UTC)
	return &governancecoverage.CoverageRecord{
		RequestSource:      "api",
		RequestID:          "req-" + expID,
		EnvelopeID:         envelopeID,
		ProcessID:          "proc-1",
		ExpectationID:      expID,
		ExpectationVersion: 1,
		ConditionType:      "process_id",
		RequiredSurfaceID:  "surf-required",
		MissingSurfaceID:   "surf-required",
		ActualSurfaceID:    "surf-actual",
		Status:             governancecoverage.CoverageStatusGap,
		Gap:                true,
		Partial:            false,
		Summary:            map[string]any{"foo": "bar"},
		CorrelationBasis:   map[string]any{"basis": "same-evaluation"},
		DetectedAt:         &d,
		GapDetectedAt:      &g,
	}
}

// ---------------------------------------------------------------------------
// GET /v1/coverage
// ---------------------------------------------------------------------------

func TestListCoverage_NilService_Returns501(t *testing.T) {
	srv := newCoverageServer(nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/coverage", nil)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", rec.Code)
	}
}

func TestListCoverage_MethodNotAllowed(t *testing.T) {
	svc := &mockCoverageReadService{}
	srv := newCoverageServer(svc)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := performRequest(t, srv, method, "/v1/coverage", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: expected 405, got %d", method, rec.Code)
		}
	}
}

func TestListCoverage_Success_CoveredAndGap(t *testing.T) {
	covered := makeCoveredRecord("env-1", "exp-1")
	gap := makeGapRecord("env-2", "exp-2")

	svc := &mockCoverageReadService{
		listFn: func(_ context.Context, _ governancecoverage.CoverageFilter) ([]*governancecoverage.CoverageRecord, error) {
			return []*governancecoverage.CoverageRecord{covered, gap}, nil
		},
	}

	srv := newCoverageServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/coverage", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[coverageListResponse](t, rec)
	if len(resp.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(resp.Records))
	}
	if resp.Records[0].Status != "covered" {
		t.Errorf("records[0].status: want covered, got %q", resp.Records[0].Status)
	}
	if resp.Records[0].ActualSurfaceID != "surf-required" {
		t.Errorf("records[0].actual_surface_id: want surf-required, got %q", resp.Records[0].ActualSurfaceID)
	}
	if resp.Records[1].Status != "gap" {
		t.Errorf("records[1].status: want gap, got %q", resp.Records[1].Status)
	}
	if !resp.Records[1].Gap {
		t.Error("records[1].gap: want true")
	}
	if resp.Records[1].GapDetectedAt == nil {
		t.Error("records[1].gap_detected_at: want non-nil")
	}
}

func TestListCoverage_EmptyResult_RecordsArrayNotNil(t *testing.T) {
	svc := &mockCoverageReadService{
		listFn: func(_ context.Context, _ governancecoverage.CoverageFilter) ([]*governancecoverage.CoverageRecord, error) {
			return nil, nil
		},
	}

	srv := newCoverageServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/coverage", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Decode raw to verify "records":[] not "records":null on the wire —
	// callers who iterate without nil checks must succeed against an empty list.
	body := rec.Body.String()
	if !strings.Contains(body, `"records":[]`) {
		t.Errorf("expected records:[] in body, got: %s", body)
	}
}

func TestListCoverage_LimitationsArray_Verbatim(t *testing.T) {
	svc := &mockCoverageReadService{}
	srv := newCoverageServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/coverage", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	resp := decodeJSON[coverageListResponse](t, rec)
	want := []string{
		"scope=process",
		"correlation=same-evaluation",
		"no-bypass-detection",
		"no-time-window-correlation",
	}
	if !reflect.DeepEqual(resp.Limitations, want) {
		t.Errorf("limitations:\n  want %v\n  got  %v", want, resp.Limitations)
	}
}

func TestListCoverage_InvalidLimit_Returns400(t *testing.T) {
	svc := &mockCoverageReadService{}
	srv := newCoverageServer(svc)

	for _, bad := range []string{"abc", "-1", "0", "3.5", "1e2"} {
		rec := performRequest(t, srv, http.MethodGet, "/v1/coverage?limit="+bad, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("limit=%q: expected 400, got %d", bad, rec.Code)
		}
	}
}

func TestListCoverage_LimitExceedsMax_Returns400(t *testing.T) {
	svc := &mockCoverageReadService{}
	srv := newCoverageServer(svc)

	rec := performRequest(t, srv, http.MethodGet,
		fmt.Sprintf("/v1/coverage?limit=%d", audit.MaxListLimit+1), nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	errResp := decodeError(t, rec)
	if errResp["error"] == "" {
		t.Error("expected non-empty error message")
	}
}

func TestListCoverage_ValidLimit_PassedToService(t *testing.T) {
	var captured governancecoverage.CoverageFilter
	svc := &mockCoverageReadService{
		listFn: func(_ context.Context, f governancecoverage.CoverageFilter) ([]*governancecoverage.CoverageRecord, error) {
			captured = f
			return nil, nil
		},
	}

	srv := newCoverageServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/coverage?limit=25", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if captured.Limit != 25 {
		t.Errorf("expected Limit=25, got %d", captured.Limit)
	}
}

func TestListCoverage_FilterParams_PassedToService(t *testing.T) {
	var captured governancecoverage.CoverageFilter
	svc := &mockCoverageReadService{
		listFn: func(_ context.Context, f governancecoverage.CoverageFilter) ([]*governancecoverage.CoverageRecord, error) {
			captured = f
			return nil, nil
		},
	}

	srv := newCoverageServer(svc)
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/coverage?request_source=api&request_id=r-1&envelope_id=env-1&process_id=proc-1&expectation_id=exp-1", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if captured.RequestSource != "api" {
		t.Errorf("RequestSource: want api, got %q", captured.RequestSource)
	}
	if captured.RequestID != "r-1" {
		t.Errorf("RequestID: want r-1, got %q", captured.RequestID)
	}
	if captured.EnvelopeID != "env-1" {
		t.Errorf("EnvelopeID: want env-1, got %q", captured.EnvelopeID)
	}
	if captured.ProcessID != "proc-1" {
		t.Errorf("ProcessID: want proc-1, got %q", captured.ProcessID)
	}
	if captured.ExpectationID != "exp-1" {
		t.Errorf("ExpectationID: want exp-1, got %q", captured.ExpectationID)
	}
}

func TestListCoverage_TimeRange_PassedToService(t *testing.T) {
	var captured governancecoverage.CoverageFilter
	svc := &mockCoverageReadService{
		listFn: func(_ context.Context, f governancecoverage.CoverageFilter) ([]*governancecoverage.CoverageRecord, error) {
			captured = f
			return nil, nil
		},
	}

	srv := newCoverageServer(svc)
	since := "2026-04-01T00:00:00Z"
	until := "2026-04-02T00:00:00Z"
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/coverage?since="+since+"&until="+until, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	wantSince, _ := time.Parse(time.RFC3339, since)
	wantUntil, _ := time.Parse(time.RFC3339, until)
	if !captured.Since.Equal(wantSince) {
		t.Errorf("Since: want %v, got %v", wantSince, captured.Since)
	}
	if !captured.Until.Equal(wantUntil) {
		t.Errorf("Until: want %v, got %v", wantUntil, captured.Until)
	}
}

func TestListCoverage_BadTimeRange_Returns400(t *testing.T) {
	svc := &mockCoverageReadService{}
	srv := newCoverageServer(svc)

	cases := []struct {
		name  string
		query string
	}{
		{"bad_since", "/v1/coverage?since=not-a-time"},
		{"bad_until", "/v1/coverage?until=2026/04/01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := performRequest(t, srv, http.MethodGet, tc.query, nil)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", rec.Code)
			}
		})
	}
}

func TestListCoverage_ServiceError_Returns500(t *testing.T) {
	svc := &mockCoverageReadService{
		listFn: func(_ context.Context, _ governancecoverage.CoverageFilter) ([]*governancecoverage.CoverageRecord, error) {
			return nil, fmt.Errorf("audit list failed")
		},
	}

	srv := newCoverageServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/coverage", nil)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /explorer/coverage  +  Production / Explorer isolation
// ---------------------------------------------------------------------------

// seedCoverageEventPair appends a matched (detected, gap) pair to repo
// for a single (envelope, expectation) so the read service merges them
// into one gap record. Used by the isolation tests.
func seedCoverageEventPair(t *testing.T, repo audit.AuditEventRepository, envelopeID, expID, requestSource, requestID string) {
	t.Helper()
	det := audit.NewEvent(envelopeID, requestSource, requestID,
		audit.AuditEventGovernanceConditionDetected,
		audit.EventPerformerSystem, "midas-orchestrator",
		map[string]any{
			"expectation_id":      expID,
			"expectation_version": 1,
			"process_id":          "proc-iso",
			"required_surface_id": "surf-required",
			"condition_type":      "process_id",
		})
	if err := repo.Append(context.Background(), det); err != nil {
		t.Fatalf("seed detected: %v", err)
	}
	gap := audit.NewEvent(envelopeID, requestSource, requestID,
		audit.AuditEventGovernanceCoverageGap,
		audit.EventPerformerSystem, "midas-orchestrator",
		map[string]any{
			"expectation_id":      expID,
			"expectation_version": 1,
			"process_id":          "proc-iso",
			"missing_surface_id":  "surf-required",
			"actual_surface_id":   "surf-actual",
		})
	if err := repo.Append(context.Background(), gap); err != nil {
		t.Fatalf("seed gap: %v", err)
	}
}

// TestExplorerCoverage_Disabled_Returns404 confirms that when Explorer is
// not enabled, the /explorer/coverage route is not registered.
func TestExplorerCoverage_Disabled_Returns404(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/coverage", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 when explorer disabled, got %d", rec.Code)
	}
}

// TestExplorerCoverage_Enabled_EmptyByDefault verifies that with the
// Explorer enabled but no events emitted, /explorer/coverage returns
// 200 with an empty records array. Sanity check that initExplorerRuntime
// wired the route and the service returns successfully.
func TestExplorerCoverage_Enabled_EmptyByDefault(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/coverage", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON[coverageListResponse](t, rec)
	if len(resp.Records) != 0 {
		t.Errorf("want 0 records on fresh Explorer runtime, got %d", len(resp.Records))
	}
	if !reflect.DeepEqual(resp.Limitations, coverageLimitations) {
		t.Errorf("limitations: want %v, got %v", coverageLimitations, resp.Limitations)
	}
}

// TestCoverage_Isolation_ProductionAndExplorerDisjoint is the
// load-bearing isolation test for Issue #56's Cluster D contract: an
// event in the production audit repository must not appear in
// /explorer/coverage, and vice versa. The two coverage views are
// backed by disjoint audit repositories — production via
// WithCoverageReadService bound to a stub repo, Explorer via
// initExplorerRuntime's isolated memory store — and this test pins
// the disjointness.
func TestCoverage_Isolation_ProductionAndExplorerDisjoint(t *testing.T) {
	prodAudit := audit.NewMemoryRepository()
	prodCoverage := governancecoverage.NewReadService(prodAudit)

	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithCoverageReadService(prodCoverage).
		WithExplorerEnabled(true)

	// Sanity: explorer init populated its own audit repo, distinct
	// from prodAudit. Without this distinctness the rest of the
	// test is vacuously true.
	if srv.explorerAudit == nil {
		t.Fatal("Explorer audit repo not initialised")
	}
	if srv.explorerAudit == prodAudit {
		t.Fatal("Explorer audit must be a distinct repository from the production audit")
	}

	seedCoverageEventPair(t, prodAudit, "env-prod", "exp-prod", "api", "req-prod")
	seedCoverageEventPair(t, srv.explorerAudit, "env-exp", "exp-explorer", "explorer", "req-exp")

	// Production view: should see only the production envelope.
	prodResp := performRequest(t, srv, http.MethodGet, "/v1/coverage", nil)
	if prodResp.Code != http.StatusOK {
		t.Fatalf("prod /v1/coverage: want 200, got %d: %s", prodResp.Code, prodResp.Body.String())
	}
	prodList := decodeJSON[coverageListResponse](t, prodResp)
	if len(prodList.Records) != 1 {
		t.Fatalf("/v1/coverage: want 1 record (prod only), got %d: %+v", len(prodList.Records), prodList.Records)
	}
	if prodList.Records[0].EnvelopeID != "env-prod" {
		t.Errorf("/v1/coverage: want envelope_id=env-prod, got %q", prodList.Records[0].EnvelopeID)
	}
	if prodList.Records[0].ExpectationID != "exp-prod" {
		t.Errorf("/v1/coverage: want expectation_id=exp-prod, got %q", prodList.Records[0].ExpectationID)
	}
	for _, rec := range prodList.Records {
		if rec.EnvelopeID == "env-exp" || rec.ExpectationID == "exp-explorer" {
			t.Errorf("/v1/coverage leaked Explorer event: %+v", rec)
		}
	}

	// Explorer view: should see only the Explorer envelope. The
	// Explorer's seeded demo runtime may also emit its own coverage
	// events on init — the assertion is that env-prod / exp-prod
	// from the production trail are absent, not that the count is
	// exactly 1. (Demo seeding is a side-effect of WithExplorerEnabled
	// and is not under this test's control.)
	expResp := performRequest(t, srv, http.MethodGet, "/explorer/coverage", nil)
	if expResp.Code != http.StatusOK {
		t.Fatalf("explorer /explorer/coverage: want 200, got %d: %s", expResp.Code, expResp.Body.String())
	}
	expList := decodeJSON[coverageListResponse](t, expResp)
	sawExplorer := false
	for _, rec := range expList.Records {
		if rec.EnvelopeID == "env-prod" || rec.ExpectationID == "exp-prod" {
			t.Errorf("/explorer/coverage leaked production event: %+v", rec)
		}
		if rec.EnvelopeID == "env-exp" && rec.ExpectationID == "exp-explorer" {
			sawExplorer = true
		}
	}
	if !sawExplorer {
		t.Errorf("/explorer/coverage: expected to see Explorer-emitted record env-exp/exp-explorer, got %+v", expList.Records)
	}
}

func TestListCoverage_WireShape_AllFields(t *testing.T) {
	gap := makeGapRecord("env-1", "exp-1")
	svc := &mockCoverageReadService{
		listFn: func(_ context.Context, _ governancecoverage.CoverageFilter) ([]*governancecoverage.CoverageRecord, error) {
			return []*governancecoverage.CoverageRecord{gap}, nil
		},
	}

	srv := newCoverageServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/coverage", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	resp := decodeJSON[coverageListResponse](t, rec)
	if len(resp.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(resp.Records))
	}
	got := resp.Records[0]
	want := coverageRecordResponse{
		RequestSource:      gap.RequestSource,
		RequestID:          gap.RequestID,
		EnvelopeID:         gap.EnvelopeID,
		ProcessID:          gap.ProcessID,
		ExpectationID:      gap.ExpectationID,
		ExpectationVersion: gap.ExpectationVersion,
		ConditionType:      gap.ConditionType,
		RequiredSurfaceID:  gap.RequiredSurfaceID,
		MissingSurfaceID:   gap.MissingSurfaceID,
		ActualSurfaceID:    gap.ActualSurfaceID,
		Status:             string(gap.Status),
		Gap:                gap.Gap,
		Partial:            gap.Partial,
		Summary:            gap.Summary,
		CorrelationBasis:   gap.CorrelationBasis,
		DetectedAt:         gap.DetectedAt,
		GapDetectedAt:      gap.GapDetectedAt,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wire shape mismatch:\n  want %+v\n  got  %+v", want, got)
	}
}
