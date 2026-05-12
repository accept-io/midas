package drift

import (
	"strings"
	"testing"
	"time"
)

// validDefinition returns a definition that passes Validate. Tests
// mutate one field at a time to assert the relevant validation.
func validDefinition() *DriftDefinition {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	return &DriftDefinition{
		ID:               "approve-rate-drift",
		Version:          1,
		Name:             "Approve rate drift",
		Description:      "Outcome-distribution drift on the approve outcome.",
		Status:           DriftDefinitionStatusDraft,
		EffectiveDate:    now,
		BusinessOwner:    "risk-ops",
		TechnicalOwner:   "platform-team",
		TargetEntityKind: TargetEntityKindDecisionSurface,
		TargetEntityID:   "credit-approval",
		Metrics: []DriftMetricDefinition{
			validMetric("outcome-psi"),
		},
		Origin:    DriftOriginManual,
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "alice",
	}
}

func validMetric(metricID string) DriftMetricDefinition {
	return DriftMetricDefinition{
		MetricID:           metricID,
		DriftType:          DriftTypeOutcome,
		BaselineStrategy:   BaselineStrategyFixedGoverned,
		WindowSeconds:      3600,
		Cadence:            CadenceHour,
		WarningThreshold:   0.10,
		BreachedThreshold:  0.20,
		ThresholdDirection: ThresholdDirectionAscending,
		Description:        "PSI on outcome distribution.",
	}
}

func TestValidate_HappyPath(t *testing.T) {
	if errs := Validate(validDefinition()); len(errs) != 0 {
		t.Errorf("expected no errors, got %d:\n%v", len(errs), errs)
	}
}

func TestValidate_NilRejected(t *testing.T) {
	if errs := Validate(nil); len(errs) == 0 {
		t.Error("Validate(nil) must return at least one error")
	}
}

func TestValidate_RejectsExcludedDriftTypes(t *testing.T) {
	for _, banned := range []DriftType{"population", "data", "prediction", "performance", "concept"} {
		t.Run(string(banned), func(t *testing.T) {
			d := validDefinition()
			d.Metrics[0].DriftType = banned
			errs := Validate(d)
			if len(errs) == 0 {
				t.Fatalf("DriftType %q must be rejected by Validate", banned)
			}
			joined := joinErrors(errs)
			if !strings.Contains(joined, string(banned)) {
				t.Errorf("error message must name the rejected type %q; got: %s", banned, joined)
			}
			if !strings.Contains(joined, "V2") && !strings.Contains(joined, "Drift-1a") {
				t.Errorf("error message should mention V2 / Drift-1a deferral; got: %s", joined)
			}
		})
	}
}

func TestValidate_RejectsExcludedBaselineStrategies(t *testing.T) {
	for _, banned := range []BaselineStrategy{"champion_challenger", "seasonality_aware"} {
		t.Run(string(banned), func(t *testing.T) {
			d := validDefinition()
			d.Metrics[0].BaselineStrategy = banned
			errs := Validate(d)
			if len(errs) == 0 {
				t.Fatalf("BaselineStrategy %q must be rejected by Validate", banned)
			}
		})
	}
}

func TestValidate_FieldErrors(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*DriftDefinition)
		want   string
	}{
		{"empty ID", func(d *DriftDefinition) { d.ID = "" }, "ID must not be empty"},
		{"non-kebab ID", func(d *DriftDefinition) { d.ID = "Approve_Rate" }, "kebab-case"},
		{"zero version", func(d *DriftDefinition) { d.Version = 0 }, "Version must be > 0"},
		{"empty name", func(d *DriftDefinition) { d.Name = "" }, "Name must not be empty"},
		{"empty business owner", func(d *DriftDefinition) { d.BusinessOwner = "" }, "BusinessOwner"},
		{"empty technical owner", func(d *DriftDefinition) { d.TechnicalOwner = "" }, "TechnicalOwner"},
		{"invalid status", func(d *DriftDefinition) { d.Status = "weird" }, "Status"},
		{"zero effective date", func(d *DriftDefinition) { d.EffectiveDate = time.Time{} }, "EffectiveDate"},
		{"effective_until before effective_date", func(d *DriftDefinition) {
			t := d.EffectiveDate.Add(-time.Hour)
			d.EffectiveUntil = &t
		}, "EffectiveUntil must be after EffectiveDate"},
		{"retired_at before effective_date", func(d *DriftDefinition) {
			t := d.EffectiveDate.Add(-time.Hour)
			d.RetiredAt = &t
		}, "RetiredAt"},
		{"invalid origin", func(d *DriftDefinition) { d.Origin = "weird" }, "Origin"},
		{"replaces self", func(d *DriftDefinition) { d.Replaces = d.ID }, "Replaces"},
		{"invalid target kind", func(d *DriftDefinition) { d.TargetEntityKind = "weird" }, "TargetEntityKind"},
		{"empty target id", func(d *DriftDefinition) { d.TargetEntityID = "" }, "TargetEntityID"},
		{"zero created at", func(d *DriftDefinition) { d.CreatedAt = time.Time{} }, "CreatedAt"},
		{"zero updated at", func(d *DriftDefinition) { d.UpdatedAt = time.Time{} }, "UpdatedAt"},
		{"empty created by", func(d *DriftDefinition) { d.CreatedBy = "" }, "CreatedBy"},
		{"no metrics", func(d *DriftDefinition) { d.Metrics = nil }, "at least one"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := validDefinition()
			tc.mutate(d)
			errs := Validate(d)
			if !containsSubstring(errs, tc.want) {
				t.Errorf("expected error containing %q, got:\n%s", tc.want, joinErrors(errs))
			}
		})
	}
}

