package memory

import (
	"context"
	"errors"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/outbox"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/surface"
)

// SurfaceRepo is a thread-unsafe in-memory implementation of
// surface.SurfaceRepository. It maintains full version history per logical
// surface ID, matching the semantics of the Postgres implementation:
//   - Create appends a new version; it never overwrites an existing one.
//   - FindLatestByID returns the highest-version entry.
//   - FindByIDVersion returns a specific (ID, version) pair.
//   - ListVersions returns all versions in descending order (latest first).
//   - Update modifies an existing (ID, version) entry in place.
//
// Versions are stored in ascending insertion order. Because the apply executor
// always increments version numbers monotonically (1, 2, 3 …), the last
// element is always the latest version. All methods rely on this invariant.
type SurfaceRepo struct {
	// versions maps logical surface ID → versions in ascending-version order.
	// The last element is the latest version.
	versions map[string][]*surface.DecisionSurface
}

func NewSurfaceRepo() *SurfaceRepo {
	return &SurfaceRepo{versions: make(map[string][]*surface.DecisionSurface)}
}

// FindLatestByID returns the highest-version surface for the given logical ID.
// Returns nil, nil when no surface with that ID exists.
func (r *SurfaceRepo) FindLatestByID(_ context.Context, id string) (*surface.DecisionSurface, error) {
	vs := r.versions[id]
	if len(vs) == 0 {
		return nil, nil
	}
	return vs[len(vs)-1], nil
}

// FindByIDVersion returns the surface with the given logical ID and exact version.
// Returns nil, nil when the (ID, version) pair does not exist.
func (r *SurfaceRepo) FindByIDVersion(_ context.Context, id string, version int) (*surface.DecisionSurface, error) {
	for _, s := range r.versions[id] {
		if s.Version == version {
			return s, nil
		}
	}
	return nil, nil
}

// FindActiveAt returns the surface version that is active at the given time:
// status == active, effective_from <= at, and (effective_until IS NULL OR
// effective_until > at). When multiple versions satisfy the condition (an
// invariant violation), the highest-version one is returned.
func (r *SurfaceRepo) FindActiveAt(_ context.Context, id string, at time.Time) (*surface.DecisionSurface, error) {
	var best *surface.DecisionSurface
	for _, s := range r.versions[id] {
		if s.Status != surface.SurfaceStatusActive {
			continue
		}
		if s.EffectiveFrom.After(at) {
			continue
		}
		if s.EffectiveUntil != nil && !s.EffectiveUntil.After(at) {
			continue
		}
		if best == nil || s.Version > best.Version {
			best = s
		}
	}
	return best, nil
}

// ListVersions returns all versions of the surface in descending version order
// (latest first), matching the Postgres implementation behaviour.
// Returns an empty slice when the surface does not exist.
func (r *SurfaceRepo) ListVersions(_ context.Context, id string) ([]*surface.DecisionSurface, error) {
	vs := r.versions[id]
	if len(vs) == 0 {
		return []*surface.DecisionSurface{}, nil
	}
	// Stored ascending; return descending copy to match ORDER BY version DESC.
	out := make([]*surface.DecisionSurface, len(vs))
	for i, s := range vs {
		out[len(vs)-1-i] = s
	}
	return out, nil
}

// ListAll returns the latest version of each surface.
func (r *SurfaceRepo) ListAll(_ context.Context) ([]*surface.DecisionSurface, error) {
	var out []*surface.DecisionSurface
	for _, vs := range r.versions {
		if len(vs) > 0 {
			out = append(out, vs[len(vs)-1])
		}
	}
	return out, nil
}

// ListByStatus returns the latest version of each surface that has the given status.
func (r *SurfaceRepo) ListByStatus(_ context.Context, status surface.SurfaceStatus) ([]*surface.DecisionSurface, error) {
	var out []*surface.DecisionSurface
	for _, vs := range r.versions {
		if len(vs) == 0 {
			continue
		}
		latest := vs[len(vs)-1]
		if latest.Status == status {
			out = append(out, latest)
		}
	}
	return out, nil
}

// ListByDomain returns the latest version of each surface in the given domain.
func (r *SurfaceRepo) ListByDomain(_ context.Context, domain string) ([]*surface.DecisionSurface, error) {
	var out []*surface.DecisionSurface
	for _, vs := range r.versions {
		if len(vs) == 0 {
			continue
		}
		latest := vs[len(vs)-1]
		if latest.Domain == domain {
			out = append(out, latest)
		}
	}
	return out, nil
}

