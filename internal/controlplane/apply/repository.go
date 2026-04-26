package apply

import (
	"context"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/businessservicecapability"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/controlaudit"
	"github.com/accept-io/midas/internal/process"
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

// BusinessServiceCapabilityRepository is the persistence interface for the
// apply service's M:N junction operations between BusinessService and
// Capability. Exists is used by the planner to detect already-linked pairs;
// Create persists a new link.
type BusinessServiceCapabilityRepository interface {
	Exists(ctx context.Context, businessServiceID, capabilityID string) (bool, error)
	Create(ctx context.Context, bsc *businessservicecapability.BusinessServiceCapability) error
}

type RepositorySet struct {
	Surfaces                   SurfaceRepository
	Agents                     AgentRepository
	Profiles                   ProfileRepository
	Grants                     GrantRepository
	Processes                  ProcessRepository
	Capabilities               CapabilityRepository
	BusinessServices           BusinessServiceRepository
	BusinessServiceCapabilities BusinessServiceCapabilityRepository
	ControlAudit               controlaudit.Repository
	// Tx, when non-nil, wraps the executor's mutation loop in an
	// atomic transaction. The callback receives a scoped *RepositorySet
	// whose repositories are bound to the transaction; on callback-
	// returned error the transaction is rolled back and no mutations
	// from the bundle remain persisted. When Tx is nil the executor
	// still aborts on the first persistence error, but cannot roll
	// prior writes back — this is the dev/memory-store posture.
	Tx TxRunner
}

// TxRunner executes a callback inside a storage transaction. The callback
// receives a *RepositorySet whose repositories write through the transaction
// rather than against auto-commit connections. Implementations must:
//
//   - commit when fn returns nil
//   - roll back when fn returns a non-nil error, and propagate that error
//   - roll back and re-panic when fn panics, not swallow the panic
//
// ControlAudit on the scoped set may be nil or may be a no-op. By the
// control-plane audit policy (ADR-041b) audit writes do not participate in
// this transaction: the executor buffers audit records during the loop and
// flushes them only after the transaction commits. Implementations should
// therefore feel free to leave the scoped ControlAudit unset.
type TxRunner interface {
	WithTx(ctx context.Context, operation string, fn func(*RepositorySet) error) error
}
