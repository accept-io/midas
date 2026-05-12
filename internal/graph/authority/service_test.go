package authoritygraph

// service_test.go — projection algorithm unit tests.
//
// All tests use stub readers backed by simple maps; the authority
// repos are exercised indirectly via the seeded test
// (service_seeded_test.go). These tests pin: full-chain traversal,
// per-stage filtering (active surfaces / active profiles / active
// grants), defensive skip on missing agents and missing active
// fail-mode-policies, dedupe of shared agents and shared policies,
// deterministic sort order, view/id/depth validation, and depth=0
// behaviour.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/escalation"
	"github.com/accept-io/midas/internal/failmode"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// Stub readers
// ---------------------------------------------------------------------------

type stubBSReader struct {
	items map[string]*businessservice.BusinessService
	err   error
}

func (s *stubBSReader) GetByID(_ context.Context, id string) (*businessservice.BusinessService, error) {
	if s.err != nil {
		return nil, s.err
	}
	if v, ok := s.items[id]; ok {
		return v, nil
	}
	return nil, nil
}

type stubProcessLister struct {
	byBS map[string][]*process.Process
	err  error
}

func (s *stubProcessLister) ListByBusinessService(_ context.Context, bsID string) ([]*process.Process, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.byBS[bsID], nil
}

type stubSurfaceLister struct {
	byProcess map[string][]*surface.DecisionSurface
	err       error
}

func (s *stubSurfaceLister) ListByProcessID(_ context.Context, processID string) ([]*surface.DecisionSurface, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.byProcess[processID], nil
}

type stubProfileLister struct {
	bySurface map[string][]*authority.AuthorityProfile
	err       error
}

func (s *stubProfileLister) ListBySurface(_ context.Context, surfaceID string) ([]*authority.AuthorityProfile, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.bySurface[surfaceID], nil
}

type stubGrantLister struct {
	byProfile map[string][]*authority.AuthorityGrant
	err       error
}

func (s *stubGrantLister) ListByProfile(_ context.Context, profileID string) ([]*authority.AuthorityGrant, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.byProfile[profileID], nil
}

type stubAgentReader struct {
	items map[string]*agent.Agent
	err   error
}

func (s *stubAgentReader) GetByID(_ context.Context, id string) (*agent.Agent, error) {
	if s.err != nil {
		return nil, s.err
	}
	if v, ok := s.items[id]; ok {
		return v, nil
	}
	return nil, nil
}

type stubFailModeResolver struct {
	items map[string]*failmode.FailModePolicy
	err   error
}

func (s *stubFailModeResolver) FindActiveAt(_ context.Context, id string, _ time.Time) (*failmode.FailModePolicy, error) {
	if s.err != nil {
		return nil, s.err
	}
	if v, ok := s.items[id]; ok {
		return v, nil
	}
	return nil, nil
}

// stubEscalationTargetResolver mirrors stubFailModeResolver. Empty
// items maps to (nil, nil) on lookup — the projection treats that as
// a dangling reference. err short-circuits the projection.
type stubEscalationTargetResolver struct {
	items map[string]*escalation.EscalationTarget
	err   error
}

