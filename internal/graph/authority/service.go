package authoritygraph

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/escalation"
	"github.com/accept-io/midas/internal/externalref"
	"github.com/accept-io/midas/internal/failmode"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// Narrow reader interfaces
// ---------------------------------------------------------------------------
//
// Each interface declares exactly the method(s) the projection needs.
// Real repositories already satisfy these subsets — the Authority
// Graph package does not import repository types directly, keeping
// the dependency surface minimal and the test seam easy to stub.

// BusinessServiceReader loads a business service by id.
type BusinessServiceReader interface {
	GetByID(ctx context.Context, id string) (*businessservice.BusinessService, error)
}

// ProcessLister lists processes under a business service.
type ProcessLister interface {
	ListByBusinessService(ctx context.Context, businessServiceID string) ([]*process.Process, error)
}

// SurfaceLister lists the latest version of each surface under a
// process. The projection filters returned surfaces to active status
// in code.
type SurfaceLister interface {
	ListByProcessID(ctx context.Context, processID string) ([]*surface.DecisionSurface, error)
}

// ProfileLister lists every authority profile attached to a surface.
// The projection filters returned profiles to active status in code.
type ProfileLister interface {
	ListBySurface(ctx context.Context, surfaceID string) ([]*authority.AuthorityProfile, error)
}

// GrantLister lists every grant linked to an authority profile by
// logical profile id (grants are not versioned). The projection
// filters returned grants to active status in code.
type GrantLister interface {
	ListByProfile(ctx context.Context, profileID string) ([]*authority.AuthorityGrant, error)
}

// AgentReader loads an agent by id.
type AgentReader interface {
	GetByID(ctx context.Context, id string) (*agent.Agent, error)
}

// FailModePolicyResolver returns the active FailModePolicy at the
// supplied instant for the given logical policy id. Returns (nil,
// nil) when no active version is available — the projection treats
// that as "skip the policy node and edge defensively" without
// failing the call.
type FailModePolicyResolver interface {
	FindActiveAt(ctx context.Context, id string, at time.Time) (*failmode.FailModePolicy, error)
}

// EscalationTargetResolver returns the active EscalationTarget at
// the supplied instant for the given logical target id (D31l).
// Mirrors FailModePolicyResolver: returns (nil, nil) when no active
// version is available — the projection treats that as
// "configuration drift" and emits an escalation_target_reference_dangling
// diagnostic without failing the call.
type EscalationTargetResolver interface {
	FindActiveAt(ctx context.Context, id string, at time.Time) (*escalation.EscalationTarget, error)
}

// Readers bundles the read dependencies for the Authority Graph
// projection. All readers are required for ViewService. A nil reader
// will produce ErrInvalidView when Project is called with a view
// whose projector requires the missing dependency.
//
// D31l adds EscalationTargets. When this resolver is nil but the
// projection encounters a profile carrying a non-empty
// EscalationTargetID, the projector returns
// ErrEscalationTargetReaderNotConfigured so wiring regressions
// surface loudly.
type Readers struct {
	BusinessServices  BusinessServiceReader
	Processes         ProcessLister
	Surfaces          SurfaceLister
	Profiles          ProfileLister
	Grants            GrantLister
	Agents            AgentReader
	FailModePolicies  FailModePolicyResolver
	EscalationTargets EscalationTargetResolver
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// viewProjector is the per-view projection function signature. Each
// supported view registers one projector at construction time.
type viewProjector func(ctx context.Context, id string, depth int) (*Projection, error)

// Service projects authority-flavoured domain entities into a
// generic node/edge graph.
type Service struct {
	readers    Readers
	projectors map[string]viewProjector

	// now is a clock seam for time-dependent operations (FailModePolicy
	// FindActiveAt). Production wires this to time.Now; tests can
	// override via NewServiceWithClock to pin "now" for deterministic
	// active-version resolution.
	now func() time.Time
}

// NewService constructs a Service with the supplied readers and a
// real wall-clock. Views are registered when their required readers
// are non-nil; missing readers produce ErrInvalidView at Project
// time for the affected views (no panic).
func NewService(r Readers) *Service {
	return NewServiceWithClock(r, time.Now)
}

// NewServiceWithClock constructs a Service with the supplied readers
// and a custom clock. Use NewService in production; this constructor
// is the deterministic seam for tests that depend on
// FailModePolicy.FindActiveAt resolution at a pinned instant.
func NewServiceWithClock(r Readers, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	s := &Service{
		readers:    r,
		projectors: map[string]viewProjector{},
		now:        now,
	}
	// Register ViewService when every required reader is wired. Other
	// views (agent, surface) are reserved for later tranches; they
	// are not registered here, so requests for those views return
	// ErrInvalidView.
	if r.BusinessServices != nil && r.Processes != nil && r.Surfaces != nil &&
		r.Profiles != nil && r.Grants != nil && r.Agents != nil &&
		r.FailModePolicies != nil {
		s.projectors[ViewService] = s.projectServiceView
	}
	return s
}

// Project dispatches to the view-specific projector after validating
// inputs in order: view → id → depth.
//
// Validation rules:
//
//	view must be a registered projector — otherwise ErrInvalidView.
//	id must be non-empty — otherwise ErrInvalidID.
//	depth must be >= 0 — otherwise ErrInvalidDepth. depth > MaxDepth
//	  is silently clamped (no Truncated signal).
func (s *Service) Project(ctx context.Context, view, id string, depth int) (*Projection, error) {
	p, ok := s.projectors[view]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidView, view)
	}
	if id == "" {
		return nil, ErrInvalidID
	}
	if depth < 0 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidDepth, depth)
	}
	if depth > MaxDepth {
		depth = MaxDepth
	}
	return p(ctx, id, depth)
}

// ---------------------------------------------------------------------------
// view=service projector
// ---------------------------------------------------------------------------

// projectServiceView is the registered projector for ViewService.
// Walks the authority spine rooted at the supplied business service.
//
// D31g algorithm:
//
//  1. Load BS by id (404 on miss; wrap repo error otherwise).
//  2. Build the FULL pre-depth-filter graph via authorityBuilder.
//     The builder enforces effective-time filtering on profiles and
//     grants, accumulates diagnostics (missing references, dangling
//     policies, validity-window violations, inheritance/override
//     posture), and tracks per-surface authority-path completeness.
//  3. Compute Summary + Diagnostics from the builder's accumulators
//     (and from the full node/edge set). Both describe the
//     unfiltered service-view projection.
//  4. Apply depth-bounded BFS filtering to nodes + edges only;
//     Summary + Diagnostics survive verbatim so depth=0 callers
//     still receive backend posture.
//  5. Sort deterministically and return.
func (s *Service) projectServiceView(ctx context.Context, id string, depth int) (*Projection, error) {
	bs, err := s.readers.BusinessServices.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("authoritygraph: load business service %q: %w", id, err)
	}
	if bs == nil {
		return nil, fmt.Errorf("%w: business_service:%s", ErrNotFound, id)
	}

	b := newAuthorityBuilder(s, bs)
	if err := b.run(ctx); err != nil {
		return nil, err
	}

	summary := b.makeSummary()
	diagnostics := b.makeDiagnostics()
	// D31m: rollup + per-surface posture run AFTER diagnostics so
	// the rollup sees the final sorted slice. Both describe the FULL
	// pre-depth-filter projection.
	diagnosticSummary := b.makeDiagnosticSummary(diagnostics)
	surfacePosture := b.makeSurfacePosture(diagnostics)

	root := NodeRef{Kind: NodeKindBusinessService, ID: bs.ID}
	visible := bfsVisible(root, b.edges, depth)

	keptNodes := make([]Node, 0, len(b.nodes))
	for _, n := range b.nodes {
		if _, ok := visible[nodeKey(n.Kind, n.ID)]; ok {
			keptNodes = append(keptNodes, n)
		}
	}
	sort.Slice(keptNodes, func(i, j int) bool {
		if keptNodes[i].Kind != keptNodes[j].Kind {
			return keptNodes[i].Kind < keptNodes[j].Kind
		}
		return keptNodes[i].ID < keptNodes[j].ID
	})

	keptEdges := make([]Edge, 0, len(b.edges))
	for _, e := range b.edges {
		if _, ok := visible[nodeKey(e.Src.Kind, e.Src.ID)]; !ok {
			continue
		}
		if _, ok := visible[nodeKey(e.Dst.Kind, e.Dst.ID)]; !ok {
			continue
		}
		keptEdges = append(keptEdges, e)
	}
	sort.Slice(keptEdges, func(i, j int) bool {
		a, b := keptEdges[i], keptEdges[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Src.Kind != b.Src.Kind {
			return a.Src.Kind < b.Src.Kind
		}
		if a.Src.ID != b.Src.ID {
			return a.Src.ID < b.Src.ID
		}
		if a.Dst.Kind != b.Dst.Kind {
			return a.Dst.Kind < b.Dst.Kind
		}
		return a.Dst.ID < b.Dst.ID
	})

	return &Projection{
		Root:              root,
		View:              ViewService,
		Depth:             depth,
		Nodes:             keptNodes,
		Edges:             keptEdges,
		Summary:           summary,
		Diagnostics:       diagnostics,
		DiagnosticSummary: diagnosticSummary,
		SurfacePosture:    surfacePosture,
	}, nil
}

