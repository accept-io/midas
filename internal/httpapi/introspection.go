package httpapi

import (
	"context"
	"sort"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/surface"
)

// ImpactSummary holds aggregate counts for a surface's dependency graph.
// Computed by GetSurfaceImpact in the service layer, not the HTTP handler.
type ImpactSummary struct {
	ProfileCount       int
	GrantCount         int
	AgentCount         int
	ActiveProfileCount int
	ActiveGrantCount   int
	ActiveAgentCount   int
}

// SurfaceImpactResult is the assembled dependency analysis for a decision surface:
// the surface itself, all profiles referencing it, all grants referencing those
// profiles, and the distinct agents referenced by those grants. Ordering is
// stable: profiles, grants, and agents are sorted by ID ascending. Warnings
// are deterministic, based only on active-count thresholds.
type SurfaceImpactResult struct {
	Surface  *surface.DecisionSurface
	Profiles []*authority.AuthorityProfile // sorted by ID ascending
	Grants   []*authority.AuthorityGrant   // sorted by ID ascending; one row per grant across all profiles
	Agents   []*agent.Agent               // deduplicated, sorted by ID ascending
	Summary  ImpactSummary
	Warnings []string
}

// SurfaceReader is the surface repository subset needed for introspection.
type SurfaceReader interface {
	FindLatestByID(ctx context.Context, id string) (*surface.DecisionSurface, error)
	ListVersions(ctx context.Context, id string) ([]*surface.DecisionSurface, error)
}

// ProfileReader is the profile repository subset needed for introspection.
type ProfileReader interface {
	FindByID(ctx context.Context, id string) (*authority.AuthorityProfile, error)
	ListBySurface(ctx context.Context, surfaceID string) ([]*authority.AuthorityProfile, error)
	ListVersions(ctx context.Context, id string) ([]*authority.AuthorityProfile, error)
}

// AgentReader is the agent repository subset needed for introspection.
type AgentReader interface {
	GetByID(ctx context.Context, id string) (*agent.Agent, error)
}

// GrantReader is the grant repository subset needed for introspection.
type GrantReader interface {
	FindByID(ctx context.Context, id string) (*authority.AuthorityGrant, error)
	ListByAgent(ctx context.Context, agentID string) ([]*authority.AuthorityGrant, error)
	ListByProfile(ctx context.Context, profileID string) ([]*authority.AuthorityGrant, error)
}

// IntrospectionService satisfies the introspectionService interface by delegating
// to the underlying repository implementations.
type IntrospectionService struct {
	surfaces SurfaceReader
	profiles ProfileReader
	agents   AgentReader
	grants   GrantReader
}

// NewIntrospectionService constructs an IntrospectionService with surface and
// profile readers. Use NewIntrospectionServiceFull to also enable agent and grant
// endpoints.
func NewIntrospectionService(surfaces SurfaceReader, profiles ProfileReader) *IntrospectionService {
	return &IntrospectionService{
		surfaces: surfaces,
		profiles: profiles,
	}
}

// NewIntrospectionServiceFull constructs an IntrospectionService with all readers wired.
// All parameters must be non-nil.
func NewIntrospectionServiceFull(surfaces SurfaceReader, profiles ProfileReader, agents AgentReader, grants GrantReader) *IntrospectionService {
	return &IntrospectionService{
		surfaces: surfaces,
		profiles: profiles,
		agents:   agents,
		grants:   grants,
	}
}

// GetSurface returns the latest persisted version of a surface.
// Returns nil, nil when the surface does not exist.
func (s *IntrospectionService) GetSurface(ctx context.Context, id string) (*surface.DecisionSurface, error) {
	return s.surfaces.FindLatestByID(ctx, id)
}

// ListSurfaceVersions returns all versions of a surface in ascending version order.
// Returns an empty slice when the surface does not exist.
func (s *IntrospectionService) ListSurfaceVersions(ctx context.Context, id string) ([]*surface.DecisionSurface, error) {
	return s.surfaces.ListVersions(ctx, id)
}

// ListProfilesBySurface returns all profiles for the given surface.
// Returns an empty slice when no profiles are attached.
func (s *IntrospectionService) ListProfilesBySurface(ctx context.Context, surfaceID string) ([]*authority.AuthorityProfile, error) {
	return s.profiles.ListBySurface(ctx, surfaceID)
}

// GetProfile returns the latest version of a profile by its logical ID.
// Returns nil, nil when the profile does not exist.
func (s *IntrospectionService) GetProfile(ctx context.Context, id string) (*authority.AuthorityProfile, error) {
	if s.profiles == nil {
		return nil, nil
	}
	return s.profiles.FindByID(ctx, id)
}

// ListProfileVersions returns all versions of a profile ordered by version
// descending (latest first). Returns an empty slice when no profile with that
// logical ID exists.
func (s *IntrospectionService) ListProfileVersions(ctx context.Context, id string) ([]*authority.AuthorityProfile, error) {
	if s.profiles == nil {
		return []*authority.AuthorityProfile{}, nil
	}
	return s.profiles.ListVersions(ctx, id)
}

