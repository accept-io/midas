package postgres

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/drift"
)

// cleanupDriftAll wipes every drift table in the dependency order
// required by the FK chain. Drift-1b: this is the canonical reset for
// any drift-related Postgres test.
func cleanupDriftAll(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, q := range []string{
		`DELETE FROM drift_annotations`,
		`DELETE FROM drift_observations`,
		`DELETE FROM drift_series_points`,
		`DELETE FROM drift_series`,
		`DELETE FROM drift_metric_definitions`,
		`DELETE FROM drift_definitions`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("cleanup %q: %v", q, err)
		}
	}
}

func makeTestDefinition(id string, version int, status drift.DriftDefinitionStatus, t0 time.Time) *drift.DriftDefinition {
	return &drift.DriftDefinition{
		ID:               id,
		Version:          version,
		Name:             "Test " + id,
		Description:      "Postgres parity fixture.",
		Status:           status,
		EffectiveDate:    t0,
		BusinessOwner:    "owner@example.com",
		TechnicalOwner:   "platform-team",
		TargetEntityKind: drift.TargetEntityKindDecisionSurface,
		TargetEntityID:   "credit-approval",
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
				Description:        "PSI on outcome distribution.",
			},
		},
		CreatedAt: t0,
		UpdatedAt: t0,
		CreatedBy: "alice",
	}
}