func TestValidate_DuplicateMetricIDs_Rejected(t *testing.T) {
	d := validDefinition()
	d.Metrics = []DriftMetricDefinition{
		validMetric("outcome-psi"),
		validMetric("outcome-psi"),
	}
	errs := Validate(d)
	if !containsSubstring(errs, "duplicate MetricID") {
		t.Errorf("expected duplicate-metric error, got:\n%s", joinErrors(errs))
	}
}

func TestValidate_ThresholdBandCoherence(t *testing.T) {
	cases := []struct {
		name      string
		direction ThresholdDirection
		warning   float64
		breached  float64
		valid     bool
	}{
		{"ascending warning < breached", ThresholdDirectionAscending, 0.10, 0.20, true},
		{"ascending warning >= breached", ThresholdDirectionAscending, 0.30, 0.20, false},
		{"descending warning > breached", ThresholdDirectionDescending, 0.95, 0.85, true},
		{"descending warning <= breached", ThresholdDirectionDescending, 0.85, 0.95, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := validDefinition()
			d.Metrics[0].ThresholdDirection = tc.direction
			d.Metrics[0].WarningThreshold = tc.warning
			d.Metrics[0].BreachedThreshold = tc.breached
			errs := Validate(d)
			if tc.valid && containsSubstring(errs, "Threshold") {
				t.Errorf("expected no threshold error, got:\n%s", joinErrors(errs))
			}
			if !tc.valid && !containsSubstring(errs, "Threshold") {
				t.Errorf("expected threshold-coherence error, got:\n%s", joinErrors(errs))
			}
		})
	}
}

func TestValidate_MetricFieldErrors(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*DriftMetricDefinition)
		want   string
	}{
		{"empty metric id", func(m *DriftMetricDefinition) { m.MetricID = "" }, "MetricID"},
		{"non-kebab metric id", func(m *DriftMetricDefinition) { m.MetricID = "Outcome_PSI" }, "kebab-case"},
		{"invalid drift type", func(m *DriftMetricDefinition) { m.DriftType = "weird" }, "DriftType"},
		{"invalid baseline strategy", func(m *DriftMetricDefinition) { m.BaselineStrategy = "weird" }, "BaselineStrategy"},
		{"zero window seconds", func(m *DriftMetricDefinition) { m.WindowSeconds = 0 }, "WindowSeconds"},
		{"negative baseline window seconds", func(m *DriftMetricDefinition) { m.BaselineWindowSeconds = -1 }, "BaselineWindowSeconds"},
		{"invalid cadence", func(m *DriftMetricDefinition) { m.Cadence = "fortnight" }, "Cadence"},
		{"invalid threshold direction", func(m *DriftMetricDefinition) { m.ThresholdDirection = "weird" }, "ThresholdDirection"},
		{"negative governance expectation version", func(m *DriftMetricDefinition) {
			m.GovernanceExpectationRef = "expectation-x"
			m.GovernanceExpectationVer = -1
		}, "GovernanceExpectationVer"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validMetric("metric-x")
			tc.mutate(&m)
			errs := ValidateMetric(m)
			if !containsSubstring(errs, tc.want) {
				t.Errorf("expected error containing %q, got:\n%s", tc.want, joinErrors(errs))
			}
		})
	}
}

