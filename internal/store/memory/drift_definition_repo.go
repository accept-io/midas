package memory

import (
	"context"
	"time"

	"github.com/accept-io/midas/internal/drift"
)

// DriftDefinitionRepo is an in-memory implementation of
// drift.DriftDefinitionRepository. The implementation mirrors the
// existing versioned authority/control artefact pattern (ProfileRepo,
// FailModePolicyRepo) — keyed by logical ID with a slice of versions
// ordered by insertion (which matches monotonically increasing Version
// numbers assigned by the caller).
//
// Concurrency: not goroutine-safe (matches the package convention).
// Defensive copies: not made (matches the package convention).
type DriftDefinitionRepo struct {
	// items maps logical definition ID → versions in ascending-version
	// order. The last element is the latest version.
	items map[string][]*drift.DriftDefinition
}

// NewDriftDefinitionRepo constructs an empty in-memory repository.
func NewDriftDefinitionRepo() *DriftDefinitionRepo {
	return &DriftDefinitionRepo{items: make(map[string][]*drift.DriftDefinition)}
}

// FindByID returns the latest version (highest Version) for the
// logical definition ID. Returns (nil, nil) when no rows exist.
func (r *DriftDefinitionRepo) FindByID(_ context.Context, id string) (*drift.DriftDefinition, error) {
	versions := r.items[id]
	if len(versions) == 0 {
		return nil, nil
	}
	return versions[len(versions)-1], nil
}

// FindByIDAndVersion returns the exact (id, version) pair. Returns
// (nil, nil) when the pair does not exist.
func (r *DriftDefinitionRepo) FindByIDAndVersion(_ context.Context, id string, version int) (*drift.DriftDefinition, error) {
	for _, d := range r.items[id] {
		if d.Version == version {
			return d, nil
		}
	}
	return nil, nil
}

// FindActiveAt returns the version active at the given time:
// Status==active, EffectiveDate <= at, EffectiveUntil IS NULL OR
// EffectiveUntil > at. When multiple versions satisfy the predicate
// (an invariant violation), the highest Version wins.
func (r *DriftDefinitionRepo) FindActiveAt(_ context.Context, id string, at time.Time) (*drift.DriftDefinition, error) {
	var best *drift.DriftDefinition
	for _, d := range r.items[id] {
		if d.Status != drift.DriftDefinitionStatusActive {
			continue
		}
		if d.EffectiveDate.After(at) {
			continue
		}
		if d.EffectiveUntil != nil && !d.EffectiveUntil.After(at) {
			continue
		}
		if best == nil || d.Version > best.Version {
			best = d
		}
	}
	return best, nil
}

// ListVersions returns versions in descending Version order. Returns
// an empty slice (not nil) when the ID has no rows.
func (r *DriftDefinitionRepo) ListVersions(_ context.Context, id string) ([]*drift.DriftDefinition, error) {
	versions := r.items[id]
	if len(versions) == 0 {
		return []*drift.DriftDefinition{}, nil
	}
	out := make([]*drift.DriftDefinition, len(versions))
	for i, d := range versions {
		out[len(versions)-1-i] = d
	}
	return out, nil
}

// Create appends a new version for the definition's logical ID. The
// caller assigns a monotonically increasing Version number.
func (r *DriftDefinitionRepo) Create(_ context.Context, d *drift.DriftDefinition) error {
	r.items[d.ID] = append(r.items[d.ID], d)
	return nil
}

// Update replaces the matching (ID, Version) entry in place. Silent
// no-op when the (ID, Version) does not exist (matches ProfileRepo
// and FailModePolicyRepo posture).
func (r *DriftDefinitionRepo) Update(_ context.Context, d *drift.DriftDefinition) error {
	for i, existing := range r.items[d.ID] {
		if existing.Version == d.Version {
			r.items[d.ID][i] = d
			return nil
		}
	}
	return nil
}

var _ drift.DriftDefinitionRepository = (*DriftDefinitionRepo)(nil)
