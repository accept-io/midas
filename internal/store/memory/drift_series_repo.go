package memory

import (
	"context"
	"time"

	"github.com/accept-io/midas/internal/drift"
)

// DriftSeriesRepo is an in-memory implementation of
// drift.DriftSeriesRepository. Series are flat (no version history) —
// keyed directly by their stable ID. Continuity-group lookups walk
// the map.
type DriftSeriesRepo struct {
	items map[string]*drift.DriftSeries
}

func NewDriftSeriesRepo() *DriftSeriesRepo {
	return &DriftSeriesRepo{items: make(map[string]*drift.DriftSeries)}
}

func (r *DriftSeriesRepo) Create(_ context.Context, s *drift.DriftSeries) error {
	r.items[s.ID] = s
	return nil
}

func (r *DriftSeriesRepo) FindByID(_ context.Context, id string) (*drift.DriftSeries, error) {
	if s, ok := r.items[id]; ok {
		return s, nil
	}
	return nil, nil
}

// FindByDefinitionAndMetric resolves the (definition_id,
// definition_version, metric_id) tuple. Returns (nil, nil) when no
// row matches.
func (r *DriftSeriesRepo) FindByDefinitionAndMetric(
	_ context.Context,
	definitionID string,
	definitionVersion int,
	metricID string,
) (*drift.DriftSeries, error) {
	for _, s := range r.items {
		if s.DefinitionID == definitionID &&
			s.DefinitionVersion == definitionVersion &&
			s.MetricID == metricID {
			return s, nil
		}
	}
	return nil, nil
}

// ListByDefinition returns every series for the given logical
// definition ID across all revisions. Order is unspecified.
func (r *DriftSeriesRepo) ListByDefinition(_ context.Context, definitionID string) ([]*drift.DriftSeries, error) {
	var out []*drift.DriftSeries
	for _, s := range r.items {
		if s.DefinitionID == definitionID {
			out = append(out, s)
		}
	}
	return out, nil
}

// ListByContinuityGroup returns every series sharing the supplied
// ContinuityGroupID. Order is unspecified — callers sort if needed.
func (r *DriftSeriesRepo) ListByContinuityGroup(_ context.Context, groupID string) ([]*drift.DriftSeries, error) {
	var out []*drift.DriftSeries
	for _, s := range r.items {
		if s.ContinuityGroupID == groupID {
			out = append(out, s)
		}
	}
	return out, nil
}

// UpdateStatus mutates only the rolled-up Status. Silent no-op when
// the series ID does not exist (matches the package convention).
func (r *DriftSeriesRepo) UpdateStatus(_ context.Context, seriesID string, status drift.DriftSeriesStatus) error {
	if s, ok := r.items[seriesID]; ok {
		s.Status = status
	}
	return nil
}

// Seal sets SealedAt. Silent no-op when the series ID does not exist.
func (r *DriftSeriesRepo) Seal(_ context.Context, seriesID string, at time.Time) error {
	if s, ok := r.items[seriesID]; ok {
		t := at
		s.SealedAt = &t
	}
	return nil
}

var _ drift.DriftSeriesRepository = (*DriftSeriesRepo)(nil)
