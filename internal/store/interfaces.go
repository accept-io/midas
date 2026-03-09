package store

import (
	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/surface"
)

type Repositories struct {
	Surfaces  surface.SurfaceRepository
	Agents    agent.AgentRepository
	Profiles  authority.ProfileRepository
	Grants    authority.GrantRepository
	Envelopes envelope.EnvelopeRepository
	Audit     audit.AuditEventRepository
}
