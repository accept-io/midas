package authoritygraph

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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

// ---------------------------------------------------------------------------
// Stubs for ai_system view tests.
//
// Each stub satisfies one of the narrow reader interfaces declared in
// service.go. Missing-id behaviour: nil result, no error — matches the
// "missing scope target" contract the projector exercises.
// ---------------------------------------------------------------------------

type stubAISystemReader struct {
	items map[string]*aisystem.AISystem
}

func (r *stubAISystemReader) GetByID(_ context.Context, id string) (*aisystem.AISystem, error) {
	return r.items[id], nil
}

type stubBindingRepo struct {
	byAISystem map[string][]*aisystem.AISystemBinding
	bySurface  map[string][]*aisystem.AISystemBinding
}

func (r *stubBindingRepo) ListByAISystem(_ context.Context, id string) ([]*aisystem.AISystemBinding, error) {
	return r.byAISystem[id], nil
}

// ListBySurface satisfies the widened AISystemBindingRepository surface
// added for the decision_surface view. Tests that don't populate
// bySurface get nil/empty (the projection's ListBySurface call returns
// no bindings — equivalent to a surface with no AI bindings).
func (r *stubBindingRepo) ListBySurface(_ context.Context, id string) ([]*aisystem.AISystemBinding, error) {
	return r.bySurface[id], nil
}

type stubBSReader struct {
	items map[string]*businessservice.BusinessService
}

func (r *stubBSReader) GetByID(_ context.Context, id string) (*businessservice.BusinessService, error) {
	return r.items[id], nil
}

type stubCapReader struct {
	items map[string]*capability.Capability
}

func (r *stubCapReader) GetByID(_ context.Context, id string) (*capability.Capability, error) {
	return r.items[id], nil
}

type stubProcReader struct {
	items map[string]*process.Process
}

func (r *stubProcReader) GetByID(_ context.Context, id string) (*process.Process, error) {
	return r.items[id], nil
}

type stubSurfReader struct {
	items map[string]*surface.DecisionSurface
}

func (r *stubSurfReader) FindLatestByID(_ context.Context, id string) (*surface.DecisionSurface, error) {
	return r.items[id], nil
}

// aiViewStubs bundles the six stubs the ai_system view requires.
type aiViewStubs struct {
	systems  *stubAISystemReader
	bindings *stubBindingRepo
	bss      *stubBSReader
	caps     *stubCapReader
	procs    *stubProcReader
	surfs    *stubSurfReader
}

func newAIViewStubs() *aiViewStubs {
	return &aiViewStubs{
		systems: &stubAISystemReader{items: map[string]*aisystem.AISystem{}},
		bindings: &stubBindingRepo{
			byAISystem: map[string][]*aisystem.AISystemBinding{},
			bySurface:  map[string][]*aisystem.AISystemBinding{},
		},
		bss:   &stubBSReader{items: map[string]*businessservice.BusinessService{}},
		caps:  &stubCapReader{items: map[string]*capability.Capability{}},
		procs: &stubProcReader{items: map[string]*process.Process{}},
		surfs: &stubSurfReader{items: map[string]*surface.DecisionSurface{}},
	}
}

// readers builds the Readers struct for an ai_system-view-only Service
// (GovernanceMap left nil so view=service is unregistered, allowing
// the test to assert per-view registration semantics independently).
func (s *aiViewStubs) readers() Readers {
	return Readers{
		AISystem:         s.systems,
		AISystemBindings: s.bindings,
		BusinessServices: s.bss,
		Capabilities:     s.caps,
		Processes:        s.procs,
		Surfaces:         s.surfs,
	}
}

// readersWithGovMap returns Readers with both views configured.
func (s *aiViewStubs) readersWithGovMap(gmap GovernanceMapReader) Readers {
	r := s.readers()
	r.GovernanceMap = gmap
	return r
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
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{}})
	_, err := svc.Project(context.Background(), "agent", "bs-1", 3)
	if !errors.Is(err, ErrInvalidView) {
		t.Errorf("want ErrInvalidView, got %v", err)
	}
}

func TestService_EmptyView_ReturnsErrInvalidView(t *testing.T) {
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{}})
	_, err := svc.Project(context.Background(), "", "bs-1", 3)
	if !errors.Is(err, ErrInvalidView) {
		t.Errorf("want ErrInvalidView for empty view, got %v", err)
	}
}

func TestService_EmptyID_ReturnsErrInvalidID(t *testing.T) {
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{}})
	_, err := svc.Project(context.Background(), ViewService, "", 3)
	if !errors.Is(err, ErrInvalidID) {
		t.Errorf("want ErrInvalidID, got %v", err)
	}
}

func TestService_NegativeDepth_ReturnsErrInvalidDepth(t *testing.T) {
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{}})
	_, err := svc.Project(context.Background(), ViewService, "bs-1", -1)
	if !errors.Is(err, ErrInvalidDepth) {
		t.Errorf("want ErrInvalidDepth, got %v", err)
	}
}

func TestService_NotFoundID_ReturnsErrNotFound(t *testing.T) {
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{items: map[string]*governancemap.Map{}}})
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
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{items: map[string]*governancemap.Map{"bs-1": m}}})
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
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{items: map[string]*governancemap.Map{"bs-1": m}}})
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
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{items: map[string]*governancemap.Map{"bs-1": m}}})
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
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{items: map[string]*governancemap.Map{"bs-1": m}}})
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
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{items: map[string]*governancemap.Map{"bs-1": m}}})
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
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{items: map[string]*governancemap.Map{"bs-root": m}}})
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
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{items: map[string]*governancemap.Map{"bs-root": m}}})
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
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{items: map[string]*governancemap.Map{"bs-root": m}}})
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
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{items: map[string]*governancemap.Map{"bs-1": m}}})
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

// ===========================================================================
// AI System view (Phase 2B Step 9) — dispatch + projection content
// ===========================================================================
//
// Tests below cover the deliverable's contract:
//   - dispatch: Project routes view=ai_system to the new projector,
//     and unregistered views (or services constructed without
//     ai-system readers) return ErrInvalidView consistently
//   - projection content: scope-target resolution per binding scope
//     kind (surface / process / capability / business_service), node
//     and edge directionality, deduplication, missing-target tolerance
//   - depth: BFS from the ai_system root produces the documented hop
//     sets
//
// All stubs swallow errors and return nil for unknown ids — exercising
// the "missing scope target" graceful-degradation path is just adding
// a binding whose scope id is absent from the corresponding stub.

// ---------------------------------------------------------------------------
// A. Dispatch and validation
// ---------------------------------------------------------------------------

// TestService_Dispatch_AIViewWhenReadersMissing_ReturnsErrInvalidView
// pins that constructing a Service without the ai-system readers does
// NOT register the ai_system view; requests for it return
// ErrInvalidView (NOT ErrNotFound).
func TestService_Dispatch_AIViewWhenReadersMissing_ReturnsErrInvalidView(t *testing.T) {
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{}})
	_, err := svc.Project(context.Background(), ViewAISystem, "ai-1", 3)
	if !errors.Is(err, ErrInvalidView) {
		t.Errorf("ai_system view without readers must return ErrInvalidView; got %v", err)
	}
}