func (s *stubEscalationTargetResolver) FindActiveAt(_ context.Context, id string, _ time.Time) (*escalation.EscalationTarget, error) {
	if s.err != nil {
		return nil, s.err
	}
	if v, ok := s.items[id]; ok {
		return v, nil
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Fixture builders
// ---------------------------------------------------------------------------

// fixedNow is the pinned wall-clock used by NewServiceWithClock so
// FindActiveAt resolves deterministically.
var fixedNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// minimalReaders returns a Readers struct with non-nil but empty
// stubs for every dependency. Tests then plant data into the
// fields they want to exercise.
type fixture struct {
	bs       *stubBSReader
	procs    *stubProcessLister
	surfs    *stubSurfaceLister
	profiles *stubProfileLister
	grants   *stubGrantLister
	agents   *stubAgentReader
	fmpols   *stubFailModeResolver
	etgts    *stubEscalationTargetResolver
}

func newFixture() *fixture {
	return &fixture{
		bs:       &stubBSReader{items: map[string]*businessservice.BusinessService{}},
		procs:    &stubProcessLister{byBS: map[string][]*process.Process{}},
		surfs:    &stubSurfaceLister{byProcess: map[string][]*surface.DecisionSurface{}},
		profiles: &stubProfileLister{bySurface: map[string][]*authority.AuthorityProfile{}},
		grants:   &stubGrantLister{byProfile: map[string][]*authority.AuthorityGrant{}},
		agents:   &stubAgentReader{items: map[string]*agent.Agent{}},
		fmpols:   &stubFailModeResolver{items: map[string]*failmode.FailModePolicy{}},
		etgts:    &stubEscalationTargetResolver{items: map[string]*escalation.EscalationTarget{}},
	}
}

func (f *fixture) readers() Readers {
	return Readers{
		BusinessServices:  f.bs,
		Processes:         f.procs,
		Surfaces:          f.surfs,
		Profiles:          f.profiles,
		Grants:            f.grants,
		Agents:            f.agents,
		FailModePolicies:  f.fmpols,
		EscalationTargets: f.etgts,
	}
}

func (f *fixture) service() *Service {
	return NewServiceWithClock(f.readers(), func() time.Time { return fixedNow })
}

func makeBS(id, name string) *businessservice.BusinessService {
	return &businessservice.BusinessService{
		ID: id, Name: name, Status: "active",
		ServiceType: businessservice.ServiceTypeInternal,
		OwnerID:     "ops",
	}
}

func makeProc(id, bsID string) *process.Process {
	return &process.Process{ID: id, Name: "proc " + id, BusinessServiceID: bsID, Status: "active"}
}

func makeSurface(id, processID string) *surface.DecisionSurface {
	return &surface.DecisionSurface{
		ID: id, Version: 1, Name: "surface " + id, ProcessID: processID,
		Status: surface.SurfaceStatusActive,
	}
}

func makeProfile(id, surfaceID string) *authority.AuthorityProfile {
	return &authority.AuthorityProfile{
		ID: id, Version: 1, SurfaceID: surfaceID, Name: "profile " + id,
		Status:              authority.ProfileStatusActive,
		ConfidenceThreshold: 0.8,
		EscalationMode:      authority.EscalationModeAuto,
		FailMode:            authority.FailModeClosed,
	}
}

func makeGrant(id, profileID, agentID string) *authority.AuthorityGrant {
	return &authority.AuthorityGrant{
		ID: id, ProfileID: profileID, AgentID: agentID,
		Status: authority.GrantStatusActive,
	}
}

func makeAgent(id, name string) *agent.Agent {
	return &agent.Agent{
		ID: id, Name: name, Type: agent.AgentTypeAI,
		OperationalState: agent.OperationalStateActive,
	}
}

func makePolicy(id, name string) *failmode.FailModePolicy {
	return &failmode.FailModePolicy{
		ID: id, Version: 1, Name: name,
		Status:  failmode.FailModePolicyStatus("active"),
		Origin:  "manual",
		Managed: true,
		Rules: []failmode.FailModePolicyRule{
			{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity},
			{CorrectnessClass: failmode.CorrectnessClassInput},
		},
	}
}

// makeEscalationTarget builds an active role-kind EscalationTarget
// suitable for D31l projection tests. Effective date is one hour
// before fixedNow so FindActiveAt resolves at the pinned clock.
func makeEscalationTarget(id, name string) *escalation.EscalationTarget {
	return &escalation.EscalationTarget{
		ID:            id,
		Version:       1,
		Name:          name,
		Kind:          escalation.KindRole,
		Handle:        "governance." + id,
		Status:        escalation.StatusActive,
		EffectiveDate: fixedNow.Add(-time.Hour),
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func countKind(nodes []Node, kind string) int {
	n := 0
	for _, x := range nodes {
		if x.Kind == kind {
			n++
		}
	}
	return n
}

func countEdgeKind(edges []Edge, kind string) int {
	n := 0
	for _, e := range edges {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Happy path
// ---------------------------------------------------------------------------

func TestAuthorityGraph_Project_ServiceRoot_FullChain(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-1")}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent One")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got.Root.Kind != NodeKindBusinessService || got.Root.ID != "bs-1" {
		t.Errorf("root: %+v", got.Root)
	}
	if got.View != ViewService {
		t.Errorf("view: %q", got.View)
	}

	// All five chain kinds must be present (no policy in this case).
	for _, kind := range []string{
		NodeKindBusinessService, NodeKindDecisionSurface,
		NodeKindAuthorityProfile, NodeKindAuthorityGrant, NodeKindAgent,
	} {
		if countKind(got.Nodes, kind) < 1 {
			t.Errorf("missing node kind %q in projection; got %+v", kind, got.Nodes)
		}
	}
	// Edges: 4 of the 6 kinds (no fail-mode in this fixture).
	for _, kind := range []string{
		EdgeKindBusinessServiceHasSurface, EdgeKindSurfaceUsesProfile,
		EdgeKindProfileHasGrant, EdgeKindGrantAuthorisesAgent,
	} {
		if countEdgeKind(got.Edges, kind) < 1 {
			t.Errorf("missing edge kind %q; got %+v", kind, got.Edges)
		}
	}
}

func TestAuthorityGraph_Project_ServiceRoot_NoSurfaces(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	// No processes for bs-1 — empty graph.

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(got.Nodes) != 1 {
		t.Errorf("want 1 root node, got %d: %+v", len(got.Nodes), got.Nodes)
	}
	if got.Nodes[0].Kind != NodeKindBusinessService {
		t.Errorf("root must be business_service; got %+v", got.Nodes[0])
	}
	if len(got.Edges) != 0 {
		t.Errorf("want 0 edges, got %d: %+v", len(got.Edges), got.Edges)
	}
}

func TestAuthorityGraph_Project_SurfaceWithNoProfiles(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	// No profiles attached to surf-1.

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindAuthorityProfile) != 0 {
		t.Errorf("must NOT emit profile nodes; got %d", countKind(got.Nodes, NodeKindAuthorityProfile))
	}
	if countKind(got.Nodes, NodeKindDecisionSurface) != 1 {
		t.Errorf("surface node missing; got %+v", got.Nodes)
	}
	if countEdgeKind(got.Edges, EdgeKindBusinessServiceHasSurface) != 1 {
		t.Errorf("BS→Surface edge missing; got %+v", got.Edges)
	}
}

func TestAuthorityGraph_Project_ProfileWithNoGrants(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	// No grants attached to prof-1.

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindAuthorityGrant) != 0 {
		t.Errorf("must NOT emit grant nodes; got %d", countKind(got.Nodes, NodeKindAuthorityGrant))
	}
	if countKind(got.Nodes, NodeKindAgent) != 0 {
		t.Errorf("must NOT emit agent nodes when no grants; got %d", countKind(got.Nodes, NodeKindAgent))
	}
	if countKind(got.Nodes, NodeKindAuthorityProfile) != 1 {
		t.Errorf("profile node missing; got %+v", got.Nodes)
	}
}

// ---------------------------------------------------------------------------
// Missing-reference handling
// ---------------------------------------------------------------------------

func TestAuthorityGraph_Project_GrantWithMissingAgent(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-missing")}
	// Note: agent-missing is intentionally not added.

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindAuthorityGrant) != 1 {
		t.Errorf("grant node must still be emitted; got %+v", got.Nodes)
	}
	if countKind(got.Nodes, NodeKindAgent) != 0 {
		t.Errorf("agent node must be skipped when GetByID returns (nil, nil); got %d", countKind(got.Nodes, NodeKindAgent))
	}
	if countEdgeKind(got.Edges, EdgeKindGrantAuthorisesAgent) != 0 {
		t.Errorf("grant_authorises_agent edge must be skipped; got %d", countEdgeKind(got.Edges, EdgeKindGrantAuthorisesAgent))
	}
}

func TestAuthorityGraph_Project_DedupeSharedAgent(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{
		makeSurface("surf-1", "proc-1"),
		makeSurface("surf-2", "proc-1"),
	}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	f.profiles.bySurface["surf-2"] = []*authority.AuthorityProfile{makeProfile("prof-2", "surf-2")}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-shared")}
	f.grants.byProfile["prof-2"] = []*authority.AuthorityGrant{makeGrant("grant-2", "prof-2", "agent-shared")}
	f.agents.items["agent-shared"] = makeAgent("agent-shared", "Shared Agent")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindAgent) != 1 {
		t.Errorf("shared agent must be deduped; got %d agent nodes", countKind(got.Nodes, NodeKindAgent))
	}
	if countEdgeKind(got.Edges, EdgeKindGrantAuthorisesAgent) != 2 {
		t.Errorf("two grants → same agent must emit two edges; got %d", countEdgeKind(got.Edges, EdgeKindGrantAuthorisesAgent))
	}
}

// ---------------------------------------------------------------------------
// Fail-mode policy edges + labels
// ---------------------------------------------------------------------------

func TestAuthorityGraph_Project_BusinessServiceFailModeDefault(t *testing.T) {
	f := newFixture()
	bs := makeBS("bs-1", "BS One")
	bs.FailModePolicyID = "fmp-default"
	f.bs.items["bs-1"] = bs
	f.fmpols.items["fmp-default"] = makePolicy("fmp-default", "Default Policy")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindFailModePolicy) != 1 {
		t.Errorf("policy node missing; got %+v", got.Nodes)
	}
	if countEdgeKind(got.Edges, EdgeKindBusinessServiceHasFailModePolicy) != 1 {
		t.Errorf("BS-has-policy edge missing; got %+v", got.Edges)
	}
	// Edge must carry the "default" label.
	for _, e := range got.Edges {
		if e.Kind == EdgeKindBusinessServiceHasFailModePolicy && e.Label != EdgeLabelDefault {
			t.Errorf("BS-has-policy edge must carry label %q; got %q", EdgeLabelDefault, e.Label)
		}
	}
}

