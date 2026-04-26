package store

import (
	"github.com/accept-io/midas/internal/adminaudit"
	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/businessservicecapability"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/controlaudit"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/localiam"
	"github.com/accept-io/midas/internal/outbox"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

type Repositories struct {
	Surfaces     surface.SurfaceRepository
	Agents       agent.AgentRepository
	Profiles     authority.ProfileRepository
	Grants       authority.GrantRepository
	Envelopes    envelope.EnvelopeRepository
	Audit        audit.AuditEventRepository
	ControlAudit controlaudit.Repository
	// AdminAudit is the append-only administrative audit trail (Issue #41).
	// Nil-safe: emission sites must check for nil before appending so that
	// tests and memory-mode deployments not configured with the repository
	// continue to function.
	AdminAudit adminaudit.Repository
	// Outbox is the transactional outbox repository. It is nil-safe at the
	// orchestrator level: callers must check for nil before appending events,
	// which preserves existing behaviour when the outbox is not configured.
	Outbox outbox.Repository
	// LocalUsers and LocalSessions are nil when local platform IAM is disabled.
	LocalUsers    localiam.UserRepository
	LocalSessions localiam.SessionRepository

	Capabilities                capability.CapabilityRepository
	Processes                   process.ProcessRepository
	BusinessServices            businessservice.BusinessServiceRepository
	BusinessServiceCapabilities businessservicecapability.BusinessServiceCapabilityRepository
}
