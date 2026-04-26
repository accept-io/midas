package memory

import (
	"context"
	"fmt"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/businessservicecapability"
	"github.com/accept-io/midas/internal/capability"
)

// BusinessServiceCapabilityRepo is an in-memory implementation of
// businessservicecapability.BusinessServiceCapabilityRepository. The composite
// key (BusinessServiceID, CapabilityID) is enforced: duplicate creates return
// an error. Optional foreign-key validators are populated by NewRepositories
// so Create rejects links that reference non-existent rows.
type BusinessServiceCapabilityRepo struct {
	// keyed by "businessServiceID\x00capabilityID"
	items        map[string]*businessservicecapability.BusinessServiceCapability
	businessSvcs businessservice.BusinessServiceRepository
	capabilities capability.CapabilityRepository
}

func NewBusinessServiceCapabilityRepo() *BusinessServiceCapabilityRepo {
	return &BusinessServiceCapabilityRepo{
		items: map[string]*businessservicecapability.BusinessServiceCapability{},
	}
}

func bscCompositeKey(businessServiceID, capabilityID string) string {
	return businessServiceID + "\x00" + capabilityID
}

func (r *BusinessServiceCapabilityRepo) Create(ctx context.Context, bsc *businessservicecapability.BusinessServiceCapability) error {
	if r.businessSvcs != nil {
		ok, err := r.businessSvcs.Exists(ctx, bsc.BusinessServiceID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("business service %q does not exist", bsc.BusinessServiceID)
		}
	}
	if r.capabilities != nil {
		ok, err := r.capabilities.Exists(ctx, bsc.CapabilityID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("capability %q does not exist", bsc.CapabilityID)
		}
	}
	k := bscCompositeKey(bsc.BusinessServiceID, bsc.CapabilityID)
	if _, exists := r.items[k]; exists {
		return fmt.Errorf("business service capability link between business service %q and capability %q already exists",
			bsc.BusinessServiceID, bsc.CapabilityID)
	}
	r.items[k] = bsc
	return nil
}

func (r *BusinessServiceCapabilityRepo) Exists(_ context.Context, businessServiceID, capabilityID string) (bool, error) {
	_, ok := r.items[bscCompositeKey(businessServiceID, capabilityID)]
	return ok, nil
}

func (r *BusinessServiceCapabilityRepo) ListByBusinessServiceID(_ context.Context, businessServiceID string) ([]*businessservicecapability.BusinessServiceCapability, error) {
	var out []*businessservicecapability.BusinessServiceCapability
	for _, bsc := range r.items {
		if bsc.BusinessServiceID == businessServiceID {
			out = append(out, bsc)
		}
	}
	return out, nil
}

func (r *BusinessServiceCapabilityRepo) ListByCapabilityID(_ context.Context, capabilityID string) ([]*businessservicecapability.BusinessServiceCapability, error) {
	var out []*businessservicecapability.BusinessServiceCapability
	for _, bsc := range r.items {
		if bsc.CapabilityID == capabilityID {
			out = append(out, bsc)
		}
	}
	return out, nil
}

func (r *BusinessServiceCapabilityRepo) Delete(_ context.Context, businessServiceID, capabilityID string) error {
	delete(r.items, bscCompositeKey(businessServiceID, capabilityID))
	return nil
}

var _ businessservicecapability.BusinessServiceCapabilityRepository = (*BusinessServiceCapabilityRepo)(nil)