// Search returns the latest version of each surface whose latest version
// matches the given search criteria.
func (r *SurfaceRepo) Search(_ context.Context, criteria surface.SearchCriteria) ([]*surface.DecisionSurface, error) {
	var out []*surface.DecisionSurface
	for _, vs := range r.versions {
		if len(vs) == 0 {
			continue
		}
		latest := vs[len(vs)-1]
		if matchesCriteria(latest, criteria) {
			out = append(out, latest)
		}
	}
	return out, nil
}

// matchesCriteria checks if a surface matches search criteria
func matchesCriteria(s *surface.DecisionSurface, criteria surface.SearchCriteria) bool {
	// Domain: exact match (case-insensitive)
	if criteria.Domain != "" && s.Domain != criteria.Domain {
		return false
	}

	// Category: exact match (case-insensitive)
	if criteria.Category != "" && s.Category != criteria.Category {
		return false
	}

	// Tags: ANY match
	if len(criteria.Tags) > 0 {
		hasTag := false
		for _, criteriaTag := range criteria.Tags {
			for _, surfaceTag := range s.Tags {
				if surfaceTag == criteriaTag {
					hasTag = true
					break
				}
			}
			if hasTag {
				break
			}
		}
		if !hasTag {
			return false
		}
	}

	// Taxonomy: prefix match
	if len(criteria.Taxonomy) > 0 {
		if len(s.Taxonomy) < len(criteria.Taxonomy) {
			return false
		}
		for i, taxon := range criteria.Taxonomy {
			if s.Taxonomy[i] != taxon {
				return false
			}
		}
	}

	// Status: ANY match (OR)
	if len(criteria.Status) > 0 {
		hasStatus := false
		for _, status := range criteria.Status {
			if s.Status == status {
				hasStatus = true
				break
			}
		}
		if !hasStatus {
			return false
		}
	}

	return true
}

// Create appends a new version for the surface's logical ID. The caller
// (the apply executor) is responsible for assigning a monotonically increasing
// Version number.
func (r *SurfaceRepo) Create(_ context.Context, s *surface.DecisionSurface) error {
	r.versions[s.ID] = append(r.versions[s.ID], s)
	return nil
}

// Update replaces the matching (ID, Version) entry in place.
// Returns nil without error if the (ID, Version) does not exist (no-op).
func (r *SurfaceRepo) Update(_ context.Context, s *surface.DecisionSurface) error {
	for i, existing := range r.versions[s.ID] {
		if existing.Version == s.Version {
			r.versions[s.ID][i] = s
			return nil
		}
	}
	return nil
}

type AgentRepo struct {
	items map[string]*agent.Agent
}

func NewAgentRepo() *AgentRepo {
	return &AgentRepo{items: map[string]*agent.Agent{}}
}

func (r *AgentRepo) GetByID(ctx context.Context, id string) (*agent.Agent, error) {
	return r.items[id], nil
}

func (r *AgentRepo) Create(ctx context.Context, a *agent.Agent) error {
	r.items[a.ID] = a
	return nil
}

func (r *AgentRepo) Update(ctx context.Context, a *agent.Agent) error {
	r.items[a.ID] = a
	return nil
}

func (r *AgentRepo) List(ctx context.Context) ([]*agent.Agent, error) {
	var out []*agent.Agent
	for _, v := range r.items {
		out = append(out, v)
	}
	return out, nil
}

// ProfileRepo is an in-memory implementation of authority.ProfileRepository.
// Profiles are stored as version slices keyed by their logical ID. Within each
// slice, versions are ordered by insertion order which mirrors ascending version
// numbers (callers assign monotonically increasing versions). This matches the
// postgres implementation: the most recently inserted version is the "latest".
type ProfileRepo struct {
	// items maps logical profile ID → versions in ascending-version order.
	// The last element is the latest version, matching FindByID behaviour.
	items map[string][]*authority.AuthorityProfile
}

func NewProfileRepo() *ProfileRepo {
	return &ProfileRepo{items: map[string][]*authority.AuthorityProfile{}}
}

// FindByID returns the latest version (highest version number) for the logical
// profile ID. Returns nil, nil when no profile with that ID exists.
func (r *ProfileRepo) FindByID(ctx context.Context, id string) (*authority.AuthorityProfile, error) {
	versions := r.items[id]
	if len(versions) == 0 {
		return nil, nil
	}
	return versions[len(versions)-1], nil
}

// FindByIDAndVersion returns the exact (id, version) profile. Returns nil, nil
// when the logical ID does not exist or does not have the requested version.
func (r *ProfileRepo) FindByIDAndVersion(ctx context.Context, id string, version int) (*authority.AuthorityProfile, error) {
	for _, p := range r.items[id] {
		if p.Version == version {
			return p, nil
		}
	}
	return nil, nil
}