func TestValidatePoint_BackfillFields(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	base := &DriftSeriesPoint{
		ID:               "p1",
		SeriesID:         "s1",
		WindowStart:      now,
		WindowEnd:        now.Add(time.Hour),
		BaselineWindowID: "b1",
		Status:           DriftSeriesPointStatusHealthy,
		ComputationMode:  DriftPointComputationModeRealtime,
		ComputedAt:       now.Add(time.Hour),
	}
	if errs := ValidatePoint(base); len(errs) != 0 {
		t.Errorf("base realtime point must validate cleanly; got:\n%s", joinErrors(errs))
	}

	// Backfilled requires BackfillRunID.
	bf := *base
	bf.ComputationMode = DriftPointComputationModeBackfilled
	if !containsSubstring(ValidatePoint(&bf), "BackfillRunID") {
		t.Error("ComputationMode=backfilled without BackfillRunID must be rejected")
	}

	bf.BackfillRunID = "run-abc"
	if errs := ValidatePoint(&bf); len(errs) != 0 {
		t.Errorf("backfilled point with BackfillRunID must validate; got:\n%s", joinErrors(errs))
	}
}

func TestValidatePoint_BaselineWindowIDRequiredUnlessInsufficientData(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	for _, status := range []DriftSeriesPointStatus{
		DriftSeriesPointStatusHealthy,
		DriftSeriesPointStatusWarning,
		DriftSeriesPointStatusBreached,
		DriftSeriesPointStatusUnknownDetectorError,
	} {
		p := &DriftSeriesPoint{
			ID:              "p1",
			SeriesID:        "s1",
			WindowStart:     now,
			WindowEnd:       now.Add(time.Hour),
			Status:          status,
			ComputationMode: DriftPointComputationModeRealtime,
			ComputedAt:      now.Add(time.Hour),
			// BaselineWindowID intentionally empty.
		}
		if !containsSubstring(ValidatePoint(p), "BaselineWindowID") {
			t.Errorf("status %q must require BaselineWindowID", status)
		}
	}

	// unknown_insufficient_data is the one exception.
	p := &DriftSeriesPoint{
		ID:              "p1",
		SeriesID:        "s1",
		WindowStart:     now,
		WindowEnd:       now.Add(time.Hour),
		Status:          DriftSeriesPointStatusUnknownInsufficientData,
		ComputationMode: DriftPointComputationModeRealtime,
		ComputedAt:      now.Add(time.Hour),
	}
	if containsSubstring(ValidatePoint(p), "BaselineWindowID") {
		t.Errorf("status unknown_insufficient_data must NOT require BaselineWindowID; got:\n%s",
			joinErrors(ValidatePoint(p)))
	}
}

func TestValidatePoint_WindowOrdering(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	p := &DriftSeriesPoint{
		ID:               "p1",
		SeriesID:         "s1",
		WindowStart:      now.Add(time.Hour),
		WindowEnd:        now,
		BaselineWindowID: "b1",
		Status:           DriftSeriesPointStatusHealthy,
		ComputationMode:  DriftPointComputationModeRealtime,
		ComputedAt:       now,
	}
	if !containsSubstring(ValidatePoint(p), "WindowStart must be before WindowEnd") {
		t.Error("WindowStart >= WindowEnd must be rejected")
	}
}

func TestValidateObservation_BackfilledImpliesBackfillRunID(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	base := &DriftObservation{
		ID:                  "o1",
		DefinitionID:        "approve-rate-drift",
		DefinitionVersion:   1,
		SeriesID:            "s1",
		PointID:             "p1",
		TargetEntityKind:    TargetEntityKindDecisionSurface,
		TargetEntityID:     "credit-approval",
		DriftType:          DriftTypeOutcome,
		DetectorStatus:     DriftObservationDetectorStatusBreached,
		OperatorStatus:     DriftObservationOperatorStatusOpen,
		BaselineWindowID:   "b1",
		ObservedWindowStart: now,
		ObservedWindowEnd:   now.Add(time.Hour),
		DetectedAt:          now.Add(time.Hour),
		EmittedAt:           now.Add(time.Hour),
	}
	if errs := ValidateObservation(base); len(errs) != 0 {
		t.Errorf("base observation must validate; got:\n%s", joinErrors(errs))
	}

	bf := *base
	bf.Backfilled = true
	if !containsSubstring(ValidateObservation(&bf), "BackfillRunID") {
		t.Error("Backfilled=true without BackfillRunID must be rejected")
	}
	bf.BackfillRunID = "run-abc"
	if errs := ValidateObservation(&bf); len(errs) != 0 {
		t.Errorf("backfilled observation with BackfillRunID must validate; got:\n%s", joinErrors(errs))
	}
}

