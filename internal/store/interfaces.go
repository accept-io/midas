package store

import (
	"github.com/accept-io/midas/internal/adminaudit"
	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/businessservicecapability"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/controlaudit"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/governanceexpectation"
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
	// BusinessServiceRelationships is the repository for the directed
	// junction between two BusinessServices (Epic 1, PR 1). Like the
	// other junctions it has no lifecycle of its own; nil-safe at apply
	// time so unconfigured deployments degrade gracefully.
	BusinessServiceRelationships businessservice.RelationshipRepository

	// GovernanceExpectations is the repository for declared
	// governance-coverage rules (Issue #51). Used by the matching engine
	// added in a later issue. Nil-safe: emission/lookup sites must check
	// for nil before use, matching the convention for other newer repos.
	GovernanceExpectations governanceexpectation.Repository

	// AI System Registration substrate (Epic 1, PR 2). All three
	// repositories are nil-safe at apply / read time so unconfigured
	// deployments degrade gracefully — mirrors the BSR/GovernanceExpectation
	// posture for newer repos.
	AISystems        aisystem.SystemRepository
	AISystemVersions aisystem.VersionRepository
	AISystemBindings aisystem.BindingRepository
}
