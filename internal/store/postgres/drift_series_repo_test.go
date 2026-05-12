package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/drift"
)

// seedDefinitionWithMetric inserts a definition revision with one metric
// so the drift_series FK target exists for series-test rows.
func seedDefinitionWithMetric(t *testing.T, ctx context.Context, defRepo *DriftDefinitionRepo, defID string, defVer int, metricID string, t0 time.Time) {
	t.Helper()
	d := makeTestDefinition(defID, defVer, drift.DriftDefinitionStatusActive, t0)
	d.Metrics[0].MetricID = metricID
	if err := defRepo.Create(ctx, d); err != nil {
		t.Fatalf("seed Create: %v", err)
	}
}

// seedDefinitionWithMetrics inserts a single definition revision
// carrying multiple metrics. Used when a test needs more than one
// drift_metric_definitions row under the same (definition_id,
// definition_version) parent — the atomic-revision invariant means
// multiple metrics live inside ONE definition revision rather than as
// separate revisions.
func seedDefinitionWithMetrics(t *testing.T, ctx context.Context, defRepo *DriftDefinitionRepo, defID string, defVer int, metricIDs []string, t0 time.Time) {
	t.Helper()
	d := makeTestDefinition(defID, defVer, drift.DriftDefinitionStatusActive, t0)
	if len(metricIDs) == 0 {
		t.Fatal("seedDefinitionWithMetrics requires at least one metric ID")
	}
	d.Metrics = make([]drift.DriftMetricDefinition, 0, len(metricIDs))
	for _, m := range metricIDs {
		d.Metrics = append(d.Metrics, drift.DriftMetricDefinition{
			MetricID:           m,
			DriftType:          drift.DriftTypeOutcome,
			BaselineStrategy:   drift.BaselineStrategyFixedGoverned,
			WindowSeconds:      3600,
			Cadence:            drift.CadenceHour,
			WarningThreshold:   0.10,
			BreachedThreshold:  0.20,
			ThresholdDirection: drift.ThresholdDirectionAscending,
		})
	}
	if err := defRepo.Create(ctx, d); err != nil {
		t.Fatalf("seedDefinitionWithMetrics: %v", err)
	}
}

func makeTestSeries(id, defID string, defVer int, metricID, group string, t0 time.Time) *drift.DriftSeries {
	return &drift.DriftSeries{
		ID:                id,
		DefinitionID:      defID,
		DefinitionVersion: defVer,
		MetricID:          metricID,
		Cadence:           drift.CadenceHour,
		Status:            drift.DriftSeriesStatusHealthy,
		ContinuityGroupID: group,
		CreatedAt:         t0,
		UpdatedAt:         t0,
	}
}

func TestPostgresDriftSeriesRepo_CreateAndFindByID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	defRepo, _ := NewDriftDefinitionRepo(db)
	r, _ := NewDriftSeriesRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	seedDefinitionWithMetric(t, ctx, defRepo, "approve-rate-drift", 1, "outcome-psi", now)

	s := makeTestSeries("ser-1", "approve-rate-drift", 1, "outcome-psi", "approve-rate-drift", now)
	if err := r.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.FindByID(ctx, "ser-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil || got.ID != "ser-1" {
		t.Errorf("FindByID returned %+v", got)
	}

	missing, err := r.FindByID(ctx, "missing")
	if err != nil {
		t.Fatalf("FindByID missing: %v", err)
	}
	if missing != nil {
		t.Errorf("missing should be nil; got %+v", missing)
	}
}

func TestPostgresDriftSeriesRepo_FindByDefinitionAndMetric(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	defRepo, _ := NewDriftDefinitionRepo(db)
	r, _ := NewDriftSeriesRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// v1 carries both outcome-psi and latency-p95 in the same atomic
	// revision; v2 carries outcome-psi only. This gives three distinct
	// (definition_id, definition_version, metric_id) tuples to attach
	// series to without duplicating the parent definition row.
	seedDefinitionWithMetrics(t, ctx, defRepo, "approve-rate-drift", 1, []string{"outcome-psi", "latency-p95"}, now)
	seedDefinitionWithMetric(t, ctx, defRepo, "approve-rate-drift", 2, "outcome-psi", now)

	if err := r.Create(ctx, makeTestSeries("ser-1", "approve-rate-drift", 1, "outcome-psi", "approve-rate-drift", now)); err != nil {
		t.Fatalf("Create ser-1: %v", err)
	}
	if err := r.Create(ctx, makeTestSeries("ser-2", "approve-rate-drift", 2, "outcome-psi", "approve-rate-drift", now)); err != nil {
		t.Fatalf("Create ser-2: %v", err)
	}
	if err := r.Create(ctx, makeTestSeries("ser-3", "approve-rate-drift", 1, "latency-p95", "approve-rate-drift", now)); err != nil {
		t.Fatalf("Create ser-3: %v", err)
	}

	got, err := r.FindByDefinitionAndMetric(ctx, "approve-rate-drift", 2, "outcome-psi")
	if err != nil {
		t.Fatalf("FindByDefinitionAndMetric: %v", err)
	}
	if got == nil || got.ID != "ser-2" {
		t.Errorf("expected ser-2; got %+v", got)
	}

	missing, err := r.FindByDefinitionAndMetric(ctx, "approve-rate-drift", 9, "outcome-psi")
	if err != nil {
		t.Fatalf("FindByDefinitionAndMetric missing: %v", err)
	}
	if missing != nil {
		t.Errorf("missing should be nil; got %+v", missing)
	}
}

