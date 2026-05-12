package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/drift"
)

func dseries(id, defID string, defVer int, metricID, group string) *drift.DriftSeries {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	return &drift.DriftSeries{
		ID:                id,
		DefinitionID:      defID,
		DefinitionVersion: defVer,
		MetricID:          metricID,
		Cadence:           drift.CadenceHour,
		Status:            drift.DriftSeriesStatusHealthy,
		ContinuityGroupID: group,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func TestDriftSeriesRepo_CreateAndFindByID(t *testing.T) {
	ctx := context.Background()
	r := NewDriftSeriesRepo()
	s := dseries("ser-1", "approve", 1, "outcome-psi", "approve")
	_ = r.Create(ctx, s)

	got, err := r.FindByID(ctx, "ser-1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got == nil || got.ID != "ser-1" {
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

func TestDriftSeriesRepo_FindByDefinitionAndMetric(t *testing.T) {
	ctx := context.Background()
	r := NewDriftSeriesRepo()
	_ = r.Create(ctx, dseries("ser-1", "approve", 1, "outcome-psi", "approve"))
	_ = r.Create(ctx, dseries("ser-2", "approve", 2, "outcome-psi", "approve"))
	_ = r.Create(ctx, dseries("ser-3", "approve", 1, "latency-p95", "approve"))

	got, err := r.FindByDefinitionAndMetric(ctx, "approve", 2, "outcome-psi")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got == nil || got.ID != "ser-2" {
		t.Errorf("expected ser-2, got %+v", got)
	}

	missing, err := r.FindByDefinitionAndMetric(ctx, "approve", 9, "outcome-psi")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if missing != nil {
		t.Errorf("missing expected nil, got %+v", missing)
	}
}

func TestDriftSeriesRepo_ListByDefinition(t *testing.T) {
	ctx := context.Background()
	r := NewDriftSeriesRepo()
	_ = r.Create(ctx, dseries("ser-1", "approve", 1, "outcome-psi", "approve"))
	_ = r.Create(ctx, dseries("ser-2", "approve", 2, "outcome-psi", "approve"))
	_ = r.Create(ctx, dseries("ser-3", "other", 1, "outcome-psi", "other"))

	got, err := r.ListByDefinition(ctx, "approve")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestDriftSeriesRepo_ListByContinuityGroup(t *testing.T) {
	ctx := context.Background()
	r := NewDriftSeriesRepo()
	_ = r.Create(ctx, dseries("ser-1", "approve", 1, "outcome-psi", "approve"))
	_ = r.Create(ctx, dseries("ser-2", "approve", 2, "outcome-psi", "approve"))
	_ = r.Create(ctx, dseries("ser-3", "other", 1, "outcome-psi", "other"))

	got, err := r.ListByContinuityGroup(ctx, "approve")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("ListByContinuityGroup len = %d, want 2", len(got))
	}
}

func TestDriftSeriesRepo_UpdateStatus(t *testing.T) {
	ctx := context.Background()
	r := NewDriftSeriesRepo()
	_ = r.Create(ctx, dseries("ser-1", "approve", 1, "outcome-psi", "approve"))

	if err := r.UpdateStatus(ctx, "ser-1", drift.DriftSeriesStatusBreached); err != nil {
		t.Fatalf("UpdateStatus err = %v", err)
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

func TestDriftSeriesRepo_Seal(t *testing.T) {
	ctx := context.Background()
	r := NewDriftSeriesRepo()
	_ = r.Create(ctx, dseries("ser-1", "approve", 1, "outcome-psi", "approve"))

	at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if err := r.Seal(ctx, "ser-1", at); err != nil {
		t.Fatalf("Seal err = %v", err)
	}
	got, _ := r.FindByID(ctx, "ser-1")
	if got.SealedAt == nil || !got.SealedAt.Equal(at) {
		t.Errorf("SealedAt = %v, want %v", got.SealedAt, at)
	}
}