// TestService_Dispatch_AIViewRegisteredWithFullReaders pins the
// happy-path registration: NewServiceWithReaders with all six
// ai-system readers makes view=ai_system addressable.
func TestService_Dispatch_AIViewRegisteredWithFullReaders(t *testing.T) {
	stubs := newAIViewStubs()
	stubs.systems.items["ai-1"] = &aisystem.AISystem{ID: "ai-1", Name: "AI One"}
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewAISystem, "ai-1", 3)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if p == nil || p.View != ViewAISystem {
		t.Errorf("View: want %q, got %+v", ViewAISystem, p)
	}
}

// TestService_Dispatch_ServiceViewStillWorksAfterRefactor pins the
// regression contract: the Phase 1 service-view path is unchanged.
func TestService_Dispatch_ServiceViewStillWorksAfterRefactor(t *testing.T) {
	m := makeFullDemoMap()
	svc := NewServiceWithReaders(Readers{
		GovernanceMap: &stubReader{items: map[string]*governancemap.Map{"bs-1": m}},
	})
	p, err := svc.Project(context.Background(), ViewService, "bs-1", MaxDepth)
	if err != nil {
		t.Fatalf("service-view project: %v", err)
	}
	if p == nil || p.View != ViewService {
		t.Errorf("View: want %q, got %+v", ViewService, p)
	}
}

// TestService_Dispatch_ValidationOrder_ViewBeforeID confirms that
// view validation happens BEFORE id validation. An unregistered view
// with an empty id must report ErrInvalidView, never ErrInvalidID.
func TestService_Dispatch_ValidationOrder_ViewBeforeID(t *testing.T) {
	svc := NewServiceWithReaders(Readers{})
	_, err := svc.Project(context.Background(), "totally-invalid-view", "", 3)
	if !errors.Is(err, ErrInvalidView) {
		t.Errorf("validation order: want ErrInvalidView (view checked first), got %v", err)
	}
}

// TestService_Dispatch_ValidationOrder_IDBeforeDepth confirms id is
// checked before depth.
func TestService_Dispatch_ValidationOrder_IDBeforeDepth(t *testing.T) {
	stubs := newAIViewStubs()
	svc := NewServiceWithReaders(stubs.readers())
	_, err := svc.Project(context.Background(), ViewAISystem, "", -1)
	if !errors.Is(err, ErrInvalidID) {
		t.Errorf("validation order: want ErrInvalidID (id checked before depth), got %v", err)
	}
}

// TestService_AIView_EmptyID_ReturnsErrInvalidID pins per-view id
// validation.
func TestService_AIView_EmptyID_ReturnsErrInvalidID(t *testing.T) {
	stubs := newAIViewStubs()
	svc := NewServiceWithReaders(stubs.readers())
	_, err := svc.Project(context.Background(), ViewAISystem, "", 3)
	if !errors.Is(err, ErrInvalidID) {
		t.Errorf("want ErrInvalidID, got %v", err)
	}
}

// TestService_AIView_NegativeDepth_ReturnsErrInvalidDepth pins
// per-view depth validation.
func TestService_AIView_NegativeDepth_ReturnsErrInvalidDepth(t *testing.T) {
	stubs := newAIViewStubs()
	stubs.systems.items["ai-1"] = &aisystem.AISystem{ID: "ai-1"}
	svc := NewServiceWithReaders(stubs.readers())
	_, err := svc.Project(context.Background(), ViewAISystem, "ai-1", -1)
	if !errors.Is(err, ErrInvalidDepth) {
		t.Errorf("want ErrInvalidDepth, got %v", err)
	}
}

// TestService_AIView_DepthClamp_LargeIsSameAsMax pins the depth clamp
// applies to the ai_system view exactly as it does for the service
// view.
func TestService_AIView_DepthClamp_LargeIsSameAsMax(t *testing.T) {
	stubs := newAIViewStubs()
	stubs.systems.items["ai-1"] = &aisystem.AISystem{ID: "ai-1"}
	svc := NewServiceWithReaders(stubs.readers())

	pBig, err := svc.Project(context.Background(), ViewAISystem, "ai-1", 999)
	if err != nil {
		t.Fatalf("big depth: %v", err)
	}
	pMax, err := svc.Project(context.Background(), ViewAISystem, "ai-1", MaxDepth)
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
// B. Projection content
// ---------------------------------------------------------------------------

// B1. Unknown AI system → ErrNotFound.
func TestService_AIView_NotFound(t *testing.T) {
	stubs := newAIViewStubs()
	svc := NewServiceWithReaders(stubs.readers())
	_, err := svc.Project(context.Background(), ViewAISystem, "ai-missing", 3)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// B2. Zero bindings: root-only projection.
func TestService_AIView_ZeroBindings(t *testing.T) {
	stubs := newAIViewStubs()
	stubs.systems.items["ai-1"] = &aisystem.AISystem{ID: "ai-1", Name: "AI One"}
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewAISystem, "ai-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(p.Nodes) != 1 {
		t.Fatalf("want 1 node, got %d (%+v)", len(p.Nodes), p.Nodes)
	}
	if p.Nodes[0].Kind != NodeKindAISystem || p.Nodes[0].ID != "ai-1" {
		t.Errorf("root node: want ai_system:ai-1, got %+v", p.Nodes[0])
	}
	if len(p.Edges) != 0 {
		t.Errorf("want 0 edges, got %d (%+v)", len(p.Edges), p.Edges)
	}
	if p.Root.Kind != NodeKindAISystem || p.Root.ID != "ai-1" {
		t.Errorf("Root: want (ai_system, ai-1), got %+v", p.Root)
	}
	if p.View != ViewAISystem {
		t.Errorf("View: want %q, got %q", ViewAISystem, p.View)
	}
}

// B3. Surface-scoped binding: 5 nodes, 4 edges (system_of, bound_to,
// has_surface, has_process).
func TestService_AIView_SurfaceScopedBinding(t *testing.T) {
	stubs := newAIViewStubs()
	stubs.systems.items["ai-1"] = &aisystem.AISystem{ID: "ai-1", Name: "AI One"}
	stubs.bindings.byAISystem["ai-1"] = []*aisystem.AISystemBinding{
		{ID: "bind-1", AISystemID: "ai-1", SurfaceID: "surf-1", ProcessID: "proc-1"},
	}
	stubs.surfs.items["surf-1"] = &surface.DecisionSurface{
		ID: "surf-1", Version: 1, Name: "Surface One", ProcessID: "proc-1",
		Status: surface.SurfaceStatusActive,
	}
	stubs.procs.items["proc-1"] = &process.Process{
		ID: "proc-1", Name: "Process One", BusinessServiceID: "bs-1",
	}
	stubs.bss.items["bs-1"] = &businessservice.BusinessService{ID: "bs-1", Name: "BS One"}

	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewAISystem, "ai-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(p.Nodes) != 5 {
		t.Errorf("node count: want 5, got %d (%+v)", len(p.Nodes), p.Nodes)
	}
	if len(p.Edges) != 4 {
		t.Errorf("edge count: want 4, got %d (%+v)", len(p.Edges), p.Edges)
	}
	wantEdges := map[string]bool{
		"system_of|ai_system_binding:bind-1->ai_system:ai-1":         false,
		"bound_to|ai_system_binding:bind-1->decision_surface:surf-1": false,
		"has_surface|process:proc-1->decision_surface:surf-1":        false,
		"has_process|business_service:bs-1->process:proc-1":          false,
	}
	for _, e := range p.Edges {
		key := e.Kind + "|" + e.Src.Kind + ":" + e.Src.ID + "->" + e.Dst.Kind + ":" + e.Dst.ID
		if _, ok := wantEdges[key]; ok {
			wantEdges[key] = true
		} else {
			t.Errorf("unexpected edge: %s", key)
		}
	}
	for k, seen := range wantEdges {
		if !seen {
			t.Errorf("missing edge: %s", k)
		}
	}
}

