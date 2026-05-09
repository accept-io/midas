package memory

import (
	"context"
	"time"

	"github.com/accept-io/midas/internal/failmode"
)

// FailModePolicyRepo is an in-memory implementation of
// failmode.PolicyRepository. The implementation mirrors the existing
// versioned authority/control artefact pattern (see ProfileRepo) — keyed by
// logical ID with a slice of versions ordered by insertion (which matches
// monotonically increasing Version numbers assigned by the caller).
//
// Concurrency: not goroutine-safe. The existing memory repositories in this
// package are single-goroutine; FailModePolicyRepo follows the same posture.
//
// Defensive copies: not made. Callers retain ownership; mutating a returned
// pointer mutates the in-memory record. The Postgres implementation is the
// integrity backstop; the memory implementation prioritises parity with
// existing repos over isolation guarantees.
type FailModePolicyRepo struct {
	// items maps logical policy ID → versions in ascending-version order.
	// The last element is the latest version.
	items map[string][]*failmode.FailModePolicy
}

// NewFailModePolicyRepo constructs an empty in-memory FailModePolicyRepo.
func NewFailModePolicyRepo() *FailModePolicyRepo {
	return &FailModePolicyRepo{items: make(map[string][]*failmode.FailModePolicy)}
}

// FindByID returns the latest version (highest Version) for the logical
// policy ID. Returns nil, nil when no policy with that ID exists. Status
// is not considered — see FindActiveAt for active-policy resolution.
func (r *FailModePolicyRepo) FindByID(_ context.Context, id string) (*failmode.FailModePolicy, error) {
	versions := r.items[id]
	if len(versions) == 0 {
		return nil, nil
	}
	return versions[len(versions)-1], nil
}

// FindByIDAndVersion returns the exact (id, version) pair. Returns nil, nil
// when the logical ID does not exist or does not have the requested version.
func (r *FailModePolicyRepo) FindByIDAndVersion(_ context.Context, id string, version int) (*failmode.FailModePolicy, error) {
	for _, p := range r.items[id] {
		if p.Version == version {
			return p, nil
		}
	}
	return nil, nil
}

// FindActiveAt returns the version of the policy that is active at the
// given time:
//
//   - Status == active
//   - EffectiveDate <= at
//   - EffectiveUntil IS NULL OR EffectiveUntil > at
//
// When multiple versions satisfy the predicate (an invariant violation),
// the highest Version is returned. Returns nil, nil when no version matches.
func (r *FailModePolicyRepo) FindActiveAt(_ context.Context, id string, at time.Time) (*failmode.FailModePolicy, error) {
	var best *failmode.FailModePolicy
	for _, p := range r.items[id] {
		if p.Status != failmode.FailModePolicyStatusActive {
			continue
		}
		if p.EffectiveDate.After(at) {
			continue
		}
		if p.EffectiveUntil != nil && !p.EffectiveUntil.After(at) {
			continue
		}
		if best == nil || p.Version > best.Version {
			best = p
		}
	}
	return best, nil
}

// ListVersions returns all versions of the policy ordered by Version DESC
// (latest first), matching the Postgres implementation. Returns an empty
// slice (not nil) when the policy ID has no rows.
func (r *FailModePolicyRepo) ListVersions(_ context.Context, id string) ([]*failmode.FailModePolicy, error) {
	versions := r.items[id]
	if len(versions) == 0 {
		return []*failmode.FailModePolicy{}, nil
	}
	out := make([]*failmode.FailModePolicy, len(versions))
	for i, p := range versions {
		out[len(versions)-1-i] = p
	}
	return out, nil
}

// Create appends a new version for the policy's logical ID. The caller is
// responsible for assigning a monotonically increasing Version number
// (matching ProfileRepo posture).
func (r *FailModePolicyRepo) Create(_ context.Context, p *failmode.FailModePolicy) error {
	r.items[p.ID] = append(r.items[p.ID], p)
	return nil
}

// Update replaces the matching (ID, Version) entry in place. Returns nil
// without error if the (ID, Version) does not exist (silent no-op),
// mirroring authority.ProfileRepo.Update.
func (r *FailModePolicyRepo) Update(_ context.Context, p *failmode.FailModePolicy) error {
	for i, existing := range r.items[p.ID] {
		if existing.Version == p.Version {
			r.items[p.ID][i] = p
			return nil
		}
	}
	return nil
}

var _ failmode.PolicyRepository = (*FailModePolicyRepo)(nil)