// ---------------------------------------------------------------------------
// authorityBuilder (D31g)
// ---------------------------------------------------------------------------
//
// authorityBuilder constructs the full pre-depth-filter graph plus
// the supporting accumulators that Summary and Diagnostics derive
// from. Splitting the build from the projection wrapper keeps the
// projector simple and isolates the per-traversal state.

type authorityBuilder struct {
	svc *Service
	bs  *businessservice.BusinessService
	now time.Time

	nodes []Node
	edges []Edge

	seenNode map[string]struct{}

	// Pre-resolved BS-level fail-mode policy (nil when BS has no
	// reference or when the reference is dangling).
	bsResolvedPolicy *failmode.FailModePolicy

	// Ordered emitted-entity ids and per-entity completeness flags.
	// These power per-surface / per-profile / per-grant diagnostics
	// and the summary's gap lists.
	emittedSurfaceIDs []string
	emittedProfileIDs []string
	emittedGrantIDs   []string

	surfaceHasProfile      map[string]bool
	profileHasGrant        map[string]bool
	grantHasAgent          map[string]bool
	surfaceHasCompletePath map[string]bool

	// Per-surface logical-profile-id counts. A logical id with a
	// count > 1 means multiple effective versions are simultaneously
	// active under one surface — an invariant violation that earns a
	// critical diagnostic.
	surfaceProfileLogicalCounts map[string]map[string]int

	// Summary accumulators (effective-policy posture).
	surfacesWithoutEffectivePolicy []NodeRef
	surfacesWithPolicyOverride     int
	surfacesInheritingBSPolicy     int

	// D31i accumulators — counted once per emitted authority_grant
	// node so they survive depth filtering (Summary describes the
	// full pre-depth-filter projection, like every other Summary
	// field).
	grantsWithStopCapability  int
	grantsWithConstraints     int
	grantsWithoutCapabilities []NodeRef

	// Deduped set of dangling fail-mode policy ids — driven by both
	// the BS-level and surface-level lookups returning (nil, nil).
	policiesMissingDeduped map[string]struct{}

	// D31l accumulators for escalation-target posture.
	//
	// emittedEscalationTargetIDs preserves emission order for tests
	// that want a deterministic round-trip; the dedupe is via
	// b.seenNode like every other emitted node.
	//
	// profilesWithoutEscalationTarget lists effective profiles whose
	// EscalationTargetID was empty — paired with the
	// profile_has_no_escalation_target info diagnostic. Append-order
	// is profile-emission order; sortNodeRefs runs in makeSummary
	// for determinism.
	//
	// profilesWithDanglingEscalationTarget lists effective profiles
	// whose configured target id did not resolve to an active version
	// — paired with the escalation_target_reference_dangling warning
	// diagnostic.
	emittedEscalationTargetIDs           []string
	profilesWithoutEscalationTarget      []NodeRef
	profilesWithDanglingEscalationTarget []NodeRef
	profilesWithEscalationTargetCount    int

	// D31m per-surface posture accumulators.
	//
	// profileSurface maps a logical profile id to the surface id it
	// attaches to. Profile.SurfaceID is single-valued, so this map
	// is single-valued. Populated when the profile is first emitted.
	//
	// grantSurface maps a logical grant id to the surface id its
	// profile attaches to. Single-valued.
	//
	// agentSurfaces maps an agent id to the SET of surface ids it
	// backs (an agent can serve grants under multiple surfaces).
	//
	// surfaceFailModeStatus stores the fail-mode posture per surface
	// using FailModePolicyStatus* constants. Populated by
	// resolveSurfacePolicy with the four-way override / inherited /
	// missing / dangling discriminator.
	//
	// surfaceEscalationStatus stores the escalation posture per
	// surface using EscalationStatus* constants. Default is
	// not_targeted; promoted to targeted on first resolved target;
	// promoted to dangling on any dangling reference (terminal).
	//
	// surfaceBlockedAgent marks a surface that has at least one grant
	// with a critical missing/inactive-agent diagnostic. Once set
	// true, the surface's agent_status is "blocked" regardless of
	// any healthy agent that may also back another grant.
	//
	// surfaceCompletePathCount counts per-surface profile→grant→
	// agent triples where an agent record resolved (inactive agents
	// still count as a "complete path" in path-counting terms; the
	// agent_status field separately surfaces the blocked condition).
	//
	// surfaceIncompletePathCount counts:
	//   - profiles under the surface that had no effective grant
	//   - grants under the surface whose AgentID was empty or whose
	//     agent record was missing (the two critical missing-agent
	//     diagnostic branches)
	profileSurface             map[string]string
	grantSurface               map[string]string
	agentSurfaces              map[string]map[string]struct{}
	surfaceFailModeStatus      map[string]string
	surfaceEscalationStatus    map[string]string
	surfaceBlockedAgent        map[string]bool
	surfaceCompletePathCount   map[string]int
	surfaceIncompletePathCount map[string]int

	// bsFailModePolicyDangling is true when the business service
	// declared a FailModePolicyID that failed to resolve at Phase 0
	// — used by resolveSurfacePolicy's no-override + dangling-BS
	// default branch to surface fail_mode_policy_status="dangling"
	// on the affected surfaces (per the D31m spec).
	bsFailModePolicyDangling bool

	// Accumulated diagnostics, unsorted. makeDiagnostics returns
	// them sorted deterministically.
	diagnostics []Diagnostic
}

// newAuthorityBuilder constructs a builder for one business service.
// All maps and slices are pre-initialised so per-surface tracking
// can start at the first emission.
func newAuthorityBuilder(svc *Service, bs *businessservice.BusinessService) *authorityBuilder {
	return &authorityBuilder{
		svc:                         svc,
		bs:                          bs,
		now:                         svc.now(),
		seenNode:                    map[string]struct{}{},
		surfaceHasProfile:           map[string]bool{},
		profileHasGrant:             map[string]bool{},
		grantHasAgent:               map[string]bool{},
		surfaceHasCompletePath:      map[string]bool{},
		surfaceProfileLogicalCounts: map[string]map[string]int{},
		policiesMissingDeduped:      map[string]struct{}{},
		profileSurface:              map[string]string{},
		grantSurface:                map[string]string{},
		agentSurfaces:               map[string]map[string]struct{}{},
		surfaceFailModeStatus:       map[string]string{},
		surfaceEscalationStatus:     map[string]string{},
		surfaceBlockedAgent:         map[string]bool{},
		surfaceCompletePathCount:    map[string]int{},
		surfaceIncompletePathCount:  map[string]int{},
	}
}

// mark records that a (kind, id) node has been emitted. Returns
// true on first sighting (caller should emit the node), false on
// duplicate (caller should skip emission to keep nodes deduped).
func (b *authorityBuilder) mark(kind, id string) bool {
	k := nodeKey(kind, id)
	if _, ok := b.seenNode[k]; ok {
		return false
	}
	b.seenNode[k] = struct{}{}
	return true
}

// addDiagnostic appends a diagnostic to the accumulator. Sort
// happens in makeDiagnostics; callers don't need to order their
// inserts.
func (b *authorityBuilder) addDiagnostic(kind, severity, message string, refs ...NodeRef) {
	b.diagnostics = append(b.diagnostics, Diagnostic{
		Kind:     kind,
		Severity: severity,
		NodeRefs: refs,
		Message:  message,
	})
}