// B4. Process-scoped binding: 4 nodes, 3 edges.
func TestService_AIView_ProcessScopedBinding(t *testing.T) {
	stubs := newAIViewStubs()
	stubs.systems.items["ai-1"] = &aisystem.AISystem{ID: "ai-1"}
	stubs.bindings.byAISystem["ai-1"] = []*aisystem.AISystemBinding{
		{ID: "bind-1", AISystemID: "ai-1", ProcessID: "proc-1"},
	}
	stubs.procs.items["proc-1"] = &process.Process{
		ID: "proc-1", Name: "P", BusinessServiceID: "bs-1",
	}
	stubs.bss.items["bs-1"] = &businessservice.BusinessService{ID: "bs-1"}

	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewAISystem, "ai-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(p.Nodes) != 4 {
		t.Errorf("node count: want 4, got %d", len(p.Nodes))
	}
	if len(p.Edges) != 3 {
		t.Errorf("edge count: want 3, got %d", len(p.Edges))
	}
	want := map[string]bool{
		"system_of|ai_system_binding:bind-1->ai_system:ai-1":  false,
		"bound_to|ai_system_binding:bind-1->process:proc-1":   false,
		"has_process|business_service:bs-1->process:proc-1":   false,
	}
	for _, e := range p.Edges {
		key := e.Kind + "|" + e.Src.Kind + ":" + e.Src.ID + "->" + e.Dst.Kind + ":" + e.Dst.ID
		if _, ok := want[key]; ok {
			want[key] = true
		} else {
			t.Errorf("unexpected edge: %s", key)
		}
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("missing edge: %s", k)
		}
	}
}

// B5. Capability-scoped binding: capability + bs (when binding row
// names a BS). has_capability bs -> cap.
func TestService_AIView_CapabilityScopedBinding(t *testing.T) {
	stubs := newAIViewStubs()
	stubs.systems.items["ai-1"] = &aisystem.AISystem{ID: "ai-1"}
	stubs.bindings.byAISystem["ai-1"] = []*aisystem.AISystemBinding{
		{ID: "bind-1", AISystemID: "ai-1", CapabilityID: "cap-1", BusinessServiceID: "bs-1"},
	}
	stubs.caps.items["cap-1"] = &capability.Capability{ID: "cap-1", Name: "C"}
	stubs.bss.items["bs-1"] = &businessservice.BusinessService{ID: "bs-1"}

	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewAISystem, "ai-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(p.Nodes) != 4 {
		t.Errorf("node count: want 4, got %d", len(p.Nodes))
	}
	if len(p.Edges) != 3 {
		t.Errorf("edge count: want 3, got %d", len(p.Edges))
	}
	want := map[string]bool{
		"system_of|ai_system_binding:bind-1->ai_system:ai-1":      false,
		"bound_to|ai_system_binding:bind-1->capability:cap-1":     false,
		"has_capability|business_service:bs-1->capability:cap-1":  false,
	}
	for _, e := range p.Edges {
		key := e.Kind + "|" + e.Src.Kind + ":" + e.Src.ID + "->" + e.Dst.Kind + ":" + e.Dst.ID
		if _, ok := want[key]; ok {
			want[key] = true
		} else {
			t.Errorf("unexpected edge: %s", key)
		}
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("missing edge: %s", k)
		}
	}
}

// B6. Business-service-scoped binding: bs node only, no parent
// context.
func TestService_AIView_BusinessServiceScopedBinding(t *testing.T) {
	stubs := newAIViewStubs()
	stubs.systems.items["ai-1"] = &aisystem.AISystem{ID: "ai-1"}
	stubs.bindings.byAISystem["ai-1"] = []*aisystem.AISystemBinding{
		{ID: "bind-1", AISystemID: "ai-1", BusinessServiceID: "bs-1"},
	}
	stubs.bss.items["bs-1"] = &businessservice.BusinessService{ID: "bs-1"}

	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewAISystem, "ai-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(p.Nodes) != 3 {
		t.Errorf("node count: want 3 (system + binding + bs), got %d", len(p.Nodes))
	}
	if len(p.Edges) != 2 {
		t.Errorf("edge count: want 2, got %d", len(p.Edges))
	}
	want := map[string]bool{
		"system_of|ai_system_binding:bind-1->ai_system:ai-1":          false,
		"bound_to|ai_system_binding:bind-1->business_service:bs-1":    false,
	}
	for _, e := range p.Edges {
		key := e.Kind + "|" + e.Src.Kind + ":" + e.Src.ID + "->" + e.Dst.Kind + ":" + e.Dst.ID
		if _, ok := want[key]; ok {
			want[key] = true
		} else {
			t.Errorf("unexpected edge: %s", key)
		}
	}
}

// B7. Mixed bindings sharing a parent BS dedupe to one BS node.
func TestService_AIView_MixedBindings_SharedBSDedup(t *testing.T) {
	stubs := newAIViewStubs()
	stubs.systems.items["ai-1"] = &aisystem.AISystem{ID: "ai-1"}
	stubs.bindings.byAISystem["ai-1"] = []*aisystem.AISystemBinding{
		{ID: "bind-1", AISystemID: "ai-1", ProcessID: "proc-1"},
		{ID: "bind-2", AISystemID: "ai-1", SurfaceID: "surf-2", ProcessID: "proc-2"},
	}
	stubs.procs.items["proc-1"] = &process.Process{ID: "proc-1", BusinessServiceID: "bs-1"}
	stubs.procs.items["proc-2"] = &process.Process{ID: "proc-2", BusinessServiceID: "bs-1"}
	stubs.surfs.items["surf-2"] = &surface.DecisionSurface{ID: "surf-2", ProcessID: "proc-2", Status: surface.SurfaceStatusActive}
	stubs.bss.items["bs-1"] = &businessservice.BusinessService{ID: "bs-1"}

	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewAISystem, "ai-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	bsCount := 0
	for _, n := range p.Nodes {
		if n.Kind == NodeKindBusinessService {
			bsCount++
		}
	}
	if bsCount != 1 {
		t.Errorf("shared BS must dedup to 1 node, got %d", bsCount)
	}
	// Sanity: 2 has_process edges (one per process), 1 has_surface,
	// 2 system_of, 2 bound_to => 7 edges total.
	if len(p.Edges) != 7 {
		t.Errorf("edge count: want 7, got %d (%+v)", len(p.Edges), p.Edges)
	}
}

