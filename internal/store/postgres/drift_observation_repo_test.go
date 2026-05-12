package postgres

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/drift"
)

// seedSeriesWithPoint inserts a definition + series + point so the
// observation FKs (series_id, point_id) resolve.
func seedSeriesWithPoint(t *testing.T, ctx context.Context, db dbWithRepos, defID string, defVer int, metricID, seriesID, pointID string, t0 time.Time) {
	t.Helper()
	d := makeTestDefinition(defID, defVer, drift.DriftDefinitionStatusActive, t0)
	d.Metrics[0].MetricID = metricID
	if err := db.def.Create(ctx, d); err != nil {
		t.Fatalf("seed def Create: %v", err)
	}
	if err := db.series.Create(ctx, makeTestSeries(seriesID, defID, defVer, metricID, defID, t0)); err != nil {
		t.Fatalf("seed series Create: %v", err)
	}
	if err := db.points.Create(ctx, makeTestPoint(pointID, seriesID, t0, drift.DriftPointComputationModeRealtime)); err != nil {
		t.Fatalf("seed point Create: %v", err)
	}
}

type dbWithRepos struct {
	def    *DriftDefinitionRepo
	series *DriftSeriesRepo
	points *DriftSeriesPointRepo
}

func makeTestObservation(id, defID string, defVer int, seriesID, pointID, entityID string, backfilled bool, t0 time.Time) *drift.DriftObservation {
	o := &drift.DriftObservation{
		ID:                  id,
		DefinitionID:        defID,
		DefinitionVersion:   defVer,
		SeriesID:            seriesID,
		PointID:             pointID,
		TargetEntityKind:    drift.TargetEntityKindDecisionSurface,
		TargetEntityID:      entityID,
		DriftType:           drift.DriftTypeOutcome,
		Magnitude:           0.25,
		DetectorStatus:      drift.DriftObservationDetectorStatusBreached,
		OperatorStatus:      drift.DriftObservationOperatorStatusOpen,
		BaselineWindowID:    "b1",
		ObservedWindowStart: t0,
		ObservedWindowEnd:   t0.Add(time.Hour),
		DetectedAt:          t0.Add(time.Hour),
		EmittedAt:           t0.Add(time.Hour),
		Backfilled:          backfilled,
		CreatedAt:           t0,
		UpdatedAt:           t0,
	}
	if backfilled {
		o.BackfillRunID = "run-abc"
	}
	return o
}

func TestPostgresDriftObservationRepo_CreateAndFindByID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	defRepo, _ := NewDriftDefinitionRepo(db)
	seriesRepo, _ := NewDriftSeriesRepo(db)
	pointRepo, _ := NewDriftSeriesPointRepo(db)
	obsRepo, _ := NewDriftObservationRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	seeds := dbWithRepos{def: defRepo, series: seriesRepo, points: pointRepo}
	seedSeriesWithPoint(t, ctx, seeds, "approve-rate-drift", 1, "outcome-psi", "ser-1", "p1", now)

	if err := obsRepo.Create(ctx, makeTestObservation("o1", "approve-rate-drift", 1, "ser-1", "p1", "credit-approval", false, now)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := obsRepo.FindByID(ctx, "o1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil || got.ID != "o1" {
		t.Errorf("FindByID returned %+v", got)
	}

	missing, _ := obsRepo.FindByID(ctx, "missing")
	if missing != nil {
		t.Errorf("missing should be nil; got %+v", missing)
	}
}

func TestPostgresDriftObservationRepo_EvidenceEnvelopeIDsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	defRepo, _ := NewDriftDefinitionRepo(db)
	seriesRepo, _ := NewDriftSeriesRepo(db)
	pointRepo, _ := NewDriftSeriesPointRepo(db)
	obsRepo, _ := NewDriftObservationRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	seeds := dbWithRepos{def: defRepo, series: seriesRepo, points: pointRepo}
	seedSeriesWithPoint(t, ctx, seeds, "approve-rate-drift", 1, "outcome-psi", "ser-1", "p1", now)

	o := makeTestObservation("o-json", "approve-rate-drift", 1, "ser-1", "p1", "credit-approval", false, now)
	o.EvidenceEnvelopeIDs = []string{"env-1", "env-2"}
	if err := obsRepo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, _ := obsRepo.FindByID(ctx, "o-json")
	if !reflect.DeepEqual(got.EvidenceEnvelopeIDs, []string{"env-1", "env-2"}) {
		t.Errorf("EvidenceEnvelopeIDs round-trip mismatch: %+v", got.EvidenceEnvelopeIDs)
	}

	// Empty / nil round-trip.
	oEmpty := makeTestObservation("o-empty", "approve-rate-drift", 1, "ser-1", "p1", "credit-approval", false, now)
	oEmpty.EvidenceEnvelopeIDs = nil
	if err := obsRepo.Create(ctx, oEmpty); err != nil {
		t.Fatalf("Create empty: %v", err)
	}
	gotEmpty, _ := obsRepo.FindByID(ctx, "o-empty")
	if gotEmpty.EvidenceEnvelopeIDs == nil {
		t.Error("empty EvidenceEnvelopeIDs should round-trip as non-nil empty slice")
	}
	if len(gotEmpty.EvidenceEnvelopeIDs) != 0 {
		t.Errorf("expected empty; got %+v", gotEmpty.EvidenceEnvelopeIDs)
	}
}