// run executes the full traversal. Repository errors are wrapped
// and returned; "missing reference" cases (nil agent for a grant,
// nil active policy for a non-empty FailModePolicyID, missing
// surface in the surface store) produce diagnostics rather than
// failing the projection.
func (b *authorityBuilder) run(ctx context.Context) error {
	bs := b.bs
	bsRef := NodeRef{Kind: NodeKindBusinessService, ID: bs.ID}

	// Phase 0: Resolve BS-level default fail-mode policy.
	if bs.FailModePolicyID != "" {
		pol, err := b.svc.readers.FailModePolicies.FindActiveAt(ctx, bs.FailModePolicyID, b.now)
		if err != nil {
			return fmt.Errorf("authoritygraph: resolve BS fail-mode policy %q: %w", bs.FailModePolicyID, err)
		}
		if pol != nil {
			b.bsResolvedPolicy = pol
		} else {
			// Dangling BS reference.
			b.policiesMissingDeduped[bs.FailModePolicyID] = struct{}{}
			b.bsFailModePolicyDangling = true
			b.addDiagnostic(
				DiagnosticKindFailModePolicyReferenceDangling,
				DiagnosticSeverityWarning,
				fmt.Sprintf("business service references fail-mode policy %q with no active version", bs.FailModePolicyID),
				bsRef,
				NodeRef{Kind: NodeKindFailModePolicy, ID: bs.FailModePolicyID},
			)
		}
	}

	// Phase 1: Emit BS root node.
	if b.mark(NodeKindBusinessService, bs.ID) {
		b.nodes = append(b.nodes, b.svc.businessServiceNode(bs))
	}

	// Phase 2: Emit BS-level fail-mode policy node + edge when resolved.
	if b.bsResolvedPolicy != nil {
		pol := b.bsResolvedPolicy
		if b.mark(NodeKindFailModePolicy, pol.ID) {
			b.nodes = append(b.nodes, b.svc.failModePolicyNode(pol))
		}
		b.edges = append(b.edges, Edge{
			Kind:  EdgeKindBusinessServiceHasFailModePolicy,
			Src:   bsRef,
			Dst:   NodeRef{Kind: NodeKindFailModePolicy, ID: pol.ID},
			Label: EdgeLabelDefault,
		})
	}

	// Phase 3: List processes.
	procs, err := b.svc.readers.Processes.ListByBusinessService(ctx, bs.ID)
	if err != nil {
		return fmt.Errorf("authoritygraph: list processes for BS %q: %w", bs.ID, err)
	}

	// Phase 4: Walk processes → active surfaces → authority spine.
	for _, proc := range procs {
		if proc == nil {
			continue
		}
		surfs, err := b.svc.readers.Surfaces.ListByProcessID(ctx, proc.ID)
		if err != nil {
			return fmt.Errorf("authoritygraph: list surfaces for process %q: %w", proc.ID, err)
		}
		for _, surf := range surfs {
			if surf == nil {
				continue
			}
			if surf.Status != surface.SurfaceStatusActive {
				continue
			}
			if err := b.processSurface(ctx, surf); err != nil {
				return err
			}
		}
	}

	// Phase 5: Per-surface duplicate-profile-version diagnostics.
	for _, surfID := range b.emittedSurfaceIDs {
		counts := b.surfaceProfileLogicalCounts[surfID]
		for profID, n := range counts {
			if n > 1 {
				b.addDiagnostic(
					DiagnosticKindDuplicateActiveProfileVersionsForSurface,
					DiagnosticSeverityCritical,
					fmt.Sprintf("surface %q has %d effective versions of logical profile %q", surfID, n, profID),
					NodeRef{Kind: NodeKindDecisionSurface, ID: surfID},
					NodeRef{Kind: NodeKindAuthorityProfile, ID: profID},
				)
			}
		}
	}

	// Phase 6: BS-level "no active surface" diagnostic.
	if len(b.emittedSurfaceIDs) == 0 {
		b.addDiagnostic(
			DiagnosticKindBusinessServiceHasNoActiveSurface,
			DiagnosticSeverityWarning,
			fmt.Sprintf("business service %q has no active decision surfaces", bs.ID),
			bsRef,
		)
	}

	// Phase 7: Per-surface "no active profile" diagnostics.
	for _, surfID := range b.emittedSurfaceIDs {
		if !b.surfaceHasProfile[surfID] {
			b.addDiagnostic(
				DiagnosticKindSurfaceHasNoActiveProfile,
				DiagnosticSeverityWarning,
				fmt.Sprintf("decision surface %q has no effective authority profile", surfID),
				NodeRef{Kind: NodeKindDecisionSurface, ID: surfID},
			)
		}
	}

	// Phase 8: Per-profile "no active grant" diagnostics.
	for _, profID := range b.emittedProfileIDs {
		if !b.profileHasGrant[profID] {
			b.addDiagnostic(
				DiagnosticKindProfileHasNoActiveGrant,
				DiagnosticSeverityWarning,
				fmt.Sprintf("authority profile %q has no effective grant", profID),
				NodeRef{Kind: NodeKindAuthorityProfile, ID: profID},
			)
		}
	}

	// Phase 9 (D31i): per-grant "no capabilities" diagnostics. The
	// list is built from b.grantsWithoutCapabilities, which already
	// dedupes via mark() — at most one diagnostic per logical grant
	// id even when the grant is reachable through multiple profiles.
	// Sorted by id for determinism.
	noCaps := append([]NodeRef(nil), b.grantsWithoutCapabilities...)
	sortNodeRefs(noCaps)
	for _, gref := range noCaps {
		b.addDiagnostic(
			DiagnosticKindGrantHasNoCapabilities,
			DiagnosticSeverityWarning,
			fmt.Sprintf("authority grant %q carries no capabilities — orchestrator capability checks against it always reject", gref.ID),
			gref,
		)
	}

	return nil
}

// processSurface handles one active surface: resolves its effective
// fail-mode policy (including BS-default fallback), emits the
// surface and policy nodes/edges as appropriate, then walks its
// profiles, grants, and agents.
func (b *authorityBuilder) processSurface(ctx context.Context, surf *surface.DecisionSurface) error {
	surfRef := NodeRef{Kind: NodeKindDecisionSurface, ID: surf.ID}
	bsRef := NodeRef{Kind: NodeKindBusinessService, ID: b.bs.ID}

	// Resolve the effective fail-mode policy for this surface.
	eff, resolvedOverride, err := b.resolveSurfacePolicy(ctx, surf)
	if err != nil {
		return err
	}

	// Emit the surface node carrying its effective-policy metadata.
	if b.mark(NodeKindDecisionSurface, surf.ID) {
		b.nodes = append(b.nodes, b.svc.decisionSurfaceNode(surf, b.bs.ID, eff))
		b.emittedSurfaceIDs = append(b.emittedSurfaceIDs, surf.ID)
		b.surfaceProfileLogicalCounts[surf.ID] = map[string]int{}
	}
	b.edges = append(b.edges, Edge{
		Kind: EdgeKindBusinessServiceHasSurface,
		Src:  bsRef,
		Dst:  surfRef,
	})

	// Emit the surface-override edge only when the override actually
	// resolved (i.e. effective source is "override"). Inherited
	// defaults do NOT add an edge from the surface — operators read
	// the BS→policy edge to see the fallback.
	if resolvedOverride != nil {
		if b.mark(NodeKindFailModePolicy, resolvedOverride.ID) {
			b.nodes = append(b.nodes, b.svc.failModePolicyNode(resolvedOverride))
		}
		b.edges = append(b.edges, Edge{
			Kind:  EdgeKindSurfaceHasFailModePolicy,
			Src:   surfRef,
			Dst:   NodeRef{Kind: NodeKindFailModePolicy, ID: resolvedOverride.ID},
			Label: EdgeLabelOverride,
		})
	}

	// Walk profiles attached to this surface.
	profiles, err := b.svc.readers.Profiles.ListBySurface(ctx, surf.ID)
	if err != nil {
		return fmt.Errorf("authoritygraph: list profiles for surface %q: %w", surf.ID, err)
	}
	for _, prof := range profiles {
		if prof == nil {
			continue
		}
		if prof.Status != authority.ProfileStatusActive {
			continue
		}

		switch classifyProfileValidity(prof, b.now) {
		case validityFutureDated:
			b.addDiagnostic(
				DiagnosticKindProfileFutureDated,
				DiagnosticSeverityWarning,
				fmt.Sprintf("authority profile %q is active but its effective_date is in the future", prof.ID),
				NodeRef{Kind: NodeKindAuthorityProfile, ID: prof.ID},
				surfRef,
			)
			continue
		case validityExpired:
			b.addDiagnostic(
				DiagnosticKindProfileExpired,
				DiagnosticSeverityWarning,
				fmt.Sprintf("authority profile %q is active but its effective_until has passed", prof.ID),
				NodeRef{Kind: NodeKindAuthorityProfile, ID: prof.ID},
				surfRef,
			)
			continue
		}
		// classifyProfileValidity returned validityEffective —
		// emit the profile.

		// Track per-surface duplicate detection.
		b.surfaceProfileLogicalCounts[surf.ID][prof.ID]++

		profRef := NodeRef{Kind: NodeKindAuthorityProfile, ID: prof.ID}
		profileFirstEmission := b.mark(NodeKindAuthorityProfile, prof.ID)
		if profileFirstEmission {
			b.nodes = append(b.nodes, b.svc.authorityProfileNode(prof))
			b.emittedProfileIDs = append(b.emittedProfileIDs, prof.ID)
			// D31m: anchor the profile to its surface for posture-
			// time diagnostic association.
			b.profileSurface[prof.ID] = surf.ID
		}
		b.edges = append(b.edges, Edge{
			Kind: EdgeKindSurfaceUsesProfile,
			Src:  surfRef,
			Dst:  profRef,
		})
		b.surfaceHasProfile[surf.ID] = true

		// D31l: escalation-target resolution. Run on each profile's
		// FIRST emission only — a profile reachable through multiple
		// surfaces still has exactly one EscalationTargetID, and
		// emitting the diagnostic/edge per surface would double-count
		// in the summary.
		if profileFirstEmission {
			if err := b.processProfileEscalation(ctx, prof, surf.ID); err != nil {
				return err
			}
		}

		if err := b.processProfileGrants(ctx, prof, surf.ID); err != nil {
			return err
		}

		// D31m: per-surface incomplete-path accounting for the
		// profile-without-grant case. processProfileGrants flips
		// b.profileHasGrant[prof.ID] when at least one effective
		// grant emits; the absence is one breakage in the
		// surface's path graph.
		if !b.profileHasGrant[prof.ID] {
			b.surfaceIncompletePathCount[surf.ID]++
		}
	}
	return nil
}