func TestAuthorityGraph_Project_SurfaceFailModeOverride(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	surf := makeSurface("surf-1", "proc-1")
	surf.FailModePolicyID = "fmp-override"
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{surf}
	f.fmpols.items["fmp-override"] = makePolicy("fmp-override", "Override Policy")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindFailModePolicy) != 1 {
		t.Errorf("policy node missing; got %+v", got.Nodes)
	}
	if countEdgeKind(got.Edges, EdgeKindSurfaceHasFailModePolicy) != 1 {
		t.Errorf("surface-has-policy edge missing; got %+v", got.Edges)
	}
	for _, e := range got.Edges {
		if e.Kind == EdgeKindSurfaceHasFailModePolicy && e.Label != EdgeLabelOverride {
			t.Errorf("surface-has-policy edge must carry label %q; got %q", EdgeLabelOverride, e.Label)
		}
	}
}

func TestAuthorityGraph_Project_FailModePolicyDedupe_BothEdges(t *testing.T) {
	// Same policy id referenced by both BS default and surface
	// override. One policy node, two edges with different labels.
	f := newFixture()
	bs := makeBS("bs-1", "BS One")
	bs.FailModePolicyID = "fmp-shared"
	f.bs.items["bs-1"] = bs
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	surf := makeSurface("surf-1", "proc-1")
	surf.FailModePolicyID = "fmp-shared"
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{surf}
	f.fmpols.items["fmp-shared"] = makePolicy("fmp-shared", "Shared Policy")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindFailModePolicy) != 1 {
		t.Errorf("shared policy must be deduped; got %d policy nodes", countKind(got.Nodes, NodeKindFailModePolicy))
	}
	if countEdgeKind(got.Edges, EdgeKindBusinessServiceHasFailModePolicy) != 1 {
		t.Errorf("BS edge missing")
	}
	if countEdgeKind(got.Edges, EdgeKindSurfaceHasFailModePolicy) != 1 {
		t.Errorf("Surface edge missing")
	}
}

func TestAuthorityGraph_Project_FailModePolicyMissingActiveVersion(t *testing.T) {
	f := newFixture()
	bs := makeBS("bs-1", "BS One")
	bs.FailModePolicyID = "fmp-no-active"
	f.bs.items["bs-1"] = bs
	// Note: fmp-no-active intentionally not present in resolver — simulates
	// "policy id exists in the reference but no active version available".

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindFailModePolicy) != 0 {
		t.Errorf("policy node must NOT be emitted; got %d", countKind(got.Nodes, NodeKindFailModePolicy))
	}
	if countEdgeKind(got.Edges, EdgeKindBusinessServiceHasFailModePolicy) != 0 {
		t.Errorf("BS-has-policy edge must NOT be emitted; got %d", countEdgeKind(got.Edges, EdgeKindBusinessServiceHasFailModePolicy))
	}
}

// ---------------------------------------------------------------------------
// Filter pins — inactive surfaces / profiles / grants
// ---------------------------------------------------------------------------

func TestAuthorityGraph_Project_InactiveSurfacesFiltered(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	inactive := makeSurface("surf-deprecated", "proc-1")
	inactive.Status = surface.SurfaceStatusDeprecated
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{inactive}

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindDecisionSurface) != 0 {
		t.Errorf("deprecated surfaces must be filtered; got %d", countKind(got.Nodes, NodeKindDecisionSurface))
	}
}

