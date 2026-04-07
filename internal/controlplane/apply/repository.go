package apply

import (
	"context"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/controlaudit"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/processbusinessservice"
	"github.com/accept-io/midas/internal/processcapability"
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

// ProcessRepository is the persistence interface for the apply service's process operations.
// Exists is used to validate Surface.spec.process_id references. GetByID and Create
// are used when Process documents are included in the applied bundle.
type ProcessRepository interface {
	Exists(ctx context.Context, id string) (bool, error)
	GetByID(ctx context.Context, id string) (*process.Process, error)
	Create(ctx context.Context, p *process.Process) error
}

// CapabilityRepository is the persistence interface for the apply service's capability operations.
// Exists is used to validate Process.spec.capability_id references. GetByID and Create
// are used when Capability documents are included in the applied bundle.
type CapabilityRepository interface {
	Exists(ctx context.Context, id string) (bool, error)
	GetByID(ctx context.Context, id string) (*capability.Capability, error)
	Create(ctx context.Context, c *capability.Capability) error
}

// BusinessServiceRepository is the persistence interface for the apply service's
// business service operations. GetByID and Create are used when BusinessService
// documents are included in the applied bundle.
type BusinessServiceRepository interface {
	Exists(ctx context.Context, id string) (bool, error)
	GetByID(ctx context.Context, id string) (*businessservice.BusinessService, error)
	Create(ctx context.Context, s *businessservice.BusinessService) error
}

// ProcessCapabilityRepository is the persistence interface for the apply service's
// process capability operations. ListByProcessID is used to check whether a
// (process_id, capability_id) link already exists during planning. Create persists
// a new link.
type ProcessCapabilityRepository interface {
	Create(ctx context.Context, pc *processcapability.ProcessCapability) error
	ListByProcessID(ctx context.Context, processID string) ([]*processcapability.ProcessCapability, error)
}

// ProcessBusinessServiceRepository is the persistence interface for the apply service's
// process business service operations. ListByProcessID is used to check whether a
// (process_id, business_service_id) link already exists during planning. Create persists
// a new link.
type ProcessBusinessServiceRepository interface {
	Create(ctx context.Context, pbs *processbusinessservice.ProcessBusinessService) error
	ListByProcessID(ctx context.Context, processID string) ([]*processbusinessservice.ProcessBusinessService, error)
}

type RepositorySet struct {
	Surfaces                SurfaceRepository
	Agents                  AgentRepository
	Profiles                ProfileRepository
	Grants                  GrantRepository
	Processes               ProcessRepository
	Capabilities            CapabilityRepository
	BusinessServices        BusinessServiceRepository
	ProcessCapabilities     ProcessCapabilityRepository
	ProcessBusinessServices ProcessBusinessServiceRepository
	ControlAudit            controlaudit.Repository
}