// B8. Edge directionality is fixed: bound_to is always binding ->
// scope, system_of always binding -> ai_system, has_* always parent
// -> child.
func TestService_AIView_EdgeDirectionality(t *testing.T) {
	stubs := newAIViewStubs()
	stubs.systems.items["ai-1"] = &aisystem.AISystem{ID: "ai-1"}
	stubs.bindings.byAISystem["ai-1"] = []*aisystem.AISystemBinding{
		{ID: "bind-1", AISystemID: "ai-1", SurfaceID: "surf-1", ProcessID: "proc-1"},
	}
	stubs.surfs.items["surf-1"] = &surface.DecisionSurface{ID: "surf-1", ProcessID: "proc-1", Status: surface.SurfaceStatusActive}
	stubs.procs.items["proc-1"] = &process.Process{ID: "proc-1", BusinessServiceID: "bs-1"}
	stubs.bss.items["bs-1"] = &businessservice.BusinessService{ID: "bs-1"}

	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewAISystem, "ai-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	for _, e := range p.Edges {
		switch e.Kind {
		case EdgeKindSystemOf:
			if e.Src.Kind != NodeKindAISystemBinding || e.Dst.Kind != NodeKindAISystem {
				t.Errorf("system_of must be binding->ai_system; got %+v", e)
			}
		case EdgeKindBoundTo:
			if e.Src.Kind != NodeKindAISystemBinding {
				t.Errorf("bound_to src must be binding; got %+v", e)
			}
		case EdgeKindHasSurface:
			if e.Src.Kind != NodeKindProcess || e.Dst.Kind != NodeKindDecisionSurface {
				t.Errorf("has_surface must be process->decision_surface; got %+v", e)
			}
		case EdgeKindHasProcess:
			if e.Src.Kind != NodeKindBusinessService || e.Dst.Kind != NodeKindProcess {
				t.Errorf("has_process must be business_service->process; got %+v", e)
			}
		case EdgeKindHasCapability:
			if e.Src.Kind != NodeKindBusinessService || e.Dst.Kind != NodeKindCapability {
				t.Errorf("has_capability must be business_service->capability; got %+v", e)
			}
		default:
			t.Errorf("unexpected edge kind in ai_system view: %q", e.Kind)
		}
	}
}

// B9. Missing scope target: binding + system_of stay; bound_to and
// missing target node are omitted; no panic; projection succeeds.
func TestService_AIView_MissingScopeTarget(t *testing.T) {
	stubs := newAIViewStubs()
	stubs.systems.items["ai-1"] = &aisystem.AISystem{ID: "ai-1"}
	stubs.bindings.byAISystem["ai-1"] = []*aisystem.AISystemBinding{
		{ID: "bind-1", AISystemID: "ai-1", SurfaceID: "surf-missing"},
	}

	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewAISystem, "ai-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(p.Nodes) != 2 {
		t.Errorf("node count: want 2 (system + binding), got %d (%+v)", len(p.Nodes), p.Nodes)
	}
	for _, n := range p.Nodes {
		if n.Kind == NodeKindDecisionSurface {
			t.Errorf("missing surface must NOT be emitted as a node; got %+v", n)
		}
	}
	if len(p.Edges) != 1 {
		t.Errorf("edge count: want 1 (system_of only), got %d (%+v)", len(p.Edges), p.Edges)
	}
	if len(p.Edges) == 1 && p.Edges[0].Kind != EdgeKindSystemOf {
		t.Errorf("only edge must be system_of; got %+v", p.Edges[0])
	}
}

// ---------------------------------------------------------------------------
// C. Depth semantics (ai_system root)
// ---------------------------------------------------------------------------

// makeAIViewDepthFixture returns stubs with a 4-hop chain (undirected
// from ai-1):
//
//   ai-1 <-(system_of)- bind-1 -(bound_to)-> surf-1 <-(has_surface)- proc-1 <-(has_process)- bs-1
//
// Hops from ai-1: bind-1 = 1, surf-1 = 2, proc-1 = 3, bs-1 = 4.
func makeAIViewDepthFixture() *aiViewStubs {
	stubs := newAIViewStubs()
	stubs.systems.items["ai-1"] = &aisystem.AISystem{ID: "ai-1"}
	stubs.bindings.byAISystem["ai-1"] = []*aisystem.AISystemBinding{
		{ID: "bind-1", AISystemID: "ai-1", SurfaceID: "surf-1", ProcessID: "proc-1"},
	}
	stubs.surfs.items["surf-1"] = &surface.DecisionSurface{ID: "surf-1", ProcessID: "proc-1", Status: surface.SurfaceStatusActive}
	stubs.procs.items["proc-1"] = &process.Process{ID: "proc-1", BusinessServiceID: "bs-1"}
	stubs.bss.items["bs-1"] = &businessservice.BusinessService{ID: "bs-1"}
	return stubs
}

func TestService_AIView_DepthZero_RootOnly(t *testing.T) {
	stubs := makeAIViewDepthFixture()
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewAISystem, "ai-1", 0)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(p.Nodes) != 1 {
		t.Errorf("depth=0: want 1 node, got %d (%+v)", len(p.Nodes), p.Nodes)
	}
	if len(p.Edges) != 0 {
		t.Errorf("depth=0: want 0 edges, got %d", len(p.Edges))
	}
	if p.Depth != 0 {
		t.Errorf("Depth: want 0, got %d", p.Depth)
	}
}

func TestService_AIView_DepthOne_RootPlusBinding(t *testing.T) {
	stubs := makeAIViewDepthFixture()
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewAISystem, "ai-1", 1)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	got := map[string]bool{}
	for _, n := range p.Nodes {
		got[n.Kind+":"+n.ID] = true
	}
	for _, want := range []string{"ai_system:ai-1", "ai_system_binding:bind-1"} {
		if !got[want] {
			t.Errorf("depth=1 missing %q (got %v)", want, got)
		}
	}
	for _, illegal := range []string{"decision_surface:surf-1", "process:proc-1", "business_service:bs-1"} {
		if got[illegal] {
			t.Errorf("depth=1 must NOT include %q (got %v)", illegal, got)
		}
	}
}

func TestService_AIView_DepthTwo_AddsScopeTarget(t *testing.T) {
	stubs := makeAIViewDepthFixture()
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewAISystem, "ai-1", 2)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	got := map[string]bool{}
	for _, n := range p.Nodes {
		got[n.Kind+":"+n.ID] = true
	}
	for _, want := range []string{"ai_system:ai-1", "ai_system_binding:bind-1", "decision_surface:surf-1"} {
		if !got[want] {
			t.Errorf("depth=2 missing %q (got %v)", want, got)
		}
	}
	for _, illegal := range []string{"process:proc-1", "business_service:bs-1"} {
		if got[illegal] {
			t.Errorf("depth=2 must NOT include %q (got %v)", illegal, got)
		}
	}
}

func TestService_AIView_DepthThree_AddsParentProcess(t *testing.T) {
	stubs := makeAIViewDepthFixture()
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewAISystem, "ai-1", 3)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	got := map[string]bool{}
	for _, n := range p.Nodes {
		got[n.Kind+":"+n.ID] = true
	}
	for _, want := range []string{"ai_system:ai-1", "ai_system_binding:bind-1", "decision_surface:surf-1", "process:proc-1"} {
		if !got[want] {
			t.Errorf("depth=3 missing %q (got %v)", want, got)
		}
	}
	if got["business_service:bs-1"] {
		t.Errorf("depth=3 must NOT include business_service:bs-1 yet (it's 4 hops away)")
	}
}

// ---------------------------------------------------------------------------
// Decision Surface view — dispatch + projection tests.
// ---------------------------------------------------------------------------

// makeSurfViewStubs assembles the readers the decision_surface view
// needs (Surfaces, AISystemBindings, AISystem, Processes,
// BusinessServices). Capabilities and GovernanceMap are left at zero
// so this fixture isolates the new view's dependencies — tests that
// also need view=service or view=ai_system can compose those onto
// the result.
func makeSurfViewStubs() *aiViewStubs {
	return newAIViewStubs()
}