func TestAuthorityGraph_Project_InactiveProfilesFiltered(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	deprecated := makeProfile("prof-deprecated", "surf-1")
	deprecated.Status = authority.ProfileStatusDeprecated
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{deprecated}

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindAuthorityProfile) != 0 {
		t.Errorf("non-active profiles must be filtered; got %d", countKind(got.Nodes, NodeKindAuthorityProfile))
	}
}

func TestAuthorityGraph_Project_InactiveGrantsFiltered(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	revoked := makeGrant("grant-revoked", "prof-1", "agent-1")
	revoked.Status = authority.GrantStatusRevoked
	suspended := makeGrant("grant-suspended", "prof-1", "agent-1")
	suspended.Status = authority.GrantStatusSuspended
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{revoked, suspended}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent One")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindAuthorityGrant) != 0 {
		t.Errorf("non-active grants must be filtered; got %d", countKind(got.Nodes, NodeKindAuthorityGrant))
	}
}

// ---------------------------------------------------------------------------
// Deterministic ordering
// ---------------------------------------------------------------------------

func TestAuthorityGraph_Project_DeterministicOrdering(t *testing.T) {
	// Two surfaces, two profiles each, deliberately seeded in
	// non-sorted order. Expect output sorted by (Kind, ID).
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{
		makeSurface("surf-b", "proc-1"),
		makeSurface("surf-a", "proc-1"),
	}
	f.profiles.bySurface["surf-a"] = []*authority.AuthorityProfile{
		makeProfile("prof-z", "surf-a"),
		makeProfile("prof-y", "surf-a"),
	}
	f.profiles.bySurface["surf-b"] = []*authority.AuthorityProfile{
		makeProfile("prof-x", "surf-b"),
	}

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	// Profile-kind nodes must appear in ID order: prof-x, prof-y, prof-z.
	var profIDs []string
	for _, n := range got.Nodes {
		if n.Kind == NodeKindAuthorityProfile {
			profIDs = append(profIDs, n.ID)
		}
	}
	want := []string{"prof-x", "prof-y", "prof-z"}
	for i, w := range want {
		if i >= len(profIDs) || profIDs[i] != w {
			t.Errorf("profile node ordering: want %v, got %v", want, profIDs)
			break
		}
	}
}

// ---------------------------------------------------------------------------
// Validation errors
// ---------------------------------------------------------------------------

func TestAuthorityGraph_Project_UnsupportedView(t *testing.T) {
	f := newFixture()
	_, err := f.service().Project(context.Background(), "agent", "bs-1", DefaultDepth)
	if !errors.Is(err, ErrInvalidView) {
		t.Errorf("want ErrInvalidView, got %v", err)
	}
}

func TestAuthorityGraph_Project_UnknownService(t *testing.T) {
	f := newFixture()
	_, err := f.service().Project(context.Background(), ViewService, "bs-missing", DefaultDepth)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestAuthorityGraph_Project_EmptyID(t *testing.T) {
	f := newFixture()
	_, err := f.service().Project(context.Background(), ViewService, "", DefaultDepth)
	if !errors.Is(err, ErrInvalidID) {
		t.Errorf("want ErrInvalidID, got %v", err)
	}
}

func TestAuthorityGraph_Project_NegativeDepth(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	_, err := f.service().Project(context.Background(), ViewService, "bs-1", -1)
	if !errors.Is(err, ErrInvalidDepth) {
		t.Errorf("want ErrInvalidDepth, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Depth semantics
// ---------------------------------------------------------------------------

func TestAuthorityGraph_Project_DepthZero_RootOnly(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", 0)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].Kind != NodeKindBusinessService {
		t.Errorf("depth=0: want root only; got %+v", got.Nodes)
	}
	if len(got.Edges) != 0 {
		t.Errorf("depth=0: want zero edges; got %+v", got.Edges)
	}
}

func TestAuthorityGraph_Project_DepthClamping(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", 9999)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got.Depth != MaxDepth {
		t.Errorf("depth clamping: want %d, got %d", MaxDepth, got.Depth)
	}
}

// ---------------------------------------------------------------------------
// Repository error propagation
// ---------------------------------------------------------------------------

func TestAuthorityGraph_Project_RepoError(t *testing.T) {
	f := newFixture()
	f.bs.err = errors.New("boom: repo dead")
	_, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err == nil {
		t.Fatal("want error from repo")
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("repo error must not be conflated with ErrNotFound; got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Unconfigured Service — projector registration guard
// ---------------------------------------------------------------------------

func TestAuthorityGraph_NewService_MissingReader_DropsViewService(t *testing.T) {
	// Omit Agents reader — ViewService projector must NOT register.
	r := Readers{
		BusinessServices: &stubBSReader{items: map[string]*businessservice.BusinessService{}},
		Processes:        &stubProcessLister{},
		Surfaces:         &stubSurfaceLister{},
		Profiles:         &stubProfileLister{},
		Grants:           &stubGrantLister{},
		// Agents intentionally nil
		FailModePolicies: &stubFailModeResolver{},
	}
	svc := NewServiceWithClock(r, func() time.Time { return fixedNow })
	_, err := svc.Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if !errors.Is(err, ErrInvalidView) {
		t.Errorf("missing reader must disable ViewService → ErrInvalidView; got %v", err)
	}
}

// ---------------------------------------------------------------------------
// D31g — Effective-authority filtering, summary, diagnostics
// ---------------------------------------------------------------------------

// Helpers for D31g tests.

func hasDiagnostic(diags []Diagnostic, kind string) bool {
	for _, d := range diags {
		if d.Kind == kind {
			return true
		}
	}
	return false
}

func countDiagnostics(diags []Diagnostic, kind string) int {
	n := 0
	for _, d := range diags {
		if d.Kind == kind {
			n++
		}
	}
	return n
}

func diagnosticsBySeverity(diags []Diagnostic, sev string) []Diagnostic {
	var out []Diagnostic
	for _, d := range diags {
		if d.Severity == sev {
			out = append(out, d)
		}
	}
	return out
}

// ---------- Effective-time filtering ----------

func TestAuthorityGraph_Project_ExpiredProfile_FilteredWithDiagnostic(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}

	// Profile is status=active but EffectiveUntil is in the past.
	expired := makeProfile("prof-expired", "surf-1")
	expiredUntil := fixedNow.Add(-1 * time.Hour)
	expired.EffectiveUntil = &expiredUntil
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{expired}

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindAuthorityProfile) != 0 {
		t.Errorf("expired profile must NOT be emitted; got %d", countKind(got.Nodes, NodeKindAuthorityProfile))
	}
	if !hasDiagnostic(got.Diagnostics, DiagnosticKindProfileExpired) {
		t.Errorf("expected profile_expired diagnostic; got %+v", got.Diagnostics)
	}
}

func TestAuthorityGraph_Project_FutureDatedProfile_FilteredWithDiagnostic(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}

	future := makeProfile("prof-future", "surf-1")
	future.EffectiveDate = fixedNow.Add(24 * time.Hour)
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{future}

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindAuthorityProfile) != 0 {
		t.Errorf("future-dated profile must NOT be emitted; got %d", countKind(got.Nodes, NodeKindAuthorityProfile))
	}
	if !hasDiagnostic(got.Diagnostics, DiagnosticKindProfileFutureDated) {
		t.Errorf("expected profile_future_dated diagnostic; got %+v", got.Diagnostics)
	}
}

func TestAuthorityGraph_Project_ExpiredGrant_FilteredWithDiagnostic(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}

	expired := makeGrant("grant-expired", "prof-1", "agent-1")
	expiredAt := fixedNow.Add(-1 * time.Hour)
	expired.ExpiresAt = &expiredAt
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{expired}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent One")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindAuthorityGrant) != 0 {
		t.Errorf("expired grant must NOT be emitted; got %d", countKind(got.Nodes, NodeKindAuthorityGrant))
	}
	if !hasDiagnostic(got.Diagnostics, DiagnosticKindGrantExpired) {
		t.Errorf("expected grant_expired diagnostic; got %+v", got.Diagnostics)
	}
}

