package authoritygraph

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/externalref"
	"github.com/accept-io/midas/internal/governancemap"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

// Phase 1 depth bounds. Defaulting and clamping happen at the URL
// parsing layer (ParseDepth); Service.Project applies the same clamp
// defensively for callers that bypass ParseDepth. These constants are
// pinned: do not change without revising the Phase 1 contract.
const (
	DefaultDepth = 3
	MaxDepth     = 5
)

// Typed errors — the handler maps each to the documented status code:
//
//   ErrInvalidView   → 400
//   ErrInvalidID     → 400
//   ErrInvalidDepth  → 400
//   ErrNotFound      → 404
//
// Wrap with fmt.Errorf("...: %w", err) when adding context.
var (
	ErrInvalidView  = errors.New("authoritygraph: invalid view")
	ErrInvalidID    = errors.New("authoritygraph: invalid id")
	ErrInvalidDepth = errors.New("authoritygraph: invalid depth")
	ErrNotFound     = errors.New("authoritygraph: root entity not found")
)

// GovernanceMapReader is the narrow dependency the service-view
// projector requires. *governancemap.ReadService satisfies it.
type GovernanceMapReader interface {
	GetGovernanceMap(ctx context.Context, businessServiceID string) (*governancemap.Map, error)
}

// AISystemReader is the narrow read dependency for the ai_system view.
// *governancemap.ReadService and any wider AI-system repository satisfy
// it via GetByID.
type AISystemReader interface {
	GetByID(ctx context.Context, id string) (*aisystem.AISystem, error)
}

// AISystemBindingRepository is the narrow read dependency for the
// ai_system view's binding traversal. The aisystem.BindingRepository
// implementation satisfies it.
type AISystemBindingRepository interface {
	ListByAISystem(ctx context.Context, aiSystemID string) ([]*aisystem.AISystemBinding, error)
}

// BusinessServiceReader resolves a business-service node by id when
// the ai_system view encounters a binding scoped to (or transitively
// scoping through) a business service.
type BusinessServiceReader interface {
	GetByID(ctx context.Context, id string) (*businessservice.BusinessService, error)
}

// CapabilityReader resolves a capability node by id when the
// ai_system view encounters a capability-scoped binding.
type CapabilityReader interface {
	GetByID(ctx context.Context, id string) (*capability.Capability, error)
}

// ProcessReader resolves a process node by id when the ai_system view
// encounters a process-scoped binding (or a surface scope's parent
// process).
type ProcessReader interface {
	GetByID(ctx context.Context, id string) (*process.Process, error)
}

// SurfaceReader resolves the latest version of a decision surface by
// logical id. The ai_system view emits the latest surface version
// only; per-version drill-down is reserved for the decision_surface
// view.
type SurfaceReader interface {
	FindLatestByID(ctx context.Context, id string) (*surface.DecisionSurface, error)
}

// Readers bundles the read dependencies for every view this service
// supports. A field may be left nil to disable views that depend on
// it; the constructor only registers a view's projector when its
// required readers are non-nil.
type Readers struct {
	GovernanceMap    GovernanceMapReader
	AISystem         AISystemReader
	AISystemBindings AISystemBindingRepository
	BusinessServices BusinessServiceReader
	Capabilities     CapabilityReader
	Processes        ProcessReader
	Surfaces         SurfaceReader
}

// viewProjector is the per-view projection function signature. Each
// supported view registers one projector in Service.projectors; the
// Project entry point looks up by view name and delegates after
// validating id/depth.
type viewProjector func(ctx context.Context, id string, depth int) (*Projection, error)

// Service projects the governance map and adjacent read services into
// a generic node/edge graph. Multi-view dispatch is via the
// projectors map populated at construction time.
type Service struct {
	readers    Readers
	projectors map[string]viewProjector
}