// TestService_SurfView_RegistrationGuards pins:
//
//   - decision_surface IS registered when all its required readers are
//     present (Surfaces, AISystemBindings, AISystem, Processes,
//     BusinessServices).
//   - decision_surface is NOT registered when any required reader is
//     nil → Service.Project returns ErrInvalidView for that view.
//   - The registration of decision_surface does NOT depend on
//     Capabilities or GovernanceMap (this view doesn't traverse
//     capabilities and doesn't need the service-view governance map).
func TestService_SurfView_RegistrationGuards(t *testing.T) {
	stubs := makeSurfViewStubs()

	// Full readers: must register.
	full := stubs.readers()
	full.Capabilities = nil       // not required by decision_surface view
	full.GovernanceMap = nil      // not required by decision_surface view
	svc := NewServiceWithReaders(full)
	if _, ok := svc.projectors[ViewDecisionSurface]; !ok {
		t.Error("decision_surface projector must register when its required readers are present")
	}

	// Each required reader nil-ed in turn: must NOT register.
	cases := []struct {
		name  string
		mutate func(r *Readers)
	}{
		{"Surfaces nil", func(r *Readers) { r.Surfaces = nil }},
		{"AISystemBindings nil", func(r *Readers) { r.AISystemBindings = nil }},
		{"AISystem nil", func(r *Readers) { r.AISystem = nil }},
		{"Processes nil", func(r *Readers) { r.Processes = nil }},
		{"BusinessServices nil", func(r *Readers) { r.BusinessServices = nil }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := stubs.readers()
			c.mutate(&r)
			svc := NewServiceWithReaders(r)
			if _, ok := svc.projectors[ViewDecisionSurface]; ok {
				t.Errorf("decision_surface projector must NOT register when %s", c.name)
			}
			_, err := svc.Project(context.Background(), ViewDecisionSurface, "surf-x", 3)
			if !errors.Is(err, ErrInvalidView) {
				t.Errorf("Project(decision_surface) without readers: want ErrInvalidView, got %v", err)
			}
		})
	}
}

// TestService_SurfView_EmptyID_ReturnsErrInvalidID exercises validation
// order for the new view: id is checked AFTER view registration but
// BEFORE depth, matching the existing service / ai_system contract.
func TestService_SurfView_EmptyID_ReturnsErrInvalidID(t *testing.T) {
	stubs := makeSurfViewStubs()
	svc := NewServiceWithReaders(stubs.readers())
	_, err := svc.Project(context.Background(), ViewDecisionSurface, "", 3)
	if !errors.Is(err, ErrInvalidID) {
		t.Errorf("want ErrInvalidID, got %v", err)
	}
}

func TestService_SurfView_NegativeDepth_ReturnsErrInvalidDepth(t *testing.T) {
	stubs := makeSurfViewStubs()
	svc := NewServiceWithReaders(stubs.readers())
	_, err := svc.Project(context.Background(), ViewDecisionSurface, "surf-1", -1)
	if !errors.Is(err, ErrInvalidDepth) {
		t.Errorf("want ErrInvalidDepth, got %v", err)
	}
}

// TestService_SurfView_NotFoundID_ReturnsErrNotFound covers the
// missing-root case: FindLatestByID returns (nil, nil), the projector
// must surface ErrNotFound (not silently return an empty projection).
func TestService_SurfView_NotFoundID_ReturnsErrNotFound(t *testing.T) {
	stubs := makeSurfViewStubs()
	svc := NewServiceWithReaders(stubs.readers())
	_, err := svc.Project(context.Background(), ViewDecisionSurface, "surf-missing", 3)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// TestService_SurfView_DepthClamp pins the existing depth-clamp
// invariant for the new view: depth=999 must produce the same shape as
// depth=MaxDepth.
func TestService_SurfView_DepthClamp(t *testing.T) {
	stubs := makeSurfViewWithFullChainAndOneBinding()
	svc := NewServiceWithReaders(stubs.readers())

	pBig, err := svc.Project(context.Background(), ViewDecisionSurface, "surf-1", 999)
	if err != nil {
		t.Fatalf("project depth=999: %v", err)
	}
	pMax, err := svc.Project(context.Background(), ViewDecisionSurface, "surf-1", MaxDepth)
	if err != nil {
		t.Fatalf("project depth=MaxDepth: %v", err)
	}
	if pBig.Depth != MaxDepth {
		t.Errorf("depth=999 must clamp to MaxDepth (%d); Projection.Depth=%d", MaxDepth, pBig.Depth)
	}
	if len(pBig.Nodes) != len(pMax.Nodes) || len(pBig.Edges) != len(pMax.Edges) {
		t.Errorf("depth=999 and depth=MaxDepth must produce identical shapes; got nodes %d/%d edges %d/%d",
			len(pBig.Nodes), len(pMax.Nodes), len(pBig.Edges), len(pMax.Edges))
	}
}

// makeSurfViewWithFullChainAndOneBinding builds a fixture exercising
// the full parent chain (BS ← process ← surface) plus exactly one AI
// binding to one AI system. Used by depth and projection-shape tests.
func makeSurfViewWithFullChainAndOneBinding() *aiViewStubs {
	stubs := makeSurfViewStubs()
	stubs.surfs.items["surf-1"] = &surface.DecisionSurface{
		ID: "surf-1", Version: 3, Name: "Surface One", ProcessID: "proc-1",
		Status: surface.SurfaceStatusActive, Description: "the root surface",
	}
	stubs.procs.items["proc-1"] = &process.Process{
		ID: "proc-1", Name: "Process One", BusinessServiceID: "bs-1", Status: "active",
	}
	stubs.bss.items["bs-1"] = &businessservice.BusinessService{
		ID: "bs-1", Name: "BS One", Status: "active",
	}
	stubs.systems.items["ai-1"] = &aisystem.AISystem{
		ID: "ai-1", Name: "AI One", Status: "active", Vendor: "acme",
	}
	stubs.bindings.bySurface["surf-1"] = []*aisystem.AISystemBinding{
		{ID: "bind-1", AISystemID: "ai-1", SurfaceID: "surf-1", Role: "primary"},
	}
	return stubs
}

// TestService_SurfView_DepthZero_RootOnly pins the universal depth=0
// contract: exactly one node (the root) and no edges, regardless of
// binding / parent richness.
func TestService_SurfView_DepthZero_RootOnly(t *testing.T) {
	stubs := makeSurfViewWithFullChainAndOneBinding()
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewDecisionSurface, "surf-1", 0)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(p.Nodes) != 1 {
		t.Errorf("depth=0 must have exactly one node; got %d (%+v)", len(p.Nodes), p.Nodes)
	}
	if len(p.Nodes) == 1 && (p.Nodes[0].Kind != NodeKindDecisionSurface || p.Nodes[0].ID != "surf-1") {
		t.Errorf("depth=0 root node must be decision_surface:surf-1; got %+v", p.Nodes[0])
	}
	if len(p.Edges) != 0 {
		t.Errorf("depth=0 must have zero edges; got %d (%+v)", len(p.Edges), p.Edges)
	}
	if p.View != ViewDecisionSurface {
		t.Errorf("Projection.View: want %q, got %q", ViewDecisionSurface, p.View)
	}
	if p.Root.Kind != NodeKindDecisionSurface || p.Root.ID != "surf-1" {
		t.Errorf("Projection.Root: want decision_surface:surf-1, got %+v", p.Root)
	}
}