func TestAuthorityGraph_Project_FutureDatedGrant_FilteredWithDiagnostic(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}

	future := makeGrant("grant-future", "prof-1", "agent-1")
	future.EffectiveDate = fixedNow.Add(24 * time.Hour)
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{future}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent One")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindAuthorityGrant) != 0 {
		t.Errorf("future-dated grant must NOT be emitted; got %d", countKind(got.Nodes, NodeKindAuthorityGrant))
	}
	if !hasDiagnostic(got.Diagnostics, DiagnosticKindGrantFutureDated) {
		t.Errorf("expected grant_future_dated diagnostic; got %+v", got.Diagnostics)
	}
}

// ---------- Completeness diagnostics ----------

func TestAuthorityGraph_Diagnostic_BusinessServiceHasNoActiveSurface(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	// No processes / surfaces.

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if !hasDiagnostic(got.Diagnostics, DiagnosticKindBusinessServiceHasNoActiveSurface) {
		t.Errorf("expected business_service_has_no_active_surface diagnostic; got %+v", got.Diagnostics)
	}
}

func TestAuthorityGraph_Diagnostic_SurfaceHasNoActiveProfile(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	// No profiles.

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if !hasDiagnostic(got.Diagnostics, DiagnosticKindSurfaceHasNoActiveProfile) {
		t.Errorf("expected surface_has_no_active_profile diagnostic; got %+v", got.Diagnostics)
	}
	if got.Summary == nil || len(got.Summary.SurfacesWithoutProfiles) != 1 {
		t.Errorf("expected SurfacesWithoutProfiles len=1; got %+v", got.Summary)
	}
}

func TestAuthorityGraph_Diagnostic_ProfileHasNoActiveGrant(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	// No grants.

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if !hasDiagnostic(got.Diagnostics, DiagnosticKindProfileHasNoActiveGrant) {
		t.Errorf("expected profile_has_no_active_grant diagnostic; got %+v", got.Diagnostics)
	}
	if got.Summary == nil || len(got.Summary.ProfilesWithoutGrants) != 1 {
		t.Errorf("expected ProfilesWithoutGrants len=1; got %+v", got.Summary)
	}
}

func TestAuthorityGraph_Diagnostic_GrantReferencesMissingAgent(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-missing")}
	// agent-missing not present in agent store.

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if !hasDiagnostic(got.Diagnostics, DiagnosticKindGrantReferencesMissingAgent) {
		t.Errorf("expected grant_references_missing_agent diagnostic; got %+v", got.Diagnostics)
	}
	// Diagnostic must be critical severity.
	for _, d := range got.Diagnostics {
		if d.Kind == DiagnosticKindGrantReferencesMissingAgent && d.Severity != DiagnosticSeverityCritical {
			t.Errorf("missing-agent diagnostic must be critical; got %q", d.Severity)
		}
	}
	if got.Summary == nil || len(got.Summary.GrantsWithoutAgents) != 1 {
		t.Errorf("expected GrantsWithoutAgents len=1; got %+v", got.Summary)
	}
	// Grant node still emitted (defensive skip retains the grant).
	if countKind(got.Nodes, NodeKindAuthorityGrant) != 1 {
		t.Errorf("grant node must still be emitted; got %d", countKind(got.Nodes, NodeKindAuthorityGrant))
	}
}