// resolveSurfacePolicy computes the surface's effective fail-mode
// policy posture, emitting inheritance/override/dangling-reference
// diagnostics as appropriate. Returns the resolved EffectivePolicy
// summary plus, when applicable, the freshly-resolved override
// policy that the surface emits as a node + edge.
//
// Decision table (column 1 = surf.FailModePolicyID, column 2 = BS
// default resolved?, output = effective source + side-effects):
//
//	override resolved      → override; emit override edge; either
//	                          "matches" or "overrides" diagnostic
//	                          when BS default exists.
//	override dangling +    → BS-default; inherit; emit inherits
//	  BS default resolved    diagnostic + dangling-policy diagnostic
//	override dangling +    → none; surfaces_without_effective list +
//	  no BS default          dangling-policy diagnostic
//	no override + BS       → BS-default; inherit; emit inherits
//	  default resolved       diagnostic
//	no override + no BS    → none; surfaces_without_effective list
//	  default
func (b *authorityBuilder) resolveSurfacePolicy(ctx context.Context, surf *surface.DecisionSurface) (surfaceEffectivePolicy, *failmode.FailModePolicy, error) {
	surfRef := NodeRef{Kind: NodeKindDecisionSurface, ID: surf.ID}

	if surf.FailModePolicyID != "" {
		pol, err := b.svc.readers.FailModePolicies.FindActiveAt(ctx, surf.FailModePolicyID, b.now)
		if err != nil {
			return surfaceEffectivePolicy{}, nil, fmt.Errorf("authoritygraph: resolve surface fail-mode policy %q: %w", surf.FailModePolicyID, err)
		}
		if pol != nil {
			// Override resolved.
			b.surfacesWithPolicyOverride++
			b.surfaceFailModeStatus[surf.ID] = FailModePolicyStatusOverride
			eff := surfaceEffectivePolicy{
				source:           EffectivePolicySourceOverride,
				policyID:         pol.ID,
				policyVersion:    pol.Version,
				inheritsBSPolicy: false,
			}
			if b.bsResolvedPolicy != nil {
				if pol.ID == b.bsResolvedPolicy.ID {
					b.addDiagnostic(
						DiagnosticKindSurfaceOverrideMatchesBSDefault,
						DiagnosticSeverityInfo,
						fmt.Sprintf("surface %q overrides fail-mode policy with the same id (%q) as the BS default", surf.ID, pol.ID),
						surfRef,
						NodeRef{Kind: NodeKindFailModePolicy, ID: pol.ID},
					)
				} else {
					b.addDiagnostic(
						DiagnosticKindSurfaceOverridesBusinessServiceDefault,
						DiagnosticSeverityInfo,
						fmt.Sprintf("surface %q overrides BS default fail-mode policy %q with %q", surf.ID, b.bsResolvedPolicy.ID, pol.ID),
						surfRef,
						NodeRef{Kind: NodeKindFailModePolicy, ID: pol.ID},
						NodeRef{Kind: NodeKindFailModePolicy, ID: b.bsResolvedPolicy.ID},
					)
				}
			}
			return eff, pol, nil
		}
		// Override dangling — surface posture is "dangling" regardless
		// of whether the BS default supplies a fallback (per D31m
		// spec: configuration drift wins over silent inheritance).
		b.policiesMissingDeduped[surf.FailModePolicyID] = struct{}{}
		b.surfaceFailModeStatus[surf.ID] = FailModePolicyStatusDangling
		b.addDiagnostic(
			DiagnosticKindFailModePolicyReferenceDangling,
			DiagnosticSeverityWarning,
			fmt.Sprintf("surface %q references fail-mode policy %q with no active version", surf.ID, surf.FailModePolicyID),
			surfRef,
			NodeRef{Kind: NodeKindFailModePolicy, ID: surf.FailModePolicyID},
		)
		// Fallback to BS default when available.
		if b.bsResolvedPolicy != nil {
			b.surfacesInheritingBSPolicy++
			b.addDiagnostic(
				DiagnosticKindSurfaceInheritsBusinessServicePolicy,
				DiagnosticSeverityInfo,
				fmt.Sprintf("surface %q inherits BS default fail-mode policy %q (surface override is dangling)", surf.ID, b.bsResolvedPolicy.ID),
				surfRef,
				NodeRef{Kind: NodeKindFailModePolicy, ID: b.bsResolvedPolicy.ID},
			)
			return surfaceEffectivePolicy{
				source:           EffectivePolicySourceBusinessServiceDefault,
				policyID:         b.bsResolvedPolicy.ID,
				policyVersion:    b.bsResolvedPolicy.Version,
				inheritsBSPolicy: true,
			}, nil, nil
		}
		// No fallback available.
		b.surfacesWithoutEffectivePolicy = append(b.surfacesWithoutEffectivePolicy, surfRef)
		return surfaceEffectivePolicy{source: EffectivePolicySourceNone}, nil, nil
	}

	// surf.FailModePolicyID is empty — check BS-default fallback.
	if b.bsResolvedPolicy != nil {
		b.surfacesInheritingBSPolicy++
		b.surfaceFailModeStatus[surf.ID] = FailModePolicyStatusInherited
		b.addDiagnostic(
			DiagnosticKindSurfaceInheritsBusinessServicePolicy,
			DiagnosticSeverityInfo,
			fmt.Sprintf("surface %q inherits BS default fail-mode policy %q", surf.ID, b.bsResolvedPolicy.ID),
			surfRef,
			NodeRef{Kind: NodeKindFailModePolicy, ID: b.bsResolvedPolicy.ID},
		)
		return surfaceEffectivePolicy{
			source:           EffectivePolicySourceBusinessServiceDefault,
			policyID:         b.bsResolvedPolicy.ID,
			policyVersion:    b.bsResolvedPolicy.Version,
			inheritsBSPolicy: true,
		}, nil, nil
	}

	// No reference, no fallback. If the BS *had* a non-empty
	// FailModePolicyID but it failed to resolve (Phase 0 marked
	// bsFailModePolicyDangling), surfaces that would otherwise have
	// inherited it count as "dangling" per the D31m posture spec —
	// the broken inheritance is the operator-relevant signal. The
	// Phase 0 dangling diagnostic already exists at BS level so the
	// UI sees one source-of-truth diagnostic.
	if b.bsFailModePolicyDangling {
		b.surfaceFailModeStatus[surf.ID] = FailModePolicyStatusDangling
	} else {
		b.surfaceFailModeStatus[surf.ID] = FailModePolicyStatusMissing
	}
	b.surfacesWithoutEffectivePolicy = append(b.surfacesWithoutEffectivePolicy, surfRef)
	return surfaceEffectivePolicy{source: EffectivePolicySourceNone}, nil, nil
}

// processProfileGrants walks the grants for one emitted profile,
// applying status + validity-window filtering and emitting grant +
// agent nodes/edges. Side-effects per-surface complete-path tracking
// when an effective profile → effective grant → resolved agent
// chain is built.
func (b *authorityBuilder) processProfileGrants(ctx context.Context, prof *authority.AuthorityProfile, surfID string) error {
	profRef := NodeRef{Kind: NodeKindAuthorityProfile, ID: prof.ID}
	grants, err := b.svc.readers.Grants.ListByProfile(ctx, prof.ID)
	if err != nil {
		return fmt.Errorf("authoritygraph: list grants for profile %q: %w", prof.ID, err)
	}
	for _, g := range grants {
		if g == nil {
			continue
		}
		if g.Status != authority.GrantStatusActive {
			continue
		}

		switch classifyGrantValidity(g, b.now) {
		case validityFutureDated:
			b.addDiagnostic(
				DiagnosticKindGrantFutureDated,
				DiagnosticSeverityWarning,
				fmt.Sprintf("grant %q is active but its effective_date is in the future", g.ID),
				NodeRef{Kind: NodeKindAuthorityGrant, ID: g.ID},
				profRef,
			)
			continue
		case validityExpired:
			b.addDiagnostic(
				DiagnosticKindGrantExpired,
				DiagnosticSeverityWarning,
				fmt.Sprintf("grant %q is active but its expires_at has passed", g.ID),
				NodeRef{Kind: NodeKindAuthorityGrant, ID: g.ID},
				profRef,
			)
			continue
		}

		grantRef := NodeRef{Kind: NodeKindAuthorityGrant, ID: g.ID}
		if b.mark(NodeKindAuthorityGrant, g.ID) {
			b.nodes = append(b.nodes, b.svc.authorityGrantNode(g))
			b.emittedGrantIDs = append(b.emittedGrantIDs, g.ID)
			// D31m: anchor the grant to its surface for posture-
			// time diagnostic association.
			b.grantSurface[g.ID] = surfID
			// D31i: track capability / constraint posture per
			// emitted grant. The mark() guard guarantees we count
			// each grant exactly once even if it is reachable
			// through more than one profile.
			if authority.HasCapability(g.Capabilities, authority.CapabilityStop) {
				b.grantsWithStopCapability++
			}
			if len(g.Constraints) > 0 {
				b.grantsWithConstraints++
			}
			if len(g.Capabilities) == 0 {
				b.grantsWithoutCapabilities = append(b.grantsWithoutCapabilities, grantRef)
			}
		}
		b.edges = append(b.edges, Edge{
			Kind: EdgeKindProfileHasGrant,
			Src:  profRef,
			Dst:  grantRef,
		})
		b.profileHasGrant[prof.ID] = true

		// Agent resolution. Missing AgentID or repository lookup
		// returning (nil, nil) → critical diagnostic but the grant
		// stays. Inactive agent → warning, agent + edge still
		// emitted.
		if g.AgentID == "" {
			b.addDiagnostic(
				DiagnosticKindGrantReferencesMissingAgent,
				DiagnosticSeverityCritical,
				fmt.Sprintf("grant %q has no agent reference (AgentID empty)", g.ID),
				grantRef,
			)
			// D31m: missing-agent grant counts as one incomplete
			// path; the surface's agent_status will reflect blocked.
			b.surfaceIncompletePathCount[surfID]++
			b.surfaceBlockedAgent[surfID] = true
			continue
		}
		ag, err := b.svc.readers.Agents.GetByID(ctx, g.AgentID)
		if err != nil {
			return fmt.Errorf("authoritygraph: load agent %q for grant %q: %w", g.AgentID, g.ID, err)
		}
		if ag == nil {
			b.addDiagnostic(
				DiagnosticKindGrantReferencesMissingAgent,
				DiagnosticSeverityCritical,
				fmt.Sprintf("grant %q references agent %q with no record in the agent store", g.ID, g.AgentID),
				grantRef,
				NodeRef{Kind: NodeKindAgent, ID: g.AgentID},
			)
			// D31m: nil-agent-record grant counts as incomplete +
			// blocked.
			b.surfaceIncompletePathCount[surfID]++
			b.surfaceBlockedAgent[surfID] = true
			continue
		}
		if b.mark(NodeKindAgent, ag.ID) {
			b.nodes = append(b.nodes, b.svc.agentNode(ag))
		}
		b.edges = append(b.edges, Edge{
			Kind: EdgeKindGrantAuthorisesAgent,
			Src:  grantRef,
			Dst:  NodeRef{Kind: NodeKindAgent, ID: ag.ID},
		})
		b.grantHasAgent[g.ID] = true
		// D31m: track agent → surface association for diagnostic
		// posture lookup. An agent may back grants under multiple
		// surfaces — the set semantics are correct.
		if b.agentSurfaces[ag.ID] == nil {
			b.agentSurfaces[ag.ID] = map[string]struct{}{}
		}
		b.agentSurfaces[ag.ID][surfID] = struct{}{}
		// Per-surface completeness: the surface this profile is
		// attached to has at least one effective profile →
		// effective grant → resolved agent chain.
		b.surfaceHasCompletePath[surfID] = true
		b.surfaceCompletePathCount[surfID]++

		if ag.OperationalState != agent.OperationalStateActive {
			// D31j: severity is critical because the orchestrator
			// now treats a non-active agent as a hard runtime
			// authority gate — any request for this grant rejects
			// with AGENT_OPERATIONAL_STATE_BLOCKED before
			// capability/constraint/threshold evaluation. The
			// diagnostic message names the runtime implication so
			// operators reading the Authority Graph see the
			// consequence, not just the static condition.
			b.addDiagnostic(
				DiagnosticKindGrantReferencesInactiveAgent,
				DiagnosticSeverityCritical,
				fmt.Sprintf("grant %q references agent %q whose operational_state is %q; runtime authority will be blocked", g.ID, ag.ID, ag.OperationalState),
				grantRef,
				NodeRef{Kind: NodeKindAgent, ID: ag.ID},
			)
			// D31m: an inactive agent still counts as a complete
			// path in path-counting terms (every node + edge in
			// the chain is emitted), but the surface posture must
			// surface the runtime blockade.
			b.surfaceBlockedAgent[surfID] = true
		}
	}
	return nil
}