// TestService_SurfView_DepthOne_ImmediateNeighbours pins the depth=1
// shape: root + parent process + binding(s) + each binding's AI system
// (the AI system is one hop from the binding, which is itself one hop
// from the root — so ai_system is exactly 2 hops, NOT in depth=1).
// Parent BS is two hops from the root via the parent process and
// therefore not in depth=1 either.
func TestService_SurfView_DepthOne_ImmediateNeighbours(t *testing.T) {
	stubs := makeSurfViewWithFullChainAndOneBinding()
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewDecisionSurface, "surf-1", 1)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	got := map[string]bool{}
	for _, n := range p.Nodes {
		got[n.Kind+":"+n.ID] = true
	}
	for _, want := range []string{
		"decision_surface:surf-1",
		"process:proc-1",
		"ai_system_binding:bind-1",
	} {
		if !got[want] {
			t.Errorf("depth=1 missing %q (got %v)", want, got)
		}
	}
	for _, illegal := range []string{
		"business_service:bs-1", // 2 hops via process
		"ai_system:ai-1",        // 2 hops via binding
	} {
		if got[illegal] {
			t.Errorf("depth=1 must NOT include %q (got %v)", illegal, got)
		}
	}
}

// TestService_SurfView_DepthTwo_AddsBSAndAISystem pins that the
// 2-hop neighbours appear at depth=2: parent BS (via process) and the
// AI system (via binding).
func TestService_SurfView_DepthTwo_AddsBSAndAISystem(t *testing.T) {
	stubs := makeSurfViewWithFullChainAndOneBinding()
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewDecisionSurface, "surf-1", 2)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	got := map[string]bool{}
	for _, n := range p.Nodes {
		got[n.Kind+":"+n.ID] = true
	}
	for _, want := range []string{
		"decision_surface:surf-1",
		"process:proc-1",
		"ai_system_binding:bind-1",
		"business_service:bs-1",
		"ai_system:ai-1",
	} {
		if !got[want] {
			t.Errorf("depth=2 missing %q (got %v)", want, got)
		}
	}
}

// TestService_SurfView_FullProjection_NodeAndEdgeCounts pins the
// exact node/edge counts at MaxDepth for the canonical fixture:
//
//   nodes (5): decision_surface, process, business_service,
//              ai_system_binding, ai_system
//   edges (4): has_surface (proc → surf), has_process (bs → proc),
//              bound_to (binding → surf), system_of (binding → ai)
//
// Edge directionality is asserted alongside the count so a regression
// that flipped a direction surfaces here.
func TestService_SurfView_FullProjection_NodeAndEdgeCounts(t *testing.T) {
	stubs := makeSurfViewWithFullChainAndOneBinding()
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewDecisionSurface, "surf-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(p.Nodes) != 5 {
		t.Errorf("want 5 nodes, got %d (%+v)", len(p.Nodes), p.Nodes)
	}
	if len(p.Edges) != 4 {
		t.Errorf("want 4 edges, got %d (%+v)", len(p.Edges), p.Edges)
	}

	type edgeSig struct {
		Kind, SrcKind, SrcID, DstKind, DstID string
	}
	gotEdges := map[edgeSig]bool{}
	for _, e := range p.Edges {
		gotEdges[edgeSig{e.Kind, e.Src.Kind, e.Src.ID, e.Dst.Kind, e.Dst.ID}] = true
	}
	wantEdges := []edgeSig{
		{EdgeKindHasSurface, NodeKindProcess, "proc-1", NodeKindDecisionSurface, "surf-1"},
		{EdgeKindHasProcess, NodeKindBusinessService, "bs-1", NodeKindProcess, "proc-1"},
		{EdgeKindBoundTo, NodeKindAISystemBinding, "bind-1", NodeKindDecisionSurface, "surf-1"},
		{EdgeKindSystemOf, NodeKindAISystemBinding, "bind-1", NodeKindAISystem, "ai-1"},
	}
	for _, w := range wantEdges {
		if !gotEdges[w] {
			t.Errorf("missing edge %+v (got %+v)", w, gotEdges)
		}
	}
}

// TestService_SurfView_NoParentProcess pins the contract that a
// surface with empty ProcessID emits the surface root and any
// bindings, but no process / BS context — and no has_surface /
// has_process edges.
func TestService_SurfView_NoParentProcess(t *testing.T) {
	stubs := makeSurfViewStubs()
	stubs.surfs.items["surf-orphan"] = &surface.DecisionSurface{
		ID: "surf-orphan", Version: 1, Name: "Orphan Surface",
		Status: surface.SurfaceStatusActive,
		// ProcessID intentionally empty.
	}
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewDecisionSurface, "surf-orphan", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(p.Nodes) != 1 {
		t.Errorf("orphan surface: want 1 node, got %d (%+v)", len(p.Nodes), p.Nodes)
	}
	if len(p.Edges) != 0 {
		t.Errorf("orphan surface: want 0 edges, got %d (%+v)", len(p.Edges), p.Edges)
	}
}

// TestService_SurfView_ProcessOnlyNoBS pins the partial-context path:
// surface has ProcessID, the process resolves, but the process has no
// BusinessServiceID — must emit process + has_surface and stop there.
func TestService_SurfView_ProcessOnlyNoBS(t *testing.T) {
	stubs := makeSurfViewStubs()
	stubs.surfs.items["surf-1"] = &surface.DecisionSurface{
		ID: "surf-1", Version: 1, Name: "Surface", ProcessID: "proc-1",
		Status: surface.SurfaceStatusActive,
	}
	stubs.procs.items["proc-1"] = &process.Process{
		ID: "proc-1", Name: "Proc", Status: "active",
		// BusinessServiceID intentionally empty.
	}
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewDecisionSurface, "surf-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	got := map[string]bool{}
	for _, n := range p.Nodes {
		got[n.Kind+":"+n.ID] = true
	}
	if !got["decision_surface:surf-1"] || !got["process:proc-1"] {
		t.Errorf("want surface + process; got %v", got)
	}
	if got["business_service:"] {
		t.Errorf("must NOT emit any business_service node when process has no BS pointer; got %v", got)
	}
	for _, e := range p.Edges {
		if e.Kind == EdgeKindHasProcess {
			t.Errorf("must NOT emit has_process when no parent BS resolves; got %+v", e)
		}
	}
}

// TestService_SurfView_MissingProcessLookup pins missing-lookup
// graceful behaviour: ProcessID is set on the surface, but the reader
// returns nil — projection succeeds, surface emitted alone, no
// has_surface or has_process edges.
func TestService_SurfView_MissingProcessLookup(t *testing.T) {
	stubs := makeSurfViewStubs()
	stubs.surfs.items["surf-1"] = &surface.DecisionSurface{
		ID: "surf-1", Version: 1, Name: "Surface", ProcessID: "proc-missing",
		Status: surface.SurfaceStatusActive,
	}
	// procs.items intentionally lacks proc-missing → GetByID returns nil.
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewDecisionSurface, "surf-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(p.Nodes) != 1 {
		t.Errorf("missing parent process: want 1 node (root surface only), got %d (%+v)", len(p.Nodes), p.Nodes)
	}
	if len(p.Edges) != 0 {
		t.Errorf("missing parent process: want 0 edges, got %d (%+v)", len(p.Edges), p.Edges)
	}
}

