package drift

import (
	"testing"
	"time"
)

// validSeries returns a DriftSeries that passes ValidateSeries. Tests
// mutate one field at a time to assert the relevant continuity rule.
func validSeries() *DriftSeries {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	return &DriftSeries{
		ID:                "ser-1",
		DefinitionID:      "approve-rate-drift",
		DefinitionVersion: 1,
		MetricID:          "outcome-psi",
		Cadence:           CadenceHour,
		Status:            DriftSeriesStatusHealthy,
		ContinuityGroupID: "approve-rate-drift",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func TestValidateSeries_HappyPath(t *testing.T) {
	if errs := ValidateSeries(validSeries()); len(errs) != 0 {
		t.Errorf("expected no errors, got:\n%s", joinErrors(errs))
	}
}

func TestValidateSeries_ContinuityGroupID_Required(t *testing.T) {
	s := validSeries()
	s.ContinuityGroupID = ""
	if !containsSubstring(ValidateSeries(s), "ContinuityGroupID") {
		t.Error("empty ContinuityGroupID must be rejected")
	}
}

func TestValidateSeries_PreviousSeriesID_NotEqualOwn(t *testing.T) {
	s := validSeries()
	s.PreviousSeriesID = s.ID
	if !containsSubstring(ValidateSeries(s), "PreviousSeriesID") {
		t.Error("PreviousSeriesID == own ID must be rejected")
	}
}

func TestValidateSeries_SupersededBySeriesID_NotEqualOwn(t *testing.T) {
	s := validSeries()
	s.SupersededBySeriesID = s.ID
	if !containsSubstring(ValidateSeries(s), "SupersededBySeriesID") {
		t.Error("SupersededBySeriesID == own ID must be rejected")
	}
}

func TestValidateSeries_PreviousAndSupersededDistinct(t *testing.T) {
	s := validSeries()
	other := "ser-other"
	s.PreviousSeriesID = other
	s.SupersededBySeriesID = other
	if !containsSubstring(ValidateSeries(s),
		"PreviousSeriesID and SupersededBySeriesID must not be equal") {
		t.Error("equal Previous and SupersededBy must be rejected")
	}
}

func TestValidateSeries_SealedAtNotBeforeCreatedAt(t *testing.T) {
	s := validSeries()
	earlier := s.CreatedAt.Add(-time.Hour)
	s.SealedAt = &earlier
	if !containsSubstring(ValidateSeries(s), "SealedAt must not be before CreatedAt") {
		t.Error("SealedAt before CreatedAt must be rejected")
	}
}

func TestValidateSeries_NilRejected(t *testing.T) {
	if errs := ValidateSeries(nil); len(errs) == 0 {
		t.Error("ValidateSeries(nil) must return at least one error")
	}
}

func TestValidateSeries_InvalidStatus(t *testing.T) {
	s := validSeries()
	s.Status = "weird"
	if !containsSubstring(ValidateSeries(s), "Status") {
		t.Error("invalid Status must be rejected")
	}
}

func TestValidateSeries_InvalidCadence(t *testing.T) {
	s := validSeries()
	s.Cadence = "fortnight"
	if !containsSubstring(ValidateSeries(s), "Cadence") {
		t.Error("invalid Cadence must be rejected")
	}
}
