package memory

import (
	"context"
	"fmt"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/processbusinessservice"
)

// ProcessBusinessServiceRepo is an in-memory implementation of
// processbusinessservice.ProcessBusinessServiceRepository.
// The composite key (ProcessID, BusinessServiceID) is enforced: duplicate creates return an error.
type ProcessBusinessServiceRepo struct {
	// keyed by "processID\x00businessServiceID"
	items         map[string]*processbusinessservice.ProcessBusinessService
	processes     process.ProcessRepository
	businessSvcs  businessservice.BusinessServiceRepository
}

func NewProcessBusinessServiceRepo() *ProcessBusinessServiceRepo {
	return &ProcessBusinessServiceRepo{items: map[string]*processbusinessservice.ProcessBusinessService{}}
}

func pbsCompositeKey(processID, businessServiceID string) string {
	return processID + "\x00" + businessServiceID
}

func (r *ProcessBusinessServiceRepo) Create(ctx context.Context, pbs *processbusinessservice.ProcessBusinessService) error {
	if r.processes != nil {
		ok, err := r.processes.Exists(ctx, pbs.ProcessID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("process %q does not exist", pbs.ProcessID)
		}
	}
	if r.businessSvcs != nil {
		ok, err := r.businessSvcs.Exists(ctx, pbs.BusinessServiceID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("business service %q does not exist", pbs.BusinessServiceID)
		}
	}
	k := pbsCompositeKey(pbs.ProcessID, pbs.BusinessServiceID)
	if _, exists := r.items[k]; exists {
		return fmt.Errorf("process business service link between process %q and business service %q already exists",
			pbs.ProcessID, pbs.BusinessServiceID)
	}
	r.items[k] = pbs
	return nil
}

func (r *ProcessBusinessServiceRepo) ListByProcessID(_ context.Context, processID string) ([]*processbusinessservice.ProcessBusinessService, error) {
	var out []*processbusinessservice.ProcessBusinessService
	for _, pbs := range r.items {
		if pbs.ProcessID == processID {
			out = append(out, pbs)
		}
	}
	return out, nil
}

func (r *ProcessBusinessServiceRepo) ListByBusinessServiceID(_ context.Context, businessServiceID string) ([]*processbusinessservice.ProcessBusinessService, error) {
	var out []*processbusinessservice.ProcessBusinessService
	for _, pbs := range r.items {
		if pbs.BusinessServiceID == businessServiceID {
			out = append(out, pbs)
		}
	}
	return out, nil
}

func (r *ProcessBusinessServiceRepo) Delete(_ context.Context, processID, businessServiceID string) error {
	delete(r.items, pbsCompositeKey(processID, businessServiceID))
	return nil
}

var _ processbusinessservice.ProcessBusinessServiceRepository = (*ProcessBusinessServiceRepo)(nil)
