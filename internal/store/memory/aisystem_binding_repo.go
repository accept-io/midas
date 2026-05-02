package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/process"
)

// AISystemBindingRepo is an in-memory implementation of
// aisystem.BindingRepository.
//
// Constraints enforced in code (mirroring the Postgres CHECK / FK constraints):
//
//   - At least one of business_service_id, capability_id, process_id, surface_id
//     is set                                          (chk_ai_bindings_at_least_one_target)
//   - id is unique                                    (PK)
//   - ai_system_id references an existing AISystem    (FK to ai_systems.id)
//   - (ai_system_id, ai_system_version) references an existing version
//     when ai_system_version is set                   (composite FK fk_ai_bindings_version)
//   - business_service_id, capability_id, process_id reference existing rows
//     when set                                        (FKs to respective tables)
//   - surface_id has no FK validation (surfaces are versioned; bindings
//     reference logical ID only)
//
// Validators (aiSystems / aiVersions / bsvcs / capabilities / processes)
// are optional. NewRepositories wires them; standalone unit tests can
// leave them nil to focus on the constraint logic local to bindings.
//
// Concurrency: a sync.RWMutex guards all reads and writes. Reads return
// defensive copies so callers cannot mutate stored values.
type AISystemBindingRepo struct {
	mu    sync.RWMutex
	items map[string]*aisystem.AISystemBinding

	aiSystems    aisystem.SystemRepository
	aiVersions   aisystem.VersionRepository
	bsvcs        businessservice.BusinessServiceRepository
	capabilities capability.CapabilityRepository
	processes    process.ProcessRepository
}

// NewAISystemBindingRepo constructs an empty repo.
func NewAISystemBindingRepo() *AISystemBindingRepo {
	return &AISystemBindingRepo{
		items: map[string]*aisystem.AISystemBinding{},
	}
}

func (r *AISystemBindingRepo) Create(ctx context.Context, b *aisystem.AISystemBinding) error {
	if b == nil {
		return aisystem.ErrAISystemBindingNotFound
	}
	if !b.HasContextReference() {
		return aisystem.ErrBindingMissingContext
	}

	if r.aiSystems != nil {
		ok, err := r.aiSystems.Exists(ctx, b.AISystemID)
		if err != nil {
			return err
		}
		if !ok {
			return aisystem.ErrAISystemNotFound
		}
	}
	if b.AISystemVersion != nil && r.aiVersions != nil {
		_, err := r.aiVersions.GetByIDAndVersion(ctx, b.AISystemID, *b.AISystemVersion)
		if err != nil {
			return err
		}
	}
	if b.BusinessServiceID != "" && r.bsvcs != nil {
		ok, err := r.bsvcs.Exists(ctx, b.BusinessServiceID)
		if err != nil {
			return err
		}
		if !ok {
			return &missingFKErr{entity: "business_service", id: b.BusinessServiceID}
		}
	}
	if b.CapabilityID != "" && r.capabilities != nil {
		ok, err := r.capabilities.Exists(ctx, b.CapabilityID)
		if err != nil {
			return err
		}
		if !ok {
			return &missingFKErr{entity: "capability", id: b.CapabilityID}
		}
	}
	if b.ProcessID != "" && r.processes != nil {
		ok, err := r.processes.Exists(ctx, b.ProcessID)
		if err != nil {
			return err
		}
		if !ok {
			return &missingFKErr{entity: "process", id: b.ProcessID}
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[b.ID]; exists {
		return aisystem.ErrAISystemBindingAlreadyExists
	}
	r.items[b.ID] = cloneAISystemBinding(b)
	return nil
}

func (r *AISystemBindingRepo) GetByID(_ context.Context, id string) (*aisystem.AISystemBinding, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.items[id]
	if !ok {
		return nil, aisystem.ErrAISystemBindingNotFound
	}
	return cloneAISystemBinding(b), nil
}

func (r *AISystemBindingRepo) List(_ context.Context) ([]*aisystem.AISystemBinding, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*aisystem.AISystemBinding, 0, len(r.items))
	for _, b := range r.items {
		out = append(out, cloneAISystemBinding(b))
	}
	sortBindings(out)
	return out, nil
}

func (r *AISystemBindingRepo) ListByAISystem(_ context.Context, aiSystemID string) ([]*aisystem.AISystemBinding, error) {
	return r.listFiltered(func(b *aisystem.AISystemBinding) bool { return b.AISystemID == aiSystemID }), nil
}

func (r *AISystemBindingRepo) ListByBusinessService(_ context.Context, bsID string) ([]*aisystem.AISystemBinding, error) {
	return r.listFiltered(func(b *aisystem.AISystemBinding) bool { return b.BusinessServiceID == bsID }), nil
}

func (r *AISystemBindingRepo) ListByCapability(_ context.Context, capID string) ([]*aisystem.AISystemBinding, error) {
	return r.listFiltered(func(b *aisystem.AISystemBinding) bool { return b.CapabilityID == capID }), nil
}

func (r *AISystemBindingRepo) ListByProcess(_ context.Context, procID string) ([]*aisystem.AISystemBinding, error) {
	return r.listFiltered(func(b *aisystem.AISystemBinding) bool { return b.ProcessID == procID }), nil
}

func (r *AISystemBindingRepo) ListBySurface(_ context.Context, surfID string) ([]*aisystem.AISystemBinding, error) {
	return r.listFiltered(func(b *aisystem.AISystemBinding) bool { return b.SurfaceID == surfID }), nil
}

func (r *AISystemBindingRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return aisystem.ErrAISystemBindingNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *AISystemBindingRepo) listFiltered(pred func(*aisystem.AISystemBinding) bool) []*aisystem.AISystemBinding {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*aisystem.AISystemBinding, 0)
	for _, b := range r.items {
		if pred(b) {
			out = append(out, cloneAISystemBinding(b))
		}
	}
	sortBindings(out)
	return out
}

func cloneAISystemBinding(in *aisystem.AISystemBinding) *aisystem.AISystemBinding {
	cp := *in
	if in.AISystemVersion != nil {
		v := *in.AISystemVersion
		cp.AISystemVersion = &v
	}
	return &cp
}

func sortBindings(bs []*aisystem.AISystemBinding) {
	sort.Slice(bs, func(i, j int) bool {
		if !bs[i].CreatedAt.Equal(bs[j].CreatedAt) {
			return bs[i].CreatedAt.After(bs[j].CreatedAt)
		}
		return bs[i].ID < bs[j].ID
	})
}

// missingFKErr matches the error shape used by the BSR memory repo
// (wrapMissingBSRef): a structured "referenced X not found" error
// returned in place of the FK-violation a Postgres backend would surface.
type missingFKErr struct {
	entity string
	id     string
}

func (e *missingFKErr) Error() string {
	return "create ai system binding: referenced " + e.entity + " " + e.id + " not found"
}

var _ aisystem.BindingRepository = (*AISystemBindingRepo)(nil)
