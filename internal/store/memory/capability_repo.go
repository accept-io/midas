package memory

import (
	"context"

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

var _ capability.CapabilityRepository = (*CapabilityRepo)(nil)
