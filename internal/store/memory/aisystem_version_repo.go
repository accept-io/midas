package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/accept-io/midas/internal/aisystem"
)

// AISystemVersionRepo is an in-memory implementation of
// aisystem.VersionRepository. Versions are keyed by the composite
// (ai_system_id, version) tuple.
//
// Constraints enforced in code (mirroring the Postgres CHECK / FK constraints):
//
//   - status ∈ {review, active, deprecated, retired}  (chk_ai_versions_status)
//   - version >= 1                                     (chk_ai_versions_version_positive)
//   - effective_until > effective_from when both set   (chk_ai_versions_effective_range)
//   - (ai_system_id, version) is unique                (PK)
//   - ai_system_id references an existing AISystem
//     (FK to ai_systems.id) when the optional aiSystems validator is set
//
// Concurrency: a sync.RWMutex guards all reads and writes. Reads return
// defensive copies so callers cannot mutate stored values.
type AISystemVersionRepo struct {
	mu    sync.RWMutex
	items []*aisystem.AISystemVersion

	// aiSystems is an optional FK validator populated by NewRepositories.
	// When set, Create rejects versions referencing non-existent AI systems.
	aiSystems aisystem.SystemRepository
}

// NewAISystemVersionRepo constructs an empty repo.
func NewAISystemVersionRepo() *AISystemVersionRepo {
	return &AISystemVersionRepo{}
}

func (r *AISystemVersionRepo) Create(ctx context.Context, ver *aisystem.AISystemVersion) error {
	if ver == nil {
		return aisystem.ErrAISystemVersionNotFound
	}
	if !aisystem.IsValidAISystemVersionStatus(ver.Status) {
		return aisystem.ErrInvalidStatus
	}
	if ver.Version < 1 {
		return aisystem.ErrInvalidVersion
	}
	if ver.EffectiveUntil != nil && !ver.EffectiveUntil.After(ver.EffectiveFrom) {
		return aisystem.ErrInvalidEffectiveRange
	}

	if r.aiSystems != nil {
		ok, err := r.aiSystems.Exists(ctx, ver.AISystemID)
		if err != nil {
			return err
		}
		if !ok {
			return aisystem.ErrAISystemNotFound
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.items {
		if existing.AISystemID == ver.AISystemID && existing.Version == ver.Version {
			return aisystem.ErrAISystemVersionAlreadyExists
		}
	}
	r.items = append(r.items, cloneAISystemVersion(ver))
	return nil
}

func (r *AISystemVersionRepo) GetByIDAndVersion(_ context.Context, aiSystemID string, version int) (*aisystem.AISystemVersion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, v := range r.items {
		if v.AISystemID == aiSystemID && v.Version == version {
			return cloneAISystemVersion(v), nil
		}
	}
	return nil, aisystem.ErrAISystemVersionNotFound
}

func (r *AISystemVersionRepo) ListBySystem(_ context.Context, aiSystemID string) ([]*aisystem.AISystemVersion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*aisystem.AISystemVersion, 0)
	for _, v := range r.items {
		if v.AISystemID == aiSystemID {
			out = append(out, cloneAISystemVersion(v))
		}
	}
	// Latest version first — mirrors the idx_ai_versions_system_version_desc index.
	sort.Slice(out, func(i, j int) bool {
		return out[i].Version > out[j].Version
	})
	return out, nil
}

func (r *AISystemVersionRepo) GetActiveBySystem(_ context.Context, aiSystemID string) (*aisystem.AISystemVersion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var best *aisystem.AISystemVersion
	for _, v := range r.items {
		if v.AISystemID != aiSystemID {
			continue
		}
		if v.Status != aisystem.AISystemVersionStatusActive {
			continue
		}
		if best == nil || v.Version > best.Version {
			best = v
		}
	}
	if best == nil {
		return nil, nil
	}
	return cloneAISystemVersion(best), nil
}

func (r *AISystemVersionRepo) Update(_ context.Context, ver *aisystem.AISystemVersion) error {
	if ver == nil {
		return aisystem.ErrAISystemVersionNotFound
	}
	if !aisystem.IsValidAISystemVersionStatus(ver.Status) {
		return aisystem.ErrInvalidStatus
	}
	if ver.EffectiveUntil != nil && !ver.EffectiveUntil.After(ver.EffectiveFrom) {
		return aisystem.ErrInvalidEffectiveRange
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for i, existing := range r.items {
		if existing.AISystemID == ver.AISystemID && existing.Version == ver.Version {
			r.items[i] = cloneAISystemVersion(ver)
			return nil
		}
	}
	return aisystem.ErrAISystemVersionNotFound
}

func cloneAISystemVersion(in *aisystem.AISystemVersion) *aisystem.AISystemVersion {
	cp := *in
	if in.EffectiveUntil != nil {
		t := *in.EffectiveUntil
		cp.EffectiveUntil = &t
	}
	if in.RetiredAt != nil {
		t := *in.RetiredAt
		cp.RetiredAt = &t
	}
	if in.ComplianceFrameworks != nil {
		frameworks := make([]string, len(in.ComplianceFrameworks))
		copy(frameworks, in.ComplianceFrameworks)
		cp.ComplianceFrameworks = frameworks
	}
	return &cp
}

var _ aisystem.VersionRepository = (*AISystemVersionRepo)(nil)
