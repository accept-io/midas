package store

import (
	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlaudit"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/outbox"
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
	// Outbox is the transactional outbox repository. It is nil-safe at the
	// orchestrator level: callers must check for nil before appending events,
	// which preserves existing behaviour when the outbox is not configured.
	Outbox outbox.Repository
}