// processProfileEscalation resolves the active EscalationTarget for
// one emitted profile and emits the corresponding node + edge +
// diagnostics (D31l). Cases:
//
//   - empty EscalationTargetID: append the profile to
//     profilesWithoutEscalationTarget; emit profile_has_no_escalation_target
//     (info).
//   - configured but resolver unwired: return
//     ErrEscalationTargetReaderNotConfigured so wiring regressions
//     surface loudly.
//   - configured and resolver returns active target: emit the
//     escalation_target node (deduped by id via b.mark) and the
//     profile_escalates_to edge.
//   - configured and resolver returns (nil, nil): append the profile
//     to profilesWithDanglingEscalationTarget; emit
//     escalation_target_reference_dangling (warning).
//   - configured and resolver returns error: wrap and propagate so
//     the projection caller sees the underlying failure.
//
// D31m: surfID is used to update the per-surface escalation_status
// rollup. Precedence (worst → best): dangling > targeted >
// not_targeted; once a surface reaches dangling it never drops back.
// Empty EscalationTargetID does NOT downgrade an already-targeted
// surface — a surface with three profiles where two target and one
// is empty stays "targeted".
func (b *authorityBuilder) processProfileEscalation(ctx context.Context, prof *authority.AuthorityProfile, surfID string) error {
	profRef := NodeRef{Kind: NodeKindAuthorityProfile, ID: prof.ID}

	if prof.EscalationTargetID == "" {
		b.profilesWithoutEscalationTarget = append(b.profilesWithoutEscalationTarget, profRef)
		b.addDiagnostic(
			DiagnosticKindProfileHasNoEscalationTarget,
			DiagnosticSeverityInfo,
			fmt.Sprintf("authority profile %q has no explicit escalation target; escalation will preserve the current outcome without target routing", prof.ID),
			profRef,
		)
		// D31m: do not overwrite surfaceEscalationStatus[surfID] if
		// it was already promoted by an earlier profile.
		return nil
	}

	// Profile carries a target id — the resolver MUST be wired.
	if b.svc.readers.EscalationTargets == nil {
		return fmt.Errorf("%w: profile %q references escalation_target %q",
			ErrEscalationTargetReaderNotConfigured, prof.ID, prof.EscalationTargetID)
	}

	t, err := b.svc.readers.EscalationTargets.FindActiveAt(ctx, prof.EscalationTargetID, b.now)
	if err != nil {
		return fmt.Errorf("authoritygraph: resolve escalation target %q for profile %q: %w",
			prof.EscalationTargetID, prof.ID, err)
	}
	if t == nil {
		// Configured but no active version — dangling reference.
		b.profilesWithDanglingEscalationTarget = append(b.profilesWithDanglingEscalationTarget, profRef)
		b.addDiagnostic(
			DiagnosticKindEscalationTargetReferenceDangling,
			DiagnosticSeverityWarning,
			fmt.Sprintf("authority profile %q references escalation target %q, but no active version is available", prof.ID, prof.EscalationTargetID),
			profRef,
			NodeRef{Kind: NodeKindEscalationTarget, ID: prof.EscalationTargetID},
		)
		// D31m: dangling is terminal — once set, stays.
		b.surfaceEscalationStatus[surfID] = EscalationStatusDangling
		return nil
	}

	// Resolved → emit the target node (deduped via b.mark) and the
	// profile_escalates_to edge. The summary counter only ticks on
	// FIRST sighting of the profile's resolved-target combination —
	// not per edge — but since we run this whole helper on profile
	// first-emission only, every increment is a unique profile.
	tgtRef := NodeRef{Kind: NodeKindEscalationTarget, ID: t.ID}
	if b.mark(NodeKindEscalationTarget, t.ID) {
		b.nodes = append(b.nodes, b.svc.escalationTargetNode(t))
		b.emittedEscalationTargetIDs = append(b.emittedEscalationTargetIDs, t.ID)
	}
	b.edges = append(b.edges, Edge{
		Kind: EdgeKindProfileEscalatesTo,
		Src:  profRef,
		Dst:  tgtRef,
	})
	b.profilesWithEscalationTargetCount++
	// D31m: promote to targeted only if not already dangling.
	if b.surfaceEscalationStatus[surfID] != EscalationStatusDangling {
		b.surfaceEscalationStatus[surfID] = EscalationStatusTargeted
	}
	return nil
}

// makeSummary computes the projection's Summary from the builder's
// accumulators + the deduped pre-depth node set. Counts walk the
// nodes once; gap lists come from the per-entity tracking maps.
func (b *authorityBuilder) makeSummary() *Summary {
	out := &Summary{}

	// Walk emitted nodes for counts. (Cheaper than maintaining
	// running counters because the dedupe is in seenNode already.)
	for _, n := range b.nodes {
		switch n.Kind {
		case NodeKindDecisionSurface:
			out.SurfaceCount++
		case NodeKindAuthorityProfile:
			out.ActiveProfileCount++
		case NodeKindAuthorityGrant:
			out.ActiveGrantCount++
		case NodeKindAgent:
			out.ActiveAgentCount++
		case NodeKindFailModePolicy:
			out.FailModePolicyCount++
		case NodeKindEscalationTarget:
			out.EscalationTargetCount++
		}
	}

	// Per-surface completeness (count at surface granularity).
	for _, surfID := range b.emittedSurfaceIDs {
		if b.surfaceHasCompletePath[surfID] {
			out.CompleteAuthorityPaths++
		} else {
			out.IncompleteAuthorityPaths++
		}
	}

	// Gap lists, sorted by id ascending for determinism.
	for _, surfID := range b.emittedSurfaceIDs {
		if !b.surfaceHasProfile[surfID] {
			out.SurfacesWithoutProfiles = append(out.SurfacesWithoutProfiles, NodeRef{Kind: NodeKindDecisionSurface, ID: surfID})
		}
	}
	sortNodeRefs(out.SurfacesWithoutProfiles)

	for _, profID := range b.emittedProfileIDs {
		if !b.profileHasGrant[profID] {
			out.ProfilesWithoutGrants = append(out.ProfilesWithoutGrants, NodeRef{Kind: NodeKindAuthorityProfile, ID: profID})
		}
	}
	sortNodeRefs(out.ProfilesWithoutGrants)

	for _, grantID := range b.emittedGrantIDs {
		if !b.grantHasAgent[grantID] {
			out.GrantsWithoutAgents = append(out.GrantsWithoutAgents, NodeRef{Kind: NodeKindAuthorityGrant, ID: grantID})
		}
	}
	sortNodeRefs(out.GrantsWithoutAgents)

	out.SurfacesWithoutEffectiveFailModePolicy = append([]NodeRef(nil), b.surfacesWithoutEffectivePolicy...)
	sortNodeRefs(out.SurfacesWithoutEffectiveFailModePolicy)

	out.SurfacesWithPolicyOverride = b.surfacesWithPolicyOverride
	out.SurfacesInheritingBSPolicy = b.surfacesInheritingBSPolicy

	for polID := range b.policiesMissingDeduped {
		out.PoliciesMissingActiveVersion = append(out.PoliciesMissingActiveVersion, NodeRef{Kind: NodeKindFailModePolicy, ID: polID})
	}
	sortNodeRefs(out.PoliciesMissingActiveVersion)

	// D31i — capability / constraint posture rollups.
	out.GrantsWithStopCapability = b.grantsWithStopCapability
	out.GrantsWithConstraints = b.grantsWithConstraints
	out.GrantsWithoutCapabilities = append([]NodeRef(nil), b.grantsWithoutCapabilities...)
	sortNodeRefs(out.GrantsWithoutCapabilities)

	// D31l — escalation-target posture rollups.
	out.ProfilesWithEscalationTarget = b.profilesWithEscalationTargetCount
	out.ProfilesWithoutEscalationTarget = append([]NodeRef(nil), b.profilesWithoutEscalationTarget...)
	sortNodeRefs(out.ProfilesWithoutEscalationTarget)
	out.ProfilesWithDanglingEscalationTarget = append([]NodeRef(nil), b.profilesWithDanglingEscalationTarget...)
	sortNodeRefs(out.ProfilesWithDanglingEscalationTarget)

	return out
}

