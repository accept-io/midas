package memory

import (
	"context"
	"time"

	"github.com/accept-io/midas/internal/drift"
)

// DriftAnnotationRepo is an in-memory implementation of
// drift.DriftAnnotationRepository. Annotations are stored flat by
// stable ID; (TargetKind, TargetID) lookups walk the map.
type DriftAnnotationRepo struct {
	items map[string]*drift.DriftAnnotation
}

func NewDriftAnnotationRepo() *DriftAnnotationRepo {
	return &DriftAnnotationRepo{items: make(map[string]*drift.DriftAnnotation)}
}

func (r *DriftAnnotationRepo) Create(_ context.Context, a *drift.DriftAnnotation) error {
	r.items[a.ID] = a
	return nil
}

func (r *DriftAnnotationRepo) FindByID(_ context.Context, id string) (*drift.DriftAnnotation, error) {
	if a, ok := r.items[id]; ok {
		return a, nil
	}
	return nil, nil
}

func (r *DriftAnnotationRepo) ListByTarget(
	_ context.Context,
	kind drift.DriftAnnotationTargetKind,
	targetID string,
) ([]*drift.DriftAnnotation, error) {
	var out []*drift.DriftAnnotation
	for _, a := range r.items {
		if a.TargetKind == kind && a.TargetID == targetID {
			out = append(out, a)
		}
	}
	return out, nil
}

// Supersede sets Status=superseded, SupersededByID, and UpdatedAt on
// the named annotation. Silent no-op when the annotation ID does not
// exist.
func (r *DriftAnnotationRepo) Supersede(
	_ context.Context,
	annotationID string,
	supersededByID string,
	at time.Time,
) error {
	if a, ok := r.items[annotationID]; ok {
		a.Status = drift.DriftAnnotationStatusSuperseded
		a.SupersededByID = supersededByID
		a.UpdatedAt = at
	}
	return nil
}

var _ drift.DriftAnnotationRepository = (*DriftAnnotationRepo)(nil)
