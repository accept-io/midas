package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/drift"
)

func dobs(id, defID string, defVer int, seriesID, pointID, entityID string, backfilled bool) *drift.DriftObservation {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	o := &drift.DriftObservation{
		ID:                  id,
		DefinitionID:        defID,
		DefinitionVersion:   defVer,
		SeriesID:            seriesID,
		PointID:             pointID,
		TargetEntityKind:    drift.TargetEntityKindDecisionSurface,
		TargetEntityID:      entityID,
		DriftType:           drift.DriftTypeOutcome,
		DetectorStatus:      drift.DriftObservationDetectorStatusBreached,
		OperatorStatus:      drift.DriftObservationOperatorStatusOpen,
		BaselineWindowID:    "b1",
		ObservedWindowStart: now,
		ObservedWindowEnd:   now.Add(time.Hour),
		DetectedAt:          now.Add(time.Hour),
		EmittedAt:           now.Add(time.Hour),
		Backfilled:          backfilled,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if backfilled {
		o.BackfillRunID = "run-abc"
	}
	return o
}

func TestDriftObservationRepo_CreateAndFindByID(t *testing.T) {
	ctx := context.Background()
	r := NewDriftObservationRepo()
	_ = r.Create(ctx, dobs("o1", "approve", 1, "ser-1", "p1", "credit-approval", false))

	got, err := r.FindByID(ctx, "o1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got == nil || got.ID != "o1" {
		t.Errorf("FindByID returned %+v", got)
	}

	missing, err := r.FindByID(ctx, "missing")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if missing != nil {
		t.Errorf("missing expected nil, got %+v", missing)
	}
}

func TestDriftObservationRepo_ListBySeries(t *testing.T) {
	ctx := context.Background()
	r := NewDriftObservationRepo()
	_ = r.Create(ctx, dobs("o1", "approve", 1, "ser-1", "p1", "credit-approval", false))
	_ = r.Create(ctx, dobs("o2", "approve", 1, "ser-1", "p2", "credit-approval", false))
	_ = r.Create(ctx, dobs("o3", "approve", 2, "ser-2", "p1", "credit-approval", false))

	got, err := r.ListBySeries(ctx, "ser-1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestDriftObservationRepo_ListByDefinition(t *testing.T) {
	ctx := context.Background()
	r := NewDriftObservationRepo()
	_ = r.Create(ctx, dobs("o1", "approve", 1, "ser-1", "p1", "credit-approval", false))
	_ = r.Create(ctx, dobs("o2", "approve", 2, "ser-2", "p1", "credit-approval", false))
	_ = r.Create(ctx, dobs("o3", "other", 1, "ser-x", "px", "other-surface", false))

	got, err := r.ListByDefinition(ctx, "approve")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestDriftObservationRepo_ListByEntity(t *testing.T) {
	ctx := context.Background()
	r := NewDriftObservationRepo()
	_ = r.Create(ctx, dobs("o1", "approve", 1, "ser-1", "p1", "credit-approval", false))
	_ = r.Create(ctx, dobs("o2", "other", 1, "ser-x", "px", "credit-approval", false))
	_ = r.Create(ctx, dobs("o3", "other", 1, "ser-y", "py", "fraud-screen", false))

	got, err := r.ListByEntity(ctx, drift.TargetEntityKindDecisionSurface, "credit-approval")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}

	got, err = r.ListByEntity(ctx, drift.TargetEntityKindAISystem, "credit-approval")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("kind mismatch should return empty; got %d", len(got))
	}
}

func TestDriftObservationRepo_UpdateOperatorStatus(t *testing.T) {
	ctx := context.Background()
	r := NewDriftObservationRepo()
	_ = r.Create(ctx, dobs("o1", "approve", 1, "ser-1", "p1", "credit-approval", false))

	if err := r.UpdateOperatorStatus(ctx, "o1", drift.DriftObservationOperatorStatusTriaged); err != nil {
		t.Fatalf("UpdateOperatorStatus err = %v", err)
	}
	got, _ := r.FindByID(ctx, "o1")
	if got.OperatorStatus != drift.DriftObservationOperatorStatusTriaged {
		t.Errorf("OperatorStatus = %q, want triaged", got.OperatorStatus)
	}
	// Detector-side status must be untouched.
	if got.DetectorStatus != drift.DriftObservationDetectorStatusBreached {
		t.Errorf("DetectorStatus changed unexpectedly: %q", got.DetectorStatus)
	}

	// Silent no-op on missing ID.
	if err := r.UpdateOperatorStatus(ctx, "missing", drift.DriftObservationOperatorStatusResolved); err != nil {
		t.Errorf("UpdateOperatorStatus on missing should be silent no-op; got %v", err)
	}
}

func TestDriftObservationRepo_BackfillFieldsRoundTrip(t *testing.T) {
	ctx := context.Background()
	r := NewDriftObservationRepo()
	_ = r.Create(ctx, dobs("o-bf", "approve", 1, "ser-1", "p1", "credit-approval", true))

	got, _ := r.FindByID(ctx, "o-bf")
	if !got.Backfilled {
		t.Error("Backfilled flag lost across round-trip")
	}
	if got.BackfillRunID != "run-abc" {
		t.Errorf("BackfillRunID = %q, want run-abc", got.BackfillRunID)
	}
}