// NewServiceWithReaders constructs a Service with the full set of
// view-specific readers. Each view is registered iff all its required
// readers are non-nil; otherwise requests for that view return
// ErrInvalidView (the projector is simply absent from the dispatch
// map).
func NewServiceWithReaders(r Readers) *Service {
	s := &Service{readers: r, projectors: map[string]viewProjector{}}
	if r.GovernanceMap != nil {
		s.projectors[ViewService] = s.projectServiceViewEntry
	}
	if r.AISystem != nil && r.AISystemBindings != nil &&
		r.BusinessServices != nil && r.Capabilities != nil &&
		r.Processes != nil && r.Surfaces != nil {
		s.projectors[ViewAISystem] = s.projectAISystemViewEntry
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

// projectServiceViewEntry is the registered projector for ViewService.
// Mirrors the previous Project body: fetches the governance map for
// the BS and delegates to projectServiceView. ErrNotFound is wrapped
// to match the existing handler 404 contract.
func (s *Service) projectServiceViewEntry(ctx context.Context, id string, depth int) (*Projection, error) {
	gmap, err := s.readers.GovernanceMap.GetGovernanceMap(ctx, id)
	if err != nil {
		return nil, err
	}
	if gmap == nil {
		return nil, fmt.Errorf("%w: business_service:%s", ErrNotFound, id)
	}
	return projectServiceView(gmap, id, depth), nil
}

// projectServiceView converts the typed governance Map to a Phase 1
// projection. The function is split into "build the unfiltered graph"
// (every node and edge derivable from the Map) followed by
// "depth-bounded filter + sort". Sort keys are pinned by tests.
func projectServiceView(gmap *governancemap.Map, bsID string, depth int) *Projection {
	root := NodeRef{Kind: NodeKindBusinessService, ID: bsID}
	nodes, edges := buildServiceGraph(gmap, bsID)

	visible := bfsVisible(root, edges, depth)

	keptNodes := make([]Node, 0, len(nodes))
	for _, n := range nodes {
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

	keptEdges := make([]Edge, 0, len(edges))
	for _, e := range edges {
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
		Root:  root,
		View:  ViewService,
		Depth: depth,
		Nodes: keptNodes,
		Edges: keptEdges,
	}
}

// buildServiceGraph constructs the unfiltered node and edge slices for
// the service view. Every Phase 1 node/edge kind is emitted here; the
// downstream BFS filter prunes by depth.
//
// Source data lineage:
//
//   - business_service          ← gmap.BusinessService
//   - related_business_service  ← gmap.Relationships.Outgoing only
//                                 (incoming is intentionally omitted in
//                                 Phase 1; the relates_to edge is
//                                 directed BS → related_BS).
//   - capability                ← gmap.Capabilities (linked via BSC)
//   - process                   ← gmap.Processes
//   - decision_surface          ← gmap.Surfaces
//   - ai_system                 ← gmap.AISystems[].System
//                                 (deduped by AI system id)
//   - ai_system_binding         ← gmap.AISystems[].Bindings
//                                 (deduped by binding id)
//   - authority_summary         ← synthetic, one per projection
//   - coverage                  ← synthetic, one per projection
//
// Edges:
//
//   - has_process is BS → process (NOT capability → process). The
//     governance Map carries no Capability ↔ Process FK; modelling
//     the edge as BS → process matches the Map's actual structure
//     and avoids inventing a relationship that doesn't exist in the
//     metamodel.
//
//   - bound_to uses the most-specific non-empty scope on the binding
//     (surface > process > capability > business_service) — same
//     precedence the existing gmap connector renderer uses. A binding
//     with no scope (which the schema's chk_ai_bindings_at_least_one_target
//     prevents at apply time) emits no bound_to edge defensively.
//
//   - system_of always emits — it is the binding ↔ AI system edge.
func buildServiceGraph(gmap *governancemap.Map, bsID string) ([]Node, []Edge) {
	var nodes []Node
	var edges []Edge

	// Root business service. Typed data mirrors the governance-map
	// businessService DTO for the fields available on the domain type.
	bsLabel := bsID
	var bsData *BusinessServiceData
	if gmap.BusinessService != nil && gmap.BusinessService.BusinessService != nil {
		bs := gmap.BusinessService.BusinessService
		if bs.Name != "" {
			bsLabel = bs.Name
		}
		bsData = &BusinessServiceData{
			ID:              bs.ID,
			Name:            bs.Name,
			Description:     bs.Description,
			Status:          bs.Status,
			Owner:           bs.OwnerID,
			ServiceType:     string(bs.ServiceType),
			RegulatoryScope: bs.RegulatoryScope,
			ExternalRef:     toExternalRefData(bs.ExternalRef),
		}
	}
	nodes = append(nodes, Node{
		Kind:            NodeKindBusinessService,
		ID:              bsID,
		Label:           bsLabel,
		BusinessService: bsData,
	})

	// Related business services — both outgoing and incoming
	// relationships. One node per related-BS id even when the same
	// other-BS appears in both directions (the schema's uniq_bsr_triple
	// constraint allows root↔X via two distinct rows). Direction is
	// encoded by which sub-row pointer is populated on the typed data;
	// edges always carry the real business direction.
	//
	// Pass 1: walk both direction lists, accumulating sub-rows and
	// edges keyed by the related-BS id. Outgoing-row id → relationship
	// target, incoming-row id → relationship source.
	relatedData := map[string]*RelatedBusinessServiceData{}
	relatedOrder := []string{}
	upsertRelated := func(otherID, otherName string) *RelatedBusinessServiceData {
		if rd, ok := relatedData[otherID]; ok {
			// Prefer a non-empty name from a later row over an
			// earlier empty one; otherwise leave alone.
			if rd.Name == "" && otherName != "" {
				rd.Name = otherName
			}
			return rd
		}
		rd := &RelatedBusinessServiceData{ID: otherID, Name: otherName}
		relatedData[otherID] = rd
		relatedOrder = append(relatedOrder, otherID)
		return rd
	}

	for _, rn := range gmap.Relationships.Outgoing {
		if rn == nil || rn.Relationship == nil {
			continue
		}
		relID := rn.Relationship.TargetBusinessService
		if relID == "" {
			continue
		}
		rd := upsertRelated(relID, rn.OtherName)
		rd.Outgoing = &RelatedBusinessServiceRow{
			RelationshipID:   rn.Relationship.ID,
			RelationshipType: rn.Relationship.RelationshipType,
			Description:      rn.Relationship.Description,
		}
		// relates_to: root → related (outgoing direction).
		edges = append(edges, Edge{
			Kind: EdgeKindRelatesTo,
			Src:  NodeRef{Kind: NodeKindBusinessService, ID: bsID},
			Dst:  NodeRef{Kind: NodeKindRelatedBusinessService, ID: relID},
		})
	}

	for _, rn := range gmap.Relationships.Incoming {
		if rn == nil || rn.Relationship == nil {
			continue
		}
		// On the incoming list, the "other" BS is the source of the
		// relationship row.
		relID := rn.Relationship.SourceBusinessService
		if relID == "" {
			continue
		}
		rd := upsertRelated(relID, rn.OtherName)
		rd.Incoming = &RelatedBusinessServiceRow{
			RelationshipID:   rn.Relationship.ID,
			RelationshipType: rn.Relationship.RelationshipType,
			Description:      rn.Relationship.Description,
		}
		// relates_to: related → root (incoming direction).
		edges = append(edges, Edge{
			Kind: EdgeKindRelatesTo,
			Src:  NodeRef{Kind: NodeKindRelatedBusinessService, ID: relID},
			Dst:  NodeRef{Kind: NodeKindBusinessService, ID: bsID},
		})
	}

	// Pass 2: emit one node per related-BS id, in the order of first
	// appearance (downstream slice-level sort by (kind, id) enforces
	// the final wire order). Label preference: name → id.
	for _, relID := range relatedOrder {
		rd := relatedData[relID]
		relLabel := rd.Name
		if relLabel == "" {
			relLabel = relID
		}
		nodes = append(nodes, Node{
			Kind:                   NodeKindRelatedBusinessService,
			ID:                     relID,
			Label:                  relLabel,
			RelatedBusinessService: rd,
		})
	}

	// Capabilities under root.
	for _, cn := range gmap.Capabilities {
		if cn == nil || cn.Capability == nil {
			continue
		}
		c := cn.Capability
		capLabel := c.Name
		if capLabel == "" {
			capLabel = c.ID
		}
		nodes = append(nodes, Node{
			Kind:  NodeKindCapability,
			ID:    c.ID,
			Label: capLabel,
			Capability: &CapabilityData{
				ID:                 c.ID,
				Name:               c.Name,
				Description:        c.Description,
				Status:             c.Status,
				Owner:              c.Owner,
				ParentCapabilityID: c.ParentCapabilityID,
				ExternalRef:        toExternalRefData(c.ExternalRef),
			},
		})
		edges = append(edges, Edge{
			Kind: EdgeKindHasCapability,
			Src:  NodeRef{Kind: NodeKindBusinessService, ID: bsID},
			Dst:  NodeRef{Kind: NodeKindCapability, ID: c.ID},
		})
	}

	// Processes under root.
	//
	// Note: process.Process has no ExternalRef field on the domain
	// type today, so ProcessData has no external_ref field. If the
	// domain gains one, the typed data will be extended additively.
	for _, pn := range gmap.Processes {
		if pn == nil || pn.Process == nil {
			continue
		}
		p := pn.Process
		procLabel := p.Name
		if procLabel == "" {
			procLabel = p.ID
		}
		nodes = append(nodes, Node{
			Kind:  NodeKindProcess,
			ID:    p.ID,
			Label: procLabel,
			Process: &ProcessData{
				ID:                p.ID,
				Name:              p.Name,
				Description:       p.Description,
				Status:            p.Status,
				Owner:             p.Owner,
				BusinessServiceID: p.BusinessServiceID,
			},
		})
		edges = append(edges, Edge{
			Kind: EdgeKindHasProcess,
			Src:  NodeRef{Kind: NodeKindBusinessService, ID: bsID},
			Dst:  NodeRef{Kind: NodeKindProcess, ID: p.ID},
		})
	}

	// Decision surfaces — has_surface uses surface.ProcessID to anchor
	// the edge to the parent process. A surface with empty ProcessID
	// (which the schema's NOT NULL prevents) emits the surface node
	// but no edge. Typed data carries the AI binding ID arrays the
	// governance map already accumulates per surface, plus the
	// authority counts.
	for _, sn := range gmap.Surfaces {
		if sn == nil || sn.Surface == nil {
			continue
		}
		s := sn.Surface
		surfLabel := s.Name
		if surfLabel == "" {
			surfLabel = s.ID
		}
		// Defensive non-nil slice copies so the wire shape always
		// renders an array (never null) even if the upstream slices
		// are nil.
		directIDs := append([]string{}, sn.AIBindingIDs...)
		inheritedIDs := append([]string{}, sn.InheritedAIBindingIDs...)
		nodes = append(nodes, Node{
			Kind:  NodeKindDecisionSurface,
			ID:    s.ID,
			Label: surfLabel,
			DecisionSurface: &DecisionSurfaceData{
				ID:                    s.ID,
				Version:               s.Version,
				Name:                  s.Name,
				Description:           s.Description,
				Status:                string(s.Status),
				ProcessID:             s.ProcessID,
				AIBindingIDs:          directIDs,
				InheritedAIBindingIDs: inheritedIDs,
				ProfileCount:          sn.ProfileCount,
				GrantCount:            sn.GrantCount,
				AgentCount:            sn.AgentCount,
			},
		})
		if s.ProcessID != "" {
			edges = append(edges, Edge{
				Kind: EdgeKindHasSurface,
				Src:  NodeRef{Kind: NodeKindProcess, ID: s.ProcessID},
				Dst:  NodeRef{Kind: NodeKindDecisionSurface, ID: s.ID},
			})
		}
	}

	// AI Systems and bindings. AISystem typed data carries the active
	// version (when present) plus vendor / system_type / external_ref.
	// Each binding's typed data carries the four scope-id pointers
	// plus the most-specific scope summary (matching bound_to + node
	// label).
	seenAISystem := map[string]struct{}{}
	seenBinding := map[string]struct{}{}
	for _, ain := range gmap.AISystems {
		if ain == nil || ain.System == nil {
			continue
		}
		sys := ain.System
		sysID := sys.ID
		sysNameOrID := sys.Name
		if sysNameOrID == "" {
			sysNameOrID = sysID
		}

		if _, ok := seenAISystem[sysID]; !ok {
			sysData := &AISystemData{
				ID:          sysID,
				Name:        sys.Name,
				Description: sys.Description,
				Status:      sys.Status,
				Vendor:      sys.Vendor,
				SystemType:  sys.SystemType,
				ExternalRef: toExternalRefData(sys.ExternalRef),
			}
			if ain.ActiveVersion != nil {
				v := ain.ActiveVersion.Version
				sysData.ActiveVersion = &v
				sysData.ActiveVersionLabel = ain.ActiveVersion.ReleaseLabel
				sysData.ActiveVersionStatus = ain.ActiveVersion.Status
			}
			nodes = append(nodes, Node{
				Kind:     NodeKindAISystem,
				ID:       sysID,
				Label:    sysNameOrID,
				AISystem: sysData,
			})
			seenAISystem[sysID] = struct{}{}
		}

		// Sort bindings by ID before emission so the build-side
		// ordering is stable. The downstream slice-level sort over
		// edges enforces the final wire order.
		bindings := append([]*aisystem.AISystemBinding(nil), ain.Bindings...)
		sort.Slice(bindings, func(i, j int) bool {
			return bindings[i] != nil && bindings[j] != nil && bindings[i].ID < bindings[j].ID
		})

		for _, b := range bindings {
			if b == nil || b.ID == "" {
				continue
			}
			if _, ok := seenBinding[b.ID]; ok {
				continue
			}
			scopeKind, scopeID := mostSpecificBindingScope(b)
			scopeLabel := scopeTokenFor(scopeKind, scopeID)
			label := fmt.Sprintf("binding: %s → %s", sysNameOrID, scopeLabel)
			nodes = append(nodes, Node{
				Kind:  NodeKindAISystemBinding,
				ID:    b.ID,
				Label: label,
				AISystemBinding: &AISystemBindingData{
					ID:                b.ID,
					AISystemID:        b.AISystemID,
					AISystemName:      sysNameOrID,
					BusinessServiceID: stringPtrOrNil(b.BusinessServiceID),
					CapabilityID:      stringPtrOrNil(b.CapabilityID),
					ProcessID:         stringPtrOrNil(b.ProcessID),
					SurfaceID:         stringPtrOrNil(b.SurfaceID),
					Role:              b.Role,
					Description:       b.Description,
					ScopeKind:         scopeKind,
					ScopeID:           scopeID,
					ScopeLabel:        scopeLabel,
				},
			})
			seenBinding[b.ID] = struct{}{}

			// bound_to: binding → most-specific scope (when present).
			if scopeKind != "" && scopeID != "" {
				edges = append(edges, Edge{
					Kind: EdgeKindBoundTo,
					Src:  NodeRef{Kind: NodeKindAISystemBinding, ID: b.ID},
					Dst:  NodeRef{Kind: scopeKind, ID: scopeID},
				})
			}
			// system_of: binding → AI system. Always emitted.
			edges = append(edges, Edge{
				Kind: EdgeKindSystemOf,
				Src:  NodeRef{Kind: NodeKindAISystemBinding, ID: b.ID},
				Dst:  NodeRef{Kind: NodeKindAISystem, ID: sysID},
			})
		}
	}

	// Synthetic authority_summary node + summarises edge. Typed data
	// carries the four counts the governance-map AuthoritySummary DTO
	// exposes — directly comparable to the gmap response in the
	// cross-endpoint contract test.
	summaryID := "authority_summary:" + bsID
	var summaryData *AuthoritySummaryData
	if gmap.AuthoritySummary != nil {
		summaryData = &AuthoritySummaryData{
			SurfaceCount:       gmap.AuthoritySummary.SurfaceCount,
			ActiveProfileCount: gmap.AuthoritySummary.ActiveProfileCount,
			ActiveGrantCount:   gmap.AuthoritySummary.ActiveGrantCount,
			ActiveAgentCount:   gmap.AuthoritySummary.ActiveAgentCount,
		}
	}
	nodes = append(nodes, Node{
		Kind:             NodeKindAuthoritySummary,
		ID:               summaryID,
		Label:            "Authority Summary",
		AuthoritySummary: summaryData,
	})
	edges = append(edges, Edge{
		Kind: EdgeKindSummarises,
		Src:  NodeRef{Kind: NodeKindAuthoritySummary, ID: summaryID},
		Dst:  NodeRef{Kind: NodeKindBusinessService, ID: bsID},
	})

	// Synthetic coverage node + reports_coverage edge. Typed data
	// carries the four counts the governance-map Coverage DTO exposes
	// (Phase 9 explicit field naming).
	coverageID := "coverage:" + bsID
	var coverageData *CoverageData
	if gmap.Coverage != nil {
		coverageData = &CoverageData{
			SurfaceCount:                gmap.Coverage.SurfaceCount,
			SurfacesWithDirectAIBinding: gmap.Coverage.SurfacesWithDirectAIBinding,
			SurfacesWithScopedAIBinding: gmap.Coverage.SurfacesWithScopedAIBinding,
			SurfacesWithNoAIBinding:     gmap.Coverage.SurfacesWithNoAIBinding,
		}
	}
	nodes = append(nodes, Node{
		Kind:     NodeKindCoverage,
		ID:       coverageID,
		Label:    "AI Binding Coverage",
		Coverage: coverageData,
	})
	edges = append(edges, Edge{
		Kind: EdgeKindReportsCoverage,
		Src:  NodeRef{Kind: NodeKindCoverage, ID: coverageID},
		Dst:  NodeRef{Kind: NodeKindBusinessService, ID: bsID},
	})

	return nodes, edges
}

// ---------------------------------------------------------------------------
// AI System view projector
// ---------------------------------------------------------------------------

// projectAISystemViewEntry is the registered projector for
// ViewAISystem. Walks ai_system → bindings → scope targets and emits
// Phase 1 nodes / edges only:
//
//	ai_system           one (the root)
//	ai_system_binding   one per binding
//	business_service / capability / process / decision_surface
//	                    one per distinct scope target id (deduped)
//
// Edges:
//
//	system_of           binding → ai_system          (one per binding)
//	bound_to            binding → most-specific scope (one per binding,
//	                    omitted only when scope target cannot be resolved)
//	has_surface         process → decision_surface    (when surface scope)
//	has_process         business_service → process    (when surface or
//	                    process scope, anchoring the parent BS)
//	has_capability      business_service → capability (when capability scope)
//
// No new node or edge kinds. No forbidden kinds. Per-binding scope
// resolution uses the existing mostSpecificBindingScope /
// scopeTokenFor helpers to stay consistent with the service view's
// connector-rendering rule.
func (s *Service) projectAISystemViewEntry(ctx context.Context, id string, depth int) (*Projection, error) {
	sys, err := s.readers.AISystem.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if sys == nil {
		return nil, fmt.Errorf("%w: ai_system:%s", ErrNotFound, id)
	}
	bindings, err := s.readers.AISystemBindings.ListByAISystem(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.projectAISystemView(ctx, sys, bindings, depth), nil
}

// projectAISystemView is the deterministic build + filter + sort
// pipeline for the AI-system view. Side-effect-free except for the
// scope-target reader calls; pure given the same inputs.
func (s *Service) projectAISystemView(ctx context.Context, sys *aisystem.AISystem, bindings []*aisystem.AISystemBinding, depth int) *Projection {
	root := NodeRef{Kind: NodeKindAISystem, ID: sys.ID}
	nodes, edges := s.buildAISystemGraph(ctx, sys, bindings)

	visible := bfsVisible(root, edges, depth)

	keptNodes := make([]Node, 0, len(nodes))
	for _, n := range nodes {
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

	keptEdges := make([]Edge, 0, len(edges))
	for _, e := range edges {
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
		Root:  root,
		View:  ViewAISystem,
		Depth: depth,
		Nodes: keptNodes,
		Edges: keptEdges,
	}
}

// buildAISystemGraph constructs the unfiltered node and edge slices
// for the ai_system view. Scope-target nodes are deduped by (kind, id).
//
// Missing scope target behaviour: when the most-specific scope id is
// non-empty but the corresponding repository GetByID/FindLatestByID
// returns (nil, nil) or an error, the binding node and system_of edge
// are still emitted; the bound_to edge and the missing target node
// are omitted. This matches the brief's "missing scope target"
// contract — the projection succeeds with reduced detail rather than
// failing the whole call.
//
// Note on scope context-edges:
//
//   - For surface-scoped bindings: also emit the parent process and
//     parent business_service nodes (using surface.ProcessID and
//     process.BusinessServiceID), plus has_surface (proc → surf) and
//     has_process (bs → proc) edges. This keeps the surface anchored
//     to its real domain context, mirroring the service view.
//   - For process-scoped bindings: also emit the parent
//     business_service and has_process (bs → proc) edge.
//   - For capability-scoped bindings: also emit the parent
//     business_service and has_capability (bs → cap) edge.
//   - For business-service-scoped bindings: no further parent.
//
// We deliberately do NOT invent capability → process edges (the
// metamodel has no such relationship).
func (s *Service) buildAISystemGraph(ctx context.Context, sys *aisystem.AISystem, bindings []*aisystem.AISystemBinding) ([]Node, []Edge) {
	var nodes []Node
	var edges []Edge

	// Root AI system node. Typed data mirrors the service view's
	// AISystemData population for the fields available on the
	// aisystem.AISystem domain type. Active-version data is only
	// available via AISystemVersionReader, which this view does NOT
	// inject — operators wanting per-version detail should use the
	// service view (or a future ai_system_version-aware view).
	sysLabel := sys.Name
	if sysLabel == "" {
		sysLabel = sys.ID
	}
	nodes = append(nodes, Node{
		Kind:  NodeKindAISystem,
		ID:    sys.ID,
		Label: sysLabel,
		AISystem: &AISystemData{
			ID:          sys.ID,
			Name:        sys.Name,
			Description: sys.Description,
			Status:      sys.Status,
			Vendor:      sys.Vendor,
			SystemType:  sys.SystemType,
			ExternalRef: toExternalRefData(sys.ExternalRef),
		},
	})

	// Dedup scope-target nodes (and their parent context nodes) by
	// (kind, id). Same map shared across all bindings — a process
	// referenced as scope for one binding and as parent context for
	// another collapses to a single node.
	seen := map[string]struct{}{}
	mark := func(kind, id string) bool {
		k := kind + "\x00" + id
		if _, ok := seen[k]; ok {
			return false
		}
		seen[k] = struct{}{}
		return true
	}

	// Sort bindings by ID for deterministic emission order. The
	// downstream slice-level sort enforces the final wire order.
	sortedBindings := append([]*aisystem.AISystemBinding(nil), bindings...)
	sort.Slice(sortedBindings, func(i, j int) bool {
		if sortedBindings[i] == nil || sortedBindings[j] == nil {
			return sortedBindings[i] != nil
		}
		return sortedBindings[i].ID < sortedBindings[j].ID
	})

	for _, b := range sortedBindings {
		if b == nil || b.ID == "" {
			continue
		}
		scopeKind, scopeID := mostSpecificBindingScope(b)
		scopeLabel := scopeTokenFor(scopeKind, scopeID)
		bindingLabel := fmt.Sprintf("binding: %s → %s", sysLabel, scopeLabel)
		nodes = append(nodes, Node{
			Kind:  NodeKindAISystemBinding,
			ID:    b.ID,
			Label: bindingLabel,
			AISystemBinding: &AISystemBindingData{
				ID:                b.ID,
				AISystemID:        b.AISystemID,
				AISystemName:      sysLabel,
				BusinessServiceID: stringPtrOrNil(b.BusinessServiceID),
				CapabilityID:      stringPtrOrNil(b.CapabilityID),
				ProcessID:         stringPtrOrNil(b.ProcessID),
				SurfaceID:         stringPtrOrNil(b.SurfaceID),
				Role:              b.Role,
				Description:       b.Description,
				ScopeKind:         scopeKind,
				ScopeID:           scopeID,
				ScopeLabel:        scopeLabel,
			},
		})
		// system_of always emits.
		edges = append(edges, Edge{
			Kind: EdgeKindSystemOf,
			Src:  NodeRef{Kind: NodeKindAISystemBinding, ID: b.ID},
			Dst:  NodeRef{Kind: NodeKindAISystem, ID: sys.ID},
		})

		// Resolve scope target + parent context. Missing-resolution
		// branches keep the binding/system_of edge and skip the rest.
		switch scopeKind {
		case NodeKindDecisionSurface:
			surf := s.lookupSurface(ctx, scopeID)
			if surf == nil {
				continue
			}
			if mark(NodeKindDecisionSurface, surf.ID) {
				nodes = append(nodes, s.surfaceNode(surf))
			}
			edges = append(edges, Edge{
				Kind: EdgeKindBoundTo,
				Src:  NodeRef{Kind: NodeKindAISystemBinding, ID: b.ID},
				Dst:  NodeRef{Kind: NodeKindDecisionSurface, ID: surf.ID},
			})
			// Parent process (when known on the surface).
			if surf.ProcessID != "" {
				proc := s.lookupProcess(ctx, surf.ProcessID)
				if proc != nil {
					if mark(NodeKindProcess, proc.ID) {
						nodes = append(nodes, s.processNode(proc))
					}
					edges = append(edges, Edge{
						Kind: EdgeKindHasSurface,
						Src:  NodeRef{Kind: NodeKindProcess, ID: proc.ID},
						Dst:  NodeRef{Kind: NodeKindDecisionSurface, ID: surf.ID},
					})
					// Parent BS via the process.
					if proc.BusinessServiceID != "" {
						bs := s.lookupBusinessService(ctx, proc.BusinessServiceID)
						if bs != nil {
							if mark(NodeKindBusinessService, bs.ID) {
								nodes = append(nodes, s.businessServiceNode(bs))
							}
							edges = append(edges, Edge{
								Kind: EdgeKindHasProcess,
								Src:  NodeRef{Kind: NodeKindBusinessService, ID: bs.ID},
								Dst:  NodeRef{Kind: NodeKindProcess, ID: proc.ID},
							})
						}
					}
				}
			}
		case NodeKindProcess:
			proc := s.lookupProcess(ctx, scopeID)
			if proc == nil {
				continue
			}
			if mark(NodeKindProcess, proc.ID) {
				nodes = append(nodes, s.processNode(proc))
			}
			edges = append(edges, Edge{
				Kind: EdgeKindBoundTo,
				Src:  NodeRef{Kind: NodeKindAISystemBinding, ID: b.ID},
				Dst:  NodeRef{Kind: NodeKindProcess, ID: proc.ID},
			})
			// Parent BS.
			if proc.BusinessServiceID != "" {
				bs := s.lookupBusinessService(ctx, proc.BusinessServiceID)
				if bs != nil {
					if mark(NodeKindBusinessService, bs.ID) {
						nodes = append(nodes, s.businessServiceNode(bs))
					}
					edges = append(edges, Edge{
						Kind: EdgeKindHasProcess,
						Src:  NodeRef{Kind: NodeKindBusinessService, ID: bs.ID},
						Dst:  NodeRef{Kind: NodeKindProcess, ID: proc.ID},
					})
				}
			}
		case NodeKindCapability:
			cap := s.lookupCapability(ctx, scopeID)
			if cap == nil {
				continue
			}
			if mark(NodeKindCapability, cap.ID) {
				nodes = append(nodes, s.capabilityNode(cap))
			}
			edges = append(edges, Edge{
				Kind: EdgeKindBoundTo,
				Src:  NodeRef{Kind: NodeKindAISystemBinding, ID: b.ID},
				Dst:  NodeRef{Kind: NodeKindCapability, ID: cap.ID},
			})
			// Parent BS via b.BusinessServiceID, when set on the
			// binding row itself. The metamodel's capability does
			// not own a BS pointer, so the binding row is the only
			// authoritative source for "which BS this capability is
			// scoped to" in this view.
			if b.BusinessServiceID != "" {
				bs := s.lookupBusinessService(ctx, b.BusinessServiceID)
				if bs != nil {
					if mark(NodeKindBusinessService, bs.ID) {
						nodes = append(nodes, s.businessServiceNode(bs))
					}
					edges = append(edges, Edge{
						Kind: EdgeKindHasCapability,
						Src:  NodeRef{Kind: NodeKindBusinessService, ID: bs.ID},
						Dst:  NodeRef{Kind: NodeKindCapability, ID: cap.ID},
					})
				}
			}
		case NodeKindBusinessService:
			bs := s.lookupBusinessService(ctx, scopeID)
			if bs == nil {
				continue
			}
			if mark(NodeKindBusinessService, bs.ID) {
				nodes = append(nodes, s.businessServiceNode(bs))
			}
			edges = append(edges, Edge{
				Kind: EdgeKindBoundTo,
				Src:  NodeRef{Kind: NodeKindAISystemBinding, ID: b.ID},
				Dst:  NodeRef{Kind: NodeKindBusinessService, ID: bs.ID},
			})
		default:
			// scopeKind == "" — no scope on the binding (defensive;
			// schema chk_ai_bindings_at_least_one_target prevents
			// this at apply time). Keep the binding and system_of
			// edge; emit no bound_to.
			continue
		}
	}

	return nodes, edges
}

// lookupSurface / lookupProcess / lookupCapability / lookupBusinessService
// are thin wrappers around the injected reader GetByIDs that swallow
// errors. Missing-resolution semantics: any error or nil result is
// treated as "target not resolvable here" — the projection continues
// without the missing context node.
func (s *Service) lookupSurface(ctx context.Context, id string) *surface.DecisionSurface {
	v, err := s.readers.Surfaces.FindLatestByID(ctx, id)
	if err != nil {
		return nil
	}
	return v
}

func (s *Service) lookupProcess(ctx context.Context, id string) *process.Process {
	v, err := s.readers.Processes.GetByID(ctx, id)
	if err != nil {
		return nil
	}
	return v
}

func (s *Service) lookupCapability(ctx context.Context, id string) *capability.Capability {
	v, err := s.readers.Capabilities.GetByID(ctx, id)
	if err != nil {
		return nil
	}
	return v
}

func (s *Service) lookupBusinessService(ctx context.Context, id string) *businessservice.BusinessService {
	v, err := s.readers.BusinessServices.GetByID(ctx, id)
	if err != nil {
		return nil
	}
	return v
}

// surfaceNode / processNode / capabilityNode / businessServiceNode
// build the per-kind Node + typed-data tuple. Mirror the service
// view's typed-data population so consumers see the same fields
// regardless of which view rooted the projection. ai_binding_ids /
// inherited_ai_binding_ids and per-surface counts are not populated
// here — those are aggregated by governancemap.ReadService for the
// service view; the ai_system view does not run that aggregation.
func (s *Service) surfaceNode(surf *surface.DecisionSurface) Node {
	label := surf.Name
	if label == "" {
		label = surf.ID
	}
	return Node{
		Kind:  NodeKindDecisionSurface,
		ID:    surf.ID,
		Label: label,
		DecisionSurface: &DecisionSurfaceData{
			ID:                    surf.ID,
			Version:               surf.Version,
			Name:                  surf.Name,
			Description:           surf.Description,
			Status:                string(surf.Status),
			ProcessID:             surf.ProcessID,
			AIBindingIDs:          []string{},
			InheritedAIBindingIDs: []string{},
		},
	}
}

func (s *Service) processNode(proc *process.Process) Node {
	label := proc.Name
	if label == "" {
		label = proc.ID
	}
	return Node{
		Kind:  NodeKindProcess,
		ID:    proc.ID,
		Label: label,
		Process: &ProcessData{
			ID:                proc.ID,
			Name:              proc.Name,
			Description:       proc.Description,
			Status:            proc.Status,
			Owner:             proc.Owner,
			BusinessServiceID: proc.BusinessServiceID,
		},
	}
}

func (s *Service) capabilityNode(cap *capability.Capability) Node {
	label := cap.Name
	if label == "" {
		label = cap.ID
	}
	return Node{
		Kind:  NodeKindCapability,
		ID:    cap.ID,
		Label: label,
		Capability: &CapabilityData{
			ID:                 cap.ID,
			Name:               cap.Name,
			Description:        cap.Description,
			Status:             cap.Status,
			Owner:              cap.Owner,
			ParentCapabilityID: cap.ParentCapabilityID,
			ExternalRef:        toExternalRefData(cap.ExternalRef),
		},
	}
}

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
			ID:              bs.ID,
			Name:            bs.Name,
			Description:     bs.Description,
			Status:          bs.Status,
			Owner:           bs.OwnerID,
			ServiceType:     string(bs.ServiceType),
			RegulatoryScope: bs.RegulatoryScope,
			ExternalRef:     toExternalRefData(bs.ExternalRef),
		},
	}
}

// stringPtrOrNil returns &s when s is non-empty, else nil. Used by the
// AI binding scope-id pointer fields so the wire shape distinguishes
// "scope unset" (null on the wire) from "scope is the empty string".
func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// toExternalRefData maps a domain externalref.ExternalRef to the wire
// shape, mirroring the existing httpapi.toExternalRefResponse helper
// (server.go:3025). Returns nil when the reference is nil or
// IsZero so the JSON shape distinguishes "no external ref" from
// "external ref present but empty".
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

// mostSpecificBindingScope returns the binding's scope target using
// the precedence surface > process > capability > business_service.
// Mirrors the connector-rendering rule in
// internal/httpapi/explorer/index.html. Returns ("", "") only when
// every scope id on the binding is empty — chk_ai_bindings_at_least_one_target
// makes this impossible at apply time but the function stays
// defensive.
func mostSpecificBindingScope(b *aisystem.AISystemBinding) (string, string) {
	if b.SurfaceID != "" {
		return NodeKindDecisionSurface, b.SurfaceID
	}
	if b.ProcessID != "" {
		return NodeKindProcess, b.ProcessID
	}
	if b.CapabilityID != "" {
		return NodeKindCapability, b.CapabilityID
	}
	if b.BusinessServiceID != "" {
		return NodeKindBusinessService, b.BusinessServiceID
	}
	return "", ""
}

// scopeTokenFor renders the human-readable token used inside binding
// labels: "<short_kind>:<id>". The short kind matches the existing
// gmap inspection panel convention (surface, process, capability,
// business_service). Returns "unscoped" when no scope is present.
func scopeTokenFor(kind, id string) string {
	if kind == "" || id == "" {
		return "unscoped"
	}
	var prefix string
	switch kind {
	case NodeKindDecisionSurface:
		prefix = "surface"
	case NodeKindProcess:
		prefix = "process"
	case NodeKindCapability:
		prefix = "capability"
	case NodeKindBusinessService:
		prefix = "business_service"
	default:
		prefix = kind
	}
	return prefix + ":" + id
}

// bfsVisible computes the set of node keys reachable from root within
// depth hops, treating edges as undirected. Treating edges as
// undirected matches operator intent for "depth = N" — the user
// expects to see neighbourhood, not just out-edges. Returns a set
// containing the root key plus every reachable neighbour up to depth.
//
// The set encoding ("kind\x00id") mirrors nodeKey so callers can
// look up nodes and edges by the same key.
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

// ParseDepth interprets the depth query parameter, applying the
// Phase 1 default-and-clamp rules:
//
//   empty string → DefaultDepth (3)
//   non-numeric  → ErrInvalidDepth
//   negative     → ErrInvalidDepth
//   > MaxDepth   → silently clamped to MaxDepth (5)
//
// The handler invokes ParseDepth on the query parameter; Service.Project
// re-clamps as a defence-in-depth so direct callers cannot bypass the
// bound.
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
