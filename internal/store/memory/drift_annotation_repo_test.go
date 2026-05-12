package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/drift"
)

func dann(id string, kind drift.DriftAnnotationTargetKind, targetID string) *drift.DriftAnnotation {
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

func TestDriftAnnotationRepo_CreateAndFindByID(t *testing.T) {
	ctx := context.Background()
	r := NewDriftAnnotationRepo()
	_ = r.Create(ctx, dann("ann-1", drift.DriftAnnotationTargetKindObservation, "o1"))

	got, err := r.FindByID(ctx, "ann-1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got == nil || got.ID != "ann-1" {
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

func TestDriftAnnotationRepo_ListByTarget(t *testing.T) {
	ctx := context.Background()
	r := NewDriftAnnotationRepo()
	_ = r.Create(ctx, dann("ann-1", drift.DriftAnnotationTargetKindObservation, "o1"))
	_ = r.Create(ctx, dann("ann-2", drift.DriftAnnotationTargetKindObservation, "o1"))
	_ = r.Create(ctx, dann("ann-3", drift.DriftAnnotationTargetKindSeries, "ser-1"))

	got, err := r.ListByTarget(ctx, drift.DriftAnnotationTargetKindObservation, "o1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("ListByTarget(observation, o1) len = %d, want 2", len(got))
	}

	got, err = r.ListByTarget(ctx, drift.DriftAnnotationTargetKindSeries, "ser-1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 {
		t.Errorf("ListByTarget(series, ser-1) len = %d, want 1", len(got))
	}

	got, err = r.ListByTarget(ctx, drift.DriftAnnotationTargetKindObservation, "missing")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListByTarget(missing) len = %d, want 0", len(got))
	}
}

func TestDriftAnnotationRepo_Supersede(t *testing.T) {
	ctx := context.Background()
	r := NewDriftAnnotationRepo()
	_ = r.Create(ctx, dann("ann-1", drift.DriftAnnotationTargetKindSeries, "ser-1"))
	_ = r.Create(ctx, dann("ann-2", drift.DriftAnnotationTargetKindSeries, "ser-1"))

	at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if err := r.Supersede(ctx, "ann-1", "ann-2", at); err != nil {
		t.Fatalf("Supersede err = %v", err)
	}

	got, _ := r.FindByID(ctx, "ann-1")
	if got.Status != drift.DriftAnnotationStatusSuperseded {
		t.Errorf("Status = %q, want superseded", got.Status)
	}
	if got.SupersededByID != "ann-2" {
		t.Errorf("SupersededByID = %q, want ann-2", got.SupersededByID)
	}
	if !got.UpdatedAt.Equal(at) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, at)
	}

	// Silent no-op on missing.
	if err := r.Supersede(ctx, "missing", "ann-2", at); err != nil {
		t.Errorf("Supersede on missing should be silent no-op; got %v", err)
	}
}