// makeDiagnosticSummary builds the per-severity rollup over the
// supplied (already-sorted) diagnostics slice. The counts always
// sum to len(diags). HighestSeverity is "none" when diags is empty,
// otherwise the most severe present (critical > warning > info).
//
// Called from projectServiceView AFTER makeDiagnostics so the
// rollup runs over the final, sorted, deduped diagnostic set —
// guaranteeing that the wire-level diagnostic count matches the
// rollup count.
func (b *authorityBuilder) makeDiagnosticSummary(diags []Diagnostic) *DiagnosticSummary {
	out := &DiagnosticSummary{HighestSeverity: HighestSeverityNone}
	if len(diags) == 0 {
		return out
	}
	byKind := map[string]int{}
	for _, d := range diags {
		switch d.Severity {
		case DiagnosticSeverityCritical:
			out.Critical++
		case DiagnosticSeverityWarning:
			out.Warning++
		case DiagnosticSeverityInfo:
			out.Info++
		}
		byKind[d.Kind]++
	}
	switch {
	case out.Critical > 0:
		out.HighestSeverity = HighestSeverityCritical
	case out.Warning > 0:
		out.HighestSeverity = HighestSeverityWarning
	case out.Info > 0:
		out.HighestSeverity = HighestSeverityInfo
	}
	if len(byKind) > 0 {
		out.ByKind = byKind
	}
	return out
}

// makeSurfacePosture builds one SurfaceAuthorityPosture per emitted
// active decision surface, in emission order (sorted by surface id
// ascending). Operates over:
//
//   - per-surface accumulators (surfaceHasProfile,
//     surfaceCompletePathCount, surfaceIncompletePathCount,
//     surfaceFailModeStatus, surfaceEscalationStatus,
//     surfaceBlockedAgent).
//   - the final diagnostics slice — walks each diagnostic's NodeRefs
//     and unions the surfaces they touch via b.profileSurface,
//     b.grantSurface, b.agentSurfaces, or a direct surface NodeRef.
//
// Posture describes the FULL pre-depth-filter projection. The
// returned slice is sorted by surface id; diagnostic_kinds per
// surface are sorted alphabetically for determinism.
func (b *authorityBuilder) makeSurfacePosture(diags []Diagnostic) []SurfaceAuthorityPosture {
	if len(b.emittedSurfaceIDs) == 0 {
		return nil
	}

	// First pass: build per-surface diagnostic-kind set and per-
	// surface highest severity by associating each diagnostic to
	// every surface its NodeRefs touch.
	diagKinds := map[string]map[string]struct{}{}
	highest := map[string]string{}
	for _, surfID := range b.emittedSurfaceIDs {
		diagKinds[surfID] = map[string]struct{}{}
		highest[surfID] = HighestSeverityNone
	}
	for _, d := range diags {
		// Determine the SET of surfaces this diagnostic touches.
		hit := map[string]struct{}{}
		for _, ref := range d.NodeRefs {
			for _, s := range b.surfacesForRef(ref) {
				hit[s] = struct{}{}
			}
		}
		for s := range hit {
			if _, known := diagKinds[s]; !known {
				continue // diagnostic references a surface that is not emitted
			}
			diagKinds[s][d.Kind] = struct{}{}
			highest[s] = upgradeSeverity(highest[s], d.Severity)
		}
	}

	out := make([]SurfaceAuthorityPosture, 0, len(b.emittedSurfaceIDs))
	for _, surfID := range b.emittedSurfaceIDs {
		surfRef := NodeRef{Kind: NodeKindDecisionSurface, ID: surfID}
		complete := b.surfaceCompletePathCount[surfID]
		incomplete := b.surfaceIncompletePathCount[surfID]

		p := SurfaceAuthorityPosture{
			Surface:              surfRef,
			ProfileStatus:        profileStatusFor(b, surfID),
			GrantStatus:          grantStatusFor(b, surfID),
			AgentStatus:          agentStatusFor(b, surfID),
			FailModePolicyStatus: failModeStatusFor(b, surfID),
			EscalationStatus:     escalationStatusFor(b, surfID),
			CompletePaths:        complete,
			IncompletePaths:      incomplete,
			HighestSeverity:      highest[surfID],
		}

		// authority_status is a roll-up of the above plus the
		// per-surface diagnostic severity.
		p.AuthorityStatus = authorityStatusFor(
			b.surfaceHasProfile[surfID],
			complete > 0,
			p.HighestSeverity,
		)

		// Surfaces with no effective profile carry incomplete_paths=1
		// per the D31m spec — a single "one missing surface path"
		// indicator that the UI can render without branching on
		// profile_status separately.
		if !b.surfaceHasProfile[surfID] && incomplete == 0 {
			p.IncompletePaths = 1
		}

		if len(diagKinds[surfID]) > 0 {
			kinds := make([]string, 0, len(diagKinds[surfID]))
			for k := range diagKinds[surfID] {
				kinds = append(kinds, k)
			}
			sort.Strings(kinds)
			p.DiagnosticKinds = kinds
		}

		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Surface.ID < out[j].Surface.ID })
	return out
}

// surfacesForRef returns the set of surface ids a NodeRef belongs
// to. Used by makeSurfacePosture for diagnostic-to-surface
// association. Returns nil when the ref cannot be mapped to any
// emitted surface — BS-level refs are intentionally not associated
// with any surface (BS-level diagnostics do not appear in
// per-surface diagnostic_kinds).
func (b *authorityBuilder) surfacesForRef(ref NodeRef) []string {
	switch ref.Kind {
	case NodeKindDecisionSurface:
		return []string{ref.ID}
	case NodeKindAuthorityProfile:
		if s, ok := b.profileSurface[ref.ID]; ok {
			return []string{s}
		}
	case NodeKindAuthorityGrant:
		if s, ok := b.grantSurface[ref.ID]; ok {
			return []string{s}
		}
	case NodeKindAgent:
		if set, ok := b.agentSurfaces[ref.ID]; ok {
			out := make([]string, 0, len(set))
			for s := range set {
				out = append(out, s)
			}
			return out
		}
	}
	// BusinessService, FailModePolicy, EscalationTarget refs do not
	// have an authoritative single-surface mapping — they associate
	// to many surfaces indirectly. The diagnostic NodeRef vocabulary
	// always pairs them with a surface/profile/grant ref that
	// resolves through this function, so returning nil here is safe.
	return nil
}

// authorityStatusFor applies the documented precedence:
//
//	uncovered  > incomplete > degraded > complete
//
//   - uncovered: no effective profile attaches to the surface.
//   - incomplete: profile exists but no complete profile→grant→
//     agent path resolved.
//   - degraded: a complete path exists AND a warning/critical
//     diagnostic applies to the surface. Info-only diagnostics do
//     NOT degrade.
//   - complete: a complete path exists and no warning/critical
//     diagnostic applies.
func authorityStatusFor(hasProfile, hasCompletePath bool, highestSeverity string) string {
	if !hasProfile {
		return AuthorityStatusUncovered
	}
	if !hasCompletePath {
		return AuthorityStatusIncomplete
	}
	if highestSeverity == HighestSeverityWarning || highestSeverity == HighestSeverityCritical {
		return AuthorityStatusDegraded
	}
	return AuthorityStatusComplete
}

func profileStatusFor(b *authorityBuilder, surfID string) string {
	if b.surfaceHasProfile[surfID] {
		return ProfileStatusCovered
	}
	return ProfileStatusMissing
}

