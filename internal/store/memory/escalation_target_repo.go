package memory

import (
	"context"
	"sort"
	"time"

	"github.com/accept-io/midas/internal/escalation"
)

// EscalationTargetRepo is an in-memory implementation of
// escalation.Repository. The shape mirrors FailModePolicyRepo: keyed by
// logical id with a slice of versions in ascending insertion order,
// which (because the caller assigns monotonically-increasing Version
// numbers) is also ascending Version order.
//
// Concurrency: not goroutine-safe. The memory store package is single-
// goroutine; EscalationTargetRepo follows the same posture as the
// other versioned-entity repos.
//
// Defensive copies: Create and Update store a deep copy so caller
// mutation of the originally-supplied pointer (e.g. appending to a
// slice field added in a future tranche) cannot leak into the stored
// record. Reads return the stored pointer directly — callers that
// need to mutate the result must clone it themselves. This matches
// the D31i memory GrantRepo posture; the Postgres implementation is
// the authoritative integrity backstop.
type EscalationTargetRepo struct {
	// items maps logical target id → versions in ascending insertion
	// (= ascending Version) order. The last element is the latest.
	items map[string][]*escalation.EscalationTarget
}

// NewEscalationTargetRepo constructs an empty in-memory
// EscalationTargetRepo.
func NewEscalationTargetRepo() *EscalationTargetRepo {
	return &EscalationTargetRepo{items: map[string][]*escalation.EscalationTarget{}}
}

// FindByID returns the latest version (highest Version) for the
// logical target id. Returns nil, nil when no target with that id
// exists. Status is not considered — see FindActiveAt for active-
// version resolution.
func (r *EscalationTargetRepo) FindByID(_ context.Context, id string) (*escalation.EscalationTarget, error) {
	versions := r.items[id]
	if len(versions) == 0 {
		return nil, nil
	}
	return versions[len(versions)-1], nil
}

// FindByIDAndVersion returns the exact (id, version) pair. Returns
// nil, nil when the logical id does not exist or does not have the
// requested version.
func (r *EscalationTargetRepo) FindByIDAndVersion(_ context.Context, id string, version int) (*escalation.EscalationTarget, error) {
	for _, t := range r.items[id] {
		if t.Version == version {
			return t, nil
		}
	}
	return nil, nil
}

// FindActiveAt returns the version active at the given instant:
//
//   - Status == active
//   - EffectiveDate <= at
//   - EffectiveUntil IS NULL OR EffectiveUntil > at
//
// When multiple versions satisfy the predicate (an invariant
// violation), the highest Version is returned. Returns nil, nil when
// no version matches.
func (r *EscalationTargetRepo) FindActiveAt(_ context.Context, id string, at time.Time) (*escalation.EscalationTarget, error) {
	var best *escalation.EscalationTarget
	for _, t := range r.items[id] {
		if t.Status != escalation.StatusActive {
			continue
		}
		if t.EffectiveDate.After(at) {
			continue
		}
		if t.EffectiveUntil != nil && !t.EffectiveUntil.After(at) {
			continue
		}
		if best == nil || t.Version > best.Version {
			best = t
		}
	}
	return best, nil
}

// List returns the latest version of every escalation target sorted
// by id ascending. Backs the /v1/escalation-targets list endpoint.
// Returns an empty slice (not nil) when the repo is empty.
func (r *EscalationTargetRepo) List(_ context.Context) ([]*escalation.EscalationTarget, error) {
	out := make([]*escalation.EscalationTarget, 0, len(r.items))
	for _, versions := range r.items {
		if len(versions) == 0 {
			continue
		}
		out = append(out, versions[len(versions)-1])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ListVersions returns all versions of the target ordered by Version
// DESC (latest first), matching the Postgres implementation. Returns
// an empty slice (not nil) when the id has no rows.
func (r *EscalationTargetRepo) ListVersions(_ context.Context, id string) ([]*escalation.EscalationTarget, error) {
	versions := r.items[id]
	if len(versions) == 0 {
		return []*escalation.EscalationTarget{}, nil
	}
	out := make([]*escalation.EscalationTarget, len(versions))
	for i, t := range versions {
		out[len(versions)-1-i] = t
	}
	return out, nil
}

// Create appends a new version for the target's logical id. The
// caller is responsible for assigning a monotonically increasing
// Version number — same posture as FailModePolicyRepo / ProfileRepo.
// A deep copy is stored so the caller's pointer is decoupled from
// the persisted record.
func (r *EscalationTargetRepo) Create(_ context.Context, t *escalation.EscalationTarget) error {
	r.items[t.ID] = append(r.items[t.ID], cloneEscalationTarget(t))
	return nil
}

// Update replaces the matching (ID, Version) entry in place. Silent
// no-op on missing row, mirroring authority.ProfileRepo and
// FailModePolicyRepo posture. The Postgres implementation is the
// integrity backstop for missing-row errors.
func (r *EscalationTargetRepo) Update(_ context.Context, t *escalation.EscalationTarget) error {
	for i, existing := range r.items[t.ID] {
		if existing.Version == t.Version {
			r.items[t.ID][i] = cloneEscalationTarget(t)
			return nil
		}
	}
	return nil
}

// cloneEscalationTarget returns a shallow-copy plus *time.Time
// indirection so caller mutation of EffectiveUntil/ApprovedAt does
// not leak into the stored record. The struct has no slice or map
// fields in D31k-impl-1, so a shallow copy is sufficient for value
// fields.
func cloneEscalationTarget(t *escalation.EscalationTarget) *escalation.EscalationTarget {
	if t == nil {
		return nil
	}
	cp := *t
	if t.EffectiveUntil != nil {
		v := *t.EffectiveUntil
		cp.EffectiveUntil = &v
	}
	if t.ApprovedAt != nil {
		v := *t.ApprovedAt
		cp.ApprovedAt = &v
	}
	return &cp
}

var _ escalation.Repository = (*EscalationTargetRepo)(nil)
