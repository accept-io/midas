package postgres

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/drift"
)

func makeTestAnnotation(id string, kind drift.DriftAnnotationTargetKind, targetID string, t0 time.Time) *drift.DriftAnnotation {
	return &drift.DriftAnnotation{
		ID:             id,
		TargetKind:     kind,
		TargetID:       targetID,
		AnnotationType: drift.DriftAnnotationTypeRemediationNote,
		Body:           "rolled back model v17",
		Status:         drift.DriftAnnotationStatusActive,
		AuthorID:       "alice",
		CreatedAt:      t0,
		UpdatedAt:      t0,
	}
}

func TestPostgresDriftAnnotationRepo_CreateAndFindByID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	r, _ := NewDriftAnnotationRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	if err := r.Create(ctx, makeTestAnnotation("ann-1", drift.DriftAnnotationTargetKindObservation, "o1", now)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.FindByID(ctx, "ann-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil || got.ID != "ann-1" {
		t.Errorf("FindByID returned %+v", got)
	}

	missing, _ := r.FindByID(ctx, "missing")
	if missing != nil {
		t.Errorf("missing should be nil; got %+v", missing)
	}
}

func TestPostgresDriftAnnotationRepo_ReferenceEnvelopeIDsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	r, _ := NewDriftAnnotationRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	a := makeTestAnnotation("ann-json", drift.DriftAnnotationTargetKindSeries, "ser-1", now)
	a.ReferenceEnvelopeIDs = []string{"env-1", "env-2"}
	if err := r.Create(ctx, a); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := r.FindByID(ctx, "ann-json")
	if !reflect.DeepEqual(got.ReferenceEnvelopeIDs, []string{"env-1", "env-2"}) {
		t.Errorf("ReferenceEnvelopeIDs round-trip mismatch: %+v", got.ReferenceEnvelopeIDs)
	}

	aEmpty := makeTestAnnotation("ann-empty", drift.DriftAnnotationTargetKindSeries, "ser-1", now)
	aEmpty.ReferenceEnvelopeIDs = nil
	if err := r.Create(ctx, aEmpty); err != nil {
		t.Fatalf("Create empty: %v", err)
	}
	gotEmpty, _ := r.FindByID(ctx, "ann-empty")
	if gotEmpty.ReferenceEnvelopeIDs == nil {
		t.Error("nil ReferenceEnvelopeIDs should round-trip as non-nil empty slice")
	}
	if len(gotEmpty.ReferenceEnvelopeIDs) != 0 {
		t.Errorf("expected empty; got %+v", gotEmpty.ReferenceEnvelopeIDs)
	}
}

func TestPostgresDriftAnnotationRepo_ListByTarget(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	r, _ := NewDriftAnnotationRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	_ = r.Create(ctx, makeTestAnnotation("ann-1", drift.DriftAnnotationTargetKindObservation, "o1", now))
	_ = r.Create(ctx, makeTestAnnotation("ann-2", drift.DriftAnnotationTargetKindObservation, "o1", now.Add(time.Minute)))
	_ = r.Create(ctx, makeTestAnnotation("ann-3", drift.DriftAnnotationTargetKindSeries, "ser-1", now))

	gotO1, err := r.ListByTarget(ctx, drift.DriftAnnotationTargetKindObservation, "o1")
	if err != nil {
		t.Fatalf("ListByTarget: %v", err)
	}
	if len(gotO1) != 2 {
		t.Errorf("ListByTarget(observation, o1) len = %d, want 2", len(gotO1))
	}

	gotS1, _ := r.ListByTarget(ctx, drift.DriftAnnotationTargetKindSeries, "ser-1")
	if len(gotS1) != 1 {
		t.Errorf("ListByTarget(series, ser-1) len = %d, want 1", len(gotS1))
	}

	gotMissing, _ := r.ListByTarget(ctx, drift.DriftAnnotationTargetKindObservation, "missing")
	if len(gotMissing) != 0 {
		t.Errorf("ListByTarget(missing) len = %d, want 0", len(gotMissing))
	}
}

func TestPostgresDriftAnnotationRepo_Supersede(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	r, _ := NewDriftAnnotationRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	_ = r.Create(ctx, makeTestAnnotation("ann-1", drift.DriftAnnotationTargetKindSeries, "ser-1", now))
	_ = r.Create(ctx, makeTestAnnotation("ann-2", drift.DriftAnnotationTargetKindSeries, "ser-1", now.Add(time.Minute)))

	at := now.Add(time.Hour)
	if err := r.Supersede(ctx, "ann-1", "ann-2", at); err != nil {
		t.Fatalf("Supersede: %v", err)
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

	if err := r.Supersede(ctx, "missing", "ann-2", at); err != nil {
		t.Errorf("Supersede on missing should be silent no-op; got %v", err)
	}
}
