package postgres

import "testing"

// TestPostgresRepositories_Wiring_Drift pins that the Postgres
// repository builder constructs non-nil repositories for all five
// drift fields (Drift-1b). The aggregator fields are nil-safe — no
// runtime path consults them in this tranche — but tests that build a
// full Repositories via Store.Repositories() should always observe a
// working repository for each.
func TestPostgresRepositories_Wiring_Drift(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	store, err := NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	repos, err := store.Repositories()
	if err != nil {
		t.Fatalf("store.Repositories: %v", err)
	}
	if repos.DriftDefinitions == nil {
		t.Error("Repositories.DriftDefinitions must be non-nil")
	}
	if repos.DriftSeries == nil {
		t.Error("Repositories.DriftSeries must be non-nil")
	}
	if repos.DriftSeriesPoints == nil {
		t.Error("Repositories.DriftSeriesPoints must be non-nil")
	}
	if repos.DriftObservations == nil {
		t.Error("Repositories.DriftObservations must be non-nil")
	}
	if repos.DriftAnnotations == nil {
		t.Error("Repositories.DriftAnnotations must be non-nil")
	}
}
