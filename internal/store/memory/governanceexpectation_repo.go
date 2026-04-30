package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/governanceexpectation"
)

// GovernanceExpectationRepo is an in-memory implementation of
// governanceexpectation.Repository. Versions are stored per logical ID in
// ascending insertion order; callers assign monotonically increasing
// version numbers, mirroring the Postgres composite-key model.
//
// Behaviour matches the Postgres repo's contract for tests that exercise
// both backends:
//
//   - Create rejects duplicate (id, version) instead of silently appending.
//   - Update returns an error when (id, version) is not found instead of
//     no-opping. This is stricter than ProfileRepo's memory implementation
//     and matches the Postgres rowsAffected==0 branch.
//   - Reads return deep copies — including a copied ConditionPayload byte
//     slice — so callers cannot mutate stored state by holding a returned
//     pointer.
//   - Create normalises nil/empty ConditionPayload to "{}" so reads
//     round-trip the same shape the Postgres repo produces under the
//     schema's DEFAULT '{}'.
type GovernanceExpectationRepo struct {
	// items maps logical expectation ID → versions in ascending-version
	// order. The last element is the latest version.
	items map[string][]*governanceexpectation.GovernanceExpectation
}

// NewGovernanceExpectationRepo constructs an empty in-memory repo.
func NewGovernanceExpectationRepo() *GovernanceExpectationRepo {
	return &GovernanceExpectationRepo{
		items: map[string][]*governanceexpectation.GovernanceExpectation{},
	}
}

// memoryEmptyJSONObject is the canonical empty-object payload stored when
// the caller passes a nil or empty ConditionPayload. Mirrors the Postgres
// repo's payloadForInsert normalisation.
var memoryEmptyJSONObject = json.RawMessage(`{}`)

// Create appends a new (id, version) row. Returns an error when the
// (id, version) pair already exists.
func (r *GovernanceExpectationRepo) Create(_ context.Context, e *governanceexpectation.GovernanceExpectation) error {
	for _, existing := range r.items[e.ID] {
		if existing.Version == e.Version {
			return fmt.Errorf("governance expectation already exists: id=%s version=%d", e.ID, e.Version)
		}
	}
	stored := cloneExpectation(e)
	if len(stored.ConditionPayload) == 0 {
		stored.ConditionPayload = append(json.RawMessage(nil), memoryEmptyJSONObject...)
	}
	r.items[e.ID] = append(r.items[e.ID], stored)
	return nil
}

// FindByID returns a clone of the latest version for the given logical
// ID. Returns nil, nil when no expectation with that ID exists.
func (r *GovernanceExpectationRepo) FindByID(_ context.Context, id string) (*governanceexpectation.GovernanceExpectation, error) {
	versions := r.items[id]
	if len(versions) == 0 {
		return nil, nil
	}
	return cloneExpectation(versions[len(versions)-1]), nil
}

// FindByIDAndVersion returns a clone of the exact (id, version) pair.
// Returns nil, nil when the pair does not exist.
func (r *GovernanceExpectationRepo) FindByIDAndVersion(_ context.Context, id string, version int) (*governanceexpectation.GovernanceExpectation, error) {
	for _, e := range r.items[id] {
		if e.Version == version {
			return cloneExpectation(e), nil
		}
	}
	return nil, nil
}

// ListVersions returns clones of every version in version-DESC order
// (latest first). Returns an empty slice when no versions exist.
func (r *GovernanceExpectationRepo) ListVersions(_ context.Context, id string) ([]*governanceexpectation.GovernanceExpectation, error) {
	versions := r.items[id]
	if len(versions) == 0 {
		return []*governanceexpectation.GovernanceExpectation{}, nil
	}
	out := make([]*governanceexpectation.GovernanceExpectation, len(versions))
	for i, e := range versions {
		out[len(versions)-1-i] = cloneExpectation(e)
	}
	return out, nil
}

// Update persists the mutable lifecycle/audit fields onto the matching
// (id, version) row. Returns an error when no row matches. The mutable
// set mirrors the Postgres repo:
//
//   - Status
//   - EffectiveUntil
//   - RetiredAt
//   - UpdatedAt
//   - ApprovedBy
//   - ApprovedAt
//
// All other fields on the supplied struct are ignored.
func (r *GovernanceExpectationRepo) Update(_ context.Context, e *governanceexpectation.GovernanceExpectation) error {
	for _, existing := range r.items[e.ID] {
		if existing.Version == e.Version {
			existing.Status = e.Status
			existing.EffectiveUntil = clonePtrTime(e.EffectiveUntil)
			existing.RetiredAt = clonePtrTime(e.RetiredAt)
			existing.UpdatedAt = e.UpdatedAt
			existing.ApprovedBy = e.ApprovedBy
			existing.ApprovedAt = clonePtrTime(e.ApprovedAt)
			return nil
		}
	}
	return fmt.Errorf("governance expectation not found: id=%s version=%d", e.ID, e.Version)
}

// ListActiveByScope returns clones of every stored row matching the
// active-at-time predicate under (scopeKind, scopeID). Mirrors the
// Postgres impl's predicate exactly:
//
//   - Status         == active
//   - EffectiveDate  <= at
//   - EffectiveUntil == nil OR > at
//   - RetiredAt      == nil
//
// Multiple versions of the same logical ID may be returned; the caller
// is responsible for picking a single version. Order is unspecified —
// the memory walk is map-iteration order.
func (r *GovernanceExpectationRepo) ListActiveByScope(
	_ context.Context,
	scopeKind governanceexpectation.ScopeKind,
	scopeID string,
	at time.Time,
) ([]*governanceexpectation.GovernanceExpectation, error) {
	var out []*governanceexpectation.GovernanceExpectation
	for _, versions := range r.items {
		for _, e := range versions {
			if e.ScopeKind != scopeKind || e.ScopeID != scopeID {
				continue
			}
			if e.Status != governanceexpectation.ExpectationStatusActive {
				continue
			}
			if e.EffectiveDate.After(at) {
				continue
			}
			if e.EffectiveUntil != nil && !e.EffectiveUntil.After(at) {
				continue
			}
			if e.RetiredAt != nil {
				continue
			}
			out = append(out, cloneExpectation(e))
		}
	}
	return out, nil
}

// cloneExpectation returns a deep copy: every *time.Time pointer is
// independently allocated and the ConditionPayload byte slice is copied
// so callers cannot mutate stored state through a returned pointer.
func cloneExpectation(e *governanceexpectation.GovernanceExpectation) *governanceexpectation.GovernanceExpectation {
	cp := *e
	cp.EffectiveUntil = clonePtrTime(e.EffectiveUntil)
	cp.RetiredAt = clonePtrTime(e.RetiredAt)
	cp.ApprovedAt = clonePtrTime(e.ApprovedAt)
	if e.ConditionPayload != nil {
		buf := make([]byte, len(e.ConditionPayload))
		copy(buf, e.ConditionPayload)
		cp.ConditionPayload = json.RawMessage(buf)
	}
	return &cp
}

// clonePtrTime returns nil when t is nil, otherwise a fresh pointer to a
// copy of *t. Mirrors the pattern used elsewhere in this package for
// time.Time fields stored as pointers.
func clonePtrTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := *t
	return &v
}
