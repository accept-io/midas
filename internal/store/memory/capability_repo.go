package memory

import (
	"context"
	"sort"

	"github.com/accept-io/midas/internal/capability"
)

type CapabilityRepo struct {
	items map[string]*capability.Capability
}

func NewCapabilityRepo() *CapabilityRepo {
	return &CapabilityRepo{items: map[string]*capability.Capability{}}
}

func (r *CapabilityRepo) Exists(_ context.Context, id string) (bool, error) {
	_, ok := r.items[id]
	return ok, nil
}

func (r *CapabilityRepo) GetByID(_ context.Context, id string) (*capability.Capability, error) {
	return r.items[id], nil
}

func (r *CapabilityRepo) Create(_ context.Context, c *capability.Capability) error {
	r.items[c.ID] = c
	return nil
}

func (r *CapabilityRepo) Update(_ context.Context, c *capability.Capability) error {
	r.items[c.ID] = c
	return nil
}

func (r *CapabilityRepo) List(_ context.Context) ([]*capability.Capability, error) {
	out := make([]*capability.Capability, 0, len(r.items))
	for _, c := range r.items {
		out = append(out, c)
	}
	return out, nil
}

// ListByParentCapabilityID returns the direct children of parentID.
// Empty parentID short-circuits to an empty slice — a deliberate
// rejection of "all roots" semantics for this method (a future
// ListRoots can take that role if it ever becomes useful).
//
// Postgres iteration over the underlying map is non-deterministic, so
// results are sorted by ID ascending to match the postgres
// implementation's `ORDER BY capability_id` and to give a stable list
// to consumers (e.g. a future Explorer hierarchy view).
func (r *CapabilityRepo) ListByParentCapabilityID(_ context.Context, parentID string) ([]*capability.Capability, error) {
	out := make([]*capability.Capability, 0)
	if parentID == "" {
		return out, nil
	}
	for _, c := range r.items {
		if c.ParentCapabilityID == parentID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

var _ capability.CapabilityRepository = (*CapabilityRepo)(nil)
