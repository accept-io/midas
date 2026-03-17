package apply

import (
	"context"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/surface"
)

type SurfaceRepository interface {
	FindLatestByID(ctx context.Context, id string) (*surface.DecisionSurface, error)
	Create(ctx context.Context, s *surface.DecisionSurface) error
}

type AgentRepository interface {
	GetByID(ctx context.Context, id string) (*agent.Agent, error)
	Create(ctx context.Context, a *agent.Agent) error
}

type ProfileRepository interface {
	FindByID(ctx context.Context, id string) (*authority.AuthorityProfile, error)
	Create(ctx context.Context, p *authority.AuthorityProfile) error
}

type GrantRepository interface {
	FindByID(ctx context.Context, id string) (*authority.AuthorityGrant, error)
	Create(ctx context.Context, g *authority.AuthorityGrant) error
}

type RepositorySet struct {
	Surfaces SurfaceRepository
	Agents   AgentRepository
	Profiles ProfileRepository
	Grants   GrantRepository
}