// TestService_SurfView_MultipleBindings_DistinctAndShared pins:
//
//   - Two bindings to two different AI systems → 2 binding nodes,
//     2 ai_system nodes, 2 system_of edges, 2 bound_to edges.
//   - Two bindings to the SAME AI system → 2 binding nodes, exactly
//     ONE ai_system node (deduped), 2 system_of edges, 2 bound_to.
func TestService_SurfView_MultipleBindings_DistinctAndShared(t *testing.T) {
	stubs := makeSurfViewStubs()
	stubs.surfs.items["surf-1"] = &surface.DecisionSurface{
		ID: "surf-1", Version: 1, Name: "Surface", ProcessID: "",
		Status: surface.SurfaceStatusActive,
	}
	stubs.systems.items["ai-1"] = &aisystem.AISystem{ID: "ai-1", Name: "AI One"}
	stubs.systems.items["ai-2"] = &aisystem.AISystem{ID: "ai-2", Name: "AI Two"}
	stubs.bindings.bySurface["surf-1"] = []*aisystem.AISystemBinding{
		{ID: "bind-a", AISystemID: "ai-1", SurfaceID: "surf-1", Role: "primary"},
		{ID: "bind-b", AISystemID: "ai-2", SurfaceID: "surf-1", Role: "fallback"},
		{ID: "bind-c", AISystemID: "ai-1", SurfaceID: "surf-1", Role: "shadow"},
	}

	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewDecisionSurface, "surf-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	bindingCount := 0
	aiCount := 0
	got := map[string]bool{}
	for _, n := range p.Nodes {
		got[n.Kind+":"+n.ID] = true
		switch n.Kind {
		case NodeKindAISystemBinding:
			bindingCount++
		case NodeKindAISystem:
			aiCount++
		}
	}
	if bindingCount != 3 {
		t.Errorf("want 3 binding nodes, got %d (%v)", bindingCount, got)
	}
	if aiCount != 2 {
		t.Errorf("want 2 ai_system nodes (ai-1 deduped across bind-a/bind-c, plus ai-2), got %d (%v)", aiCount, got)
	}

	boundToCount := 0
	systemOfCount := 0
	for _, e := range p.Edges {
		switch e.Kind {
		case EdgeKindBoundTo:
			boundToCount++
		case EdgeKindSystemOf:
			systemOfCount++
		}
	}
	if boundToCount != 3 {
		t.Errorf("want 3 bound_to edges (one per binding), got %d", boundToCount)
	}
	if systemOfCount != 3 {
		t.Errorf("want 3 system_of edges (one per binding), got %d", systemOfCount)
	}
}

// TestService_SurfView_MissingAISystemLookup pins the
// "binding present, AI system missing" case: the binding node and the
// bound_to edge to the surface are still emitted; the system_of edge
// and the ai_system node are skipped (no failure).
func TestService_SurfView_MissingAISystemLookup(t *testing.T) {
	stubs := makeSurfViewStubs()
	stubs.surfs.items["surf-1"] = &surface.DecisionSurface{
		ID: "surf-1", Version: 1, Name: "Surface",
		Status: surface.SurfaceStatusActive,
	}
	// systems.items lacks ai-ghost → GetByID returns nil.
	stubs.bindings.bySurface["surf-1"] = []*aisystem.AISystemBinding{
		{ID: "bind-x", AISystemID: "ai-ghost", SurfaceID: "surf-1"},
	}
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewDecisionSurface, "surf-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	got := map[string]bool{}
	for _, n := range p.Nodes {
		got[n.Kind+":"+n.ID] = true
	}
	if !got["decision_surface:surf-1"] || !got["ai_system_binding:bind-x"] {
		t.Errorf("want surface + binding; got %v", got)
	}
	if got["ai_system:ai-ghost"] {
		t.Errorf("must NOT emit ai_system node for unresolved AI system; got %v", got)
	}

	for _, e := range p.Edges {
		if e.Kind == EdgeKindSystemOf {
			t.Errorf("must NOT emit system_of edge for unresolved AI system; got %+v", e)
		}
	}
	// Exactly one bound_to edge, pointing from the binding to the surface.
	boundCount := 0
	for _, e := range p.Edges {
		if e.Kind == EdgeKindBoundTo {
			boundCount++
			if e.Src.Kind != NodeKindAISystemBinding || e.Src.ID != "bind-x" ||
				e.Dst.Kind != NodeKindDecisionSurface || e.Dst.ID != "surf-1" {
				t.Errorf("bound_to edge has wrong shape: %+v", e)
			}
		}
	}
	if boundCount != 1 {
		t.Errorf("want exactly 1 bound_to edge, got %d", boundCount)
	}
}

// TestService_SurfView_TypedDataPopulated pins the typed-data slots
// on the new view's nodes: surface root carries DecisionSurfaceData
// with non-zero Version + non-empty Status; binding carries
// AISystemBindingData with ScopeKind/ScopeID matching the root surface;
// AI system carries AISystemData with the seed name.
func TestService_SurfView_TypedDataPopulated(t *testing.T) {
	stubs := makeSurfViewWithFullChainAndOneBinding()
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewDecisionSurface, "surf-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	for _, n := range p.Nodes {
		switch n.Kind {
		case NodeKindDecisionSurface:
			if n.DecisionSurface == nil {
				t.Fatalf("decision_surface root must carry DecisionSurfaceData; got %+v", n)
			}
			if n.DecisionSurface.ID != "surf-1" || n.DecisionSurface.Version != 3 {
				t.Errorf("DecisionSurfaceData: want id=surf-1 version=3, got %+v", n.DecisionSurface)
			}
			if n.DecisionSurface.Status != string(surface.SurfaceStatusActive) {
				t.Errorf("DecisionSurfaceData.Status: want active, got %q", n.DecisionSurface.Status)
			}
		case NodeKindAISystemBinding:
			if n.AISystemBinding == nil {
				t.Fatalf("ai_system_binding must carry AISystemBindingData; got %+v", n)
			}
			if n.AISystemBinding.ScopeKind != NodeKindDecisionSurface || n.AISystemBinding.ScopeID != "surf-1" {
				t.Errorf("AISystemBindingData scope: want decision_surface/surf-1, got %s/%s",
					n.AISystemBinding.ScopeKind, n.AISystemBinding.ScopeID)
			}
			if n.AISystemBinding.AISystemID != "ai-1" || n.AISystemBinding.AISystemName != "AI One" {
				t.Errorf("AISystemBindingData ai-system: want ai-1/AI One, got %s/%s",
					n.AISystemBinding.AISystemID, n.AISystemBinding.AISystemName)
			}
		case NodeKindAISystem:
			if n.AISystem == nil {
				t.Fatalf("ai_system must carry AISystemData; got %+v", n)
			}
			if n.AISystem.ID != "ai-1" || n.AISystem.Name != "AI One" {
				t.Errorf("AISystemData: want ai-1/AI One, got %+v", n.AISystem)
			}
		}
	}
}

// TestService_SurfView_NoSyntheticSummaryOrCoverage pins the "no
// authority_summary, no coverage" contract for this view — those nodes
// are service-view constructs; emitting them on decision_surface would
// be a contract regression.
func TestService_SurfView_NoSyntheticSummaryOrCoverage(t *testing.T) {
	stubs := makeSurfViewWithFullChainAndOneBinding()
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewDecisionSurface, "surf-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	for _, n := range p.Nodes {
		if n.Kind == NodeKindAuthoritySummary || n.Kind == NodeKindCoverage {
			t.Errorf("decision_surface view must NOT emit %q nodes; got %+v", n.Kind, n)
		}
	}
	for _, e := range p.Edges {
		if e.Kind == EdgeKindSummarises || e.Kind == EdgeKindReportsCoverage {
			t.Errorf("decision_surface view must NOT emit %q edges; got %+v", e.Kind, e)
		}
	}
}