func TestPostgresDriftObservationRepo_ListByX(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	defRepo, _ := NewDriftDefinitionRepo(db)
	seriesRepo, _ := NewDriftSeriesRepo(db)
	pointRepo, _ := NewDriftSeriesPointRepo(db)
	obsRepo, _ := NewDriftObservationRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	seeds := dbWithRepos{def: defRepo, series: seriesRepo, points: pointRepo}
	seedSeriesWithPoint(t, ctx, seeds, "approve-rate-drift", 1, "outcome-psi", "ser-1", "p1", now)

	// Add a second point on the same series so we can have two
	// observations with distinct point_ids.
	if err := pointRepo.Create(ctx, makeTestPoint("p2", "ser-1", now.Add(time.Hour), drift.DriftPointComputationModeRealtime)); err != nil {
		t.Fatalf("seed p2: %v", err)
	}

	_ = obsRepo.Create(ctx, makeTestObservation("o1", "approve-rate-drift", 1, "ser-1", "p1", "credit-approval", false, now))
	_ = obsRepo.Create(ctx, makeTestObservation("o2", "approve-rate-drift", 1, "ser-1", "p2", "credit-approval", false, now.Add(time.Hour)))

	bySeries, err := obsRepo.ListBySeries(ctx, "ser-1")
	if err != nil {
		t.Fatalf("ListBySeries: %v", err)
	}
	if len(bySeries) != 2 {
		t.Errorf("ListBySeries len = %d, want 2", len(bySeries))
	}

	byDef, err := obsRepo.ListByDefinition(ctx, "approve-rate-drift")
	if err != nil {
		t.Fatalf("ListByDefinition: %v", err)
	}
	if len(byDef) != 2 {
		t.Errorf("ListByDefinition len = %d, want 2", len(byDef))
	}

	byEntity, err := obsRepo.ListByEntity(ctx, drift.TargetEntityKindDecisionSurface, "credit-approval")
	if err != nil {
		t.Fatalf("ListByEntity: %v", err)
	}
	if len(byEntity) != 2 {
		t.Errorf("ListByEntity len = %d, want 2", len(byEntity))
	}

	otherKind, _ := obsRepo.ListByEntity(ctx, drift.TargetEntityKindAISystem, "credit-approval")
	if len(otherKind) != 0 {
		t.Errorf("kind mismatch should return 0; got %d", len(otherKind))
	}
}

func TestPostgresDriftObservationRepo_UpdateOperatorStatus_DoesNotChangeDetector(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	defRepo, _ := NewDriftDefinitionRepo(db)
	seriesRepo, _ := NewDriftSeriesRepo(db)
	pointRepo, _ := NewDriftSeriesPointRepo(db)
	obsRepo, _ := NewDriftObservationRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	seeds := dbWithRepos{def: defRepo, series: seriesRepo, points: pointRepo}
	seedSeriesWithPoint(t, ctx, seeds, "approve-rate-drift", 1, "outcome-psi", "ser-1", "p1", now)

	_ = obsRepo.Create(ctx, makeTestObservation("o1", "approve-rate-drift", 1, "ser-1", "p1", "credit-approval", false, now))

	if err := obsRepo.UpdateOperatorStatus(ctx, "o1", drift.DriftObservationOperatorStatusTriaged); err != nil {
		t.Fatalf("UpdateOperatorStatus: %v", err)
	}
	got, _ := obsRepo.FindByID(ctx, "o1")
	if got.OperatorStatus != drift.DriftObservationOperatorStatusTriaged {
		t.Errorf("OperatorStatus = %q, want triaged", got.OperatorStatus)
	}
	if got.DetectorStatus != drift.DriftObservationDetectorStatusBreached {
		t.Errorf("DetectorStatus must be unchanged; got %q", got.DetectorStatus)
	}

	// Silent no-op on missing.
	if err := obsRepo.UpdateOperatorStatus(ctx, "missing", drift.DriftObservationOperatorStatusResolved); err != nil {
		t.Errorf("UpdateOperatorStatus on missing should be silent no-op; got %v", err)
	}
}

func TestPostgresDriftObservationRepo_BackfillFieldsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	defRepo, _ := NewDriftDefinitionRepo(db)
	seriesRepo, _ := NewDriftSeriesRepo(db)
	pointRepo, _ := NewDriftSeriesPointRepo(db)
	obsRepo, _ := NewDriftObservationRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	seeds := dbWithRepos{def: defRepo, series: seriesRepo, points: pointRepo}
	seedSeriesWithPoint(t, ctx, seeds, "approve-rate-drift", 1, "outcome-psi", "ser-1", "p1", now)

	_ = obsRepo.Create(ctx, makeTestObservation("o-bf", "approve-rate-drift", 1, "ser-1", "p1", "credit-approval", true, now))

	got, _ := obsRepo.FindByID(ctx, "o-bf")
	if !got.Backfilled {
		t.Error("Backfilled flag lost across round-trip")
	}
	if got.BackfillRunID != "run-abc" {
		t.Errorf("BackfillRunID = %q, want run-abc", got.BackfillRunID)
	}
}