// FindActiveAt returns the version of the profile that is active at the given
// time: status == active, effective_date <= at, and (effective_until IS NULL OR
// effective_until > at). When multiple versions are active at the same instant
// (which is an invariant violation), the latest version is returned.
func (r *ProfileRepo) FindActiveAt(ctx context.Context, id string, at time.Time) (*authority.AuthorityProfile, error) {
	var best *authority.AuthorityProfile
	for _, p := range r.items[id] {
		if p.Status != authority.ProfileStatusActive {
			continue
		}
		if p.EffectiveDate.After(at) {
			continue
		}
		if p.EffectiveUntil != nil && !p.EffectiveUntil.After(at) {
			continue
		}
		// Keep the highest version that qualifies.
		if best == nil || p.Version > best.Version {
			best = p
		}
	}
	return best, nil
}

// ListBySurface returns all profile versions whose SurfaceID matches, ordered
// by logical ID and then version descending. This matches the postgres
// implementation (ORDER BY id, version DESC) so that callers see all versions,
// not just the latest per logical profile.
func (r *ProfileRepo) ListBySurface(ctx context.Context, surfaceID string) ([]*authority.AuthorityProfile, error) {
	var out []*authority.AuthorityProfile
	for _, versions := range r.items {
		for _, p := range versions {
			if p.SurfaceID == surfaceID {
				out = append(out, p)
			}
		}
	}
	return out, nil
}

// ListVersions returns all versions of the profile ordered by version DESC,
// matching the postgres implementation behaviour.
func (r *ProfileRepo) ListVersions(ctx context.Context, id string) ([]*authority.AuthorityProfile, error) {
	versions := r.items[id]
	if len(versions) == 0 {
		return []*authority.AuthorityProfile{}, nil
	}
	// Return a copy in descending-version order (stored ascending).
	out := make([]*authority.AuthorityProfile, len(versions))
	for i, p := range versions {
		out[len(versions)-1-i] = p
	}
	return out, nil
}

// Create appends a new version for the profile's logical ID. The caller is
// responsible for setting a Version number that is higher than all existing
// versions for this ID.
func (r *ProfileRepo) Create(ctx context.Context, p *authority.AuthorityProfile) error {
	r.items[p.ID] = append(r.items[p.ID], p)
	return nil
}

// Update replaces the matching (ID, Version) entry in place.
// Returns nil without error if the (ID, Version) does not exist (no-op).
func (r *ProfileRepo) Update(ctx context.Context, p *authority.AuthorityProfile) error {
	for i, existing := range r.items[p.ID] {
		if existing.Version == p.Version {
			r.items[p.ID][i] = p
			return nil
		}
	}
	return nil
}

type GrantRepo struct {
	items map[string]*authority.AuthorityGrant
}

func NewGrantRepo() *GrantRepo {
	return &GrantRepo{items: map[string]*authority.AuthorityGrant{}}
}

func (r *GrantRepo) FindByID(ctx context.Context, id string) (*authority.AuthorityGrant, error) {
	return r.items[id], nil
}

// FindActiveByAgentAndProfile checks status='active' AND date range (schema v2.1)
func (r *GrantRepo) FindActiveByAgentAndProfile(ctx context.Context, agentID, profileID string) (*authority.AuthorityGrant, error) {
	now := time.Now()
	for _, g := range r.items {
		if g.AgentID != agentID || g.ProfileID != profileID {
			continue
		}
		if g.Status != authority.GrantStatusActive {
			continue
		}
		// Schema v2.1: Check date range
		if g.EffectiveDate.After(now) {
			continue
		}
		if g.ExpiresAt != nil && !g.ExpiresAt.After(now) {
			continue
		}
		return g, nil
	}
	return nil, nil
}

func (r *GrantRepo) ListByAgent(ctx context.Context, agentID string) ([]*authority.AuthorityGrant, error) {
	var out []*authority.AuthorityGrant
	for _, g := range r.items {
		if g.AgentID == agentID {
			out = append(out, g)
		}
	}
	return out, nil
}

func (r *GrantRepo) ListByProfile(ctx context.Context, profileID string) ([]*authority.AuthorityGrant, error) {
	var out []*authority.AuthorityGrant
	for _, g := range r.items {
		if g.ProfileID == profileID {
			out = append(out, g)
		}
	}
	return out, nil
}

func (r *GrantRepo) Create(ctx context.Context, g *authority.AuthorityGrant) error {
	r.items[g.ID] = g
	return nil
}