// ---------------------------------------------------------------------------
// D27j-ui-2a — FailModePolicyID propagation through every view
// ---------------------------------------------------------------------------
//
// The projection carries the BusinessService.FailModePolicyID and
// DecisionSurface.FailModePolicyID references verbatim. No policy
// lookup, no resolution, no rule inspection. These tests prove the
// data is present in typed-data when configured and nothing else
// changes (no new node kind, no new edge kind, no new top-level
// fields).

func TestService_ServiceView_BusinessServiceData_CarriesFailModePolicyID(t *testing.T) {
	m := makeFullDemoMap()
	m.BusinessService.BusinessService.FailModePolicyID = "policy-bs-default"
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{items: map[string]*governancemap.Map{"bs-1": m}}})
	p, err := svc.Project(context.Background(), ViewService, "bs-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	var bs *Node
	for i := range p.Nodes {
		if p.Nodes[i].Kind == NodeKindBusinessService && p.Nodes[i].ID == "bs-1" {
			bs = &p.Nodes[i]
			break
		}
	}
	if bs == nil || bs.BusinessService == nil {
		t.Fatalf("business_service typed data missing")
	}
	if got, want := bs.BusinessService.FailModePolicyID, "policy-bs-default"; got != want {
		t.Errorf("bs fail_mode_policy_id: want %q, got %q", want, got)
	}
}

func TestService_ServiceView_BusinessServiceData_OmitsEmptyFailModePolicyID(t *testing.T) {
	// makeFullDemoMap leaves FailModePolicyID at zero, so the empty
	// branch is the default. Marshal the projection and assert the key
	// does not appear on the BS root node.
	m := makeFullDemoMap()
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{items: map[string]*governancemap.Map{"bs-1": m}}})
	p, err := svc.Project(context.Background(), ViewService, "bs-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	for _, n := range p.Nodes {
		if n.Kind != NodeKindBusinessService || n.ID != "bs-1" || n.BusinessService == nil {
			continue
		}
		raw, err := json.Marshal(n.BusinessService)
		if err != nil {
			t.Fatalf("marshal bs: %v", err)
		}
		if strings.Contains(string(raw), `"fail_mode_policy_id"`) {
			t.Errorf("empty FailModePolicyID must be omitted from BS typed data; got %s", raw)
		}
		return
	}
	t.Fatal("business_service node missing")
}

func TestService_ServiceView_DecisionSurfaceData_CarriesFailModePolicyID(t *testing.T) {
	m := makeFullDemoMap()
	m.Surfaces[0].Surface.FailModePolicyID = "policy-surface-override"
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{items: map[string]*governancemap.Map{"bs-1": m}}})
	p, err := svc.Project(context.Background(), ViewService, "bs-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	var surf *Node
	for i := range p.Nodes {
		if p.Nodes[i].Kind == NodeKindDecisionSurface && p.Nodes[i].ID == "surf-1" {
			surf = &p.Nodes[i]
			break
		}
	}
	if surf == nil || surf.DecisionSurface == nil {
		t.Fatalf("decision_surface typed data missing")
	}
	if got, want := surf.DecisionSurface.FailModePolicyID, "policy-surface-override"; got != want {
		t.Errorf("surf fail_mode_policy_id: want %q, got %q", want, got)
	}
}

func TestService_AIView_BusinessServiceData_CarriesFailModePolicyID(t *testing.T) {
	stubs := newAIViewStubs()
	stubs.systems.items["ai-1"] = &aisystem.AISystem{ID: "ai-1", Name: "AI", Status: "active"}
	stubs.bindings.byAISystem["ai-1"] = []*aisystem.AISystemBinding{
		{ID: "bind-1", AISystemID: "ai-1", BusinessServiceID: "bs-1"},
	}
	stubs.bss.items["bs-1"] = &businessservice.BusinessService{
		ID: "bs-1", Name: "BS", Status: "active",
		FailModePolicyID: "policy-bs-default",
	}
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewAISystem, "ai-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	var bs *Node
	for i := range p.Nodes {
		if p.Nodes[i].Kind == NodeKindBusinessService && p.Nodes[i].ID == "bs-1" {
			bs = &p.Nodes[i]
			break
		}
	}
	if bs == nil || bs.BusinessService == nil {
		t.Fatalf("business_service typed data missing on ai_system view")
	}
	if got, want := bs.BusinessService.FailModePolicyID, "policy-bs-default"; got != want {
		t.Errorf("ai_system view bs fail_mode_policy_id: want %q, got %q", want, got)
	}
}

func TestService_SurfView_DecisionSurfaceData_CarriesFailModePolicyID(t *testing.T) {
	stubs := makeSurfViewWithFullChainAndOneBinding()
	stubs.surfs.items["surf-1"].FailModePolicyID = "policy-surface-override"
	svc := NewServiceWithReaders(stubs.readers())
	p, err := svc.Project(context.Background(), ViewDecisionSurface, "surf-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	var surf *Node
	for i := range p.Nodes {
		if p.Nodes[i].Kind == NodeKindDecisionSurface && p.Nodes[i].ID == "surf-1" {
			surf = &p.Nodes[i]
			break
		}
	}
	if surf == nil || surf.DecisionSurface == nil {
		t.Fatalf("decision_surface typed data missing on decision_surface view")
	}
	if got, want := surf.DecisionSurface.FailModePolicyID, "policy-surface-override"; got != want {
		t.Errorf("decision_surface view fail_mode_policy_id: want %q, got %q", want, got)
	}
}

// TestService_ServiceView_NoFailModePolicyNodeOrEdgeIntroduced is the
// structural guard for D27j-ui-2a: even when a BS and a surface both
// carry FailModePolicyID values, the projection must not introduce
// new node kinds, edge kinds, or top-level fields. The reference is
// data-only.
func TestService_ServiceView_NoFailModePolicyNodeOrEdgeIntroduced(t *testing.T) {
	m := makeFullDemoMap()
	m.BusinessService.BusinessService.FailModePolicyID = "policy-bs-default"
	m.Surfaces[0].Surface.FailModePolicyID = "policy-surface-override"
	svc := NewServiceWithReaders(Readers{GovernanceMap: &stubReader{items: map[string]*governancemap.Map{"bs-1": m}}})
	p, err := svc.Project(context.Background(), ViewService, "bs-1", MaxDepth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	for _, n := range p.Nodes {
		if strings.Contains(n.Kind, "fail_mode_policy") || strings.Contains(n.Kind, "failmode") {
			t.Errorf("D27j-ui-2a: no node kind may reference fail_mode_policy / failmode; got %+v", n)
		}
	}
	for _, e := range p.Edges {
		if strings.Contains(e.Kind, "fail_mode_policy") || strings.Contains(e.Kind, "failmode") {
			t.Errorf("D27j-ui-2a: no edge kind may reference fail_mode_policy / failmode; got %+v", e)
		}
	}
	// Marshal the whole projection and assert the only fail-mode-related
	// key is the additive fail_mode_policy_id reference (no badge,
	// node, or overlay leaks via JSON either).
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, forbidden := range []string{
		`"fail_mode_policy_node"`,
		`"fail_mode_policy_edge"`,
		`"FAIL_MODE_POLICY_RESOLVED"`,
		`"effective_fail_mode_policy"`,
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("D27j-ui-2a: forbidden key %q present in projection JSON", forbidden)
		}
	}
}
