// Package governancemap assembles a service-centric governance map for
// a single BusinessService (Epic 1, PR 4). The Map type is the read
// model; the ReadService composes existing repository readers via
// narrow per-entity interfaces and produces it via sequenced calls.
//
// Query strategy: sequenced repository calls, not multi-join SQL.
// Reasoning per the PR 4 brief:
//
//   - The endpoint serves a single business service, so query volume is
//     bounded (one BS, its relationships, its processes, etc.)
//   - Memory/Postgres parity is non-negotiable; sequenced calls give it
//     for free since each call goes through existing repository methods
//   - Each repository call is already tested in isolation; the read
//     service tests focus on aggregation logic
//   - Mirrors GCA's /v1/coverage read-service pattern from issue #56
//
// Performance optimisation (multi-join SQL, materialised views, caching)
// is deferred to a focused future PR if data volumes warrant it.
//
// Reader interface placement: the package defines its own narrow reader
// interfaces tailored to the aggregation it performs. Each interface is
// satisfied by the corresponding domain repository in
// internal/<entity>/<entity>.go. The HTTP layer's StructuralService
// (internal/httpapi/structural.go) is intentionally NOT extended for
// the governance map — the consumer-narrow pattern keeps the structural
// service stable and surfaces exactly what aggregation depends on.
//
// Surface filtering — known limitation:
//
// The read service uses SurfaceReader.ListByProcessID, which returns
// the latest version per surface logical ID, then filters to
// Status == "active" in code. This means a surface with v2 in `review`
// while v1 is `active` yields zero surfaces in the response (v2 is
// returned by ListByProcessID, then filtered out as non-active; the
// truly-active v1 is missed). This is unusual in practice — operators
// typically deprecate v1 before promoting v2 to active — but worth
// documenting. A future PR could add a multi-version-aware
// ListActiveByProcessID and use it here.
//
// Bounded N+1 on grants:
//
// active_grant_count is computed via GrantReader.ListByProfile per
// active profile. This is N+1 on profiles within the BS, but bounded
// in practice (typically <10 surfaces × 1-2 active profiles = ~10-20
// grant lookups per request). Memory mode is in-RAM map lookup; Postgres
// mode is a few extra queries. A focused performance PR can add a
// batched ListByProfiles when data volumes warrant.
//
// Recent decisions deferred:
//
// PR 4 omits the recent_decisions field entirely. The Envelope domain
// struct does not denormalise resolved_business_service_id (the column
// exists at the schema level but is not exposed on the Go type), and
// EnvelopeRepository has no ListByResolvedBusinessService method. Per
// the PR 4 brief, PR 8 will revisit envelope shape when adding AI
// system fields; the recent_decisions section ships with that PR.
package governancemap

