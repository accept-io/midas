package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/accept-io/midas/internal/aisystem"
)

// AISystemRepo is an in-memory implementation of aisystem.SystemRepository.
//
// Constraints enforced in code (mirroring the Postgres CHECK / FK constraints):
//
//   - status ∈ {active, deprecated, retired}    (chk_ai_systems_status)
//   - origin ∈ {manual, inferred}                (chk_ai_systems_origin)
//   - replaces (if non-empty) is not equal to id (chk_ai_systems_no_self_replace)
//   - id is unique                               (PK)
//
// Concurrency: a sync.RWMutex guards all reads and writes. Reads return
// defensive copies so callers cannot mutate stored values.
type AISystemRepo struct {
	mu    sync.RWMutex
	items map[string]*aisystem.AISystem
}

// NewAISystemRepo constructs an empty repo.
func NewAISystemRepo() *AISystemRepo {
	return &AISystemRepo{items: map[string]*aisystem.AISystem{}}
}

func (r *AISystemRepo) Create(_ context.Context, sys *aisystem.AISystem) error {
	if sys == nil {
		return aisystem.ErrAISystemNotFound
	}
	if !aisystem.IsValidAISystemStatus(sys.Status) {
		return aisystem.ErrInvalidStatus
	}
	if !aisystem.IsValidAISystemOrigin(sys.Origin) {
		return aisystem.ErrInvalidOrigin
	}
	if sys.Replaces != "" && sys.Replaces == sys.ID {
		return aisystem.ErrSelfReplace
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[sys.ID]; exists {
		return aisystem.ErrAISystemAlreadyExists
	}
	r.items[sys.ID] = cloneAISystem(sys)
	return nil
}

func (r *AISystemRepo) GetByID(_ context.Context, id string) (*aisystem.AISystem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sys, ok := r.items[id]
	if !ok {
		return nil, aisystem.ErrAISystemNotFound
	}
	return cloneAISystem(sys), nil
}

func (r *AISystemRepo) Exists(_ context.Context, id string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.items[id]
	return ok, nil
}

func (r *AISystemRepo) List(_ context.Context) ([]*aisystem.AISystem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*aisystem.AISystem, 0, len(r.items))
	for _, sys := range r.items {
		out = append(out, cloneAISystem(sys))
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (r *AISystemRepo) Update(_ context.Context, sys *aisystem.AISystem) error {
	if sys == nil {
		return aisystem.ErrAISystemNotFound
	}
	if !aisystem.IsValidAISystemStatus(sys.Status) {
		return aisystem.ErrInvalidStatus
	}
	if !aisystem.IsValidAISystemOrigin(sys.Origin) {
		return aisystem.ErrInvalidOrigin
	}
	if sys.Replaces != "" && sys.Replaces == sys.ID {
		return aisystem.ErrSelfReplace
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[sys.ID]; !ok {
		return aisystem.ErrAISystemNotFound
	}
	r.items[sys.ID] = cloneAISystem(sys)
	return nil
}

func cloneAISystem(in *aisystem.AISystem) *aisystem.AISystem {
	cp := *in
	return &cp
}

var _ aisystem.SystemRepository = (*AISystemRepo)(nil)