// grantStatusFor reports covered when at least one grant emits
// under any profile attached to the surface (b.profileHasGrant
// covers per-profile, b.surfaceHasCompletePath covers the BS-wide
// path; for grant-status alone the "any grant under any profile"
// signal is sufficient).
func grantStatusFor(b *authorityBuilder, surfID string) string {
	// A surface has covered grants when any of its profiles flipped
	// profileHasGrant true. Walk profileSurface to find profiles
	// attached to this surface.
	for profID, sID := range b.profileSurface {
		if sID != surfID {
			continue
		}
		if b.profileHasGrant[profID] {
			return GrantStatusCovered
		}
	}
	return GrantStatusMissing
}

// agentStatusFor applies the documented precedence:
//
//	blocked > missing > covered
//
// blocked: surfaceBlockedAgent flag is set (missing-agent or
// inactive-agent critical diagnostic emitted under this surface).
// covered: at least one complete path resolved.
// missing: surface has profile(s) and grant(s) but no resolved
// agent path.
func agentStatusFor(b *authorityBuilder, surfID string) string {
	if b.surfaceBlockedAgent[surfID] {
		return AgentStatusBlocked
	}
	if b.surfaceCompletePathCount[surfID] > 0 {
		return AgentStatusCovered
	}
	return AgentStatusMissing
}

func failModeStatusFor(b *authorityBuilder, surfID string) string {
	if v, ok := b.surfaceFailModeStatus[surfID]; ok {
		return v
	}
	// Defensive fallback — resolveSurfacePolicy sets the status for
	// every emitted surface. An unset entry means the surface was
	// processed before D31m wiring landed (impossible in production
	// but safer to surface "missing" than "").
	return FailModePolicyStatusMissing
}

func escalationStatusFor(b *authorityBuilder, surfID string) string {
	if v, ok := b.surfaceEscalationStatus[surfID]; ok && v != "" {
		return v
	}
	return EscalationStatusNotTargeted
}

// upgradeSeverity returns the more-severe of the two HighestSeverity
// values. Severity rank: critical > warning > info > none.
func upgradeSeverity(current, candidate string) string {
	if severityRollupRank(candidate) > severityRollupRank(current) {
		return candidate
	}
	return current
}

func severityRollupRank(s string) int {
	switch s {
	case HighestSeverityCritical:
		return 3
	case HighestSeverityWarning:
		return 2
	case HighestSeverityInfo:
		return 1
	default:
		return 0
	}
}

// makeDiagnostics returns the accumulated diagnostics sorted by
// (severity rank, kind, first NodeRef.Kind, first NodeRef.ID). The
// caller may rely on the order being stable across runs.
func (b *authorityBuilder) makeDiagnostics() []Diagnostic {
	if len(b.diagnostics) == 0 {
		return nil
	}
	out := append([]Diagnostic(nil), b.diagnostics...)
	sort.SliceStable(out, func(i, j int) bool {
		si := severityRank(out[i].Severity)
		sj := severityRank(out[j].Severity)
		if si != sj {
			return si < sj
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		ki, idi := firstNodeRef(out[i])
		kj, idj := firstNodeRef(out[j])
		if ki != kj {
			return ki < kj
		}
		return idi < idj
	})
	return out
}

// severityRank gives critical < warning < info ordering.
func severityRank(s string) int {
	switch s {
	case DiagnosticSeverityCritical:
		return 0
	case DiagnosticSeverityWarning:
		return 1
	case DiagnosticSeverityInfo:
		return 2
	default:
		return 99
	}
}

// firstNodeRef returns the (kind, id) of d.NodeRefs[0] when set,
// otherwise empty strings — keeps sort comparator safe for
// diagnostics that omit refs entirely.
func firstNodeRef(d Diagnostic) (string, string) {
	if len(d.NodeRefs) == 0 {
		return "", ""
	}
	return d.NodeRefs[0].Kind, d.NodeRefs[0].ID
}

// sortNodeRefs sorts a slice by (Kind, ID) ascending. Used for
// every summary gap list to keep wire output deterministic.
func sortNodeRefs(refs []NodeRef) {
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Kind != refs[j].Kind {
			return refs[i].Kind < refs[j].Kind
		}
		return refs[i].ID < refs[j].ID
	})
}

// ---------------------------------------------------------------------------
// Validity-window classification
// ---------------------------------------------------------------------------

// validityClassification classifies an authority profile or grant
// against the wall clock. effective is the emission-eligible case;
// futureDated and expired produce diagnostics and filter the entity
// out of the node set.
type validityClassification int

const (
	validityEffective validityClassification = iota
	validityFutureDated
	validityExpired
)

// classifyProfileValidity applies the same time-window semantics
// the failmode FindActiveAt repo path uses, against the supplied
// `now`. The caller is responsible for `Status == active` filtering;
// this helper only checks the validity window.
//
//	effective_date > now            → futureDated
//	effective_until != nil && <= now → expired
//	otherwise                       → effective
func classifyProfileValidity(p *authority.AuthorityProfile, now time.Time) validityClassification {
	if p.EffectiveDate.After(now) {
		return validityFutureDated
	}
	if p.EffectiveUntil != nil && !p.EffectiveUntil.After(now) {
		return validityExpired
	}
	return validityEffective
}

// classifyGrantValidity mirrors classifyProfileValidity but reads
// expires_at instead of effective_until.
//
//	effective_date > now           → futureDated
//	expires_at != nil && <= now    → expired
//	otherwise                      → effective
func classifyGrantValidity(g *authority.AuthorityGrant, now time.Time) validityClassification {
	if g.EffectiveDate.After(now) {
		return validityFutureDated
	}
	if g.ExpiresAt != nil && !g.ExpiresAt.After(now) {
		return validityExpired
	}
	return validityEffective
}

// ---------------------------------------------------------------------------
// Per-kind node builders
// ---------------------------------------------------------------------------

func (s *Service) businessServiceNode(bs *businessservice.BusinessService) Node {
	label := bs.Name
	if label == "" {
		label = bs.ID
	}
	return Node{
		Kind:  NodeKindBusinessService,
		ID:    bs.ID,
		Label: label,
		BusinessService: &BusinessServiceData{
			ID:               bs.ID,
			Name:             bs.Name,
			Status:           bs.Status,
			Owner:            bs.OwnerID,
			ServiceType:      string(bs.ServiceType),
			ExternalRef:      toExternalRefData(bs.ExternalRef),
			FailModePolicyID: bs.FailModePolicyID,
		},
	}
}

// surfaceEffectivePolicy is the resolved fail-mode-policy posture
// for one decision surface, computed during projection. Carried as
// typed data on the emitted surface node and as a basis for the
// summary/diagnostic accumulators.
type surfaceEffectivePolicy struct {
	source            string // EffectivePolicySource* (override / business_service_default / none)
	policyID          string // resolved policy logical id; empty when source == none
	policyVersion     int    // resolved policy version; 0 when source == none
	inheritsBSPolicy  bool   // true when source == business_service_default
}

func (s *Service) decisionSurfaceNode(surf *surface.DecisionSurface, bsID string, eff surfaceEffectivePolicy) Node {
	label := surf.Name
	if label == "" {
		label = surf.ID
	}
	return Node{
		Kind:  NodeKindDecisionSurface,
		ID:    surf.ID,
		Label: label,
		DecisionSurface: &DecisionSurfaceData{
			ID:                     surf.ID,
			Version:                surf.Version,
			Name:                   surf.Name,
			Status:                 string(surf.Status),
			ProcessID:              surf.ProcessID,
			BusinessServiceID:      bsID,
			FailModePolicyID:       surf.FailModePolicyID,
			EffectivePolicySource:  eff.source,
			EffectivePolicyID:      eff.policyID,
			EffectivePolicyVersion: eff.policyVersion,
			InheritsBSPolicy:       eff.inheritsBSPolicy,
		},
	}
}

func (s *Service) authorityProfileNode(prof *authority.AuthorityProfile) Node {
	label := prof.Name
	if label == "" {
		label = prof.ID
	}
	return Node{
		Kind:  NodeKindAuthorityProfile,
		ID:    prof.ID,
		Label: label,
		AuthorityProfile: &AuthorityProfileData{
			ID:                   prof.ID,
			Version:              prof.Version,
			SurfaceID:            prof.SurfaceID,
			Name:                 prof.Name,
			Status:               string(prof.Status),
			EffectiveDate:        timePtrString(prof.EffectiveDate),
			EffectiveUntil:       timePtrFromPtr(prof.EffectiveUntil),
			ValidityStatus:       ValidityStatusEffective,
			ConfidenceThreshold:  prof.ConfidenceThreshold,
			ConsequenceThreshold: toConsequenceThresholdData(prof.ConsequenceThreshold),
			EscalationMode:       string(prof.EscalationMode),
			FailMode:             string(prof.FailMode),
			PolicyReference:      prof.PolicyReference,
			ApprovedBy:           prof.ApprovedBy,
			ApprovedAt:           timePtrFromPtr(prof.ApprovedAt),
			EscalationTargetID:   prof.EscalationTargetID,
		},
	}
}