func TestAuthorityGraph_Diagnostic_GrantReferencesInactiveAgent(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-suspended")}
	suspended := makeAgent("agent-suspended", "Suspended Agent")
	suspended.OperationalState = agent.OperationalStateSuspended
	f.agents.items["agent-suspended"] = suspended

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if !hasDiagnostic(got.Diagnostics, DiagnosticKindGrantReferencesInactiveAgent) {
		t.Errorf("expected grant_references_inactive_agent diagnostic; got %+v", got.Diagnostics)
	}
	// D31j: severity bumped from warning to critical because the
	// orchestrator now treats non-active operational state as a hard
	// runtime authority gate. The diagnostic still emits the agent
	// node + edge — Authority Graph remains evidence-only.
	for _, d := range got.Diagnostics {
		if d.Kind == DiagnosticKindGrantReferencesInactiveAgent && d.Severity != DiagnosticSeverityCritical {
			t.Errorf("inactive-agent diagnostic must be critical (D31j); got %q", d.Severity)
		}
	}
	if countKind(got.Nodes, NodeKindAgent) != 1 {
		t.Errorf("inactive agent must still be emitted; got %d", countKind(got.Nodes, NodeKindAgent))
	}
}

// TestAuthorityGraph_Diagnostic_GrantReferencesRevokedAgent pins the
// D31j severity contract for revoked agents: same critical-severity
// diagnostic kind as suspended agents.
func TestAuthorityGraph_Diagnostic_GrantReferencesRevokedAgent(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-revoked")}
	revoked := makeAgent("agent-revoked", "Revoked Agent")
	revoked.OperationalState = agent.OperationalStateRevoked
	f.agents.items["agent-revoked"] = revoked

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	found := false
	for _, d := range got.Diagnostics {
		if d.Kind != DiagnosticKindGrantReferencesInactiveAgent {
			continue
		}
		found = true
		if d.Severity != DiagnosticSeverityCritical {
			t.Errorf("revoked-agent inactive-agent diagnostic must be critical; got %q", d.Severity)
		}
	}
	if !found {
		t.Errorf("expected grant_references_inactive_agent diagnostic for revoked agent; got %+v", got.Diagnostics)
	}
}

// TestAuthorityGraph_Diagnostic_ActiveAgent_NoInactiveDiagnostic pins
// the absence of grant_references_inactive_agent on a healthy chain
// with an active agent — guards against a regression that would emit
// a critical diagnostic against every grant.
func TestAuthorityGraph_Diagnostic_ActiveAgent_NoInactiveDiagnostic(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	healthy := makeGrant("grant-1", "prof-1", "agent-1")
	healthy.Capabilities = []authority.Capability{authority.CapabilityApprove}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{healthy}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent One")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	for _, d := range got.Diagnostics {
		if d.Kind == DiagnosticKindGrantReferencesInactiveAgent {
			t.Errorf("active agent must not produce inactive-agent diagnostic; got %+v", d)
		}
	}
}

// ---------- Fail-mode policy diagnostics ----------

func TestAuthorityGraph_Diagnostic_FailModePolicyReferenceDangling_BusinessService(t *testing.T) {
	f := newFixture()
	bs := makeBS("bs-1", "BS One")
	bs.FailModePolicyID = "fmp-dangling"
	f.bs.items["bs-1"] = bs
	// fmp-dangling not in resolver — FindActiveAt will return (nil, nil).

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if !hasDiagnostic(got.Diagnostics, DiagnosticKindFailModePolicyReferenceDangling) {
		t.Errorf("expected fail_mode_policy_reference_dangling diagnostic; got %+v", got.Diagnostics)
	}
	if got.Summary == nil || len(got.Summary.PoliciesMissingActiveVersion) != 1 {
		t.Errorf("expected PoliciesMissingActiveVersion len=1; got %+v", got.Summary)
	}
}

func TestAuthorityGraph_Diagnostic_FailModePolicyReferenceDangling_Surface(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	surf := makeSurface("surf-1", "proc-1")
	surf.FailModePolicyID = "fmp-dangling-surf"
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{surf}

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if !hasDiagnostic(got.Diagnostics, DiagnosticKindFailModePolicyReferenceDangling) {
		t.Errorf("expected fail_mode_policy_reference_dangling diagnostic for surface; got %+v", got.Diagnostics)
	}
	if got.Summary == nil || len(got.Summary.PoliciesMissingActiveVersion) != 1 {
		t.Errorf("expected PoliciesMissingActiveVersion len=1; got %+v", got.Summary)
	}
}

func TestAuthorityGraph_Diagnostic_SurfaceInheritsBusinessServicePolicy(t *testing.T) {
	f := newFixture()
	bs := makeBS("bs-1", "BS One")
	bs.FailModePolicyID = "fmp-bs-default"
	f.bs.items["bs-1"] = bs
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	// Surface has NO override.
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.fmpols.items["fmp-bs-default"] = makePolicy("fmp-bs-default", "BS Default")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if !hasDiagnostic(got.Diagnostics, DiagnosticKindSurfaceInheritsBusinessServicePolicy) {
		t.Errorf("expected surface_inherits_business_service_policy diagnostic; got %+v", got.Diagnostics)
	}
	if got.Summary == nil || got.Summary.SurfacesInheritingBSPolicy != 1 {
		t.Errorf("expected SurfacesInheritingBSPolicy=1; got %+v", got.Summary)
	}
}

func TestAuthorityGraph_Diagnostic_SurfaceOverridesBusinessServiceDefault(t *testing.T) {
	f := newFixture()
	bs := makeBS("bs-1", "BS One")
	bs.FailModePolicyID = "fmp-default"
	f.bs.items["bs-1"] = bs
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	surf := makeSurface("surf-1", "proc-1")
	surf.FailModePolicyID = "fmp-override"
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{surf}
	f.fmpols.items["fmp-default"] = makePolicy("fmp-default", "Default")
	f.fmpols.items["fmp-override"] = makePolicy("fmp-override", "Override")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if !hasDiagnostic(got.Diagnostics, DiagnosticKindSurfaceOverridesBusinessServiceDefault) {
		t.Errorf("expected surface_overrides_business_service_default diagnostic; got %+v", got.Diagnostics)
	}
	if got.Summary == nil || got.Summary.SurfacesWithPolicyOverride != 1 {
		t.Errorf("expected SurfacesWithPolicyOverride=1; got %+v", got.Summary)
	}
}

