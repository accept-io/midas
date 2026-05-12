package memory

import (
	"context"
	"sort"
	"time"

	"github.com/accept-io/midas/internal/drift"
)

// DriftSeriesPointRepo is an in-memory implementation of
// drift.DriftSeriesPointRepository. Points are indexed both by their
// stable ID and grouped by SeriesID so ListBySeries / DeleteBefore can
// scan in the natural order.
type DriftSeriesPointRepo struct {
	byID     map[string]*drift.DriftSeriesPoint
	bySeries map[string][]*drift.DriftSeriesPoint
}

func NewDriftSeriesPointRepo() *DriftSeriesPointRepo {
	return &DriftSeriesPointRepo{
		byID:     make(map[string]*drift.DriftSeriesPoint),
		bySeries: make(map[string][]*drift.DriftSeriesPoint),
	}
}

func (r *DriftSeriesPointRepo) Create(_ context.Context, p *drift.DriftSeriesPoint) error {
	r.byID[p.ID] = p
	r.bySeries[p.SeriesID] = append(r.bySeries[p.SeriesID], p)
	return nil
}

func (r *DriftSeriesPointRepo) FindByID(_ context.Context, id string) (*drift.DriftSeriesPoint, error) {
	if p, ok := r.byID[id]; ok {
		return p, nil
	}
	return nil, nil
}

// ListBySeries returns the points for the series whose WindowStart is
// >= fromWindow, sorted ascending by WindowStart. limit <= 0 is
// treated as no limit. Returns an empty slice (not nil) when no match.
func (r *DriftSeriesPointRepo) ListBySeries(
	_ context.Context,
	seriesID string,
	fromWindow time.Time,
	limit int,
) ([]*drift.DriftSeriesPoint, error) {
	src := r.bySeries[seriesID]
	out := make([]*drift.DriftSeriesPoint, 0, len(src))
	for _, p := range src {
		if p.WindowStart.Before(fromWindow) {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].WindowStart.Before(out[j].WindowStart)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// DeleteBefore removes points for the series whose WindowEnd is
// strictly before the given threshold. Returns the count deleted.
// Drift-1a does not exercise this from production code; it exists so
// the interface is fixed and Drift-1b's retention work doesn't churn
// the contract.
func (r *DriftSeriesPointRepo) DeleteBefore(_ context.Context, seriesID string, before time.Time) (int, error) {
	src := r.bySeries[seriesID]
	if len(src) == 0 {
		return 0, nil
	}
	kept := src[:0]
	deleted := 0
	for _, p := range src {
		if p.WindowEnd.Before(before) {
			delete(r.byID, p.ID)
			deleted++
			continue
		}
		kept = append(kept, p)
	}
	r.bySeries[seriesID] = kept
	return deleted, nil
}

var _ drift.DriftSeriesPointRepository = (*DriftSeriesPointRepo)(nil)