// escalationTargetNode builds the graph node for one resolved
// EscalationTarget (D31l). Label prefers the target's Name; falls
// back to its logical id when Name is empty, matching the
// convention used by every other node builder in this package.
func (s *Service) escalationTargetNode(t *escalation.EscalationTarget) Node {
	label := t.Name
	if label == "" {
		label = t.ID
	}
	return Node{
		Kind:  NodeKindEscalationTarget,
		ID:    t.ID,
		Label: label,
		EscalationTarget: &EscalationTargetData{
			ID:             t.ID,
			Version:        t.Version,
			Name:           t.Name,
			Description:    t.Description,
			Kind:           string(t.Kind),
			Handle:         t.Handle,
			Status:         string(t.Status),
			EffectiveDate:  timePtrString(t.EffectiveDate),
			EffectiveUntil: timePtrFromPtr(t.EffectiveUntil),
			BusinessOwner:  t.BusinessOwner,
			TechnicalOwner: t.TechnicalOwner,
			ApprovedBy:     t.ApprovedBy,
			ApprovedAt:     timePtrFromPtr(t.ApprovedAt),
		},
	}
}

func (s *Service) authorityGrantNode(g *authority.AuthorityGrant) Node {
	label := g.ID
	return Node{
		Kind:  NodeKindAuthorityGrant,
		ID:    g.ID,
		Label: label,
		AuthorityGrant: &AuthorityGrantData{
			ID:             g.ID,
			ProfileID:      g.ProfileID,
			AgentID:        g.AgentID,
			Status:         string(g.Status),
			EffectiveDate:  timePtrString(g.EffectiveDate),
			ExpiresAt:      timePtrFromPtr(g.ExpiresAt),
			ValidityStatus: ValidityStatusEffective,
			GrantedBy:      g.GrantedBy,
			Capabilities:   toCapabilityStrings(g.Capabilities),
			Constraints:    toConstraintDataList(g.Constraints),
		},
	}
}

// toCapabilityStrings maps the typed domain Capabilities slice onto
// the wire shape (a slice of strings). Returns nil for nil/empty
// input so the JSON omitempty tag collapses absent fields.
func toCapabilityStrings(caps []authority.Capability) []string {
	if len(caps) == 0 {
		return nil
	}
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		out = append(out, string(c))
	}
	return out
}

// toConstraintDataList maps the typed domain Constraints slice onto
// the wire shape. Each Constraint becomes a ConstraintData whose Kind
// is always populated and whose per-kind payload fields are populated
// according to the constraint variant. Returns nil for nil/empty
// input.
func toConstraintDataList(cs []authority.Constraint) []ConstraintData {
	if len(cs) == 0 {
		return nil
	}
	out := make([]ConstraintData, 0, len(cs))
	for _, c := range cs {
		cd := ConstraintData{Kind: string(c.Kind)}
		switch c.Kind {
		case authority.ConstraintKindConfidenceThresholdMin:
			m := c.MinConfidence
			cd.MinConfidence = &m
		case authority.ConstraintKindConsequenceThresholdMax:
			cd.MaxConsequence = toConsequenceThresholdData(c.MaxConsequence)
		case authority.ConstraintKindTimeWindow:
			cd.StartTime = timePtrString(c.StartTime)
			cd.EndTime = timePtrString(c.EndTime)
		case authority.ConstraintKindHumanOnly, authority.ConstraintKindAIOnly:
			// no payload
		}
		out = append(out, cd)
	}
	return out
}

func (s *Service) agentNode(a *agent.Agent) Node {
	label := a.Name
	if label == "" {
		label = a.ID
	}
	return Node{
		Kind:  NodeKindAgent,
		ID:    a.ID,
		Label: label,
		Agent: &AgentData{
			ID:               a.ID,
			Name:             a.Name,
			Type:             string(a.Type),
			Owner:            a.Owner,
			ModelVersion:     a.ModelVersion,
			OperationalState: string(a.OperationalState),
		},
	}
}

func (s *Service) failModePolicyNode(p *failmode.FailModePolicy) Node {
	label := p.Name
	if label == "" {
		label = p.ID
	}
	return Node{
		Kind:  NodeKindFailModePolicy,
		ID:    p.ID,
		Label: label,
		FailModePolicy: &FailModePolicyData{
			ID:               p.ID,
			Version:          p.Version,
			Name:             p.Name,
			Status:           string(p.Status),
			EffectiveDate:    timePtrString(p.EffectiveDate),
			EffectiveUntil:   timePtrFromPtr(p.EffectiveUntil),
			BusinessOwner:    p.BusinessOwner,
			TechnicalOwner:   p.TechnicalOwner,
			Origin:           p.Origin,
			Managed:          p.Managed,
			RuleCountByClass: toRuleCountByClass(p.Rules),
		},
	}
}

// ---------------------------------------------------------------------------
// Domain → wire helpers
// ---------------------------------------------------------------------------

// toExternalRefData maps a domain externalref.ExternalRef to the
// wire shape. Returns nil when the reference is nil or IsZero so
// the JSON tag's omitempty collapses the field entirely.
func toExternalRefData(ref *externalref.ExternalRef) *ExternalRefData {
	if ref.IsZero() {
		return nil
	}
	out := &ExternalRefData{
		SourceSystem:  ref.SourceSystem,
		SourceID:      ref.SourceID,
		SourceURL:     ref.SourceURL,
		SourceVersion: ref.SourceVersion,
	}
	if ref.LastSyncedAt != nil {
		s := ref.LastSyncedAt.UTC().Format(time.RFC3339)
		out.LastSyncedAt = &s
	}
	return out
}

// toConsequenceThresholdData maps the typed authority.Consequence
// to the wire shape. Returns nil when the consequence is the zero
// value (no Type recorded).
func toConsequenceThresholdData(c authority.Consequence) *ConsequenceThresholdData {
	t := consequenceTypeString(c.Type)
	if t == "" {
		return nil
	}
	return &ConsequenceThresholdData{
		Type:       t,
		Amount:     c.Amount,
		Currency:   c.Currency,
		RiskRating: string(c.RiskRating),
	}
}

// toRuleCountByClass produces a per-correctness-class rule count
// from a FailModePolicy's Rules slice. All five fields are present;
// classes with no rule are 0. Returns nil when the rules slice is
// nil/empty so the JSON omitempty collapses the field.
func toRuleCountByClass(rules []failmode.FailModePolicyRule) *RuleCountByClassData {
	if len(rules) == 0 {
		return nil
	}
	out := &RuleCountByClassData{}
	for _, r := range rules {
		switch r.CorrectnessClass {
		case failmode.CorrectnessClassGovernanceIntegrity:
			out.GovernanceIntegrity++
		case failmode.CorrectnessClassPersistence:
			out.Persistence++
		case failmode.CorrectnessClassInput:
			out.Input++
		case failmode.CorrectnessClassResource:
			out.Resource++
		case failmode.CorrectnessClassConsistency:
			out.Consistency++
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Depth-bounded BFS
// ---------------------------------------------------------------------------

// bfsVisible computes the set of node keys reachable from root
// within depth undirected hops. Treating edges as undirected matches
// operator intent for "depth = N" — neighbourhood, not out-edges
// only. Returns a set containing the root key plus every reachable
// neighbour up to depth.
//
// The encoding ("kind\x00id") mirrors nodeKey so callers can look up
// nodes and edges by the same key.
func bfsVisible(root NodeRef, edges []Edge, depth int) map[string]struct{} {
	visible := map[string]struct{}{}
	rootKey := nodeKey(root.Kind, root.ID)
	visible[rootKey] = struct{}{}
	if depth <= 0 {
		return visible
	}

	adj := map[string][]string{}
	for _, e := range edges {
		sk := nodeKey(e.Src.Kind, e.Src.ID)
		dk := nodeKey(e.Dst.Kind, e.Dst.ID)
		adj[sk] = append(adj[sk], dk)
		adj[dk] = append(adj[dk], sk)
	}

	frontier := []string{rootKey}
	for hop := 0; hop < depth; hop++ {
		var next []string
		for _, k := range frontier {
			for _, nb := range adj[k] {
				if _, seen := visible[nb]; seen {
					continue
				}
				visible[nb] = struct{}{}
				next = append(next, nb)
			}
		}
		if len(next) == 0 {
			break
		}
		frontier = next
	}
	return visible
}

// nodeKey is the stable encoding for set membership and map lookups
// keyed on (kind, id). The 0x00 separator is forbidden in real ids,
// guaranteeing no collision between e.g. ("ab", "c") and ("a", "bc").
func nodeKey(kind, id string) string {
	return kind + "\x00" + id
}

// ---------------------------------------------------------------------------
// ParseDepth
// ---------------------------------------------------------------------------

// ParseDepth interprets the depth query parameter, applying the MVP
// default-and-clamp rules:
//
//	empty string → DefaultDepth (4)
//	non-numeric  → ErrInvalidDepth
//	negative     → ErrInvalidDepth
//	> MaxDepth   → silently clamped to MaxDepth (5)
//
// The handler invokes ParseDepth on the query parameter; Project
// re-clamps as a defence-in-depth so direct callers cannot bypass
// the bound.
func ParseDepth(raw string) (int, error) {
	if raw == "" {
		return DefaultDepth, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidDepth, raw)
	}
	if n < 0 {
		return 0, fmt.Errorf("%w: %d", ErrInvalidDepth, n)
	}
	if n > MaxDepth {
		return MaxDepth, nil
	}
	return n, nil
}

// Compile-time error references to keep the import list stable when
// future error helpers land.
var (
	_ = errors.Is
)
