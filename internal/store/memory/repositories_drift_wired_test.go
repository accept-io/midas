package memory

import "testing"

// TestRepositories_Wiring_Drift pins that the memory aggregator
// constructs non-nil repositories for all five drift fields. The
// aggregator fields are nil-safe (no runtime path consults them in
// Drift-1a), but tests that build a full Repositories via
// NewRepositories should always observe a working repository for
// each.
func TestRepositories_Wiring_Drift(t *testing.T) {
	repos := NewRepositories()
	if repos == nil {
		t.Fatal("NewRepositories returned nil")
	}
	if repos.DriftDefinitions == nil {
		t.Error("Repositories.DriftDefinitions must be non-nil after NewRepositories")
	}
	if repos.DriftSeries == nil {
		t.Error("Repositories.DriftSeries must be non-nil after NewRepositories")
	}
	if repos.DriftSeriesPoints == nil {
		t.Error("Repositories.DriftSeriesPoints must be non-nil after NewRepositories")
	}
	if repos.DriftObservations == nil {
		t.Error("Repositories.DriftObservations must be non-nil after NewRepositories")
	}
	if repos.DriftAnnotations == nil {
		t.Error("Repositories.DriftAnnotations must be non-nil after NewRepositories")
	}
}
