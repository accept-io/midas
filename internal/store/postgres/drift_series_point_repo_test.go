package postgres

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/drift"
)

func makeTestPoint(id, seriesID string, windowStart time.Time, mode drift.DriftPointComputationMode) *drift.DriftSeriesPoint {
	p := &drift.DriftSeriesPoint{
		ID:                   id,
		SeriesID:             seriesID,
		WindowStart:          windowStart,
		WindowEnd:            windowStart.Add(time.Hour),
		SampleCount:          120,
		BaselineWindowID:     "b1",
		Magnitude:            0.05,
		Status:               drift.DriftSeriesPointStatusHealthy,
		ComputationMode:      mode,
		ComputedAt:           windowStart.Add(time.Hour),
		SourceWindowComplete: true,
		CreatedAt:            windowStart.Add(time.Hour),
	}
	if mode == drift.DriftPointComputationModeBackfilled {
		p.BackfillRunID = "run-abc"
	}
	return p
}

func TestPostgresDriftSeriesPointRepo_CreateAndFindByID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	defRepo, _ := NewDriftDefinitionRepo(db)
	seriesRepo, _ := NewDriftSeriesRepo(db)
	r, _ := NewDriftSeriesPointRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	seedDefinitionWithMetric(t, ctx, defRepo, "approve-rate-drift", 1, "outcome-psi", now)
	_ = seriesRepo.Create(ctx, makeTestSeries("ser-1", "approve-rate-drift", 1, "outcome-psi", "approve-rate-drift", now))

	p := makeTestPoint("p1", "ser-1", now, drift.DriftPointComputationModeRealtime)
	if err := r.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.FindByID(ctx, "p1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil || got.ID != "p1" {
		t.Errorf("FindByID returned %+v", got)
	}

	missing, _ := r.FindByID(ctx, "missing")
	if missing != nil {
		t.Errorf("missing should be nil; got %+v", missing)
	}
}

func TestPostgresDriftSeriesPointRepo_JSONBRoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	defRepo, _ := NewDriftDefinitionRepo(db)
	seriesRepo, _ := NewDriftSeriesRepo(db)
	r, _ := NewDriftSeriesPointRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	seedDefinitionWithMetric(t, ctx, defRepo, "approve-rate-drift", 1, "outcome-psi", now)
	_ = seriesRepo.Create(ctx, makeTestSeries("ser-1", "approve-rate-drift", 1, "outcome-psi", "approve-rate-drift", now))

	// Populated maps and slices.
	p := makeTestPoint("p-json", "ser-1", now, drift.DriftPointComputationModeRealtime)
	p.SummaryStats = map[string]any{
		"approve":   0.71,
		"deny":      0.27,
		"refer":     0.02,
		"meta":      map[string]any{"source": "envelopes"},
	}
	p.BaselineStats = map[string]any{
		"approve": 0.80,
		"deny":    0.18,
		"refer":   0.02,
	}
	p.ProvenanceEnvelopeIDs = []string{"env-1", "env-2", "env-3"}

	if err := r.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.FindByID(ctx, "p-json")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.SummaryStats["approve"].(float64) != 0.71 {
		t.Errorf("summary[approve] = %v, want 0.71", got.SummaryStats["approve"])
	}
	if meta, ok := got.SummaryStats["meta"].(map[string]any); !ok || meta["source"] != "envelopes" {
		t.Errorf("nested summary[meta] lost; got %+v", got.SummaryStats["meta"])
	}
	if !reflect.DeepEqual(got.ProvenanceEnvelopeIDs, []string{"env-1", "env-2", "env-3"}) {
		t.Errorf("ProvenanceEnvelopeIDs round-trip mismatch: %+v", got.ProvenanceEnvelopeIDs)
	}

	// Empty / nil containers must round-trip as non-nil empties.
	pEmpty := makeTestPoint("p-empty", "ser-1", now.Add(time.Hour), drift.DriftPointComputationModeRealtime)
	pEmpty.SummaryStats = nil
	pEmpty.BaselineStats = nil
	pEmpty.ProvenanceEnvelopeIDs = nil
	if err := r.Create(ctx, pEmpty); err != nil {
		t.Fatalf("Create empty: %v", err)
	}
	gotEmpty, _ := r.FindByID(ctx, "p-empty")
	if gotEmpty.SummaryStats == nil {
		t.Error("empty SummaryStats should round-trip as non-nil empty map")
	}
	if len(gotEmpty.SummaryStats) != 0 {
		t.Errorf("empty SummaryStats should be empty; got %+v", gotEmpty.SummaryStats)
	}
	if gotEmpty.ProvenanceEnvelopeIDs == nil {
		t.Error("empty ProvenanceEnvelopeIDs should round-trip as non-nil empty slice")
	}
	if len(gotEmpty.ProvenanceEnvelopeIDs) != 0 {
		t.Errorf("empty ProvenanceEnvelopeIDs should be empty; got %+v", gotEmpty.ProvenanceEnvelopeIDs)
	}
}

