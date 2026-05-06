package authoritygraph

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/externalref"
	"github.com/accept-io/midas/internal/governancemap"
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

// GovernanceMapReader is the narrow dependency the projection service
// requires. *governancemap.ReadService satisfies it. Defined here so
// tests can swap a stub without dragging the full governancemap
// surface, mirroring the governanceMapReadService interface in
// internal/httpapi/governance_map_handler.go.
type GovernanceMapReader interface {
	GetGovernanceMap(ctx context.Context, businessServiceID string) (*governancemap.Map, error)
}

// Service projects the governance map into a generic node/edge graph.
// Phase 1 supports view=service only.
type Service struct {
	governanceMap GovernanceMapReader
}

// NewService constructs a Service. The reader argument must be
// non-nil; callers that pass nil get a service that returns
// ErrNotFound on every call (defensive — production wiring uses the
// configured *governancemap.ReadService).
func NewService(governanceMap GovernanceMapReader) *Service {
	return &Service{governanceMap: governanceMap}
}

// Project builds a depth-bounded projection rooted at (view, id).
//
// Validation rules:
//
//   view must equal ViewService — anything else (including empty)
//     returns ErrInvalidView.
//   id must be non-empty — otherwise ErrInvalidID.
//   depth must be >= 0 — otherwise ErrInvalidDepth. depth > MaxDepth
//     is silently clamped to MaxDepth (no Truncated signal; the cap
//     is documented in the Phase 1 contract).
//
// On a not-found root the underlying governance-map reader returns
// (nil, nil); Project converts that to ErrNotFound so the handler
// can map it to 404, matching the existing
// /v1/businessservices/{id}/governance-map 404 path
// (governance_map_handler.go: gmap == nil → 404).
func (s *Service) Project(ctx context.Context, view, id string, depth int) (*Projection, error) {
	if view != ViewService {
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
	if s.governanceMap == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	gmap, err := s.governanceMap.GetGovernanceMap(ctx, id)
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
