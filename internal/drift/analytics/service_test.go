package analytics

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/drift"
	"github.com/accept-io/midas/internal/store/memory"
)

func TestService_NoDefinitionsReturnsUnavailable(t *testing.T) {
	svc := newTestService(t, nil, nil, nil, nil, nil)

	resp, err := svc.GetNodeAnalytics(context.Background(), DriftAnalyticsRequest{
		NodeKind: string(drift.TargetEntityKindDecisionSurface),
		NodeID:   "surf-x",
	})
	if err != nil {
		t.Fatalf("GetNodeAnalytics: %v", err)
	}
	if resp.DataAvailable {
		t.Fatal("DataAvailable = true, want false")
	}
	if resp.SourceClassification.ObservedSeries != "unavailable" {
		t.Errorf("ObservedSeries = %q", resp.SourceClassification.ObservedSeries)
	}
	if len(resp.Chart.Observed) != 0 || len(resp.Chart.Expected) != 0 {
		t.Fatalf("chart series must stay empty without backend data")
	}
}

func TestService_HappyPathBuildsObservedExpectedThresholdsAndProvenanceRefs(t *testing.T) {
	now := testNow()
	def := testDefinition("def-a", drift.ThresholdDirectionAscending)
	def.Metrics[0].GovernanceExpectationRef = "gov-exp:approve-rate"
	series := testSeries("ser-a", def, drift.DriftSeriesStatusWarning)
	points := []*drift.DriftSeriesPoint{
		testPoint("p1", series.ID, now.Add(-2*time.Hour), 0.10, 0.08, drift.DriftSeriesPointStatusHealthy),
		testPoint("p2", series.ID, now.Add(-1*time.Hour), 0.146, 0.09, drift.DriftSeriesPointStatusWarning),
	}
	points[1].ProvenanceEnvelopeIDs = []string{"env-point"}
	obs := testObservation("obs-1", def, series, points[1])
	obs.EvidenceEnvelopeIDs = []string{"env-obs"}
	obs.RelatedFailModePolicyID = "policy-1"
	ann := testAnnotation("ann-1", drift.DriftAnnotationTargetKindObservation, obs.ID)
	ann.ReferenceEnvelopeIDs = []string{"env-ann"}
	svc := newTestService(t, []*drift.DriftDefinition{def}, []*drift.DriftSeries{series}, points, []*drift.DriftObservation{obs}, []*drift.DriftAnnotation{ann})

	resp, err := svc.GetNodeAnalytics(context.Background(), DriftAnalyticsRequest{
		NodeKind: string(drift.TargetEntityKindDecisionSurface),
		NodeID:   "surf-x",
		RangeKey: "30d",
	})
	if err != nil {
		t.Fatalf("GetNodeAnalytics: %v", err)
	}
	if !resp.DataAvailable {
		t.Fatal("DataAvailable = false, want true")
	}
	if resp.Chart.MetricID != "approve-rate" || resp.Chart.SeriesID != "ser-a" {
		t.Fatalf("selected metric/series = %q/%q", resp.Chart.MetricID, resp.Chart.SeriesID)
	}
	if got := resp.Chart.Observed[1].Value; got != 0.146 {
		t.Fatalf("latest observed = %v, want 0.146", got)
	}
	if got := resp.Chart.Expected[1].Value; got != 0.09 {
		t.Fatalf("latest expected = %v, want 0.09", got)
	}
	if len(resp.Chart.Watch) != 2 || resp.Chart.Watch[0].Value != 0.12 {
		t.Fatalf("watch series = %#v", resp.Chart.Watch)
	}
	if len(resp.Chart.Breach) != 2 || resp.Chart.Breach[0].Value != 0.18 {
		t.Fatalf("breach series = %#v", resp.Chart.Breach)
	}
	if resp.Chart.CurrentValue == nil || *resp.Chart.CurrentValue != 0.146 {
		t.Fatalf("CurrentValue = %#v, want 0.146", resp.Chart.CurrentValue)
	}
	if resp.Chart.CurrentStatus != "warning" {
		t.Fatalf("CurrentStatus = %q, want warning", resp.Chart.CurrentStatus)
	}
	for _, want := range []string{"env-ann", "env-obs", "env-point"} {
		if !hasString(resp.Provenance.EnvelopeIDs, want) {
			t.Fatalf("missing envelope ref %q in %#v", want, resp.Provenance.EnvelopeIDs)
		}
	}
	for _, want := range []string{"gov-exp:approve-rate", "policy-1"} {
		if !hasString(resp.Provenance.PolicyRefs, want) {
			t.Fatalf("missing policy ref %q in %#v", want, resp.Provenance.PolicyRefs)
		}
	}
	if resp.Provenance.VerificationStatus != "not_requested" {
		t.Fatalf("VerificationStatus = %q", resp.Provenance.VerificationStatus)
	}
	if resp.SourceClassification.CompositeScore != "not_available" ||
		resp.SourceClassification.ContributionValues != "not_available" ||
		resp.SourceClassification.GraphOverlay != "not_implemented" {
		t.Fatalf("unexpected source classification: %#v", resp.SourceClassification)
	}
}

