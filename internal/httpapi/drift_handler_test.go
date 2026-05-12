package httpapi

// drift_handler_test.go — Drift-1d HTTP read API tests. Exercises the
// 13 GET routes against an in-memory drift repository stack.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/drift"
	"github.com/accept-io/midas/internal/store/memory"
)

// newDriftHandlerServer wires a Server with a DriftReadService whose
// every reader is attached. Optional fixtures are seeded via the memory
// repos before the server is returned.
func newDriftHandlerServer(
	t *testing.T,
	defs []*drift.DriftDefinition,
	series []*drift.DriftSeries,
	points []*drift.DriftSeriesPoint,
	observations []*drift.DriftObservation,
	annotations []*drift.DriftAnnotation,
) *Server {
	t.Helper()
	defRepo := memory.NewDriftDefinitionRepo()
	seriesRepo := memory.NewDriftSeriesRepo()
	pointRepo := memory.NewDriftSeriesPointRepo()
	obsRepo := memory.NewDriftObservationRepo()
	annRepo := memory.NewDriftAnnotationRepo()

	for _, d := range defs {
		if err := defRepo.Create(context.Background(), d); err != nil {
			t.Fatalf("seed DriftDefinition %q: %v", d.ID, err)
		}
	}
	for _, s := range series {
		if err := seriesRepo.Create(context.Background(), s); err != nil {
			t.Fatalf("seed DriftSeries %q: %v", s.ID, err)
		}
	}
	for _, p := range points {
		if err := pointRepo.Create(context.Background(), p); err != nil {
			t.Fatalf("seed DriftSeriesPoint %q: %v", p.ID, err)
		}
	}
	for _, o := range observations {
		if err := obsRepo.Create(context.Background(), o); err != nil {
			t.Fatalf("seed DriftObservation %q: %v", o.ID, err)
		}
	}
	for _, a := range annotations {
		if err := annRepo.Create(context.Background(), a); err != nil {
			t.Fatalf("seed DriftAnnotation %q: %v", a.ID, err)
		}
	}

	svc := NewDriftReadService(defRepo, seriesRepo, pointRepo, obsRepo, annRepo)
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithDriftReadService(svc)
	return srv
}

