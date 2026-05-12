package memory

import (
	"context"

	"github.com/accept-io/midas/internal/drift"
)

// DriftObservationRepo is an in-memory implementation of
// drift.DriftObservationRepository. Observations are stored flat by
// stable ID; ListByX walks the map.
type DriftObservationRepo struct {
	items map[string]*drift.DriftObservation
}

func NewDriftObservationRepo() *DriftObservationRepo {
	return &DriftObservationRepo{items: make(map[string]*drift.DriftObservation)}
}

func (r *DriftObservationRepo) Create(_ context.Context, o *drift.DriftObservation) error {
	r.items[o.ID] = o
	return nil
}

func (r *DriftObservationRepo) FindByID(_ context.Context, id string) (*drift.DriftObservation, error) {
	if o, ok := r.items[id]; ok {
		return o, nil
	}
	return nil, nil
}

func (r *DriftObservationRepo) ListBySeries(_ context.Context, seriesID string) ([]*drift.DriftObservation, error) {
	var out []*drift.DriftObservation
	for _, o := range r.items {
		if o.SeriesID == seriesID {
			out = append(out, o)
		}
	}
	return out, nil
}

func (r *DriftObservationRepo) ListByDefinition(_ context.Context, definitionID string) ([]*drift.DriftObservation, error) {
	var out []*drift.DriftObservation
	for _, o := range r.items {
		if o.DefinitionID == definitionID {
			out = append(out, o)
		}
	}
	return out, nil
}

func (r *DriftObservationRepo) ListByEntity(
	_ context.Context,
	kind drift.TargetEntityKind,
	entityID string,
) ([]*drift.DriftObservation, error) {
	var out []*drift.DriftObservation
	for _, o := range r.items {
		if o.TargetEntityKind == kind && o.TargetEntityID == entityID {
			out = append(out, o)
		}
	}
	return out, nil
}

// UpdateOperatorStatus mutates only OperatorStatus. The detector-side
// DetectorStatus is left untouched. Silent no-op when the observation
// ID does not exist.
func (r *DriftObservationRepo) UpdateOperatorStatus(
	_ context.Context,
	observationID string,
	status drift.DriftObservationOperatorStatus,
) error {
	if o, ok := r.items[observationID]; ok {
		o.OperatorStatus = status
	}
	return nil
}

var _ drift.DriftObservationRepository = (*DriftObservationRepo)(nil)