func TestPostgresDriftSeriesRepo_ListByDefinitionAndContinuity(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	defRepo, _ := NewDriftDefinitionRepo(db)
	r, _ := NewDriftSeriesRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	seedDefinitionWithMetric(t, ctx, defRepo, "approve-rate-drift", 1, "outcome-psi", now)
	seedDefinitionWithMetric(t, ctx, defRepo, "approve-rate-drift", 2, "outcome-psi", now)
	seedDefinitionWithMetric(t, ctx, defRepo, "other-drift", 1, "outcome-psi", now)

	_ = r.Create(ctx, makeTestSeries("ser-1", "approve-rate-drift", 1, "outcome-psi", "approve-rate-drift", now))
	_ = r.Create(ctx, makeTestSeries("ser-2", "approve-rate-drift", 2, "outcome-psi", "approve-rate-drift", now))
	_ = r.Create(ctx, makeTestSeries("ser-3", "other-drift", 1, "outcome-psi", "other-drift", now))

	byDef, err := r.ListByDefinition(ctx, "approve-rate-drift")
	if err != nil {
		t.Fatalf("ListByDefinition: %v", err)
	}
	if len(byDef) != 2 {
		t.Errorf("ListByDefinition len = %d, want 2", len(byDef))
	}

	byGroup, err := r.ListByContinuityGroup(ctx, "approve-rate-drift")
	if err != nil {
		t.Fatalf("ListByContinuityGroup: %v", err)
	}
	if len(byGroup) != 2 {
		t.Errorf("ListByContinuityGroup len = %d, want 2", len(byGroup))
	}
}

func TestPostgresDriftSeriesRepo_UpdateStatus(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	defRepo, _ := NewDriftDefinitionRepo(db)
	r, _ := NewDriftSeriesRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	seedDefinitionWithMetric(t, ctx, defRepo, "approve-rate-drift", 1, "outcome-psi", now)
	_ = r.Create(ctx, makeTestSeries("ser-1", "approve-rate-drift", 1, "outcome-psi", "approve-rate-drift", now))

	if err := r.UpdateStatus(ctx, "ser-1", drift.DriftSeriesStatusBreached); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, _ := r.FindByID(ctx, "ser-1")
	if got.Status != drift.DriftSeriesStatusBreached {
		t.Errorf("Status = %q, want breached", got.Status)
	}

	// Silent no-op on missing.
	if err := r.UpdateStatus(ctx, "missing", drift.DriftSeriesStatusBreached); err != nil {
		t.Errorf("UpdateStatus on missing should be silent no-op; got %v", err)
	}
}

func TestPostgresDriftSeriesRepo_Seal(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	defRepo, _ := NewDriftDefinitionRepo(db)
	r, _ := NewDriftSeriesRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	seedDefinitionWithMetric(t, ctx, defRepo, "approve-rate-drift", 1, "outcome-psi", now)
	_ = r.Create(ctx, makeTestSeries("ser-1", "approve-rate-drift", 1, "outcome-psi", "approve-rate-drift", now))

	at := now.Add(time.Hour)
	if err := r.Seal(ctx, "ser-1", at); err != nil {
		t.Fatalf("Seal: %v", err)
	}
	got, _ := r.FindByID(ctx, "ser-1")
	if got.SealedAt == nil || !got.SealedAt.Equal(at) {
		t.Errorf("SealedAt = %v, want %v", got.SealedAt, at)
	}

	if err := r.Seal(ctx, "missing", at); err != nil {
		t.Errorf("Seal on missing should be silent no-op; got %v", err)
	}
}