func TestAuthorityGraph_Diagnostic_SurfaceOverrideMatchesBusinessServiceDefault(t *testing.T) {
	f := newFixture()
	bs := makeBS("bs-1", "BS One")
	bs.FailModePolicyID = "fmp-shared"
	f.bs.items["bs-1"] = bs
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	surf := makeSurface("surf-1", "proc-1")
	surf.FailModePolicyID = "fmp-shared"
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{surf}
	f.fmpols.items["fmp-shared"] = makePolicy("fmp-shared", "Shared")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if !hasDiagnostic(got.Diagnostics, DiagnosticKindSurfaceOverrideMatchesBSDefault) {
		t.Errorf("expected surface_override_matches_business_service_default diagnostic; got %+v", got.Diagnostics)
	}
}

func TestAuthorityGraph_Diagnostic_DuplicateActiveProfileVersionsForSurface(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	// Two ACTIVE versions of the same logical profile id on the same
	// surface — invariant violation; stub data exercises it.
	v1 := makeProfile("prof-shared", "surf-1")
	v1.Version = 1
	v2 := makeProfile("prof-shared", "surf-1")
	v2.Version = 2
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{v1, v2}

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if !hasDiagnostic(got.Diagnostics, DiagnosticKindDuplicateActiveProfileVersionsForSurface) {
		t.Errorf("expected duplicate_active_profile_versions_for_surface diagnostic; got %+v", got.Diagnostics)
	}
	for _, d := range got.Diagnostics {
		if d.Kind == DiagnosticKindDuplicateActiveProfileVersionsForSurface && d.Severity != DiagnosticSeverityCritical {
			t.Errorf("duplicate diagnostic must be critical; got %q", d.Severity)
		}
	}
}

// ---------- Summary ----------

func TestAuthorityGraph_Summary_FullChain(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-1")}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent One")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got.Summary == nil {
		t.Fatal("Summary must be non-nil for full chain")
	}
	sm := got.Summary
	if sm.SurfaceCount != 1 {
		t.Errorf("SurfaceCount: want 1, got %d", sm.SurfaceCount)
	}
	if sm.ActiveProfileCount != 1 {
		t.Errorf("ActiveProfileCount: want 1, got %d", sm.ActiveProfileCount)
	}
	if sm.ActiveGrantCount != 1 {
		t.Errorf("ActiveGrantCount: want 1, got %d", sm.ActiveGrantCount)
	}
	if sm.ActiveAgentCount != 1 {
		t.Errorf("ActiveAgentCount: want 1, got %d", sm.ActiveAgentCount)
	}
	if sm.CompleteAuthorityPaths != 1 {
		t.Errorf("CompleteAuthorityPaths: want 1, got %d", sm.CompleteAuthorityPaths)
	}
	if sm.IncompleteAuthorityPaths != 0 {
		t.Errorf("IncompleteAuthorityPaths: want 0, got %d", sm.IncompleteAuthorityPaths)
	}
}

func TestAuthorityGraph_Summary_EmptyService(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	// No processes / surfaces.

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got.Summary == nil {
		t.Fatal("Summary must be non-nil even for empty service")
	}
	if got.Summary.SurfaceCount != 0 {
		t.Errorf("SurfaceCount: want 0, got %d", got.Summary.SurfaceCount)
	}
	if got.Summary.CompleteAuthorityPaths != 0 || got.Summary.IncompleteAuthorityPaths != 0 {
		t.Errorf("empty service must have zero paths; got complete=%d incomplete=%d", got.Summary.CompleteAuthorityPaths, got.Summary.IncompleteAuthorityPaths)
	}
}

func TestAuthorityGraph_Summary_SharedAgentDeduped(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{
		makeSurface("surf-1", "proc-1"),
		makeSurface("surf-2", "proc-1"),
	}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	f.profiles.bySurface["surf-2"] = []*authority.AuthorityProfile{makeProfile("prof-2", "surf-2")}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-shared")}
	f.grants.byProfile["prof-2"] = []*authority.AuthorityGrant{makeGrant("grant-2", "prof-2", "agent-shared")}
	f.agents.items["agent-shared"] = makeAgent("agent-shared", "Shared")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got.Summary == nil {
		t.Fatal("Summary nil")
	}
	if got.Summary.ActiveAgentCount != 1 {
		t.Errorf("agent must be deduped to 1; got ActiveAgentCount=%d", got.Summary.ActiveAgentCount)
	}
	if got.Summary.CompleteAuthorityPaths != 2 {
		t.Errorf("both surfaces have complete paths; want 2, got %d", got.Summary.CompleteAuthorityPaths)
	}
}

func TestAuthorityGraph_Summary_DepthZeroStillReportsFullPosture(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-1")}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent One")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", 0)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(got.Nodes) != 1 {
		t.Errorf("depth=0: want only root node; got %d", len(got.Nodes))
	}
	if got.Summary == nil {
		t.Fatal("Summary must be non-nil at depth=0 (describes full pre-depth posture)")
	}
	if got.Summary.SurfaceCount != 1 {
		t.Errorf("depth=0 summary must reflect full graph; SurfaceCount want 1, got %d", got.Summary.SurfaceCount)
	}
	if got.Summary.CompleteAuthorityPaths != 1 {
		t.Errorf("depth=0 summary CompleteAuthorityPaths: want 1, got %d", got.Summary.CompleteAuthorityPaths)
	}
}

// ---------- Effective policy source on surface ----------

