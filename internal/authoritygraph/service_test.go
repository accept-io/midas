package authoritygraph

import (
	"context"
	"errors"
	"testing"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/governancemap"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

// stubReader satisfies GovernanceMapReader for unit tests. The map is
// keyed by id; missing ids return (nil, nil), which Service.Project
// converts to ErrNotFound.
type stubReader struct {
	items map[string]*governancemap.Map
}

func (r *stubReader) GetGovernanceMap(_ context.Context, id string) (*governancemap.Map, error) {
	return r.items[id], nil
}

// makeBSMap builds a minimal *governancemap.Map with non-nil collection
// fields and a labelled root BS. Tests append to its slices to add
// entities without re-stating the boilerplate.
func makeBSMap(bsID, name string) *governancemap.Map {
	return &governancemap.Map{
		BusinessService: &governancemap.BusinessServiceNode{
			BusinessService: &businessservice.BusinessService{ID: bsID, Name: name},
		},
		Relationships: governancemap.Relationships{
			Outgoing: []*governancemap.RelationshipNode{},
			Incoming: []*governancemap.RelationshipNode{},
		},
		Capabilities:     []*governancemap.CapabilityNode{},
		Processes:        []*governancemap.ProcessNode{},
		Surfaces:         []*governancemap.SurfaceNode{},
		AISystems:        []*governancemap.AISystemNode{},
		AuthoritySummary: &governancemap.AuthoritySummary{},
		Coverage:         &governancemap.Coverage{},
	}
}

// makeFullDemoMap constructs a single-BS Map exercising every Phase 1
// node kind that has source data and every Phase 1 edge kind whose
// source data is present. Phase 2A also exercises typed-data fields
// — every entity here carries a non-zero value for fields the typed
// data should propagate (Status, Description, Owner, etc.).
func makeFullDemoMap() *governancemap.Map {
	m := makeBSMap("bs-1", "BS One")
	// Enrich the root BS so typed-data assertions can read non-zero
	// status / owner / description / service_type / regulatory_scope.
	m.BusinessService.BusinessService.Description = "Root demo service"
	m.BusinessService.BusinessService.Status = "active"
	m.BusinessService.BusinessService.OwnerID = "owner-bs"
	m.BusinessService.BusinessService.ServiceType = businessservice.ServiceTypeCustomerFacing
	m.BusinessService.BusinessService.RegulatoryScope = "GDPR,SOX"
	m.Relationships.Outgoing = []*governancemap.RelationshipNode{
		{
			Relationship: &businessservice.BusinessServiceRelationship{
				ID: "rel-1", SourceBusinessService: "bs-1", TargetBusinessService: "bs-2",
				RelationshipType: "depends_on",
			},
			OtherName: "BS Two",
		},
	}
	m.Capabilities = []*governancemap.CapabilityNode{
		{Capability: &capability.Capability{
			ID: "cap-1", Name: "Capability One",
			Description: "Cap desc", Status: "active", Owner: "owner-cap",
			ParentCapabilityID: "cap-parent",
		}},
	}
	m.Processes = []*governancemap.ProcessNode{
		{Process: &process.Process{
			ID: "proc-1", Name: "Process One",
			BusinessServiceID: "bs-1",
			Description:       "Proc desc", Status: "active", Owner: "owner-proc",
		}},
	}
	m.Surfaces = []*governancemap.SurfaceNode{
		{
			Surface: &surface.DecisionSurface{
				ID: "surf-1", Name: "Surface One", ProcessID: "proc-1",
				Version: 2, Description: "Surf desc",
				Status: surface.SurfaceStatusActive,
			},
			AIBindingIDs:          []string{"bind-1"},
			InheritedAIBindingIDs: []string{},
			ProfileCount:          3,
			GrantCount:            2,
			AgentCount:            1,
		},
	}
	v := 7
	m.AISystems = []*governancemap.AISystemNode{
		{
			System: &aisystem.AISystem{
				ID: "ai-1", Name: "AI One",
				Description: "AI desc", Status: "active",
				Vendor: "acme", SystemType: "model",
			},
			ActiveVersion: &aisystem.AISystemVersion{
				AISystemID:   "ai-1",
				Version:      v,
				ReleaseLabel: "v7-release",
				Status:       "active",
			},
			Bindings: []*aisystem.AISystemBinding{
				{
					ID: "bind-1", AISystemID: "ai-1", SurfaceID: "surf-1", ProcessID: "proc-1",
					Role:        "primary",
					Description: "Primary scoring binding",
				},
			},
		},
	}
	m.AuthoritySummary = &governancemap.AuthoritySummary{
		SurfaceCount:       1,
		ActiveProfileCount: 3,
		ActiveGrantCount:   2,
		ActiveAgentCount:   1,
	}
	m.Coverage = &governancemap.Coverage{
		SurfaceCount:                1,
		SurfacesWithDirectAIBinding: 1,
		SurfacesWithScopedAIBinding: 0,
		SurfacesWithNoAIBinding:     0,
	}
	return m
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func TestService_UnsupportedView_ReturnsErrInvalidView(t *testing.T) {
	svc := NewService(&stubReader{})
	_, err := svc.Project(context.Background(), "agent", "bs-1", 3)
	if !errors.Is(err, ErrInvalidView) {
		t.Errorf("want ErrInvalidView, got %v", err)
	}
}

func TestService_EmptyView_ReturnsErrInvalidView(t *testing.T) {
	svc := NewService(&stubReader{})
	_, err := svc.Project(context.Background(), "", "bs-1", 3)
	if !errors.Is(err, ErrInvalidView) {
		t.Errorf("want ErrInvalidView for empty view, got %v", err)
	}
}

func TestService_EmptyID_ReturnsErrInvalidID(t *testing.T) {
	svc := NewService(&stubReader{})
	_, err := svc.Project(context.Background(), ViewService, "", 3)
	if !errors.Is(err, ErrInvalidID) {
		t.Errorf("want ErrInvalidID, got %v", err)
	}
}

func TestService_NegativeDepth_ReturnsErrInvalidDepth(t *testing.T) {
	svc := NewService(&stubReader{})
	_, err := svc.Project(context.Background(), ViewService, "bs-1", -1)
	if !errors.Is(err, ErrInvalidDepth) {
		t.Errorf("want ErrInvalidDepth, got %v", err)
	}
}

func TestService_NotFoundID_ReturnsErrNotFound(t *testing.T) {
	svc := NewService(&stubReader{items: map[string]*governancemap.Map{}})
	_, err := svc.Project(context.Background(), ViewService, "bs-missing", 3)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Depth semantics
// ---------------------------------------------------------------------------

// TestService_DepthZero_RootOnly pins that depth=0 returns the root
// node alone with no edges, regardless of how rich the underlying Map
// is.
func TestService_DepthZero_RootOnly(t *testing.T) {
	m := makeFullDemoMap()
	svc := NewService(&stubReader{items: map[string]*governancemap.Map{"bs-1": m}})
	p, err := svc.Project(context.Background(), ViewService, "bs-1", 0)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(p.Nodes) != 1 {
		t.Errorf("depth=0 must have exactly one node (the root); got %d (%+v)", len(p.Nodes), p.Nodes)
	}
	if len(p.Nodes) == 1 && (p.Nodes[0].Kind != NodeKindBusinessService || p.Nodes[0].ID != "bs-1") {
		t.Errorf("depth=0 node must be the root BS; got %+v", p.Nodes[0])
	}
	if len(p.Edges) != 0 {
		t.Errorf("depth=0 must have no edges; got %+v", p.Edges)
	}
	if p.Depth != 0 {
		t.Errorf("p.Depth: want 0, got %d", p.Depth)
	}
}

// TestService_DepthOne_DirectNeighboursOfRoot pins the BFS contract at
// depth=1: the root plus every undirected 1-hop neighbour. No surfaces
// (which are 2-hop via process), no bindings (which are 2-hop via
// scope), no AI systems (which are 3-hop via binding).
func TestService_DepthOne_DirectNeighboursOfRoot(t *testing.T) {
	m := makeFullDemoMap()
	svc := NewService(&stubReader{items: map[string]*governancemap.Map{"bs-1": m}})
	p, err := svc.Project(context.Background(), ViewService, "bs-1", 1)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	got := map[string]string{}
	for _, n := range p.Nodes {
		got[n.Kind+":"+n.ID] = n.Label
	}
	for _, want := range []string{
		"business_service:bs-1",
		"related_business_service:bs-2",
		"capability:cap-1",
		"process:proc-1",
		"authority_summary:authority_summary:bs-1",
		"coverage:coverage:bs-1",
	} {
		if _, ok := got[want]; !ok {
			t.Errorf("depth=1 missing expected node %q (got: %v)", want, got)
		}
	}
	for _, illegal := range []string{
		"decision_surface:surf-1",  // 2 hops via proc-1
		"ai_system_binding:bind-1", // 3 hops via surf-1
		"ai_system:ai-1",           // 4 hops via bind-1
	} {
		if _, ok := got[illegal]; ok {
			t.Errorf("depth=1 must NOT include %q (got: %v)", illegal, got)
		}
	}
}

// TestService_DepthClamp_LargeIsSameAsMax pins that any depth above
// MaxDepth produces the same projection as MaxDepth, and that the
// projection's reported Depth field is the clamped value.
func TestService_DepthClamp_LargeIsSameAsMax(t *testing.T) {
	m := makeFullDemoMap()
	svc := NewService(&stubReader{items: map[string]*governancemap.Map{"bs-1": m}})
	pBig, err := svc.Project(context.Background(), ViewService, "bs-1", 999)
	if err != nil {
		t.Fatalf("big depth: %v", err)
	}
	pMax, err := svc.Project(context.Background(), ViewService, "bs-1", MaxDepth)
	if err != nil {
		t.Fatalf("max depth: %v", err)
	}
	if pBig.Depth != MaxDepth {
		t.Errorf("clamp: want Depth=%d, got %d", MaxDepth, pBig.Depth)
	}
	if len(pBig.Nodes) != len(pMax.Nodes) {
		t.Errorf("clamp: node count differs (big=%d, max=%d)", len(pBig.Nodes), len(pMax.Nodes))
	}
	if len(pBig.Edges) != len(pMax.Edges) {
		t.Errorf("clamp: edge count differs (big=%d, max=%d)", len(pBig.Edges), len(pMax.Edges))
	}
}

// ---------------------------------------------------------------------------
// Full demo — every Phase 1 kind with data, every Phase 1 edge with data
// ---------------------------------------------------------------------------

// TestService_FullDemo_AllKindsAndEdges is the headline coverage test:
// at MaxDepth every Phase 1 node and edge kind that has source data in
// the Map appears exactly once with the documented directionality, and
// no forbidden kinds appear.
func TestService_FullDemo_AllKindsAndEdges(t *testing.T) {
	m := makeFullDemoMap()
	svc := NewService(&stubReader{items: map[string]*governancemap.Map{"bs-1": m}})
	p, err := svc.Project(context.Background(), ViewService, "bs-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	nodeCount := map[string]int{}
	for _, n := range p.Nodes {
		nodeCount[n.Kind]++
	}
	wantNodeCounts := map[string]int{
		NodeKindBusinessService:        1,
		NodeKindRelatedBusinessService: 1,
		NodeKindCapability:             1,
		NodeKindProcess:                1,
		NodeKindDecisionSurface:        1,
		NodeKindAISystem:               1,
		NodeKindAISystemBinding:        1,
		NodeKindAuthoritySummary:       1,
		NodeKindCoverage:               1,
	}
	for kind, want := range wantNodeCounts {
		if got := nodeCount[kind]; got != want {
			t.Errorf("node kind %q: want %d, got %d (all=%v)", kind, want, got, nodeCount)
		}
	}

	edgeCount := map[string]int{}
	for _, e := range p.Edges {
		edgeCount[e.Kind]++
	}
	wantEdgeCounts := map[string]int{
		EdgeKindRelatesTo:       1,
		EdgeKindHasCapability:   1,
		EdgeKindHasProcess:      1,
		EdgeKindHasSurface:      1,
		EdgeKindBoundTo:         1,
		EdgeKindSystemOf:        1,
		EdgeKindSummarises:      1,
		EdgeKindReportsCoverage: 1,
	}
	for kind, want := range wantEdgeCounts {
		if got := edgeCount[kind]; got != want {
			t.Errorf("edge kind %q: want %d, got %d (all=%v)", kind, want, got, edgeCount)
		}
	}

	// Forbidden Phase 1 kinds — none of these may appear.
	for _, k := range []string{"authority_profile", "authority_grant", "agent", "ai_system_version"} {
		if nodeCount[k] != 0 {
			t.Errorf("forbidden node kind %q present (%d)", k, nodeCount[k])
		}
	}
	for _, k := range []string{"governed_by", "has_grant", "granted_to", "has_active_version"} {
		if edgeCount[k] != 0 {
			t.Errorf("forbidden edge kind %q present (%d)", k, edgeCount[k])
		}
	}

	// Edge directionality pins.
	wantDirections := []struct {
		kind    string
		srcKind string
		srcID   string
		dstKind string
		dstID   string
	}{
		{EdgeKindRelatesTo, NodeKindBusinessService, "bs-1", NodeKindRelatedBusinessService, "bs-2"},
		{EdgeKindHasCapability, NodeKindBusinessService, "bs-1", NodeKindCapability, "cap-1"},
		{EdgeKindHasProcess, NodeKindBusinessService, "bs-1", NodeKindProcess, "proc-1"},
		{EdgeKindHasSurface, NodeKindProcess, "proc-1", NodeKindDecisionSurface, "surf-1"},
		{EdgeKindBoundTo, NodeKindAISystemBinding, "bind-1", NodeKindDecisionSurface, "surf-1"},
		{EdgeKindSystemOf, NodeKindAISystemBinding, "bind-1", NodeKindAISystem, "ai-1"},
		{EdgeKindSummarises, NodeKindAuthoritySummary, "authority_summary:bs-1", NodeKindBusinessService, "bs-1"},
		{EdgeKindReportsCoverage, NodeKindCoverage, "coverage:bs-1", NodeKindBusinessService, "bs-1"},
	}
	for _, want := range wantDirections {
		found := false
		for _, e := range p.Edges {
			if e.Kind == want.kind &&
				e.Src.Kind == want.srcKind && e.Src.ID == want.srcID &&
				e.Dst.Kind == want.dstKind && e.Dst.ID == want.dstID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing edge %+v\nall edges: %+v", want, p.Edges)
		}
	}

	// Binding label format — pinned literal.
	for _, n := range p.Nodes {
		if n.Kind != NodeKindAISystemBinding {
			continue
		}
		const want = "binding: AI One → surface:surf-1"
		if n.Label != want {
			t.Errorf("binding label: want %q, got %q", want, n.Label)
		}
	}

	// Synthetic node ids and labels.
	for _, n := range p.Nodes {
		switch n.Kind {
		case NodeKindAuthoritySummary:
			if n.ID != "authority_summary:bs-1" {
				t.Errorf("authority_summary id: want %q, got %q", "authority_summary:bs-1", n.ID)
			}
			if n.Label != "Authority Summary" {
				t.Errorf("authority_summary label: want %q, got %q", "Authority Summary", n.Label)
			}
		case NodeKindCoverage:
			if n.ID != "coverage:bs-1" {
				t.Errorf("coverage id: want %q, got %q", "coverage:bs-1", n.ID)
			}
			if n.Label != "AI Binding Coverage" {
				t.Errorf("coverage label: want %q, got %q", "AI Binding Coverage", n.Label)
			}
		}
	}

	// Sort stability — nodes by (Kind, ID) ascending.
	for i := 1; i < len(p.Nodes); i++ {
		a, b := p.Nodes[i-1], p.Nodes[i]
		if a.Kind > b.Kind {
			t.Errorf("node sort: kinds out of order at %d: %q before %q", i, a.Kind, b.Kind)
		} else if a.Kind == b.Kind && a.ID > b.ID {
			t.Errorf("node sort: ids out of order at %d (kind=%s): %q before %q", i, a.Kind, a.ID, b.ID)
		}
	}

	// Sort stability — edges by (Kind, Src.Kind, Src.ID, Dst.Kind, Dst.ID).
	for i := 1; i < len(p.Edges); i++ {
		a, b := p.Edges[i-1], p.Edges[i]
		switch {
		case a.Kind > b.Kind:
			t.Errorf("edge sort: kinds out of order at %d", i)
		case a.Kind == b.Kind && a.Src.Kind > b.Src.Kind:
			t.Errorf("edge sort: src kinds out of order at %d", i)
		case a.Kind == b.Kind && a.Src.Kind == b.Src.Kind && a.Src.ID > b.Src.ID:
			t.Errorf("edge sort: src ids out of order at %d", i)
		case a.Kind == b.Kind && a.Src.Kind == b.Src.Kind && a.Src.ID == b.Src.ID && a.Dst.Kind > b.Dst.Kind:
			t.Errorf("edge sort: dst kinds out of order at %d", i)
		case a.Kind == b.Kind && a.Src.Kind == b.Src.Kind && a.Src.ID == b.Src.ID && a.Dst.Kind == b.Dst.Kind && a.Dst.ID > b.Dst.ID:
			t.Errorf("edge sort: dst ids out of order at %d", i)
		}
	}

	if p.Depth != MaxDepth {
		t.Errorf("p.Depth: want %d, got %d", MaxDepth, p.Depth)
	}
	if p.View != ViewService {
		t.Errorf("p.View: want %q, got %q", ViewService, p.View)
	}
	if p.Root.Kind != NodeKindBusinessService || p.Root.ID != "bs-1" {
		t.Errorf("p.Root: want (%s, bs-1), got (%s, %s)", NodeKindBusinessService, p.Root.Kind, p.Root.ID)
	}
}

// TestService_FullDemo_TypedDataPopulated exercises Phase 2A typed
// per-kind data on the same fully-populated stub fixture as
// TestService_FullDemo_AllKindsAndEdges. It pins:
//
//   - every node kind has its matching typed-data slot populated
//     (and exactly that one — exclusivity is verified by the seeded
//     test in service_seeded_test.go for the seeded path)
//   - field values mirror the fixture verbatim (status, description,
//     owner, version, scope_*, counts)
//   - the binding's typed scope data matches the most-specific scope
//     (surface) per the bound_to / label rules
//   - synthetic authority_summary and coverage nodes carry the four
//     count fields the governance-map DTOs surface
func TestService_FullDemo_TypedDataPopulated(t *testing.T) {
	m := makeFullDemoMap()
	svc := NewService(&stubReader{items: map[string]*governancemap.Map{"bs-1": m}})
	p, err := svc.Project(context.Background(), ViewService, "bs-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	byID := map[string]*Node{}
	for i := range p.Nodes {
		n := &p.Nodes[i]
		byID[n.Kind+"|"+n.ID] = n
	}

	// business_service
	bs := byID[NodeKindBusinessService+"|bs-1"]
	if bs == nil || bs.BusinessService == nil {
		t.Fatalf("business_service typed data missing")
	}
	if got, want := bs.BusinessService.ID, "bs-1"; got != want {
		t.Errorf("bs id: want %q, got %q", want, got)
	}
	if got, want := bs.BusinessService.Name, "BS One"; got != want {
		t.Errorf("bs name: want %q, got %q", want, got)
	}
	if got, want := bs.BusinessService.Description, "Root demo service"; got != want {
		t.Errorf("bs description: want %q, got %q", want, got)
	}
	if got, want := bs.BusinessService.Status, "active"; got != want {
		t.Errorf("bs status: want %q, got %q", want, got)
	}
	if got, want := bs.BusinessService.Owner, "owner-bs"; got != want {
		t.Errorf("bs owner: want %q, got %q", want, got)
	}
	if got, want := bs.BusinessService.ServiceType, string(businessservice.ServiceTypeCustomerFacing); got != want {
		t.Errorf("bs service_type: want %q, got %q", want, got)
	}
	if got, want := bs.BusinessService.RegulatoryScope, "GDPR,SOX"; got != want {
		t.Errorf("bs regulatory_scope: want %q, got %q", want, got)
	}

	// related_business_service
	rel := byID[NodeKindRelatedBusinessService+"|bs-2"]
	if rel == nil || rel.RelatedBusinessService == nil {
		t.Fatalf("related_business_service typed data missing")
	}
	if got, want := rel.RelatedBusinessService.Name, "BS Two"; got != want {
		t.Errorf("rel name: want %q, got %q", want, got)
	}
	if rel.RelatedBusinessService.Outgoing == nil {
		t.Fatalf("rel outgoing sub-row must be populated for the seeded outgoing relationship")
	}
	if got, want := rel.RelatedBusinessService.Outgoing.RelationshipType, "depends_on"; got != want {
		t.Errorf("rel outgoing type: want %q, got %q", want, got)
	}
	if got, want := rel.RelatedBusinessService.Outgoing.RelationshipID, "rel-1"; got != want {
		t.Errorf("rel outgoing relationship_id: want %q, got %q", want, got)
	}
	if rel.RelatedBusinessService.Incoming != nil {
		t.Errorf("rel incoming sub-row must be nil for an outgoing-only seed; got %+v", rel.RelatedBusinessService.Incoming)
	}

	// capability
	cap := byID[NodeKindCapability+"|cap-1"]
	if cap == nil || cap.Capability == nil {
		t.Fatalf("capability typed data missing")
	}
	if got, want := cap.Capability.Name, "Capability One"; got != want {
		t.Errorf("cap name: want %q, got %q", want, got)
	}
	if got, want := cap.Capability.Description, "Cap desc"; got != want {
		t.Errorf("cap description: want %q, got %q", want, got)
	}
	if got, want := cap.Capability.Status, "active"; got != want {
		t.Errorf("cap status: want %q, got %q", want, got)
	}
	if got, want := cap.Capability.Owner, "owner-cap"; got != want {
		t.Errorf("cap owner: want %q, got %q", want, got)
	}
	if got, want := cap.Capability.ParentCapabilityID, "cap-parent"; got != want {
		t.Errorf("cap parent: want %q, got %q", want, got)
	}

	// process
	proc := byID[NodeKindProcess+"|proc-1"]
	if proc == nil || proc.Process == nil {
		t.Fatalf("process typed data missing")
	}
	if got, want := proc.Process.Name, "Process One"; got != want {
		t.Errorf("proc name: want %q, got %q", want, got)
	}
	if got, want := proc.Process.Description, "Proc desc"; got != want {
		t.Errorf("proc description: want %q, got %q", want, got)
	}
	if got, want := proc.Process.Status, "active"; got != want {
		t.Errorf("proc status: want %q, got %q", want, got)
	}
	if got, want := proc.Process.Owner, "owner-proc"; got != want {
		t.Errorf("proc owner: want %q, got %q", want, got)
	}
	if got, want := proc.Process.BusinessServiceID, "bs-1"; got != want {
		t.Errorf("proc bs_id: want %q, got %q", want, got)
	}

	// decision_surface
	surf := byID[NodeKindDecisionSurface+"|surf-1"]
	if surf == nil || surf.DecisionSurface == nil {
		t.Fatalf("decision_surface typed data missing")
	}
	if got, want := surf.DecisionSurface.Version, 2; got != want {
		t.Errorf("surf version: want %d, got %d", want, got)
	}
	if got, want := surf.DecisionSurface.Description, "Surf desc"; got != want {
		t.Errorf("surf description: want %q, got %q", want, got)
	}
	if got, want := surf.DecisionSurface.Status, "active"; got != want {
		t.Errorf("surf status: want %q, got %q", want, got)
	}
	if got, want := surf.DecisionSurface.ProcessID, "proc-1"; got != want {
		t.Errorf("surf process_id: want %q, got %q", want, got)
	}
	if got := surf.DecisionSurface.AIBindingIDs; len(got) != 1 || got[0] != "bind-1" {
		t.Errorf("surf ai_binding_ids: want [bind-1], got %v", got)
	}
	if got := surf.DecisionSurface.InheritedAIBindingIDs; len(got) != 0 {
		t.Errorf("surf inherited_ai_binding_ids: want empty, got %v", got)
	}
	if got, want := surf.DecisionSurface.ProfileCount, 3; got != want {
		t.Errorf("surf profile_count: want %d, got %d", want, got)
	}
	if got, want := surf.DecisionSurface.GrantCount, 2; got != want {
		t.Errorf("surf grant_count: want %d, got %d", want, got)
	}
	if got, want := surf.DecisionSurface.AgentCount, 1; got != want {
		t.Errorf("surf agent_count: want %d, got %d", want, got)
	}

	// ai_system
	ai := byID[NodeKindAISystem+"|ai-1"]
	if ai == nil || ai.AISystem == nil {
		t.Fatalf("ai_system typed data missing")
	}
	if got, want := ai.AISystem.Name, "AI One"; got != want {
		t.Errorf("ai name: want %q, got %q", want, got)
	}
	if got, want := ai.AISystem.Description, "AI desc"; got != want {
		t.Errorf("ai description: want %q, got %q", want, got)
	}
	if got, want := ai.AISystem.Status, "active"; got != want {
		t.Errorf("ai status: want %q, got %q", want, got)
	}
	if got, want := ai.AISystem.Vendor, "acme"; got != want {
		t.Errorf("ai vendor: want %q, got %q", want, got)
	}
	if got, want := ai.AISystem.SystemType, "model"; got != want {
		t.Errorf("ai system_type: want %q, got %q", want, got)
	}
	if ai.AISystem.ActiveVersion == nil || *ai.AISystem.ActiveVersion != 7 {
		t.Errorf("ai active_version: want 7, got %v", ai.AISystem.ActiveVersion)
	}
	if got, want := ai.AISystem.ActiveVersionLabel, "v7-release"; got != want {
		t.Errorf("ai active_version_label: want %q, got %q", want, got)
	}
	if got, want := ai.AISystem.ActiveVersionStatus, "active"; got != want {
		t.Errorf("ai active_version_status: want %q, got %q", want, got)
	}

	// ai_system_binding
	bind := byID[NodeKindAISystemBinding+"|bind-1"]
	if bind == nil || bind.AISystemBinding == nil {
		t.Fatalf("ai_system_binding typed data missing")
	}
	bd := bind.AISystemBinding
	if got, want := bd.AISystemID, "ai-1"; got != want {
		t.Errorf("binding ai_system_id: want %q, got %q", want, got)
	}
	if got, want := bd.AISystemName, "AI One"; got != want {
		t.Errorf("binding ai_system_name: want %q, got %q", want, got)
	}
	if bd.SurfaceID == nil || *bd.SurfaceID != "surf-1" {
		t.Errorf("binding surface_id: want pointer to surf-1, got %v", bd.SurfaceID)
	}
	if bd.ProcessID == nil || *bd.ProcessID != "proc-1" {
		t.Errorf("binding process_id: want pointer to proc-1, got %v", bd.ProcessID)
	}
	if bd.CapabilityID != nil {
		t.Errorf("binding capability_id: want nil, got %v", bd.CapabilityID)
	}
	if bd.BusinessServiceID != nil {
		t.Errorf("binding business_service_id: want nil, got %v", bd.BusinessServiceID)
	}
	// Most-specific scope is surface (precedence: surface > process > capability > BS).
	if got, want := bd.ScopeKind, NodeKindDecisionSurface; got != want {
		t.Errorf("binding scope_kind: want %q, got %q", want, got)
	}
	if got, want := bd.ScopeID, "surf-1"; got != want {
		t.Errorf("binding scope_id: want %q, got %q", want, got)
	}
	if got, want := bd.ScopeLabel, "surface:surf-1"; got != want {
		t.Errorf("binding scope_label: want %q, got %q", want, got)
	}
	if got, want := bd.Role, "primary"; got != want {
		t.Errorf("binding role: want %q, got %q", want, got)
	}
	if got, want := bd.Description, "Primary scoring binding"; got != want {
		t.Errorf("binding description: want %q, got %q", want, got)
	}

	// authority_summary
	sum := byID[NodeKindAuthoritySummary+"|authority_summary:bs-1"]
	if sum == nil || sum.AuthoritySummary == nil {
		t.Fatalf("authority_summary typed data missing")
	}
	if sum.AuthoritySummary.SurfaceCount != 1 ||
		sum.AuthoritySummary.ActiveProfileCount != 3 ||
		sum.AuthoritySummary.ActiveGrantCount != 2 ||
		sum.AuthoritySummary.ActiveAgentCount != 1 {
		t.Errorf("authority_summary counts: %+v", sum.AuthoritySummary)
	}

	// coverage
	cov := byID[NodeKindCoverage+"|coverage:bs-1"]
	if cov == nil || cov.Coverage == nil {
		t.Fatalf("coverage typed data missing")
	}
	if cov.Coverage.SurfaceCount != 1 ||
		cov.Coverage.SurfacesWithDirectAIBinding != 1 ||
		cov.Coverage.SurfacesWithScopedAIBinding != 0 ||
		cov.Coverage.SurfacesWithNoAIBinding != 0 {
		t.Errorf("coverage counts: %+v", cov.Coverage)
	}
}

// TestService_RelatedBusinessService_OutgoingOnly pins the projection
// of an outgoing-only relationship: one related node, one relates_to
// edge in the root → related direction, typed data carries the
// Outgoing sub-row only.
func TestService_RelatedBusinessService_OutgoingOnly(t *testing.T) {
	m := makeBSMap("bs-root", "Root")
	m.Relationships.Outgoing = []*governancemap.RelationshipNode{
		{
			Relationship: &businessservice.BusinessServiceRelationship{
				ID: "rel-out", SourceBusinessService: "bs-root", TargetBusinessService: "bs-other",
				RelationshipType: "depends_on", Description: "Out desc",
			},
			OtherName: "Other BS",
		},
	}
	svc := NewService(&stubReader{items: map[string]*governancemap.Map{"bs-root": m}})
	p, err := svc.Project(context.Background(), ViewService, "bs-root", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	var related *Node
	relatesToEdges := []Edge{}
	for i := range p.Nodes {
		n := &p.Nodes[i]
		if n.Kind == NodeKindRelatedBusinessService && n.ID == "bs-other" {
			related = n
		}
	}
	for _, e := range p.Edges {
		if e.Kind == EdgeKindRelatesTo {
			relatesToEdges = append(relatesToEdges, e)
		}
	}
	if related == nil || related.RelatedBusinessService == nil {
		t.Fatalf("related_business_service node missing or has nil typed data")
	}
	if related.RelatedBusinessService.Outgoing == nil {
		t.Errorf("outgoing sub-row missing")
	} else if related.RelatedBusinessService.Outgoing.RelationshipID != "rel-out" {
		t.Errorf("outgoing relationship_id: want rel-out, got %q", related.RelatedBusinessService.Outgoing.RelationshipID)
	}
	if related.RelatedBusinessService.Incoming != nil {
		t.Errorf("incoming sub-row must be nil; got %+v", related.RelatedBusinessService.Incoming)
	}
	if len(relatesToEdges) != 1 {
		t.Fatalf("want 1 relates_to edge, got %d (%+v)", len(relatesToEdges), relatesToEdges)
	}
	got := relatesToEdges[0]
	if got.Src.Kind != NodeKindBusinessService || got.Src.ID != "bs-root" ||
		got.Dst.Kind != NodeKindRelatedBusinessService || got.Dst.ID != "bs-other" {
		t.Errorf("outgoing edge endpoints: want bs-root → bs-other, got %+v", got)
	}
}

// TestService_RelatedBusinessService_IncomingOnly pins the projection
// of an incoming-only relationship: one related node, one relates_to
// edge in the related → root direction, typed data carries the
// Incoming sub-row only.
func TestService_RelatedBusinessService_IncomingOnly(t *testing.T) {
	m := makeBSMap("bs-root", "Root")
	m.Relationships.Incoming = []*governancemap.RelationshipNode{
		{
			Relationship: &businessservice.BusinessServiceRelationship{
				ID: "rel-in", SourceBusinessService: "bs-other", TargetBusinessService: "bs-root",
				RelationshipType: "supports", Description: "In desc",
			},
			OtherName: "Other BS",
		},
	}
	svc := NewService(&stubReader{items: map[string]*governancemap.Map{"bs-root": m}})
	p, err := svc.Project(context.Background(), ViewService, "bs-root", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	var related *Node
	relatesToEdges := []Edge{}
	for i := range p.Nodes {
		n := &p.Nodes[i]
		if n.Kind == NodeKindRelatedBusinessService && n.ID == "bs-other" {
			related = n
		}
	}
	for _, e := range p.Edges {
		if e.Kind == EdgeKindRelatesTo {
			relatesToEdges = append(relatesToEdges, e)
		}
	}
	if related == nil || related.RelatedBusinessService == nil {
		t.Fatalf("related_business_service node missing or has nil typed data")
	}
	if related.RelatedBusinessService.Incoming == nil {
		t.Errorf("incoming sub-row missing")
	} else if related.RelatedBusinessService.Incoming.RelationshipID != "rel-in" {
		t.Errorf("incoming relationship_id: want rel-in, got %q", related.RelatedBusinessService.Incoming.RelationshipID)
	}
	if related.RelatedBusinessService.Outgoing != nil {
		t.Errorf("outgoing sub-row must be nil; got %+v", related.RelatedBusinessService.Outgoing)
	}
	if len(relatesToEdges) != 1 {
		t.Fatalf("want 1 relates_to edge, got %d (%+v)", len(relatesToEdges), relatesToEdges)
	}
	got := relatesToEdges[0]
	if got.Src.Kind != NodeKindRelatedBusinessService || got.Src.ID != "bs-other" ||
		got.Dst.Kind != NodeKindBusinessService || got.Dst.ID != "bs-root" {
		t.Errorf("incoming edge endpoints: want bs-other → bs-root, got %+v", got)
	}
}

// TestService_RelatedBusinessService_BothDirections pins the
// same-BS-both-directions invariant: one related node, two
// relates_to edges (one each way), and BOTH Outgoing + Incoming
// sub-rows populated with their distinct relationship row data.
// This is the case the projection's one-node-per-(kind,id)
// invariant exists to handle losslessly.
func TestService_RelatedBusinessService_BothDirections(t *testing.T) {
	m := makeBSMap("bs-root", "Root")
	m.Relationships.Outgoing = []*governancemap.RelationshipNode{
		{
			Relationship: &businessservice.BusinessServiceRelationship{
				ID: "rel-out", SourceBusinessService: "bs-root", TargetBusinessService: "bs-other",
				RelationshipType: "depends_on", Description: "Out desc",
			},
			OtherName: "Other BS",
		},
	}
	m.Relationships.Incoming = []*governancemap.RelationshipNode{
		{
			Relationship: &businessservice.BusinessServiceRelationship{
				ID: "rel-in", SourceBusinessService: "bs-other", TargetBusinessService: "bs-root",
				RelationshipType: "supports", Description: "In desc",
			},
			OtherName: "Other BS",
		},
	}
	svc := NewService(&stubReader{items: map[string]*governancemap.Map{"bs-root": m}})
	p, err := svc.Project(context.Background(), ViewService, "bs-root", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	relatedCount := 0
	var related *Node
	for i := range p.Nodes {
		n := &p.Nodes[i]
		if n.Kind == NodeKindRelatedBusinessService && n.ID == "bs-other" {
			relatedCount++
			related = n
		}
	}
	if relatedCount != 1 {
		t.Fatalf("same-BS-both-directions must produce exactly 1 related node, got %d", relatedCount)
	}
	if related.RelatedBusinessService.Outgoing == nil || related.RelatedBusinessService.Incoming == nil {
		t.Fatalf("both sub-rows must be populated; got Outgoing=%+v Incoming=%+v",
			related.RelatedBusinessService.Outgoing, related.RelatedBusinessService.Incoming)
	}
	if got := related.RelatedBusinessService.Outgoing; got.RelationshipID != "rel-out" || got.RelationshipType != "depends_on" || got.Description != "Out desc" {
		t.Errorf("outgoing sub-row content: %+v", got)
	}
	if got := related.RelatedBusinessService.Incoming; got.RelationshipID != "rel-in" || got.RelationshipType != "supports" || got.Description != "In desc" {
		t.Errorf("incoming sub-row content: %+v", got)
	}

	relatesToEdges := []Edge{}
	for _, e := range p.Edges {
		if e.Kind == EdgeKindRelatesTo {
			relatesToEdges = append(relatesToEdges, e)
		}
	}
	if len(relatesToEdges) != 2 {
		t.Fatalf("want 2 relates_to edges (one each direction), got %d (%+v)", len(relatesToEdges), relatesToEdges)
	}
	// Verify both directions present (order is enforced by the
	// service's slice-level edge sort; assert by membership).
	wantDirs := map[string]bool{
		"bs-root|business_service→bs-other|related_business_service": false,
		"bs-other|related_business_service→bs-root|business_service": false,
	}
	for _, e := range relatesToEdges {
		key := e.Src.ID + "|" + e.Src.Kind + "→" + e.Dst.ID + "|" + e.Dst.Kind
		if _, ok := wantDirs[key]; ok {
			wantDirs[key] = true
		} else {
			t.Errorf("unexpected relates_to edge: %+v", e)
		}
	}
	for k, seen := range wantDirs {
		if !seen {
			t.Errorf("missing relates_to edge for direction: %s", k)
		}
	}
}

// TestService_AuthoritySummaryAndCoverage_AppearOnceEachAtDepth1 pins
// that the synthetic authority_summary and coverage nodes are direct
// neighbours of the root, with their summarises / reports_coverage
// edges, regardless of how empty the rest of the Map is.
func TestService_AuthoritySummaryAndCoverage_AppearOnceEachAtDepth1(t *testing.T) {
	m := makeBSMap("bs-1", "BS One")
	svc := NewService(&stubReader{items: map[string]*governancemap.Map{"bs-1": m}})
	p, err := svc.Project(context.Background(), ViewService, "bs-1", 1)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	var summaryNodes, coverageNodes int
	for _, n := range p.Nodes {
		switch n.Kind {
		case NodeKindAuthoritySummary:
			summaryNodes++
		case NodeKindCoverage:
			coverageNodes++
		}
	}
	if summaryNodes != 1 {
		t.Errorf("authority_summary nodes: want 1, got %d (nodes=%+v)", summaryNodes, p.Nodes)
	}
	if coverageNodes != 1 {
		t.Errorf("coverage nodes: want 1, got %d (nodes=%+v)", coverageNodes, p.Nodes)
	}

	var summarises, reports int
	for _, e := range p.Edges {
		switch e.Kind {
		case EdgeKindSummarises:
			summarises++
		case EdgeKindReportsCoverage:
			reports++
		}
	}
	if summarises != 1 {
		t.Errorf("summarises edges: want 1, got %d", summarises)
	}
	if reports != 1 {
		t.Errorf("reports_coverage edges: want 1, got %d", reports)
	}
}

// ---------------------------------------------------------------------------
// ParseDepth
// ---------------------------------------------------------------------------

func TestParseDepth_DefaultEmpty(t *testing.T) {
	n, err := ParseDepth("")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if n != DefaultDepth {
		t.Errorf("default: want %d, got %d", DefaultDepth, n)
	}
}

func TestParseDepth_NonNumeric_ReturnsErrInvalidDepth(t *testing.T) {
	_, err := ParseDepth("abc")
	if !errors.Is(err, ErrInvalidDepth) {
		t.Errorf("want ErrInvalidDepth, got %v", err)
	}
}

func TestParseDepth_Negative_ReturnsErrInvalidDepth(t *testing.T) {
	_, err := ParseDepth("-1")
	if !errors.Is(err, ErrInvalidDepth) {
		t.Errorf("want ErrInvalidDepth, got %v", err)
	}
}

func TestParseDepth_ClampLarge(t *testing.T) {
	n, err := ParseDepth("999")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if n != MaxDepth {
		t.Errorf("clamp: want %d, got %d", MaxDepth, n)
	}
}

func TestParseDepth_PassesThroughInsideRange(t *testing.T) {
	for _, raw := range []string{"0", "1", "3", "5"} {
		n, err := ParseDepth(raw)
		if err != nil {
			t.Errorf("ParseDepth(%q): %v", raw, err)
			continue
		}
		// Quick correctness check: the parsed value must equal the
		// numeric input within the allowed range.
		want := map[string]int{"0": 0, "1": 1, "3": 3, "5": 5}[raw]
		if n != want {
			t.Errorf("ParseDepth(%q): want %d, got %d", raw, want, n)
		}
	}
}
