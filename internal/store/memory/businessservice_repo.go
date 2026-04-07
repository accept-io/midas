package memory

import (
	"context"

	"github.com/accept-io/midas/internal/businessservice"
)

// BusinessServiceRepo is an in-memory implementation of businessservice.BusinessServiceRepository.
type BusinessServiceRepo struct {
	items map[string]*businessservice.BusinessService
}

func NewBusinessServiceRepo() *BusinessServiceRepo {
	return &BusinessServiceRepo{items: map[string]*businessservice.BusinessService{}}
}

func (r *BusinessServiceRepo) Exists(_ context.Context, id string) (bool, error) {
	_, ok := r.items[id]
	return ok, nil
}

func (r *BusinessServiceRepo) GetByID(_ context.Context, id string) (*businessservice.BusinessService, error) {
	return r.items[id], nil
}

func (r *BusinessServiceRepo) Create(_ context.Context, s *businessservice.BusinessService) error {
	r.items[s.ID] = s
	return nil
}

func (r *BusinessServiceRepo) Update(_ context.Context, s *businessservice.BusinessService) error {
	r.items[s.ID] = s
	return nil
}

func (r *BusinessServiceRepo) List(_ context.Context) ([]*businessservice.BusinessService, error) {
	out := make([]*businessservice.BusinessService, 0, len(r.items))
	for _, s := range r.items {
		out = append(out, s)
	}
	return out, nil
}

var _ businessservice.BusinessServiceRepository = (*BusinessServiceRepo)(nil)