func TestValidateObservation_TimestampOrdering(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		mutate func(*DriftObservation)
		want   string
	}{
		{"window swapped", func(o *DriftObservation) {
			o.ObservedWindowStart, o.ObservedWindowEnd = o.ObservedWindowEnd, o.ObservedWindowStart
		}, "ObservedWindowStart must be before ObservedWindowEnd"},
		{"detected_at zero", func(o *DriftObservation) { o.DetectedAt = time.Time{} }, "DetectedAt"},
		{"emitted_at zero", func(o *DriftObservation) { o.EmittedAt = time.Time{} }, "EmittedAt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := &DriftObservation{
				ID:                  "o1",
				DefinitionID:        "approve-rate-drift",
				DefinitionVersion:   1,
				SeriesID:            "s1",
				PointID:             "p1",
				TargetEntityKind:    TargetEntityKindDecisionSurface,
				TargetEntityID:      "credit-approval",
				DriftType:           DriftTypeOutcome,
				DetectorStatus:      DriftObservationDetectorStatusBreached,
				OperatorStatus:      DriftObservationOperatorStatusOpen,
				BaselineWindowID:    "b1",
				ObservedWindowStart: now,
				ObservedWindowEnd:   now.Add(time.Hour),
				DetectedAt:          now.Add(time.Hour),
				EmittedAt:           now.Add(time.Hour),
			}
			tc.mutate(o)
			if !containsSubstring(ValidateObservation(o), tc.want) {
				t.Errorf("expected error containing %q, got:\n%s", tc.want, joinErrors(ValidateObservation(o)))
			}
		})
	}
}

func TestValidateObservation_NoSelfCorrection(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	o := &DriftObservation{
		ID:                  "o1",
		DefinitionID:        "approve-rate-drift",
		DefinitionVersion:   1,
		SeriesID:            "s1",
		PointID:             "p1",
		TargetEntityKind:    TargetEntityKindDecisionSurface,
		TargetEntityID:      "credit-approval",
		DriftType:           DriftTypeOutcome,
		DetectorStatus:      DriftObservationDetectorStatusBreached,
		OperatorStatus:      DriftObservationOperatorStatusOpen,
		BaselineWindowID:    "b1",
		ObservedWindowStart: now,
		ObservedWindowEnd:   now.Add(time.Hour),
		DetectedAt:          now.Add(time.Hour),
		EmittedAt:           now.Add(time.Hour),
		CorrectionOf:        "o1",
	}
	if !containsSubstring(ValidateObservation(o), "CorrectionOf") {
		t.Error("CorrectionOf == ID must be rejected")
	}
}

func TestValidateAnnotation_SuppressionUntilFuture(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)

	a := &DriftAnnotation{
		ID:               "ann-1",
		TargetKind:       DriftAnnotationTargetKindObservation,
		TargetID:         "o1",
		AnnotationType:   DriftAnnotationTypeSuppression,
		Body:             "campaign x runs 2026-01-15 → 02-01",
		Status:           DriftAnnotationStatusActive,
		AuthorID:         "alice",
		SuppressionUntil: &past,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if !containsSubstring(ValidateAnnotation(a, now), "SuppressionUntil") {
		t.Error("SuppressionUntil in the past must be rejected")
	}

	future := now.Add(time.Hour)
	a.SuppressionUntil = &future
	if errs := ValidateAnnotation(a, now); len(errs) != 0 {
		t.Errorf("annotation with future SuppressionUntil must validate; got:\n%s", joinErrors(errs))
	}
}

func TestValidateAnnotation_NoSelfSupersession(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	a := &DriftAnnotation{
		ID:             "ann-1",
		TargetKind:     DriftAnnotationTargetKindSeries,
		TargetID:       "s1",
		AnnotationType: DriftAnnotationTypeRemediationNote,
		Body:           "rolled back model v17",
		Status:         DriftAnnotationStatusActive,
		AuthorID:       "alice",
		SupersededByID: "ann-1",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if !containsSubstring(ValidateAnnotation(a, now), "SupersededByID") {
		t.Error("SupersededByID == ID must be rejected")
	}
}

// joinErrors collapses []error into a single string for substring checks.
func joinErrors(errs []error) string {
	if len(errs) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, e := range errs {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(e.Error())
	}
	return sb.String()
}

func containsSubstring(errs []error, want string) bool {
	return strings.Contains(joinErrors(errs), want)
}
