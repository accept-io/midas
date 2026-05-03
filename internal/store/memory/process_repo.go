package memory

import (
	"context"
	"fmt"
	"sort"

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

// ListByBusinessService returns processes whose business_service_id
// matches the given ID, ordered by process ID. Returns an empty slice
// when no processes match.
//
// Result rows are stored pointers — callers receive direct references
// to the in-memory map's values. This matches the existing List/GetByID
// pattern; defensive copies are not introduced here because none of
// the existing methods perform them. (PR 3 surfaced the
// older-pattern-no-defensive-copy issue on BusinessService and fixed
// it locally; a follow-up convention-alignment PR can extend the fix
// to Process when it next changes.)
func (r *ProcessRepo) ListByBusinessService(_ context.Context, businessServiceID string) ([]*process.Process, error) {
	out := make([]*process.Process, 0)
	for _, p := range r.items {
		if p.BusinessServiceID == businessServiceID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

var _ process.ProcessRepository = (*ProcessRepo)(nil)