func TestAuthorityGraph_DecisionSurface_EffectivePolicySource_Default(t *testing.T) {
	f := newFixture()
	bs := makeBS("bs-1", "BS One")
	bs.FailModePolicyID = "fmp-default"
	f.bs.items["bs-1"] = bs
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.fmpols.items["fmp-default"] = makePolicy("fmp-default", "Default")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	for _, n := range got.Nodes {
		if n.Kind == NodeKindDecisionSurface {
			if n.DecisionSurface == nil {
				t.Fatal("decision_surface typed data missing")
			}
			if n.DecisionSurface.EffectivePolicySource != EffectivePolicySourceBusinessServiceDefault {
				t.Errorf("EffectivePolicySource: want %q, got %q", EffectivePolicySourceBusinessServiceDefault, n.DecisionSurface.EffectivePolicySource)
			}
			if n.DecisionSurface.EffectivePolicyID != "fmp-default" {
				t.Errorf("EffectivePolicyID: want fmp-default, got %q", n.DecisionSurface.EffectivePolicyID)
			}
			if !n.DecisionSurface.InheritsBSPolicy {
				t.Errorf("InheritsBSPolicy: want true, got false")
			}
		}
	}
}

func TestAuthorityGraph_DecisionSurface_EffectivePolicySource_Override(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	surf := makeSurface("surf-1", "proc-1")
	surf.FailModePolicyID = "fmp-override"
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{surf}
	f.fmpols.items["fmp-override"] = makePolicy("fmp-override", "Override")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	for _, n := range got.Nodes {
		if n.Kind == NodeKindDecisionSurface {
			if n.DecisionSurface.EffectivePolicySource != EffectivePolicySourceOverride {
				t.Errorf("EffectivePolicySource: want %q, got %q", EffectivePolicySourceOverride, n.DecisionSurface.EffectivePolicySource)
			}
			if n.DecisionSurface.EffectivePolicyID != "fmp-override" {
				t.Errorf("EffectivePolicyID: want fmp-override, got %q", n.DecisionSurface.EffectivePolicyID)
			}
			if n.DecisionSurface.InheritsBSPolicy {
				t.Errorf("InheritsBSPolicy: want false, got true")
			}
		}
	}
}

func TestAuthorityGraph_DecisionSurface_EffectivePolicySource_None(t *testing.T) {
	f := newFixture()
	// BS has no default; surface has no override.
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	for _, n := range got.Nodes {
		if n.Kind == NodeKindDecisionSurface {
			if n.DecisionSurface.EffectivePolicySource != EffectivePolicySourceNone {
				t.Errorf("EffectivePolicySource: want %q, got %q", EffectivePolicySourceNone, n.DecisionSurface.EffectivePolicySource)
			}
			if n.DecisionSurface.InheritsBSPolicy {
				t.Errorf("InheritsBSPolicy must be false when source=none; got true")
			}
		}
	}
	if got.Summary == nil || len(got.Summary.SurfacesWithoutEffectiveFailModePolicy) != 1 {
		t.Errorf("expected SurfacesWithoutEffectiveFailModePolicy len=1; got %+v", got.Summary)
	}
}

// ---------- Diagnostic ordering ----------

// TestAuthorityGraph_Diagnostic_Ordering pins the deterministic
// severity-then-kind sort. Fixture deliberately produces one of each
// severity.
func TestAuthorityGraph_Diagnostic_Ordering(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	// Surface inherits BS default (info diagnostic).
	bs := f.bs.items["bs-1"]
	bs.FailModePolicyID = "fmp-default"
	f.fmpols.items["fmp-default"] = makePolicy("fmp-default", "Default")
	// Two surfaces — surf-a has no profiles (warning) and surf-b has
	// a grant with missing agent (critical).
	surfA := makeSurface("surf-a", "proc-1")
	surfB := makeSurface("surf-b", "proc-1")
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{surfA, surfB}
	f.profiles.bySurface["surf-b"] = []*authority.AuthorityProfile{makeProfile("prof-b", "surf-b")}
	f.grants.byProfile["prof-b"] = []*authority.AuthorityGrant{makeGrant("grant-b", "prof-b", "agent-missing")}

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(got.Diagnostics) < 2 {
		t.Fatalf("want at least 2 diagnostics; got %d: %+v", len(got.Diagnostics), got.Diagnostics)
	}
	// First diagnostic must be a critical.
	if got.Diagnostics[0].Severity != DiagnosticSeverityCritical {
		t.Errorf("first diagnostic must be critical; got %q", got.Diagnostics[0].Severity)
	}
	// Severity rank must be monotonically non-decreasing.
	for i := 1; i < len(got.Diagnostics); i++ {
		if severityRank(got.Diagnostics[i-1].Severity) > severityRank(got.Diagnostics[i].Severity) {
			t.Errorf("diagnostics not sorted by severity rank: %+v", got.Diagnostics)
			break
		}
	}
}

// TestAuthorityGraph_NoDiagnostics_HealthyProjection pins that a
// fully-populated effective chain (no inheritance, no missing
// agents) produces zero diagnostics. Operators reading
// diagnostics[] should see an empty/absent array on healthy data.
func TestAuthorityGraph_NoDiagnostics_HealthyProjection(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	surf := makeSurface("surf-1", "proc-1")
	surf.FailModePolicyID = "fmp-override"
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{surf}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	healthyGrant := makeGrant("grant-1", "prof-1", "agent-1")
	// D31i: a healthy projection requires capabilities on every
	// emitted grant — otherwise the grant_has_no_capabilities
	// warning fires.
	healthyGrant.Capabilities = []authority.Capability{authority.CapabilityApprove}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{healthyGrant}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent One")
	f.fmpols.items["fmp-override"] = makePolicy("fmp-override", "Override")
	// Note: no BS default → surface override is the only policy
	// reference; no inheritance / matches / overrides diagnostic.

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(diagnosticsBySeverity(got.Diagnostics, DiagnosticSeverityCritical)) != 0 {
		t.Errorf("healthy projection must have zero critical diagnostics; got %+v", got.Diagnostics)
	}
	if len(diagnosticsBySeverity(got.Diagnostics, DiagnosticSeverityWarning)) != 0 {
		t.Errorf("healthy projection must have zero warning diagnostics; got %+v", got.Diagnostics)
	}
}