func makeTestDriftDefinition(id string, version int) *drift.DriftDefinition {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	return &drift.DriftDefinition{
		ID:               id,
		Version:          version,
		Name:             "Drift " + id,
		Status:           drift.DriftDefinitionStatusReview,
		EffectiveDate:    now,
		BusinessOwner:    "owner",
		TechnicalOwner:   "owner",
		TargetEntityKind: drift.TargetEntityKindDecisionSurface,
		TargetEntityID:   "surf-x",
		Origin:           drift.DriftOriginManual,
		Managed:          true,
		Metrics: []drift.DriftMetricDefinition{
			{
				MetricID:           "outcome-psi",
				DriftType:          drift.DriftTypeOutcome,
				BaselineStrategy:   drift.BaselineStrategyFixedGoverned,
				WindowSeconds:      3600,
				Cadence:            drift.CadenceHour,
				WarningThreshold:   0.10,
				BreachedThreshold:  0.20,
				ThresholdDirection: drift.ThresholdDirectionAscending,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "alice",
	}
}

func makeTestDriftSeries(id, defID string, defVer int, group string) *drift.DriftSeries {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	return &drift.DriftSeries{
		ID:                id,
		DefinitionID:      defID,
		DefinitionVersion: defVer,
		MetricID:          "outcome-psi",
		Cadence:           drift.CadenceHour,
		Status:            drift.DriftSeriesStatusHealthy,
		ContinuityGroupID: group,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func makeTestDriftPoint(id, seriesID string, windowStart time.Time) *drift.DriftSeriesPoint {
	return &drift.DriftSeriesPoint{
		ID:                   id,
		SeriesID:             seriesID,
		WindowStart:          windowStart,
		WindowEnd:            windowStart.Add(time.Hour),
		SampleCount:          120,
		BaselineWindowID:     "b1",
		Magnitude:            0.05,
		Status:               drift.DriftSeriesPointStatusHealthy,
		ComputationMode:      drift.DriftPointComputationModeRealtime,
		ComputedAt:           windowStart.Add(time.Hour),
		SourceWindowComplete: true,
		CreatedAt:            windowStart.Add(time.Hour),
	}
}

func makeTestDriftObservation(id, defID, seriesID, pointID, entityID string) *drift.DriftObservation {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	return &drift.DriftObservation{
		ID:                  id,
		DefinitionID:        defID,
		DefinitionVersion:   1,
		SeriesID:            seriesID,
		PointID:             pointID,
		TargetEntityKind:    drift.TargetEntityKindDecisionSurface,
		TargetEntityID:      entityID,
		DriftType:           drift.DriftTypeOutcome,
		Magnitude:           0.25,
		DetectorStatus:      drift.DriftObservationDetectorStatusBreached,
		OperatorStatus:      drift.DriftObservationOperatorStatusOpen,
		BaselineWindowID:    "b1",
		ObservedWindowStart: now,
		ObservedWindowEnd:   now.Add(time.Hour),
		DetectedAt:          now.Add(time.Hour),
		EmittedAt:           now.Add(time.Hour),
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}

func makeTestDriftAnnotation(id string, kind drift.DriftAnnotationTargetKind, targetID string) *drift.DriftAnnotation {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	return &drift.DriftAnnotation{
		ID:             id,
		TargetKind:     kind,
		TargetID:       targetID,
		AnnotationType: drift.DriftAnnotationTypeRemediationNote,
		Body:           "rolled back model v17",
		Status:         drift.DriftAnnotationStatusActive,
		AuthorID:       "alice",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// ---------------------------------------------------------------------
// GET /v1/drift/definitions/{id}
// ---------------------------------------------------------------------

func TestDriftHandler_GetDefinition_HappyPath(t *testing.T) {
	srv := newDriftHandlerServer(t,
		[]*drift.DriftDefinition{makeTestDriftDefinition("approve-rate-drift", 1)}, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/definitions/approve-rate-drift", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp driftDefinitionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID != "approve-rate-drift" {
		t.Errorf("ID = %q", resp.ID)
	}
	if resp.Target.Kind != "decision_surface" {
		t.Errorf("Target.Kind = %q", resp.Target.Kind)
	}
	if len(resp.Metrics) != 1 {
		t.Errorf("Metrics len = %d", len(resp.Metrics))
	}
}

func TestDriftHandler_GetDefinition_NotFound(t *testing.T) {
	srv := newDriftHandlerServer(t, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/definitions/missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDriftHandler_GetDefinition_NoReader_Returns501(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/definitions/x", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}

func TestDriftHandler_GetDefinition_MethodNotAllowed(t *testing.T) {
	srv := newDriftHandlerServer(t, nil, nil, nil, nil, nil)
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := performRequest(t, srv, m, "/v1/drift/definitions/x", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want 405", m, rec.Code)
		}
	}
}

// ---------------------------------------------------------------------
// GET /v1/drift/definitions/{id}/versions[/{version}]
// ---------------------------------------------------------------------

func TestDriftHandler_ListDefinitionVersions(t *testing.T) {
	srv := newDriftHandlerServer(t,
		[]*drift.DriftDefinition{
			makeTestDriftDefinition("approve-rate-drift", 1),
			makeTestDriftDefinition("approve-rate-drift", 2),
		}, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/definitions/approve-rate-drift/versions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp driftDefinitionListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.DriftDefinitions) != 2 {
		t.Errorf("len = %d, want 2", len(resp.DriftDefinitions))
	}
	// Descending order from the repo.
	if resp.DriftDefinitions[0].Version != 2 {
		t.Errorf("first.Version = %d, want 2", resp.DriftDefinitions[0].Version)
	}
}

func TestDriftHandler_ListDefinitionVersions_UnknownID_Returns404(t *testing.T) {
	srv := newDriftHandlerServer(t, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/definitions/missing/versions", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDriftHandler_GetDefinitionVersion_HappyPath(t *testing.T) {
	srv := newDriftHandlerServer(t,
		[]*drift.DriftDefinition{
			makeTestDriftDefinition("approve-rate-drift", 1),
			makeTestDriftDefinition("approve-rate-drift", 2),
		}, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/definitions/approve-rate-drift/versions/1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp driftDefinitionResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Version != 1 {
		t.Errorf("Version = %d, want 1", resp.Version)
	}
}

func TestDriftHandler_GetDefinitionVersion_BadVersion_Returns400(t *testing.T) {
	srv := newDriftHandlerServer(t,
		[]*drift.DriftDefinition{makeTestDriftDefinition("approve-rate-drift", 1)}, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/definitions/approve-rate-drift/versions/abc", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestDriftHandler_GetDefinitionVersion_VersionNotFound_Returns404(t *testing.T) {
	srv := newDriftHandlerServer(t,
		[]*drift.DriftDefinition{makeTestDriftDefinition("approve-rate-drift", 1)}, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/definitions/approve-rate-drift/versions/99", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// ---------------------------------------------------------------------
// GET /v1/drift/series/{id} and series sub-paths
// ---------------------------------------------------------------------

func TestDriftHandler_GetSeries_HappyPath(t *testing.T) {
	srv := newDriftHandlerServer(t, nil,
		[]*drift.DriftSeries{makeTestDriftSeries("ser-1", "approve", 1, "approve")},
		nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/series/ser-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp driftSeriesResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.ID != "ser-1" {
		t.Errorf("ID = %q", resp.ID)
	}
}

func TestDriftHandler_GetSeries_NotFound(t *testing.T) {
	srv := newDriftHandlerServer(t, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/series/missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDriftHandler_ListSeriesByDefinition(t *testing.T) {
	srv := newDriftHandlerServer(t, nil,
		[]*drift.DriftSeries{
			makeTestDriftSeries("ser-1", "approve", 1, "approve"),
			makeTestDriftSeries("ser-2", "approve", 2, "approve"),
			makeTestDriftSeries("ser-3", "other", 1, "other"),
		},
		nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/definitions/approve/series", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp driftSeriesListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.DriftSeries) != 2 {
		t.Errorf("len = %d, want 2", len(resp.DriftSeries))
	}
}

func TestDriftHandler_ListSeriesByDefinition_EmptyResult_ReturnsArrayNotNull(t *testing.T) {
	srv := newDriftHandlerServer(t, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/definitions/missing/series", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"drift_series":[]`) {
		t.Errorf("expected drift_series:[] in body; got %s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------
// GET /v1/drift/series/{id}/points
// ---------------------------------------------------------------------

func TestDriftHandler_ListSeriesPoints_HappyPath(t *testing.T) {
	t0 := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	srv := newDriftHandlerServer(t, nil, nil,
		[]*drift.DriftSeriesPoint{
			makeTestDriftPoint("p1", "ser-1", t0),
			makeTestDriftPoint("p2", "ser-1", t0.Add(time.Hour)),
			makeTestDriftPoint("p3", "ser-2", t0),
		}, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/series/ser-1/points", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp driftSeriesPointListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.DriftSeriesPoints) != 2 {
		t.Errorf("len = %d, want 2", len(resp.DriftSeriesPoints))
	}
}

func TestDriftHandler_ListSeriesPoints_FromWindow(t *testing.T) {
	t0 := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	srv := newDriftHandlerServer(t, nil, nil,
		[]*drift.DriftSeriesPoint{
			makeTestDriftPoint("p1", "ser-1", t0),
			makeTestDriftPoint("p2", "ser-1", t0.Add(time.Hour)),
			makeTestDriftPoint("p3", "ser-1", t0.Add(2*time.Hour)),
		}, nil, nil)
	q := "/v1/drift/series/ser-1/points?from_window=" + t0.Add(time.Hour).Format(time.RFC3339)
	rec := performRequest(t, srv, http.MethodGet, q, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp driftSeriesPointListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.DriftSeriesPoints) != 2 {
		t.Errorf("from_window filter len = %d, want 2", len(resp.DriftSeriesPoints))
	}
}

func TestDriftHandler_ListSeriesPoints_LimitTooLow_Returns400(t *testing.T) {
	srv := newDriftHandlerServer(t, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/series/ser-1/points?limit=0", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestDriftHandler_ListSeriesPoints_LimitTooHigh_Returns400(t *testing.T) {
	srv := newDriftHandlerServer(t, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/series/ser-1/points?limit=1000", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestDriftHandler_ListSeriesPoints_BadFromWindow_Returns400(t *testing.T) {
	srv := newDriftHandlerServer(t, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/series/ser-1/points?from_window=not-a-date", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// ---------------------------------------------------------------------
// GET /v1/drift/series-points/{id}
// ---------------------------------------------------------------------

func TestDriftHandler_GetSeriesPoint_HappyPath(t *testing.T) {
	t0 := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	srv := newDriftHandlerServer(t, nil, nil,
		[]*drift.DriftSeriesPoint{makeTestDriftPoint("p1", "ser-1", t0)}, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/series-points/p1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestDriftHandler_GetSeriesPoint_NotFound(t *testing.T) {
	srv := newDriftHandlerServer(t, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/series-points/missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// ---------------------------------------------------------------------
// GET /v1/drift/observations/{id} and sub-paths
// ---------------------------------------------------------------------

func TestDriftHandler_GetObservation_HappyPath(t *testing.T) {
	srv := newDriftHandlerServer(t, nil, nil, nil,
		[]*drift.DriftObservation{makeTestDriftObservation("o1", "approve", "ser-1", "p1", "surf-x")}, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/observations/o1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp driftObservationResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Target.Kind != "decision_surface" {
		t.Errorf("Target.Kind = %q", resp.Target.Kind)
	}
}

func TestDriftHandler_ListObservationsBySeries(t *testing.T) {
	srv := newDriftHandlerServer(t, nil, nil, nil,
		[]*drift.DriftObservation{
			makeTestDriftObservation("o1", "approve", "ser-1", "p1", "surf-x"),
			makeTestDriftObservation("o2", "approve", "ser-1", "p2", "surf-x"),
			makeTestDriftObservation("o3", "approve", "ser-2", "p1", "surf-x"),
		}, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/series/ser-1/observations", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp driftObservationListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.DriftObservations) != 2 {
		t.Errorf("len = %d, want 2", len(resp.DriftObservations))
	}
}

func TestDriftHandler_ListObservationsByDefinition(t *testing.T) {
	srv := newDriftHandlerServer(t, nil, nil, nil,
		[]*drift.DriftObservation{
			makeTestDriftObservation("o1", "approve", "ser-1", "p1", "surf-x"),
			makeTestDriftObservation("o2", "other", "ser-2", "p1", "surf-x"),
		}, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/definitions/approve/observations", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp driftObservationListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.DriftObservations) != 1 {
		t.Errorf("len = %d, want 1", len(resp.DriftObservations))
	}
}

func TestDriftHandler_ListObservationsByEntity_HappyPath(t *testing.T) {
	srv := newDriftHandlerServer(t, nil, nil, nil,
		[]*drift.DriftObservation{makeTestDriftObservation("o1", "approve", "ser-1", "p1", "surf-x")}, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/entities/decision_surface/surf-x/observations", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestDriftHandler_ListObservationsByEntity_BadKind_Returns400(t *testing.T) {
	srv := newDriftHandlerServer(t, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/entities/weird_kind/x/observations", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// ---------------------------------------------------------------------
// GET /v1/drift/annotations + nested
// ---------------------------------------------------------------------

func TestDriftHandler_GetAnnotation_HappyPath(t *testing.T) {
	srv := newDriftHandlerServer(t, nil, nil, nil, nil,
		[]*drift.DriftAnnotation{makeTestDriftAnnotation("ann-1", drift.DriftAnnotationTargetKindObservation, "o1")})
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/annotations/ann-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp driftAnnotationResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Target.Kind != "observation" {
		t.Errorf("Target.Kind = %q", resp.Target.Kind)
	}
}

func TestDriftHandler_ListAnnotationsBySeries(t *testing.T) {
	srv := newDriftHandlerServer(t, nil, nil, nil, nil,
		[]*drift.DriftAnnotation{
			makeTestDriftAnnotation("ann-1", drift.DriftAnnotationTargetKindSeries, "ser-1"),
			makeTestDriftAnnotation("ann-2", drift.DriftAnnotationTargetKindObservation, "o1"),
		})
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/series/ser-1/annotations", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp driftAnnotationListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.DriftAnnotations) != 1 {
		t.Errorf("len = %d, want 1", len(resp.DriftAnnotations))
	}
}

func TestDriftHandler_ListAnnotationsByObservation(t *testing.T) {
	srv := newDriftHandlerServer(t, nil, nil, nil, nil,
		[]*drift.DriftAnnotation{
			makeTestDriftAnnotation("ann-1", drift.DriftAnnotationTargetKindSeries, "ser-1"),
			makeTestDriftAnnotation("ann-2", drift.DriftAnnotationTargetKindObservation, "o1"),
		})
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/observations/o1/annotations", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp driftAnnotationListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.DriftAnnotations) != 1 {
		t.Errorf("len = %d, want 1", len(resp.DriftAnnotations))
	}
}

// ---------------------------------------------------------------------
// JSONB / nullable shape pins
// ---------------------------------------------------------------------

func TestDriftHandler_SeriesPoint_NilStatsRoundTripAsEmptyObjects(t *testing.T) {
	t0 := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	p := makeTestDriftPoint("p1", "ser-1", t0)
	// Domain has nil maps + slice; wire shape must show {} / [].
	srv := newDriftHandlerServer(t, nil, nil, []*drift.DriftSeriesPoint{p}, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/series-points/p1", nil)
	body := rec.Body.String()
	if !strings.Contains(body, `"summary_stats":{}`) {
		t.Errorf("expected empty summary_stats object; got %s", body)
	}
	if !strings.Contains(body, `"baseline_stats":{}`) {
		t.Errorf("expected empty baseline_stats object; got %s", body)
	}
	if !strings.Contains(body, `"provenance_envelope_ids":[]`) {
		t.Errorf("expected empty provenance_envelope_ids array; got %s", body)
	}
}

func TestDriftHandler_DefinitionResponse_NullableTimestamps(t *testing.T) {
	srv := newDriftHandlerServer(t,
		[]*drift.DriftDefinition{makeTestDriftDefinition("approve-rate-drift", 1)}, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/drift/definitions/approve-rate-drift", nil)
	body := rec.Body.String()
	for _, want := range []string{`"effective_until":null`, `"retired_at":null`, `"approved_at":null`} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %s in body; got %s", want, body)
		}
	}
}

// ---------------------------------------------------------------------
// Runtime-inert source pin: no write/mutate calls in the read handler /
// response files. Drift-1d must remain runtime-inert.
// ---------------------------------------------------------------------

func TestDriftHandler_RuntimeInert_SourcePin(t *testing.T) {
	for _, file := range []string{"drift_handler.go", "drift_response.go"} {
		body, err := readSourceFile(t, file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, banned := range []string{
			".Create(",
			".Update(",
			".UpdateStatus(",
			".UpdateOperatorStatus(",
			".Seal(",
			".Supersede(",
			".DeleteBefore(",
		} {
			if strings.Contains(body, banned) {
				t.Errorf("%s must remain runtime-inert; found mutating call %q", file, banned)
			}
		}
	}
}

// readSourceFile reads a sibling source file relative to the test
// working directory (which is the package directory).
func readSourceFile(t *testing.T, name string) (string, error) {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