func TestService_SelectsScalarLatestPointBeforeMoreSevereDistributionSeries(t *testing.T) {
	now := testNow()
	def := testDefinition("def-a", drift.ThresholdDirectionAscending)
	scalar := testSeries("ser-scalar", def, drift.DriftSeriesStatusWarning)
	distribution := testSeries("ser-distribution", def, drift.DriftSeriesStatusBreached)
	scalar.MetricID = "approve-rate"
	distribution.MetricID = "approve-rate"
	points := []*drift.DriftSeriesPoint{
		testPoint("p-scalar", scalar.ID, now.Add(-1*time.Hour), 0.13, 0.08, drift.DriftSeriesPointStatusWarning),
		testPoint("p-dist", distribution.ID, now, 0, 0, drift.DriftSeriesPointStatusBreached),
	}
	points[1].SummaryStats = map[string]any{"approve": 0.71, "deny": 0.27, "refer": 0.02}
	points[1].BaselineStats = map[string]any{"approve": 0.80, "deny": 0.18, "refer": 0.02}
	svc := newTestService(t, []*drift.DriftDefinition{def}, []*drift.DriftSeries{scalar, distribution}, points, nil, nil)

	resp, err := svc.GetNodeAnalytics(context.Background(), DriftAnalyticsRequest{
		NodeKind: string(drift.TargetEntityKindDecisionSurface),
		NodeID:   "surf-x",
	})
	if err != nil {
		t.Fatalf("GetNodeAnalytics: %v", err)
	}
	if !resp.DataAvailable {
		t.Fatal("DataAvailable = false")
	}
	if resp.Chart.SeriesID != "ser-scalar" {
		t.Fatalf("SeriesID = %q, want ser-scalar", resp.Chart.SeriesID)
	}
}

func TestService_DistributionKeysOnlyAreUnavailable(t *testing.T) {
	now := testNow()
	def := testDefinition("def-a", drift.ThresholdDirectionAscending)
	series := testSeries("ser-a", def, drift.DriftSeriesStatusBreached)
	p := testPoint("p1", series.ID, now.Add(-time.Hour), 0, 0, drift.DriftSeriesPointStatusBreached)
	p.SummaryStats = map[string]any{"approve": 0.71, "deny": 0.27, "refer": 0.02}
	p.BaselineStats = map[string]any{"approve": 0.80, "deny": 0.18, "refer": 0.02}
	svc := newTestService(t, []*drift.DriftDefinition{def}, []*drift.DriftSeries{series}, []*drift.DriftSeriesPoint{p}, nil, nil)

	resp, err := svc.GetNodeAnalytics(context.Background(), DriftAnalyticsRequest{
		NodeKind: string(drift.TargetEntityKindDecisionSurface),
		NodeID:   "surf-x",
	})
	if err != nil {
		t.Fatalf("GetNodeAnalytics: %v", err)
	}
	if resp.DataAvailable {
		t.Fatal("DataAvailable = true for distribution-only stats")
	}
	if resp.SourceClassification.ExpectedBaseline != "unavailable" {
		t.Fatalf("ExpectedBaseline = %q", resp.SourceClassification.ExpectedBaseline)
	}
}