func TestPostgresDriftSeriesPointRepo_ListBySeries_Pagination(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	defRepo, _ := NewDriftDefinitionRepo(db)
	seriesRepo, _ := NewDriftSeriesRepo(db)
	r, _ := NewDriftSeriesPointRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	seedDefinitionWithMetric(t, ctx, defRepo, "approve-rate-drift", 1, "outcome-psi", now)
	_ = seriesRepo.Create(ctx, makeTestSeries("ser-1", "approve-rate-drift", 1, "outcome-psi", "approve-rate-drift", now))

	// Insert in scrambled order to confirm sort by window_start.
	for i, dt := range []time.Duration{2 * time.Hour, 0, time.Hour, 4 * time.Hour, 3 * time.Hour} {
		p := makeTestPoint(idForOffset(i), "ser-1", now.Add(dt), drift.DriftPointComputationModeRealtime)
		if err := r.Create(ctx, p); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	all, err := r.ListBySeries(ctx, "ser-1", now, 0)
	if err != nil {
		t.Fatalf("ListBySeries: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("len = %d, want 5", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].WindowStart.After(all[i].WindowStart) {
			t.Errorf("not ascending at index %d", i)
		}
	}

	later, err := r.ListBySeries(ctx, "ser-1", now.Add(2*time.Hour), 0)
	if err != nil {
		t.Fatalf("ListBySeries later: %v", err)
	}
	if len(later) != 3 {
		t.Errorf("len after fromWindow = %d, want 3", len(later))
	}

	limited, err := r.ListBySeries(ctx, "ser-1", now, 2)
	if err != nil {
		t.Fatalf("ListBySeries limited: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("len with limit=2 = %d, want 2", len(limited))
	}
}

func idForOffset(i int) string {
	return "p" + string(rune('a'+i))
}

func TestPostgresDriftSeriesPointRepo_ComputationModeAndBackfillRoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	defRepo, _ := NewDriftDefinitionRepo(db)
	seriesRepo, _ := NewDriftSeriesRepo(db)
	r, _ := NewDriftSeriesPointRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	seedDefinitionWithMetric(t, ctx, defRepo, "approve-rate-drift", 1, "outcome-psi", now)
	_ = seriesRepo.Create(ctx, makeTestSeries("ser-1", "approve-rate-drift", 1, "outcome-psi", "approve-rate-drift", now))

	for i, mode := range []drift.DriftPointComputationMode{
		drift.DriftPointComputationModeRealtime,
		drift.DriftPointComputationModeBackfilled,
		drift.DriftPointComputationModeCorrected,
		drift.DriftPointComputationModeImported,
	} {
		p := makeTestPoint("p-"+string(mode), "ser-1", now.Add(time.Duration(i)*time.Hour), mode)
		if err := r.Create(ctx, p); err != nil {
			t.Fatalf("Create %q: %v", mode, err)
		}
		got, _ := r.FindByID(ctx, "p-"+string(mode))
		if got.ComputationMode != mode {
			t.Errorf("ComputationMode = %q, want %q", got.ComputationMode, mode)
		}
		if mode == drift.DriftPointComputationModeBackfilled && got.BackfillRunID != "run-abc" {
			t.Errorf("BackfillRunID lost; got %q", got.BackfillRunID)
		}
	}
}

func TestPostgresDriftSeriesPointRepo_DeleteBefore(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupDriftAll(t, db)

	defRepo, _ := NewDriftDefinitionRepo(db)
	seriesRepo, _ := NewDriftSeriesRepo(db)
	r, _ := NewDriftSeriesPointRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	seedDefinitionWithMetric(t, ctx, defRepo, "approve-rate-drift", 1, "outcome-psi", now)
	_ = seriesRepo.Create(ctx, makeTestSeries("ser-1", "approve-rate-drift", 1, "outcome-psi", "approve-rate-drift", now))

	for i, dt := range []time.Duration{0, time.Hour, 2 * time.Hour, 3 * time.Hour} {
		p := makeTestPoint(idForOffset(i), "ser-1", now.Add(dt), drift.DriftPointComputationModeRealtime)
		_ = r.Create(ctx, p)
	}

	// Delete points whose WindowEnd is strictly before now+3h. Two
	// qualify: those with windows [t0, t0+1h) and [t0+1h, t0+2h).
	deleted, err := r.DeleteBefore(ctx, "ser-1", now.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("DeleteBefore: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}

	remaining, err := r.ListBySeries(ctx, "ser-1", now, 0)
	if err != nil {
		t.Fatalf("ListBySeries: %v", err)
	}
	if len(remaining) != 2 {
		t.Errorf("remaining len = %d, want 2", len(remaining))
	}
}
