package memory

import (
	"context"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/surface"
)

type SurfaceRepo struct {
	items map[string]*surface.DecisionSurface
}

func NewSurfaceRepo() *SurfaceRepo {
	return &SurfaceRepo{items: map[string]*surface.DecisionSurface{}}
}

// FindLatestByID returns the latest version (renamed from FindByID)
func (r *SurfaceRepo) FindLatestByID(ctx context.Context, id string) (*surface.DecisionSurface, error) {
	return r.items[id], nil
}

// FindByIDVersion returns a specific version (stub for now - memory store doesn't do versioning yet)
func (r *SurfaceRepo) FindByIDVersion(ctx context.Context, id string, version int) (*surface.DecisionSurface, error) {
	s := r.items[id]
	if s == nil {
		return nil, nil
	}
	if s.Version != version {
		return nil, nil // Version mismatch
	}
	return s, nil
}

func (r *SurfaceRepo) FindActiveAt(ctx context.Context, id string, at time.Time) (*surface.DecisionSurface, error) {
	s := r.items[id]
	if s == nil {
		return nil, nil
	}

	// Check if active at given time
	if s.Status != surface.SurfaceStatusActive {
		return nil, nil
	}
	if s.EffectiveFrom.After(at) {
		return nil, nil
	}
	if s.EffectiveUntil != nil && !s.EffectiveUntil.After(at) {
		return nil, nil
	}

	return s, nil
}

// ListVersions returns all versions of a surface (stub - memory store has one version per ID)
func (r *SurfaceRepo) ListVersions(ctx context.Context, id string) ([]*surface.DecisionSurface, error) {
	s := r.items[id]
	if s == nil {
		return []*surface.DecisionSurface{}, nil
	}
	return []*surface.DecisionSurface{s}, nil
}

// ListAll returns latest version of each surface (renamed from List)
func (r *SurfaceRepo) ListAll(ctx context.Context) ([]*surface.DecisionSurface, error) {
	var out []*surface.DecisionSurface
	for _, v := range r.items {
		out = append(out, v)
	}
	return out, nil
}

// ListByStatus returns surfaces with given status
func (r *SurfaceRepo) ListByStatus(ctx context.Context, status surface.SurfaceStatus) ([]*surface.DecisionSurface, error) {
	var out []*surface.DecisionSurface
	for _, v := range r.items {
		if v.Status == status {
			out = append(out, v)
		}
	}
	return out, nil
}

// ListByDomain returns surfaces in given domain
func (r *SurfaceRepo) ListByDomain(ctx context.Context, domain string) ([]*surface.DecisionSurface, error) {
	var out []*surface.DecisionSurface
	for _, v := range r.items {
		if v.Domain == domain {
			out = append(out, v)
		}
	}
	return out, nil
}

// Search finds surfaces matching criteria
func (r *SurfaceRepo) Search(ctx context.Context, criteria surface.SearchCriteria) ([]*surface.DecisionSurface, error) {
	var out []*surface.DecisionSurface

	for _, s := range r.items {
		if !matchesCriteria(s, criteria) {
			continue
		}
		out = append(out, s)
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

func (r *SurfaceRepo) Create(ctx context.Context, s *surface.DecisionSurface) error {
	// In memory store, we use ID as key (no versioning support yet)
	r.items[s.ID] = s
	return nil
}

func (r *SurfaceRepo) Update(ctx context.Context, s *surface.DecisionSurface) error {
	r.items[s.ID] = s
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

type ProfileRepo struct {
	items map[string]*authority.AuthorityProfile
}

func NewProfileRepo() *ProfileRepo {
	return &ProfileRepo{items: map[string]*authority.AuthorityProfile{}}
}

func (r *ProfileRepo) FindByID(ctx context.Context, id string) (*authority.AuthorityProfile, error) {
	return r.items[id], nil
}

// FindByIDAndVersion returns a specific profile version (stub - memory store has one version per ID)
func (r *ProfileRepo) FindByIDAndVersion(ctx context.Context, id string, version int) (*authority.AuthorityProfile, error) {
	p := r.items[id]
	if p == nil {
		return nil, nil
	}
	if p.Version != version {
		return nil, nil // Version mismatch
	}
	return p, nil
}

// FindActiveAt returns profile if status='active' and date checks pass (schema v2.1)
func (r *ProfileRepo) FindActiveAt(ctx context.Context, id string, at time.Time) (*authority.AuthorityProfile, error) {
	p := r.items[id]
	if p == nil {
		return nil, nil
	}

	// Schema v2.1: Check status field
	if p.Status != authority.ProfileStatusActive {
		return nil, nil
	}

	// Check effective date range
	if p.EffectiveDate.After(at) {
		return nil, nil
	}
	if p.EffectiveUntil != nil && !p.EffectiveUntil.After(at) {
		return nil, nil
	}

	return p, nil
}

func (r *ProfileRepo) ListBySurface(ctx context.Context, surfaceID string) ([]*authority.AuthorityProfile, error) {
	var out []*authority.AuthorityProfile
	for _, v := range r.items {
		if v.SurfaceID == surfaceID {
			out = append(out, v)
		}
	}
	return out, nil
}

// ListVersions returns all versions of a profile (stub - memory store has one version per ID)
func (r *ProfileRepo) ListVersions(ctx context.Context, id string) ([]*authority.AuthorityProfile, error) {
	p := r.items[id]
	if p == nil {
		return []*authority.AuthorityProfile{}, nil
	}
	return []*authority.AuthorityProfile{p}, nil
}

func (r *ProfileRepo) Create(ctx context.Context, p *authority.AuthorityProfile) error {
	r.items[p.ID] = p
	return nil
}

func (r *ProfileRepo) Update(ctx context.Context, p *authority.AuthorityProfile) error {
	r.items[p.ID] = p
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
		Surfaces:  NewSurfaceRepo(),
		Agents:    NewAgentRepo(),
		Profiles:  NewProfileRepo(),
		Grants:    NewGrantRepo(),
		Envelopes: NewEnvelopeRepo(),
		Audit:     audit.NewMemoryRepository(),
	}
}

var _ surface.SurfaceRepository = (*SurfaceRepo)(nil)
var _ agent.AgentRepository = (*AgentRepo)(nil)
var _ authority.ProfileRepository = (*ProfileRepo)(nil)
var _ authority.GrantRepository = (*GrantRepo)(nil)
var _ envelope.EnvelopeRepository = (*EnvelopeRepo)(nil)