func TestService_DescendingThresholdsDoNotEmitMisleadingBands(t *testing.T) {
	now := testNow()
	def := testDefinition("def-a", drift.ThresholdDirectionDescending)
	series := testSeries("ser-a", def, drift.DriftSeriesStatusHealthy)
	p := testPoint("p1", series.ID, now.Add(-time.Hour), 0.91, 0.95, drift.DriftSeriesPointStatusHealthy)
	svc := newTestService(t, []*drift.DriftDefinition{def}, []*drift.DriftSeries{series}, []*drift.DriftSeriesPoint{p}, nil, nil)

	resp, err := svc.GetNodeAnalytics(context.Background(), DriftAnalyticsRequest{
		NodeKind: string(drift.TargetEntityKindDecisionSurface),
		NodeID:   "surf-x",
	})
	if err != nil {
		t.Fatalf("GetNodeAnalytics: %v", err)
	}
	if !resp.DataAvailable {
		t.Fatal("DataAvailable = false")
	}
	if resp.SourceClassification.Thresholds != "unavailable" {
		t.Fatalf("Thresholds = %q, want unavailable", resp.SourceClassification.Thresholds)
	}
	if len(resp.Chart.Watch) != 0 || len(resp.Chart.Breach) != 0 {
		t.Fatalf("descending thresholds must not emit compact bands: watch=%#v breach=%#v", resp.Chart.Watch, resp.Chart.Breach)
	}
}

func TestService_ResponseDoesNotEmitCompositeOrContributionValues(t *testing.T) {
	now := testNow()
	def := testDefinition("def-a", drift.ThresholdDirectionAscending)
	series := testSeries("ser-a", def, drift.DriftSeriesStatusHealthy)
	p := testPoint("p1", series.ID, now.Add(-time.Hour), 0.11, 0.08, drift.DriftSeriesPointStatusHealthy)
	svc := newTestService(t, []*drift.DriftDefinition{def}, []*drift.DriftSeries{series}, []*drift.DriftSeriesPoint{p}, nil, nil)

	resp, err := svc.GetNodeAnalytics(context.Background(), DriftAnalyticsRequest{
		NodeKind: string(drift.TargetEntityKindDecisionSurface),
		NodeID:   "surf-x",
	})
	if err != nil {
		t.Fatalf("GetNodeAnalytics: %v", err)
	}
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, banned := range []string{"currentScore", `"contributions"`, "contributors", "provisional"} {
		if strings.Contains(string(body), banned) {
			t.Fatalf("response must not emit %q: %s", banned, string(body))
		}
	}
}

func TestService_InvalidRequest(t *testing.T) {
	svc := newTestService(t, nil, nil, nil, nil, nil)
	for _, req := range []DriftAnalyticsRequest{
		{NodeID: "surf-x"},
		{NodeKind: string(drift.TargetEntityKindDecisionSurface)},
		{NodeKind: "weird", NodeID: "surf-x"},
	} {
		if _, err := svc.GetNodeAnalytics(context.Background(), req); err == nil {
			t.Fatalf("GetNodeAnalytics(%#v) err = nil, want validation error", req)
		}
	}
}

