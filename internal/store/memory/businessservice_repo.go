package memory

import (
	"context"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/externalref"
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
	got, ok := r.items[id]
	if !ok {
		return nil, nil
	}
	return cloneBusinessService(got), nil
}

func (r *BusinessServiceRepo) Create(_ context.Context, s *businessservice.BusinessService) error {
	if err := s.ExternalRef.Validate(); err != nil {
		return err
	}
	r.items[s.ID] = cloneBusinessService(s)
	return nil
}

func (r *BusinessServiceRepo) Update(_ context.Context, s *businessservice.BusinessService) error {
	if err := s.ExternalRef.Validate(); err != nil {
		return err
	}
	r.items[s.ID] = cloneBusinessService(s)
	return nil
}

func (r *BusinessServiceRepo) List(_ context.Context) ([]*businessservice.BusinessService, error) {
	out := make([]*businessservice.BusinessService, 0, len(r.items))
	for _, s := range r.items {
		out = append(out, cloneBusinessService(s))
	}
	return out, nil
}

// cloneBusinessService returns a defensive copy. ExternalRef is canonicalised
// (IsZero values become nil) and deep-copied so the LastSyncedAt pointer is
// independent of the caller's. All other fields are value types — a struct
// copy suffices.
func cloneBusinessService(in *businessservice.BusinessService) *businessservice.BusinessService {
	cp := *in
	cp.ExternalRef = externalref.Canonicalise(in.ExternalRef).Clone()
	return &cp
}

var _ businessservice.BusinessServiceRepository = (*BusinessServiceRepo)(nil)