// Revoke sets status='revoked' and records revocation metadata (schema v2.1)
func (r *GrantRepo) Revoke(ctx context.Context, id string, revokedBy string) error {
	if g, ok := r.items[id]; ok {
		now := time.Now()
		g.Status = authority.GrantStatusRevoked
		g.RevokedAt = &now
		g.RevokedBy = revokedBy
	}
	return nil
}

// Suspend sets status='suspended' (schema v2.1)
func (r *GrantRepo) Suspend(ctx context.Context, id string) error {
	if g, ok := r.items[id]; ok {
		g.Status = authority.GrantStatusSuspended
	}
	return nil
}

// Reactivate sets status='active' from suspended (schema v2.1)
func (r *GrantRepo) Reactivate(ctx context.Context, id string) error {
	if g, ok := r.items[id]; ok {
		if g.Status == authority.GrantStatusSuspended {
			g.Status = authority.GrantStatusActive
		}
	}
	return nil
}

func (r *GrantRepo) Update(_ context.Context, g *authority.AuthorityGrant) error {
	if _, ok := r.items[g.ID]; !ok {
		return errors.New("grant not found")
	}
	cp := *g
	r.items[g.ID] = &cp
	return nil
}

type EnvelopeRepo struct {
	items       map[string]*envelope.Envelope
	byRequestID map[string]*envelope.Envelope // Legacy: by request_id only
	byScope     map[string]*envelope.Envelope // Schema v2.1: by (request_source, request_id)
}

func NewEnvelopeRepo() *EnvelopeRepo {
	return &EnvelopeRepo{
		items:       map[string]*envelope.Envelope{},
		byRequestID: map[string]*envelope.Envelope{},
		byScope:     map[string]*envelope.Envelope{},
	}
}

func (r *EnvelopeRepo) GetByID(ctx context.Context, id string) (*envelope.Envelope, error) {
	return r.items[id], nil
}

// GetByRequestID looks up by request_id only (legacy compatibility)
func (r *EnvelopeRepo) GetByRequestID(ctx context.Context, requestID string) (*envelope.Envelope, error) {
	return r.byRequestID[requestID], nil
}

// GetByRequestScope looks up by (request_source, request_id) - preferred for schema v2.1
func (r *EnvelopeRepo) GetByRequestScope(ctx context.Context, requestSource, requestID string) (*envelope.Envelope, error) {
	key := requestSource + "::" + requestID
	return r.byScope[key], nil
}

func (r *EnvelopeRepo) List(ctx context.Context) ([]*envelope.Envelope, error) {
	var out []*envelope.Envelope
	for _, v := range r.items {
		out = append(out, v)
	}
	return out, nil
}

// ListByState returns all envelopes in the given lifecycle state.
// An empty state returns all envelopes.
func (r *EnvelopeRepo) ListByState(ctx context.Context, state envelope.EnvelopeState) ([]*envelope.Envelope, error) {
	if state == "" {
		return r.List(ctx)
	}
	var out []*envelope.Envelope
	for _, v := range r.items {
		if v.State == state {
			out = append(out, v)
		}
	}
	return out, nil
}

func (r *EnvelopeRepo) Create(ctx context.Context, e *envelope.Envelope) error {
	r.items[e.ID()] = e
	r.byRequestID[e.RequestID()] = e
	scopeKey := e.RequestSource() + "::" + e.RequestID()
	r.byScope[scopeKey] = e
	return nil
}

func (r *EnvelopeRepo) Update(ctx context.Context, e *envelope.Envelope) error {
	r.items[e.ID()] = e
	// Request source/ID are immutable, so no need to update secondary indexes
	return nil
}

func NewRepositories() *store.Repositories {
	return &store.Repositories{
		Surfaces:      NewSurfaceRepo(),
		Agents:        NewAgentRepo(),
		Profiles:      NewProfileRepo(),
		Grants:        NewGrantRepo(),
		Envelopes:     NewEnvelopeRepo(),
		Audit:         audit.NewMemoryRepository(),
		ControlAudit:  NewControlAuditRepo(),
		Outbox:        outbox.NewMemoryRepository(),
		LocalUsers:    NewLocalUserRepo(),
		LocalSessions: NewLocalSessionRepo(),
	}
}

var _ surface.SurfaceRepository = (*SurfaceRepo)(nil)
var _ agent.AgentRepository = (*AgentRepo)(nil)
var _ authority.ProfileRepository = (*ProfileRepo)(nil)
var _ authority.GrantRepository = (*GrantRepo)(nil)
var _ envelope.EnvelopeRepository = (*EnvelopeRepo)(nil)