// GetAgent returns an agent by ID.
// Returns nil, nil when the agent does not exist.
func (s *IntrospectionService) GetAgent(ctx context.Context, id string) (*agent.Agent, error) {
	if s.agents == nil {
		return nil, nil
	}
	return s.agents.GetByID(ctx, id)
}

// GetGrant returns a single grant by its ID.
// Returns nil, nil when the grant does not exist.
func (s *IntrospectionService) GetGrant(ctx context.Context, id string) (*authority.AuthorityGrant, error) {
	if s.grants == nil {
		return nil, nil
	}
	return s.grants.FindByID(ctx, id)
}

// ListGrantsByAgent returns all grants for the given agent ID.
func (s *IntrospectionService) ListGrantsByAgent(ctx context.Context, agentID string) ([]*authority.AuthorityGrant, error) {
	if s.grants == nil {
		return []*authority.AuthorityGrant{}, nil
	}
	return s.grants.ListByAgent(ctx, agentID)
}

// ListGrantsByProfile returns all grants for the given profile ID.
func (s *IntrospectionService) ListGrantsByProfile(ctx context.Context, profileID string) ([]*authority.AuthorityGrant, error) {
	if s.grants == nil {
		return []*authority.AuthorityGrant{}, nil
	}
	return s.grants.ListByProfile(ctx, profileID)
}

// GetSurfaceImpact assembles the full dependency graph for a decision surface:
// profiles referencing the surface → grants referencing those profiles →
// distinct agents referenced by those grants. Returns nil, nil when the surface
// does not exist.
//
// Ordering guarantees:
//   - Profiles: sorted by ID ascending
//   - Grants: sorted by ID ascending (all profiles combined, not grouped)
//   - Agents: deduplicated by ID, sorted by ID ascending
//
// Summary counts are computed in the service layer. Warnings are deterministic,
// emitted in a fixed order based on active-count thresholds only.
//
// If the grants or agents readers are nil (partial wiring), grants and agents
// are returned as empty slices with a zero summary — the surface and profiles
// sections are still populated.
func (s *IntrospectionService) GetSurfaceImpact(ctx context.Context, surfaceID string) (*SurfaceImpactResult, error) {
	surf, err := s.surfaces.FindLatestByID(ctx, surfaceID)
	if err != nil {
		return nil, err
	}
	if surf == nil {
		return nil, nil
	}

	profiles, err := s.profiles.ListBySurface(ctx, surfaceID)
	if err != nil {
		return nil, err
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].ID < profiles[j].ID
	})

	var allGrants []*authority.AuthorityGrant
	if s.grants != nil {
		for _, p := range profiles {
			gs, err := s.grants.ListByProfile(ctx, p.ID)
			if err != nil {
				return nil, err
			}
			allGrants = append(allGrants, gs...)
		}
		sort.Slice(allGrants, func(i, j int) bool {
			return allGrants[i].ID < allGrants[j].ID
		})
	}

	var agents []*agent.Agent
	if s.agents != nil {
		seen := make(map[string]struct{})
		for _, g := range allGrants {
			if _, ok := seen[g.AgentID]; ok {
				continue
			}
			seen[g.AgentID] = struct{}{}
			ag, err := s.agents.GetByID(ctx, g.AgentID)
			if err != nil {
				return nil, err
			}
			if ag != nil {
				agents = append(agents, ag)
			}
		}
		sort.Slice(agents, func(i, j int) bool {
			return agents[i].ID < agents[j].ID
		})
	}

	summary := buildImpactSummary(profiles, allGrants, agents)
	warnings := buildImpactWarnings(summary)

	return &SurfaceImpactResult{
		Surface:  surf,
		Profiles: profiles,
		Grants:   allGrants,
		Agents:   agents,
		Summary:  summary,
		Warnings: warnings,
	}, nil
}

// buildImpactSummary computes aggregate counts from the already-assembled
// profiles, grants, and agents slices. Called once per GetSurfaceImpact.
func buildImpactSummary(profiles []*authority.AuthorityProfile, grants []*authority.AuthorityGrant, agents []*agent.Agent) ImpactSummary {
	s := ImpactSummary{
		ProfileCount: len(profiles),
		GrantCount:   len(grants),
		AgentCount:   len(agents),
	}
	for _, p := range profiles {
		if p.Status == authority.ProfileStatusActive {
			s.ActiveProfileCount++
		}
	}
	for _, g := range grants {
		if g.Status == authority.GrantStatusActive {
			s.ActiveGrantCount++
		}
	}
	for _, a := range agents {
		if a.OperationalState == agent.OperationalStateActive {
			s.ActiveAgentCount++
		}
	}
	return s
}

// buildImpactWarnings emits deterministic, human-readable warnings based on
// the active counts in the summary. Warnings are emitted in a fixed order:
// profiles first, then grants, then agents.
func buildImpactWarnings(s ImpactSummary) []string {
	var w []string
	if s.ActiveProfileCount > 0 {
		w = append(w, "surface has active profiles")
	}
	if s.ActiveGrantCount > 0 {
		w = append(w, "surface has active grants")
	}
	if s.ActiveAgentCount > 0 {
		w = append(w, "deprecating may affect active agent authority")
	}
	if w == nil {
		w = []string{} // never null in JSON
	}
	return w
}
