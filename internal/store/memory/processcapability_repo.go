package memory

import (
	"context"
	"fmt"

	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/processcapability"
)

// ProcessCapabilityRepo is an in-memory implementation of processcapability.ProcessCapabilityRepository.
// The composite key (ProcessID, CapabilityID) is enforced: duplicate creates return an error.
type ProcessCapabilityRepo struct {
	// keyed by "processID\x00capabilityID"
	items        map[string]*processcapability.ProcessCapability
	processes    process.ProcessRepository
	capabilities capability.CapabilityRepository
}

func NewProcessCapabilityRepo() *ProcessCapabilityRepo {
	return &ProcessCapabilityRepo{items: map[string]*processcapability.ProcessCapability{}}
}

func compositeKey(processID, capabilityID string) string {
	return processID + "\x00" + capabilityID
}

func (r *ProcessCapabilityRepo) Create(ctx context.Context, pc *processcapability.ProcessCapability) error {
	if r.processes != nil {
		ok, err := r.processes.Exists(ctx, pc.ProcessID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("process %q does not exist", pc.ProcessID)
		}
	}
	if r.capabilities != nil {
		ok, err := r.capabilities.Exists(ctx, pc.CapabilityID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("capability %q does not exist", pc.CapabilityID)
		}
	}
	k := compositeKey(pc.ProcessID, pc.CapabilityID)
	if _, exists := r.items[k]; exists {
		return fmt.Errorf("process capability link between process %q and capability %q already exists",
			pc.ProcessID, pc.CapabilityID)
	}
	r.items[k] = pc
	return nil
}

func (r *ProcessCapabilityRepo) ListByProcessID(_ context.Context, processID string) ([]*processcapability.ProcessCapability, error) {
	var out []*processcapability.ProcessCapability
	for _, pc := range r.items {
		if pc.ProcessID == processID {
			out = append(out, pc)
		}
	}
	return out, nil
}

func (r *ProcessCapabilityRepo) ListByCapabilityID(_ context.Context, capabilityID string) ([]*processcapability.ProcessCapability, error) {
	var out []*processcapability.ProcessCapability
	for _, pc := range r.items {
		if pc.CapabilityID == capabilityID {
			out = append(out, pc)
		}
	}
	return out, nil
}

func (r *ProcessCapabilityRepo) Delete(_ context.Context, processID, capabilityID string) error {
	delete(r.items, compositeKey(processID, capabilityID))
	return nil
}

var _ processcapability.ProcessCapabilityRepository = (*ProcessCapabilityRepo)(nil)
