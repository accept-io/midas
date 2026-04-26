package memory

import (
	"context"
	"fmt"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/process"
)

type ProcessRepo struct {
	items        map[string]*process.Process
	businessSvcs businessservice.BusinessServiceRepository
}

func NewProcessRepo() *ProcessRepo {
	return &ProcessRepo{items: map[string]*process.Process{}}
}

func (r *ProcessRepo) Exists(_ context.Context, id string) (bool, error) {
	_, ok := r.items[id]
	return ok, nil
}

func (r *ProcessRepo) GetByID(_ context.Context, id string) (*process.Process, error) {
	return r.items[id], nil
}

func (r *ProcessRepo) Create(ctx context.Context, p *process.Process) error {
	if p.BusinessServiceID == "" {
		return fmt.Errorf("process %q: business_service_id is required", p.ID)
	}
	if r.businessSvcs != nil {
		ok, err := r.businessSvcs.Exists(ctx, p.BusinessServiceID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("business service %q does not exist", p.BusinessServiceID)
		}
	}
	if p.ParentProcessID != "" {
		if _, ok := r.items[p.ParentProcessID]; !ok {
			return fmt.Errorf("parent process %q does not exist", p.ParentProcessID)
		}
	}
	r.items[p.ID] = p
	return nil
}

func (r *ProcessRepo) Update(_ context.Context, p *process.Process) error {
	r.items[p.ID] = p
	return nil
}

func (r *ProcessRepo) List(_ context.Context) ([]*process.Process, error) {
	out := make([]*process.Process, 0, len(r.items))
	for _, p := range r.items {
		out = append(out, p)
	}
	return out, nil
}

var _ process.ProcessRepository = (*ProcessRepo)(nil)