func TestService_TypedNilReturnsNotConfigured(t *testing.T) {
	var svc *Service
	var reader DriftAnalyticsReadService = svc
	_, err := reader.GetNodeAnalytics(context.Background(), DriftAnalyticsRequest{
		NodeKind: string(drift.TargetEntityKindDecisionSurface),
		NodeID:   "surf-x",
	})
	if err != ErrNotConfigured {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func newTestService(
	t *testing.T,
	defs []*drift.DriftDefinition,
	series []*drift.DriftSeries,
	points []*drift.DriftSeriesPoint,
	observations []*drift.DriftObservation,
	annotations []*drift.DriftAnnotation,
) *Service {
	t.Helper()
	defRepo := memory.NewDriftDefinitionRepo()
	seriesRepo := memory.NewDriftSeriesRepo()
	pointRepo := memory.NewDriftSeriesPointRepo()
	obsRepo := memory.NewDriftObservationRepo()
	annRepo := memory.NewDriftAnnotationRepo()
	for _, d := range defs {
		if err := defRepo.Create(context.Background(), d); err != nil {
			t.Fatalf("seed definition: %v", err)
		}
	}
	for _, s := range series {
		if err := seriesRepo.Create(context.Background(), s); err != nil {
			t.Fatalf("seed series: %v", err)
		}
	}
	for _, p := range points {
		if err := pointRepo.Create(context.Background(), p); err != nil {
			t.Fatalf("seed point: %v", err)
		}
	}
	for _, o := range observations {
		if err := obsRepo.Create(context.Background(), o); err != nil {
			t.Fatalf("seed observation: %v", err)
		}
	}
	for _, a := range annotations {
		if err := annRepo.Create(context.Background(), a); err != nil {
			t.Fatalf("seed annotation: %v", err)
		}
	}
	return NewService(defRepo, seriesRepo, pointRepo, obsRepo, annRepo, WithClock(testNow))
}

func testNow() time.Time {
	return time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)
}

func testDefinition(id string, direction drift.ThresholdDirection) *drift.DriftDefinition {
	now := testNow().Add(-24 * time.Hour)
	return &drift.DriftDefinition{
		ID:               id,
		Version:          1,
		Name:             id,
		Status:           drift.DriftDefinitionStatusActive,
		EffectiveDate:    now,
		TargetEntityKind: drift.TargetEntityKindDecisionSurface,
		TargetEntityID:   "surf-x",
		Origin:           drift.DriftOriginManual,
		Managed:          true,
		Metrics: []drift.DriftMetricDefinition{
			{
				MetricID:           "approve-rate",
				DriftType:          drift.DriftTypeOutcome,
				BaselineStrategy:   drift.BaselineStrategyRolling,
				WindowSeconds:      3600,
				Cadence:            drift.CadenceHour,
				WarningThreshold:   0.12,
				BreachedThreshold:  0.18,
				ThresholdDirection: direction,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func testSeries(id string, def *drift.DriftDefinition, status drift.DriftSeriesStatus) *drift.DriftSeries {
	now := testNow().Add(-24 * time.Hour)
	return &drift.DriftSeries{
		ID:                id,
		DefinitionID:      def.ID,
		DefinitionVersion: def.Version,
		MetricID:          def.Metrics[0].MetricID,
		Cadence:           drift.CadenceHour,
		Status:            status,
		ContinuityGroupID: id,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func testPoint(id, seriesID string, at time.Time, observed, expected float64, status drift.DriftSeriesPointStatus) *drift.DriftSeriesPoint {
	return &drift.DriftSeriesPoint{
		ID:          id,
		SeriesID:    seriesID,
		WindowStart: at,
		WindowEnd:   at.Add(time.Hour),
		SampleCount: 120,
		SummaryStats: map[string]any{
			"value": observed,
			"unit":  "ratio",
		},
		BaselineStats: map[string]any{
			"baseline": expected,
			"strategy": string(drift.BaselineStrategyRolling),
		},
		BaselineWindowID:     "baseline-1",
		Magnitude:            observed - expected,
		Status:               status,
		ComputationMode:      drift.DriftPointComputationModeRealtime,
		ComputedAt:           at.Add(time.Hour),
		SourceWindowComplete: true,
		CreatedAt:            at.Add(time.Hour),
	}
}

func testObservation(id string, def *drift.DriftDefinition, series *drift.DriftSeries, point *drift.DriftSeriesPoint) *drift.DriftObservation {
	return &drift.DriftObservation{
		ID:                  id,
		DefinitionID:        def.ID,
		DefinitionVersion:   def.Version,
		SeriesID:            series.ID,
		PointID:             point.ID,
		TargetEntityKind:    def.TargetEntityKind,
		TargetEntityID:      def.TargetEntityID,
		DriftType:           def.Metrics[0].DriftType,
		Magnitude:           point.Magnitude,
		DetectorStatus:      drift.DriftObservationDetectorStatusWarning,
		OperatorStatus:      drift.DriftObservationOperatorStatusOpen,
		BaselineWindowID:    point.BaselineWindowID,
		ObservedWindowStart: point.WindowStart,
		ObservedWindowEnd:   point.WindowEnd,
		DetectedAt:          point.WindowEnd,
		EmittedAt:           point.WindowEnd,
		CreatedAt:           point.WindowEnd,
		UpdatedAt:           point.WindowEnd,
	}
}

func testAnnotation(id string, kind drift.DriftAnnotationTargetKind, targetID string) *drift.DriftAnnotation {
	now := testNow()
	return &drift.DriftAnnotation{
		ID:             id,
		TargetKind:     kind,
		TargetID:       targetID,
		AnnotationType: drift.DriftAnnotationTypeRemediationNote,
		Body:           "operator note",
		Status:         drift.DriftAnnotationStatusActive,
		AuthorID:       "alice",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func hasString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
