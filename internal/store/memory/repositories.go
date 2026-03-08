package memory

import (
	"context"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/surface"
)

type SurfaceRepo struct {
	items map[string]*surface.DecisionSurface
}

func NewSurfaceRepo() *SurfaceRepo {
	return &SurfaceRepo{items: map[string]*surface.DecisionSurface{}}
}

func (r *SurfaceRepo) FindByID(ctx context.Context, id string) (*surface.DecisionSurface, error) {
	return r.items[id], nil
}

func (r *SurfaceRepo) FindActiveAt(ctx context.Context, id string, at time.Time) (*surface.DecisionSurface, error) {
	return r.items[id], nil
}

func (r *SurfaceRepo) Create(ctx context.Context, s *surface.DecisionSurface) error {
	r.items[s.ID] = s
	return nil
}

func (r *SurfaceRepo) Update(ctx context.Context, s *surface.DecisionSurface) error {
	r.items[s.ID] = s
	return nil
}

func (r *SurfaceRepo) List(ctx context.Context) ([]*surface.DecisionSurface, error) {
	var out []*surface.DecisionSurface
	for _, v := range r.items {
		out = append(out, v)
	}
	return out, nil
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

func (r *ProfileRepo) FindActiveAt(ctx context.Context, id string, at time.Time) (*authority.AuthorityProfile, error) {
	return r.items[id], nil
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

func (r *GrantRepo) FindActiveByAgentAndProfile(ctx context.Context, agentID, profileID string) (*authority.AuthorityGrant, error) {
	for _, g := range r.items {
		if g.AgentID == agentID && g.ProfileID == profileID && g.Status == authority.GrantStatusActive {
			return g, nil
		}
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

func (r *GrantRepo) Revoke(ctx context.Context, id string) error {
	if g, ok := r.items[id]; ok {
		g.Status = authority.GrantStatusRevoked
	}
	return nil
}

type EnvelopeRepo struct {
	items map[string]*envelope.Envelope
}

func NewEnvelopeRepo() *EnvelopeRepo {
	return &EnvelopeRepo{items: map[string]*envelope.Envelope{}}
}

func (r *EnvelopeRepo) GetByID(ctx context.Context, id string) (*envelope.Envelope, error) {
	return r.items[id], nil
}

func (r *EnvelopeRepo) List(ctx context.Context) ([]*envelope.Envelope, error) {
	var out []*envelope.Envelope
	for _, v := range r.items {
		out = append(out, v)
	}
	return out, nil
}

func (r *EnvelopeRepo) Create(ctx context.Context, e *envelope.Envelope) error {
	r.items[e.ID] = e
	return nil
}

func (r *EnvelopeRepo) Update(ctx context.Context, e *envelope.Envelope) error {
	r.items[e.ID] = e
	return nil
}

var _ surface.SurfaceRepository = (*SurfaceRepo)(nil)
var _ agent.AgentRepository = (*AgentRepo)(nil)
var _ authority.ProfileRepository = (*ProfileRepo)(nil)
var _ authority.GrantRepository = (*GrantRepo)(nil)
var _ envelope.EnvelopeRepository = (*EnvelopeRepo)(nil)