func TestPostgresDriftDefinitionRepo_CreateAndFindByID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	r, err := NewDriftDefinitionRepo(db)
	if err != nil {
		t.Fatalf("NewDriftDefinitionRepo: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	v1 := makeTestDefinition("approve-rate-drift", 1, drift.DriftDefinitionStatusActive, now)
	v2 := makeTestDefinition("approve-rate-drift", 2, drift.DriftDefinitionStatusReview, now)

	if err := r.Create(ctx, v1); err != nil {
		t.Fatalf("Create v1: %v", err)
	}
	if err := r.Create(ctx, v2); err != nil {
		t.Fatalf("Create v2: %v", err)
	}

	got, err := r.FindByID(ctx, "approve-rate-drift")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil || got.Version != 2 {
		t.Fatalf("FindByID: want v2, got %+v", got)
	}
	if len(got.Metrics) != 1 {
		t.Errorf("expected 1 metric, got %d", len(got.Metrics))
	}
	if got.Metrics[0].MetricID != "outcome-psi" {
		t.Errorf("expected MetricID outcome-psi, got %q", got.Metrics[0].MetricID)
	}
	if got.Metrics[0].DriftType != drift.DriftTypeOutcome {
		t.Errorf("expected DriftType outcome, got %q", got.Metrics[0].DriftType)
	}
}

func TestPostgresDriftDefinitionRepo_FindByID_UnknownReturnsNil(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	r, _ := NewDriftDefinitionRepo(db)
	got, err := r.FindByID(context.Background(), "missing")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got != nil {
		t.Errorf("FindByID: want nil for missing id, got %+v", got)
	}
}

func TestPostgresDriftDefinitionRepo_FindByIDAndVersion(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	r, _ := NewDriftDefinitionRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	if err := r.Create(ctx, makeTestDefinition("approve-rate-drift", 1, drift.DriftDefinitionStatusActive, now)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := r.Create(ctx, makeTestDefinition("approve-rate-drift", 2, drift.DriftDefinitionStatusReview, now)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.FindByIDAndVersion(ctx, "approve-rate-drift", 1)
	if err != nil {
		t.Fatalf("FindByIDAndVersion: %v", err)
	}
	if got == nil || got.Version != 1 {
		t.Errorf("expected v1, got %+v", got)
	}

	missing, err := r.FindByIDAndVersion(ctx, "approve-rate-drift", 99)
	if err != nil {
		t.Fatalf("FindByIDAndVersion: %v", err)
	}
	if missing != nil {
		t.Errorf("missing version expected nil, got %+v", missing)
	}
}

func TestPostgresDriftDefinitionRepo_FindActiveAt(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	r, _ := NewDriftDefinitionRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	earlier := now.Add(-2 * time.Hour)

	v1 := makeTestDefinition("approve-rate-drift", 1, drift.DriftDefinitionStatusActive, earlier)
	until := now.Add(time.Hour)
	v1.EffectiveUntil = &until
	if err := r.Create(ctx, v1); err != nil {
		t.Fatalf("Create v1: %v", err)
	}

	v2 := makeTestDefinition("approve-rate-drift", 2, drift.DriftDefinitionStatusActive, now)
	if err := r.Create(ctx, v2); err != nil {
		t.Fatalf("Create v2: %v", err)
	}

	// Earlier point: only v1 is active.
	got, err := r.FindActiveAt(ctx, "approve-rate-drift", earlier.Add(time.Minute))
	if err != nil {
		t.Fatalf("FindActiveAt: %v", err)
	}
	if got == nil || got.Version != 1 {
		t.Errorf("expected v1 active, got %+v", got)
	}

	// Later point: both are active windows; highest Version wins.
	got, err = r.FindActiveAt(ctx, "approve-rate-drift", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("FindActiveAt: %v", err)
	}
	if got == nil || got.Version != 2 {
		t.Errorf("expected v2 (highest version), got %+v", got)
	}
}

func TestPostgresDriftDefinitionRepo_ListVersions_Descending(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	r, _ := NewDriftDefinitionRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	if err := r.Create(ctx, makeTestDefinition("approve-rate-drift", 1, drift.DriftDefinitionStatusDeprecated, now)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := r.Create(ctx, makeTestDefinition("approve-rate-drift", 2, drift.DriftDefinitionStatusActive, now)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := r.Create(ctx, makeTestDefinition("approve-rate-drift", 3, drift.DriftDefinitionStatusReview, now)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.ListVersions(ctx, "approve-rate-drift")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, want := range []int{3, 2, 1} {
		if got[i].Version != want {
			t.Errorf("got[%d].Version = %d, want %d", i, got[i].Version, want)
		}
	}

	empty, err := r.ListVersions(ctx, "missing")
	if err != nil {
		t.Fatalf("ListVersions missing: %v", err)
	}
	if empty == nil {
		t.Error("ListVersions missing should return non-nil empty slice")
	}
	if len(empty) != 0 {
		t.Errorf("ListVersions missing len = %d, want 0", len(empty))
	}
}

func TestPostgresDriftDefinitionRepo_Update_OnlyParentFields(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	r, _ := NewDriftDefinitionRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	original := makeTestDefinition("approve-rate-drift", 1, drift.DriftDefinitionStatusReview, now)
	if err := r.Create(ctx, original); err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated := makeTestDefinition("approve-rate-drift", 1, drift.DriftDefinitionStatusActive, now)
	approved := now.Add(time.Hour)
	updated.ApprovedBy = "bob"
	updated.ApprovedAt = &approved
	updated.UpdatedAt = approved
	// Sneak in a metric mutation; Update should NOT propagate it.
	updated.Metrics[0].WarningThreshold = 0.99
	if err := r.Update(ctx, updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := r.FindByIDAndVersion(ctx, "approve-rate-drift", 1)
	if err != nil {
		t.Fatalf("FindByIDAndVersion: %v", err)
	}
	if got.Status != drift.DriftDefinitionStatusActive {
		t.Errorf("Status = %q, want active", got.Status)
	}
	if got.ApprovedBy != "bob" {
		t.Errorf("ApprovedBy = %q, want bob", got.ApprovedBy)
	}
	if got.Metrics[0].WarningThreshold != 0.10 {
		t.Errorf("Update must not mutate child metrics; got WarningThreshold=%v want 0.10",
			got.Metrics[0].WarningThreshold)
	}
}

func TestPostgresDriftDefinitionRepo_Update_MissingRow_WrappedError(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	r, _ := NewDriftDefinitionRepo(db)
	now := time.Now().UTC().Truncate(time.Microsecond)
	d := makeTestDefinition("missing", 7, drift.DriftDefinitionStatusActive, now)

	err := r.Update(context.Background(), d)
	if err == nil {
		t.Fatal("Update on missing parent must return wrapped error")
	}
	if !strings.Contains(err.Error(), "drift_definition not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPostgresDriftDefinitionRepo_RejectsExcludedDriftType(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	r, _ := NewDriftDefinitionRepo(db)
	now := time.Now().UTC().Truncate(time.Microsecond)
	d := makeTestDefinition("v2-drift", 1, drift.DriftDefinitionStatusReview, now)
	// Sneak in a V2-deferred drift type. The Postgres CHECK constraint
	// must reject this even when the Go validator is bypassed.
	d.Metrics[0].DriftType = "population"

	err := r.Create(context.Background(), d)
	if err == nil {
		t.Fatal("Create must reject V2-deferred drift_type via CHECK constraint")
	}
	if !strings.Contains(err.Error(), "chk_drift_metric_drift_type") {
		t.Errorf("expected CHECK violation on chk_drift_metric_drift_type; got: %v", err)
	}
}