import (
	"context"
	"errors"
	"sort"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/businessservicecapability"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// Reader interfaces — narrow, consumer-fit, satisfied by existing repos
// ---------------------------------------------------------------------------

// BusinessServiceReader is the BS subset needed to load the root.
type BusinessServiceReader interface {
	GetByID(ctx context.Context, id string) (*businessservice.BusinessService, error)
}

// BusinessServiceRelationshipReader covers both relationship directions.
type BusinessServiceRelationshipReader interface {
	ListBySourceBusinessService(ctx context.Context, sourceID string) ([]*businessservice.BusinessServiceRelationship, error)
	ListByTargetBusinessService(ctx context.Context, targetID string) ([]*businessservice.BusinessServiceRelationship, error)
}

// BusinessServiceCapabilityReader is the BSC subset needed to fetch the
// capability links for a single business service.
type BusinessServiceCapabilityReader interface {
	ListByBusinessServiceID(ctx context.Context, businessServiceID string) ([]*businessservicecapability.BusinessServiceCapability, error)
}

// CapabilityReader is the capability subset needed to dereference BSC
// rows into Capability records.
type CapabilityReader interface {
	GetByID(ctx context.Context, id string) (*capability.Capability, error)
}

// ProcessReader uses the new ListByBusinessService method added by
// Cluster B (a single domain interface extension) to avoid scanning
// the full processes table.
type ProcessReader interface {
	ListByBusinessService(ctx context.Context, businessServiceID string) ([]*process.Process, error)
}

// SurfaceReader returns latest-version surfaces under a given process.
// The read service filters to Status == "active" in code.
type SurfaceReader interface {
	ListByProcessID(ctx context.Context, processID string) ([]*surface.DecisionSurface, error)
}

// ProfileReader is the AuthorityProfile subset needed for per-surface
// authority counts. ListBySurface returns all versions; the read service
// filters to active in code.
type ProfileReader interface {
	ListBySurface(ctx context.Context, surfaceID string) ([]*authority.AuthorityProfile, error)
}

// GrantReader is the AuthorityGrant subset needed for per-profile grant
// counts. ListByProfile returns all grants; the read service filters
// to active in code.
type GrantReader interface {
	ListByProfile(ctx context.Context, profileID string) ([]*authority.AuthorityGrant, error)
}

// AISystemReader resolves binding ai_system_id to system records.
type AISystemReader interface {
	GetByID(ctx context.Context, id string) (*aisystem.AISystem, error)
}

// AISystemVersionReader resolves the active version per AI system.
type AISystemVersionReader interface {
	GetActiveBySystem(ctx context.Context, aiSystemID string) (*aisystem.AISystemVersion, error)
}

// AISystemBindingReader is the binding subset needed for the four-way
// OR inclusion logic (BS / Capability / Process / Surface).
type AISystemBindingReader interface {
	ListByBusinessService(ctx context.Context, bsID string) ([]*aisystem.AISystemBinding, error)
	ListByCapability(ctx context.Context, capID string) ([]*aisystem.AISystemBinding, error)
	ListByProcess(ctx context.Context, procID string) ([]*aisystem.AISystemBinding, error)
	ListBySurface(ctx context.Context, surfID string) ([]*aisystem.AISystemBinding, error)
}

// ---------------------------------------------------------------------------
// Map — the read model
// ---------------------------------------------------------------------------

// Map is the complete governance map for a single business service.
// All slices are non-nil (possibly empty). AuthoritySummary and Coverage
// are always non-nil — operators rely on the field's presence even when
// counts are zero.
//
// RecentDecisions is always nil in PR 4 (per Step 0.5 deferral). PR 8
// will populate this field when the envelope substrate is ready.
type Map struct {
	BusinessService  *BusinessServiceNode
	Relationships    Relationships
	Capabilities     []*CapabilityNode
	Processes        []*ProcessNode
	Surfaces         []*SurfaceNode
	AISystems        []*AISystemNode
	AuthoritySummary *AuthoritySummary
	Coverage         *Coverage
	RecentDecisions  *RecentDecisions
}

// BusinessServiceNode wraps the root business service. Kept as a struct
// (not a bare BusinessService pointer) so future fields can be added
// without churning callers.
type BusinessServiceNode struct {
	BusinessService *businessservice.BusinessService
}

// Relationships partitions BSR rows by direction. Both slices are
// always non-nil.
type Relationships struct {
	Outgoing []*RelationshipNode
	Incoming []*RelationshipNode
}

// RelationshipNode pairs a BSR row with the opposing BS's name (target
// name for outgoing, source name for incoming) when cheaply available.
// OtherName is empty when the opposing BS cannot be loaded.
type RelationshipNode struct {
	Relationship *businessservice.BusinessServiceRelationship
	OtherName    string
}

// CapabilityNode wraps a capability linked to the root BS via a
// BusinessServiceCapability junction row.
type CapabilityNode struct {
	Capability *capability.Capability
}

// ProcessNode wraps a process under the root BS.
type ProcessNode struct {
	Process *process.Process
}

// SurfaceNode is an active surface plus per-surface counts and AI
// binding IDs. AIBindingIDs is non-nil (possibly empty).
type SurfaceNode struct {
	Surface      *surface.DecisionSurface
	AIBindingIDs []string
	ProfileCount int
	GrantCount   int
	AgentCount   int
}

// AISystemNode wraps an included AI system. ActiveVersion may be nil
// when no version is in active status. Bindings contains only the
// bindings relevant to the root BS context (not all bindings of the
// AI system) — see the brief's "unrelated bindings exclusion" rule.
type AISystemNode struct {
	System        *aisystem.AISystem
	ActiveVersion *aisystem.AISystemVersion
	Bindings      []*aisystem.AISystemBinding
}

// AuthoritySummary aggregates the four authority counts across all
// surfaces in the response. Each count uses distinct sets at its level
// (profiles distinct across surfaces, grants distinct across profiles,
// agents distinct across grants).
type AuthoritySummary struct {
	SurfaceCount       int
	ActiveProfileCount int
	ActiveGrantCount   int
	ActiveAgentCount   int
}

// Coverage captures AI binding coverage across the root BS's surfaces.
type Coverage struct {
	SurfaceCount             int
	SurfacesWithAIBinding    int
	SurfacesWithoutAIBinding int
}

// RecentDecisions is reserved for PR 8. Always nil in PR 4.
type RecentDecisions struct {
	WindowSeconds int
	ByOutcome     map[string]int
}

// ---------------------------------------------------------------------------
// ReadService
// ---------------------------------------------------------------------------

// ReadService composes the narrow readers above to produce a Map.
// All readers are required — passing nil for any reader makes
// GetGovernanceMap return ErrServiceNotConfigured. The HTTP handler
// uses HasAllReaders() to map this to a 501 response.
type ReadService struct {
	businessServices             BusinessServiceReader
	businessServiceRelationships BusinessServiceRelationshipReader
	businessServiceCapabilities  BusinessServiceCapabilityReader
	capabilities                 CapabilityReader
	processes                    ProcessReader
	surfaces                     SurfaceReader
	profiles                     ProfileReader
	grants                       GrantReader
	aiSystems                    AISystemReader
	aiSystemVersions             AISystemVersionReader
	aiSystemBindings             AISystemBindingReader
}

// NewReadService constructs a ReadService with all readers wired.
// Any reader may be nil at construction; GetGovernanceMap returns
// ErrServiceNotConfigured when invoked with a missing reader, which
// the handler maps to 501.
func NewReadService(
	businessServices BusinessServiceReader,
	businessServiceRelationships BusinessServiceRelationshipReader,
	businessServiceCapabilities BusinessServiceCapabilityReader,
	capabilities CapabilityReader,
	processes ProcessReader,
	surfaces SurfaceReader,
	profiles ProfileReader,
	grants GrantReader,
	aiSystems AISystemReader,
	aiSystemVersions AISystemVersionReader,
	aiSystemBindings AISystemBindingReader,
) *ReadService {
	return &ReadService{
		businessServices:             businessServices,
		businessServiceRelationships: businessServiceRelationships,
		businessServiceCapabilities:  businessServiceCapabilities,
		capabilities:                 capabilities,
		processes:                    processes,
		surfaces:                     surfaces,
		profiles:                     profiles,
		grants:                       grants,
		aiSystems:                    aiSystems,
		aiSystemVersions:             aiSystemVersions,
		aiSystemBindings:             aiSystemBindings,
	}
}

// HasAllReaders reports whether every required reader is wired.
// The HTTP handler uses this to return 501 when the read service is
// missing a dependency rather than panicking on nil dereference.
func (s *ReadService) HasAllReaders() bool {
	return s != nil &&
		s.businessServices != nil &&
		s.businessServiceRelationships != nil &&
		s.businessServiceCapabilities != nil &&
		s.capabilities != nil &&
		s.processes != nil &&
		s.surfaces != nil &&
		s.profiles != nil &&
		s.grants != nil &&
		s.aiSystems != nil &&
		s.aiSystemVersions != nil &&
		s.aiSystemBindings != nil
}

// ErrServiceNotConfigured is returned when GetGovernanceMap is called on
// a ReadService with one or more nil readers.
var ErrServiceNotConfigured = errors.New("governance map read service: required reader not configured")

// GetGovernanceMap returns the governance map for the given business
// service ID. Contract:
//
//   - (nil, nil) when the business service does not exist (handler maps to 404)
//   - (nil, ErrServiceNotConfigured) when readers are missing (handler maps to 501)
//   - (nil, err) on any repository error (handler maps to 500)
//   - (map, nil) on success (handler maps to 200)
//
// All slice fields on the returned Map are non-nil. AuthoritySummary
// and Coverage are non-nil with zero values for an empty service.
// RecentDecisions is always nil in PR 4 (deferred to PR 8).
func (s *ReadService) GetGovernanceMap(ctx context.Context, businessServiceID string) (*Map, error) {
	if !s.HasAllReaders() {
		return nil, ErrServiceNotConfigured
	}

	// 1. Root business service.
	bs, err := s.businessServices.GetByID(ctx, businessServiceID)
	if err != nil {
		return nil, err
	}
	if bs == nil {
		return nil, nil
	}

	out := &Map{
		BusinessService: &BusinessServiceNode{BusinessService: bs},
		Relationships: Relationships{
			Outgoing: []*RelationshipNode{},
			Incoming: []*RelationshipNode{},
		},
		Capabilities:     []*CapabilityNode{},
		Processes:        []*ProcessNode{},
		Surfaces:         []*SurfaceNode{},
		AISystems:        []*AISystemNode{},
		AuthoritySummary: &AuthoritySummary{},
		Coverage:         &Coverage{},
		// RecentDecisions intentionally nil — Step 0.5 deferral to PR 8.
	}

	// 2. Relationships, with opposing-BS name lookups (bounded).
	if err := s.loadRelationships(ctx, bs.ID, out); err != nil {
		return nil, err
	}

	// 3. Capabilities via BSC junction.
	if err := s.loadCapabilities(ctx, bs.ID, out); err != nil {
		return nil, err
	}

	// 4. Processes.
	procs, err := s.processes.ListByBusinessService(ctx, bs.ID)
	if err != nil {
		return nil, err
	}
	for _, p := range procs {
		out.Processes = append(out.Processes, &ProcessNode{Process: p})
	}

	// 5. Surfaces (active only) under each process.
	if err := s.loadSurfacesAndAuthority(ctx, out); err != nil {
		return nil, err
	}

	// 6. AI systems via four-way OR + dedup.
	if err := s.loadAISystems(ctx, bs.ID, out); err != nil {
		return nil, err
	}

	// 7. Coverage counts derive from the surfaces array.
	out.Coverage.SurfaceCount = len(out.Surfaces)
	for _, sn := range out.Surfaces {
		if len(sn.AIBindingIDs) > 0 {
			out.Coverage.SurfacesWithAIBinding++
		} else {
			out.Coverage.SurfacesWithoutAIBinding++
		}
	}
	out.AuthoritySummary.SurfaceCount = len(out.Surfaces)

	return out, nil
}

// loadRelationships populates outgoing and incoming relationship nodes,
// resolving the opposing BS name for each via a single GetByID. Names
// that fail to resolve are left empty rather than aborting the request.
func (s *ReadService) loadRelationships(ctx context.Context, bsID string, out *Map) error {
	outgoing, err := s.businessServiceRelationships.ListBySourceBusinessService(ctx, bsID)
	if err != nil {
		return err
	}
	incoming, err := s.businessServiceRelationships.ListByTargetBusinessService(ctx, bsID)
	if err != nil {
		return err
	}

	// Cache opposing-BS name lookups so a relationship pointing at the
	// same BS multiple times only resolves once.
	nameCache := map[string]string{}
	resolveName := func(ctx context.Context, id string) (string, error) {
		if id == "" {
			return "", nil
		}
		if cached, ok := nameCache[id]; ok {
			return cached, nil
		}
		other, err := s.businessServices.GetByID(ctx, id)
		if err != nil {
			return "", err
		}
		var name string
		if other != nil {
			name = other.Name
		}
		nameCache[id] = name
		return name, nil
	}

	for _, rel := range outgoing {
		name, err := resolveName(ctx, rel.TargetBusinessService)
		if err != nil {
			return err
		}
		out.Relationships.Outgoing = append(out.Relationships.Outgoing, &RelationshipNode{
			Relationship: rel,
			OtherName:    name,
		})
	}
	for _, rel := range incoming {
		name, err := resolveName(ctx, rel.SourceBusinessService)
		if err != nil {
			return err
		}
		out.Relationships.Incoming = append(out.Relationships.Incoming, &RelationshipNode{
			Relationship: rel,
			OtherName:    name,
		})
	}
	return nil
}

// loadCapabilities walks the BSC junction and dereferences each entry
// to a Capability record.
func (s *ReadService) loadCapabilities(ctx context.Context, bsID string, out *Map) error {
	bscEntries, err := s.businessServiceCapabilities.ListByBusinessServiceID(ctx, bsID)
	if err != nil {
		return err
	}
	for _, bsc := range bscEntries {
		cap, err := s.capabilities.GetByID(ctx, bsc.CapabilityID)
		if err != nil {
			return err
		}
		if cap == nil {
			// BSC row points at a deleted capability — skip rather than
			// surface an inconsistent shape on the wire.
			continue
		}
		out.Capabilities = append(out.Capabilities, &CapabilityNode{Capability: cap})
	}
	return nil
}

// loadSurfacesAndAuthority loads active surfaces under each process,
// per-surface AI binding IDs, per-surface authority counts, and
// populates the AuthoritySummary aggregate using distinct sets at each
// level (profiles distinct across surfaces, grants distinct across
// profiles, agents distinct across grants).
func (s *ReadService) loadSurfacesAndAuthority(ctx context.Context, out *Map) error {
	// Distinct sets for the AuthoritySummary aggregate.
	distinctActiveProfiles := map[string]struct{}{}
	distinctActiveGrants := map[string]struct{}{}
	distinctActiveAgents := map[string]struct{}{}

	for _, pn := range out.Processes {
		surfs, err := s.surfaces.ListByProcessID(ctx, pn.Process.ID)
		if err != nil {
			return err
		}
		for _, surf := range surfs {
			// Filter to active. ListByProcessID returns latest-version
			// per surface; non-active means the latest version is
			// either review or deprecated/retired. The known limitation
			// (active v1 hidden by review v2) is documented in the
			// package doc.
			if surf.Status != surface.SurfaceStatusActive {
				continue
			}
			node, err := s.buildSurfaceNode(ctx, surf,
				distinctActiveProfiles, distinctActiveGrants, distinctActiveAgents)
			if err != nil {
				return err
			}
			out.Surfaces = append(out.Surfaces, node)
		}
	}

	out.AuthoritySummary.ActiveProfileCount = len(distinctActiveProfiles)
	out.AuthoritySummary.ActiveGrantCount = len(distinctActiveGrants)
	out.AuthoritySummary.ActiveAgentCount = len(distinctActiveAgents)
	return nil
}

// buildSurfaceNode produces a single SurfaceNode and updates the
// distinct-set accumulators for the AuthoritySummary aggregate.
func (s *ReadService) buildSurfaceNode(
	ctx context.Context,
	surf *surface.DecisionSurface,
	distinctActiveProfiles, distinctActiveGrants, distinctActiveAgents map[string]struct{},
) (*SurfaceNode, error) {
	node := &SurfaceNode{
		Surface:      surf,
		AIBindingIDs: []string{},
	}

	// AI binding IDs for this surface.
	bindings, err := s.aiSystemBindings.ListBySurface(ctx, surf.ID)
	if err != nil {
		return nil, err
	}
	for _, b := range bindings {
		node.AIBindingIDs = append(node.AIBindingIDs, b.ID)
	}

	// Authority counts for this surface, with per-surface distinct sets
	// so a profile linked to multiple surfaces counts once at this
	// level (matches the aggregate's distinct-by-level posture).
	perSurfaceProfiles := map[string]struct{}{}
	perSurfaceGrants := map[string]struct{}{}
	perSurfaceAgents := map[string]struct{}{}

	profiles, err := s.profiles.ListBySurface(ctx, surf.ID)
	if err != nil {
		return nil, err
	}
	for _, p := range profiles {
		if p.Status != authority.ProfileStatusActive {
			continue
		}
		perSurfaceProfiles[p.ID] = struct{}{}
		distinctActiveProfiles[p.ID] = struct{}{}

		grants, err := s.grants.ListByProfile(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		for _, g := range grants {
			if g.Status != authority.GrantStatusActive {
				continue
			}
			perSurfaceGrants[g.ID] = struct{}{}
			distinctActiveGrants[g.ID] = struct{}{}
			if g.AgentID != "" {
				perSurfaceAgents[g.AgentID] = struct{}{}
				distinctActiveAgents[g.AgentID] = struct{}{}
			}
		}
	}
	node.ProfileCount = len(perSurfaceProfiles)
	node.GrantCount = len(perSurfaceGrants)
	node.AgentCount = len(perSurfaceAgents)
	return node, nil
}

// loadAISystems applies the four-way OR inclusion logic, deduplicates
// bindings by ID, groups by ai_system_id, and emits one AISystemNode
// per included AI system. Each node carries only the bindings relevant
// to the root BS context — bindings to other BSes for the same AI
// system are excluded.
func (s *ReadService) loadAISystems(ctx context.Context, bsID string, out *Map) error {
	// Collect bindings from all four paths into a single dedup map
	// keyed by binding ID.
	bindingByID := map[string]*aisystem.AISystemBinding{}

	addAll := func(bs []*aisystem.AISystemBinding) {
		for _, b := range bs {
			if b == nil {
				continue
			}
			if _, seen := bindingByID[b.ID]; !seen {
				bindingByID[b.ID] = b
			}
		}
	}

	// Path 1: bindings to root BS directly.
	bs1, err := s.aiSystemBindings.ListByBusinessService(ctx, bsID)
	if err != nil {
		return err
	}
	addAll(bs1)

	// Path 2: bindings to capabilities under root.
	for _, cn := range out.Capabilities {
		bs2, err := s.aiSystemBindings.ListByCapability(ctx, cn.Capability.ID)
		if err != nil {
			return err
		}
		addAll(bs2)
	}

	// Path 3: bindings to processes under root.
	for _, pn := range out.Processes {
		bs3, err := s.aiSystemBindings.ListByProcess(ctx, pn.Process.ID)
		if err != nil {
			return err
		}
		addAll(bs3)
	}

	// Path 4: bindings to surfaces under root. (Surfaces array is
	// already active-filtered.)
	for _, sn := range out.Surfaces {
		bs4, err := s.aiSystemBindings.ListBySurface(ctx, sn.Surface.ID)
		if err != nil {
			return err
		}
		addAll(bs4)
	}

	// Group by AI system ID. The four-way OR is dedup-by-binding-ID:
	// one AI system with bindings via both process_id AND surface_id
	// (under that process) yields BOTH bindings under that system's
	// node, not a single "merged" binding.
	bindingsByAISystem := map[string][]*aisystem.AISystemBinding{}
	for _, b := range bindingByID {
		bindingsByAISystem[b.AISystemID] = append(bindingsByAISystem[b.AISystemID], b)
	}

	// Stable iteration: sort AI system IDs alphabetically so the
	// response is deterministic across runs (map iteration is random
	// in Go).
	aiSystemIDs := make([]string, 0, len(bindingsByAISystem))
	for id := range bindingsByAISystem {
		aiSystemIDs = append(aiSystemIDs, id)
	}
	sort.Strings(aiSystemIDs)

	for _, aiSystemID := range aiSystemIDs {
		sys, err := s.aiSystems.GetByID(ctx, aiSystemID)
		if err != nil {
			return err
		}
		if sys == nil {
			// Binding references a deleted AI system — skip.
			continue
		}
		// Active version may be nil when no version is in active status.
		// AI systems are included regardless of status (per brief), but
		// version resolution proceeds normally.
		activeVer, err := s.aiSystemVersions.GetActiveBySystem(ctx, aiSystemID)
		if err != nil {
			return err
		}

		bindings := bindingsByAISystem[aiSystemID]
		// Deterministic binding ordering by binding ID.
		sort.Slice(bindings, func(i, j int) bool {
			return bindings[i].ID < bindings[j].ID
		})

		out.AISystems = append(out.AISystems, &AISystemNode{
			System:        sys,
			ActiveVersion: activeVer,
			Bindings:      bindings,
		})
	}

	return nil
}
