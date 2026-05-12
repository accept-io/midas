package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/drift"
)

func dpoint(id, seriesID string, windowStart time.Time, mode drift.DriftPointComputationMode) *drift.DriftSeriesPoint {
	return &drift.DriftSeriesPoint{
		ID:               id,
		SeriesID:         seriesID,
		WindowStart:      windowStart,
		WindowEnd:        windowStart.Add(time.Hour),
		BaselineWindowID: "b1",
		Status:           drift.DriftSeriesPointStatusHealthy,
		ComputationMode:  mode,
		ComputedAt:       windowStart.Add(time.Hour),
		BackfillRunID:    backfillRunForMode(mode),
	}
}

func backfillRunForMode(mode drift.DriftPointComputationMode) string {
	if mode == drift.DriftPointComputationModeBackfilled {
		return "run-abc"
	}
	return ""
}

func TestDriftSeriesPointRepo_CreateAndFindByID(t *testing.T) {
	ctx := context.Background()
	r := NewDriftSeriesPointRepo()
	t0 := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	p := dpoint("p1", "ser-1", t0, drift.DriftPointComputationModeRealtime)
	_ = r.Create(ctx, p)

	got, err := r.FindByID(ctx, "p1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got == nil || got.ID != "p1" {
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

func TestDriftSeriesPointRepo_ListBySeries_Pagination(t *testing.T) {
	ctx := context.Background()
	r := NewDriftSeriesPointRepo()
	t0 := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Insert in scrambled order to confirm sort by WindowStart.
	for _, dt := range []time.Duration{2 * time.Hour, 0, time.Hour, 4 * time.Hour, 3 * time.Hour} {
		p := dpoint("p"+dt.String(), "ser-1", t0.Add(dt), drift.DriftPointComputationModeRealtime)
		_ = r.Create(ctx, p)
	}

	all, err := r.ListBySeries(ctx, "ser-1", t0, 0)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("ListBySeries len = %d, want 5", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].WindowStart.After(all[i].WindowStart) {
			t.Errorf("not ascending at index %d: %v > %v",
				i, all[i-1].WindowStart, all[i].WindowStart)
		}
	}

	// fromWindow filter excludes earlier windows.
	later, err := r.ListBySeries(ctx, "ser-1", t0.Add(2*time.Hour), 0)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(later) != 3 {
		t.Errorf("len after fromWindow = %d, want 3", len(later))
	}

	// limit caps the result.
	limited, err := r.ListBySeries(ctx, "ser-1", t0, 2)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("len with limit=2 = %d, want 2", len(limited))
	}
}

func TestDriftSeriesPointRepo_ComputationModeRoundTrip(t *testing.T) {
	ctx := context.Background()
	r := NewDriftSeriesPointRepo()
	t0 := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	for _, mode := range []drift.DriftPointComputationMode{
		drift.DriftPointComputationModeRealtime,
		drift.DriftPointComputationModeBackfilled,
		drift.DriftPointComputationModeCorrected,
		drift.DriftPointComputationModeImported,
	} {
		p := dpoint("p-"+string(mode), "ser-1", t0, mode)
		_ = r.Create(ctx, p)
		got, _ := r.FindByID(ctx, "p-"+string(mode))
		if got.ComputationMode != mode {
			t.Errorf("ComputationMode = %q, want %q", got.ComputationMode, mode)
		}
		if mode == drift.DriftPointComputationModeBackfilled && got.BackfillRunID == "" {
			t.Errorf("BackfillRunID lost across round-trip for backfilled mode")
		}
	}
}

func TestDriftSeriesPointRepo_DeleteBefore(t *testing.T) {
	ctx := context.Background()
	r := NewDriftSeriesPointRepo()
	t0 := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	for _, dt := range []time.Duration{0, time.Hour, 2 * time.Hour, 3 * time.Hour} {
		_ = r.Create(ctx, dpoint("p"+dt.String(), "ser-1", t0.Add(dt), drift.DriftPointComputationModeRealtime))
	}

	// Delete points whose WindowEnd is strictly before t0+3h. The
	// first two points have windows [t0, t0+1h) and [t0+1h, t0+2h),
	// so their WindowEnds (t0+1h and t0+2h) qualify. The third point
	// has WindowEnd = t0+3h which is NOT strictly before t0+3h.
	deleted, err := r.DeleteBefore(ctx, "ser-1", t0.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}
	remaining, _ := r.ListBySeries(ctx, "ser-1", t0, 0)
	if len(remaining) != 2 {
		t.Errorf("remaining len = %d, want 2", len(remaining))
	}
}
