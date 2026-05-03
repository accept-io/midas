package governancemap

// Read service tests for the governance map (Epic 1, PR 4).
//
// Test posture: tests use stub readers backed by simple maps rather
// than wiring full memory repositories. The aggregation logic is the
// thing under test; repository correctness is covered by per-repo
// tests. The stub approach keeps fixtures small and lets each test
// focus on a single aggregation rule.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/businessservicecapability"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// Stub readers
// ---------------------------------------------------------------------------

type stubBSReader struct {
	items map[string]*businessservice.BusinessService
}

func (r *stubBSReader) GetByID(_ context.Context, id string) (*businessservice.BusinessService, error) {
	return r.items[id], nil
}

type stubBSRReader struct {
	items []*businessservice.BusinessServiceRelationship
}

func (r *stubBSRReader) ListBySourceBusinessService(_ context.Context, sourceID string) ([]*businessservice.BusinessServiceRelationship, error) {
	out := []*businessservice.BusinessServiceRelationship{}
	for _, rel := range r.items {
		if rel.SourceBusinessService == sourceID {
			out = append(out, rel)
		}
	}
	return out, nil
}

func (r *stubBSRReader) ListByTargetBusinessService(_ context.Context, targetID string) ([]*businessservice.BusinessServiceRelationship, error) {
	out := []*businessservice.BusinessServiceRelationship{}
	for _, rel := range r.items {
		if rel.TargetBusinessService == targetID {
			out = append(out, rel)
		}
	}
	return out, nil
}

type stubBSCReader struct {
	items []*businessservicecapability.BusinessServiceCapability
}

func (r *stubBSCReader) ListByBusinessServiceID(_ context.Context, bsID string) ([]*businessservicecapability.BusinessServiceCapability, error) {
	out := []*businessservicecapability.BusinessServiceCapability{}
	for _, b := range r.items {
		if b.BusinessServiceID == bsID {
			out = append(out, b)
		}
	}
	return out, nil
}

type stubCapabilityReader struct {
	items map[string]*capability.Capability
}

func (r *stubCapabilityReader) GetByID(_ context.Context, id string) (*capability.Capability, error) {
	return r.items[id], nil
}

type stubProcessReader struct{ items []*process.Process }

func (r *stubProcessReader) ListByBusinessService(_ context.Context, bsID string) ([]*process.Process, error) {
	out := []*process.Process{}
	for _, p := range r.items {
		if p.BusinessServiceID == bsID {
			out = append(out, p)
		}
	}
	return out, nil
}

type stubSurfaceReader struct{ items []*surface.DecisionSurface }

func (r *stubSurfaceReader) ListByProcessID(_ context.Context, procID string) ([]*surface.DecisionSurface, error) {
	out := []*surface.DecisionSurface{}
	for _, s := range r.items {
		if s.ProcessID == procID {
			out = append(out, s)
		}
	}
	return out, nil
}

type stubProfileReader struct{ items []*authority.AuthorityProfile }

func (r *stubProfileReader) ListBySurface(_ context.Context, surfaceID string) ([]*authority.AuthorityProfile, error) {
	out := []*authority.AuthorityProfile{}
	for _, p := range r.items {
		if p.SurfaceID == surfaceID {
			out = append(out, p)
		}
	}
	return out, nil
}

type stubGrantReader struct{ items []*authority.AuthorityGrant }

func (r *stubGrantReader) ListByProfile(_ context.Context, profileID string) ([]*authority.AuthorityGrant, error) {
	out := []*authority.AuthorityGrant{}
	for _, g := range r.items {
		if g.ProfileID == profileID {
			out = append(out, g)
		}
	}
	return out, nil
}

type stubAISystemReader struct{ items map[string]*aisystem.AISystem }

func (r *stubAISystemReader) GetByID(_ context.Context, id string) (*aisystem.AISystem, error) {
	return r.items[id], nil
}

type stubAIVersionReader struct {
	items map[string]*aisystem.AISystemVersion
}

func (r *stubAIVersionReader) GetActiveBySystem(_ context.Context, aiSystemID string) (*aisystem.AISystemVersion, error) {
	return r.items[aiSystemID], nil
}

type stubAIBindingReader struct{ items []*aisystem.AISystemBinding }

func (r *stubAIBindingReader) ListByBusinessService(_ context.Context, bsID string) ([]*aisystem.AISystemBinding, error) {
	out := []*aisystem.AISystemBinding{}
	for _, b := range r.items {
		if b.BusinessServiceID == bsID {
			out = append(out, b)
		}
	}
	return out, nil
}

func (r *stubAIBindingReader) ListByCapability(_ context.Context, capID string) ([]*aisystem.AISystemBinding, error) {
	out := []*aisystem.AISystemBinding{}
	for _, b := range r.items {
		if b.CapabilityID == capID {
			out = append(out, b)
		}
	}
	return out, nil
}

func (r *stubAIBindingReader) ListByProcess(_ context.Context, procID string) ([]*aisystem.AISystemBinding, error) {
	out := []*aisystem.AISystemBinding{}
	for _, b := range r.items {
		if b.ProcessID == procID {
			out = append(out, b)
		}
	}
	return out, nil
}

func (r *stubAIBindingReader) ListBySurface(_ context.Context, surfID string) ([]*aisystem.AISystemBinding, error) {
	out := []*aisystem.AISystemBinding{}
	for _, b := range r.items {
		if b.SurfaceID == surfID {
			out = append(out, b)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Fixture builder
// ---------------------------------------------------------------------------

// fixture wires a ReadService backed by stub readers. Each field is
// directly mutable so tests can populate exactly the rows they need.
type fixture struct {
	bs       *stubBSReader
	bsr      *stubBSRReader
	bsc      *stubBSCReader
	caps     *stubCapabilityReader
	procs    *stubProcessReader
	surfaces *stubSurfaceReader
	profiles *stubProfileReader
	grants   *stubGrantReader
	aiSys    *stubAISystemReader
	aiVer    *stubAIVersionReader
	aiBind   *stubAIBindingReader
}

func newFixture() *fixture {
	return &fixture{
		bs:       &stubBSReader{items: map[string]*businessservice.BusinessService{}},
		bsr:      &stubBSRReader{},
		bsc:      &stubBSCReader{},
		caps:     &stubCapabilityReader{items: map[string]*capability.Capability{}},
		procs:    &stubProcessReader{},
		surfaces: &stubSurfaceReader{},
		profiles: &stubProfileReader{},
		grants:   &stubGrantReader{},
		aiSys:    &stubAISystemReader{items: map[string]*aisystem.AISystem{}},
		aiVer:    &stubAIVersionReader{items: map[string]*aisystem.AISystemVersion{}},
		aiBind:   &stubAIBindingReader{},
	}
}

func (f *fixture) service() *ReadService {
	return NewReadService(f.bs, f.bsr, f.bsc, f.caps, f.procs, f.surfaces,
		f.profiles, f.grants, f.aiSys, f.aiVer, f.aiBind)
}

func makeBS(id, name string) *businessservice.BusinessService {
	return &businessservice.BusinessService{ID: id, Name: name, Status: "active"}
}

// ---------------------------------------------------------------------------
// Empty / not found / unconfigured
// ---------------------------------------------------------------------------

func TestGetGovernanceMap_BSNotFound_ReturnsNilNil(t *testing.T) {
	f := newFixture()
	got, err := f.service().GetGovernanceMap(context.Background(), "missing")
	if err != nil || got != nil {
		t.Errorf("want (nil, nil) for missing BS; got (%+v, %v)", got, err)
	}
}

func TestGetGovernanceMap_EmptyService_AllArraysEmpty_AllCountsZero(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-empty"] = makeBS("bs-empty", "Empty")

	got, err := f.service().GetGovernanceMap(context.Background(), "bs-empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("Map nil")
	}
	if got.BusinessService == nil || got.BusinessService.BusinessService.ID != "bs-empty" {
		t.Errorf("root BS not loaded")
	}
	// Slice fields must be non-nil empty (operators iterate without nil checks).
	if got.Relationships.Outgoing == nil || got.Relationships.Incoming == nil {
		t.Error("Relationships slices must be non-nil")
	}
	if got.Capabilities == nil || got.Processes == nil || got.Surfaces == nil || got.AISystems == nil {
		t.Error("collection slices must be non-nil")
	}
	if len(got.Relationships.Outgoing) != 0 || len(got.Relationships.Incoming) != 0 ||
		len(got.Capabilities) != 0 || len(got.Processes) != 0 ||
		len(got.Surfaces) != 0 || len(got.AISystems) != 0 {
		t.Errorf("empty service should produce empty collections; got %+v", got)
	}
	if got.AuthoritySummary == nil || got.Coverage == nil {
		t.Error("AuthoritySummary and Coverage must be non-nil even for empty service")
	}
	if *got.AuthoritySummary != (AuthoritySummary{}) || *got.Coverage != (Coverage{}) {
		t.Errorf("empty service should produce zero-valued summaries; got %+v / %+v",
			got.AuthoritySummary, got.Coverage)
	}
	// Step 0.5 deferral: RecentDecisions is always nil in PR 4.
	if got.RecentDecisions != nil {
		t.Errorf("RecentDecisions must be nil in PR 4 (deferred to PR 8); got %+v", got.RecentDecisions)
	}
}

func TestGetGovernanceMap_ServiceNotConfigured_ReturnsErr(t *testing.T) {
	// Nil ReadService receiver path.
	var s *ReadService
	_, err := s.GetGovernanceMap(context.Background(), "bs")
	if !errors.Is(err, ErrServiceNotConfigured) {
		t.Errorf("nil receiver: want ErrServiceNotConfigured, got %v", err)
	}

	// Missing one reader.
	svc := NewReadService(
		&stubBSReader{items: map[string]*businessservice.BusinessService{}},
		nil, // missing
		&stubBSCReader{}, &stubCapabilityReader{}, &stubProcessReader{},
		&stubSurfaceReader{}, &stubProfileReader{}, &stubGrantReader{},
		&stubAISystemReader{}, &stubAIVersionReader{}, &stubAIBindingReader{},
	)
	_, err = svc.GetGovernanceMap(context.Background(), "bs")
	if !errors.Is(err, ErrServiceNotConfigured) {
		t.Errorf("missing reader: want ErrServiceNotConfigured, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Relationships
// ---------------------------------------------------------------------------

func TestGetGovernanceMap_Relationships_OutgoingAndIncomingWithNames(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-root"] = makeBS("bs-root", "Root")
	f.bs.items["bs-other"] = makeBS("bs-other", "Other")
	f.bs.items["bs-third"] = makeBS("bs-third", "Third")

	f.bsr.items = []*businessservice.BusinessServiceRelationship{
		{ID: "rel-out-1", SourceBusinessService: "bs-root", TargetBusinessService: "bs-other", RelationshipType: "depends_on"},
		{ID: "rel-in-1", SourceBusinessService: "bs-third", TargetBusinessService: "bs-root", RelationshipType: "supports"},
	}

	got, _ := f.service().GetGovernanceMap(context.Background(), "bs-root")

	if len(got.Relationships.Outgoing) != 1 || got.Relationships.Outgoing[0].OtherName != "Other" {
		t.Errorf("outgoing name: %+v", got.Relationships.Outgoing)
	}
	if len(got.Relationships.Incoming) != 1 || got.Relationships.Incoming[0].OtherName != "Third" {
		t.Errorf("incoming name: %+v", got.Relationships.Incoming)
	}
}

// ---------------------------------------------------------------------------
// Capabilities
// ---------------------------------------------------------------------------

func TestGetGovernanceMap_Capabilities_LoadedViaBSC(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS")
	f.caps.items["cap-a"] = &capability.Capability{ID: "cap-a", Name: "Cap A", Status: "active"}
	f.bsc.items = []*businessservicecapability.BusinessServiceCapability{
		{BusinessServiceID: "bs-1", CapabilityID: "cap-a"},
	}

	got, _ := f.service().GetGovernanceMap(context.Background(), "bs-1")
	if len(got.Capabilities) != 1 || got.Capabilities[0].Capability.ID != "cap-a" {
		t.Errorf("capabilities: %+v", got.Capabilities)
	}
}

func TestGetGovernanceMap_Capabilities_DanglingBSC_Skipped(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS")
	// BSC points at a capability that doesn't exist.
	f.bsc.items = []*businessservicecapability.BusinessServiceCapability{
		{BusinessServiceID: "bs-1", CapabilityID: "cap-deleted"},
	}

	got, _ := f.service().GetGovernanceMap(context.Background(), "bs-1")
	if len(got.Capabilities) != 0 {
		t.Errorf("dangling BSC must produce zero capabilities; got %+v", got.Capabilities)
	}
}

// ---------------------------------------------------------------------------
// Processes
// ---------------------------------------------------------------------------

func TestGetGovernanceMap_Processes_FilteredToBusinessService(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS")
	f.procs.items = []*process.Process{
		{ID: "proc-mine", BusinessServiceID: "bs-1", Status: "active"},
		{ID: "proc-other", BusinessServiceID: "bs-2", Status: "active"},
	}

	got, _ := f.service().GetGovernanceMap(context.Background(), "bs-1")
	if len(got.Processes) != 1 || got.Processes[0].Process.ID != "proc-mine" {
		t.Errorf("processes: %+v", got.Processes)
	}
}

// ---------------------------------------------------------------------------
// Surfaces — active filter, per-surface counts
// ---------------------------------------------------------------------------

func TestGetGovernanceMap_Surfaces_FilteredToActive(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS")
	f.procs.items = []*process.Process{{ID: "proc-1", BusinessServiceID: "bs-1", Status: "active"}}
	f.surfaces.items = []*surface.DecisionSurface{
		{ID: "surf-active", ProcessID: "proc-1", Status: surface.SurfaceStatusActive, Version: 1},
		{ID: "surf-review", ProcessID: "proc-1", Status: surface.SurfaceStatusReview, Version: 1},
		{ID: "surf-deprecated", ProcessID: "proc-1", Status: surface.SurfaceStatusDeprecated, Version: 1},
	}

	got, _ := f.service().GetGovernanceMap(context.Background(), "bs-1")
	if len(got.Surfaces) != 1 || got.Surfaces[0].Surface.ID != "surf-active" {
		t.Errorf("surface active filter: %+v", got.Surfaces)
	}
}

func TestGetGovernanceMap_Surfaces_PerSurfaceAuthorityCounts(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS")
	f.procs.items = []*process.Process{{ID: "proc-1", BusinessServiceID: "bs-1", Status: "active"}}
	f.surfaces.items = []*surface.DecisionSurface{
		{ID: "surf-1", ProcessID: "proc-1", Status: surface.SurfaceStatusActive, Version: 1},
	}
	f.profiles.items = []*authority.AuthorityProfile{
		{ID: "prof-active", SurfaceID: "surf-1", Status: authority.ProfileStatusActive},
		{ID: "prof-deprecated", SurfaceID: "surf-1", Status: authority.ProfileStatusDeprecated},
	}
	f.grants.items = []*authority.AuthorityGrant{
		{ID: "grant-1", ProfileID: "prof-active", Status: authority.GrantStatusActive, AgentID: "agent-1"},
		{ID: "grant-2", ProfileID: "prof-active", Status: authority.GrantStatusActive, AgentID: "agent-2"},
		{ID: "grant-revoked", ProfileID: "prof-active", Status: authority.GrantStatusRevoked, AgentID: "agent-3"},
	}

	got, _ := f.service().GetGovernanceMap(context.Background(), "bs-1")
	if len(got.Surfaces) != 1 {
		t.Fatalf("surfaces: %+v", got.Surfaces)
	}
	sn := got.Surfaces[0]
	if sn.ProfileCount != 1 || sn.GrantCount != 2 || sn.AgentCount != 2 {
		t.Errorf("per-surface counts: profile=%d grant=%d agent=%d (want 1/2/2)",
			sn.ProfileCount, sn.GrantCount, sn.AgentCount)
	}
}

// ---------------------------------------------------------------------------
// AI systems — four-way OR + dedup + filtering rules
// ---------------------------------------------------------------------------

func TestGetGovernanceMap_AISystems_FourWayOR_AllPathsContribute(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS")
	f.caps.items["cap-1"] = &capability.Capability{ID: "cap-1", Name: "Cap", Status: "active"}
	f.bsc.items = []*businessservicecapability.BusinessServiceCapability{{BusinessServiceID: "bs-1", CapabilityID: "cap-1"}}
	f.procs.items = []*process.Process{{ID: "proc-1", BusinessServiceID: "bs-1", Status: "active"}}
	f.surfaces.items = []*surface.DecisionSurface{
		{ID: "surf-1", ProcessID: "proc-1", Status: surface.SurfaceStatusActive, Version: 1},
	}

	// Four AI systems, one binding per context path.
	for _, id := range []string{"ai-bs", "ai-cap", "ai-proc", "ai-surf"} {
		f.aiSys.items[id] = &aisystem.AISystem{ID: id, Name: id, Status: "active"}
	}
	f.aiBind.items = []*aisystem.AISystemBinding{
		{ID: "b-bs", AISystemID: "ai-bs", BusinessServiceID: "bs-1"},
		{ID: "b-cap", AISystemID: "ai-cap", CapabilityID: "cap-1"},
		{ID: "b-proc", AISystemID: "ai-proc", ProcessID: "proc-1"},
		{ID: "b-surf", AISystemID: "ai-surf", SurfaceID: "surf-1"},
	}

	got, err := f.service().GetGovernanceMap(context.Background(), "bs-1")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, ai := range got.AISystems {
		ids[ai.System.ID] = true
	}
	for _, want := range []string{"ai-bs", "ai-cap", "ai-proc", "ai-surf"} {
		if !ids[want] {
			t.Errorf("AI system %q missing from response (path-OR not applied)", want)
		}
	}
}

func TestGetGovernanceMap_AISystems_DedupByBindingID(t *testing.T) {
	// One AI system with one binding that is reachable via two paths
	// (process_id AND surface_id under that process). The binding must
	// appear ONCE under the AI system, not twice.
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS")
	f.procs.items = []*process.Process{{ID: "proc-1", BusinessServiceID: "bs-1", Status: "active"}}
	f.surfaces.items = []*surface.DecisionSurface{
		{ID: "surf-1", ProcessID: "proc-1", Status: surface.SurfaceStatusActive, Version: 1},
	}
	f.aiSys.items["ai-1"] = &aisystem.AISystem{ID: "ai-1", Name: "AI", Status: "active"}
	f.aiBind.items = []*aisystem.AISystemBinding{
		// One binding with both ProcessID and SurfaceID set — reachable
		// via path 3 (ListByProcess) and path 4 (ListBySurface).
		{ID: "b-1", AISystemID: "ai-1", ProcessID: "proc-1", SurfaceID: "surf-1"},
	}

	got, _ := f.service().GetGovernanceMap(context.Background(), "bs-1")
	if len(got.AISystems) != 1 || len(got.AISystems[0].Bindings) != 1 {
		t.Errorf("dedup-by-binding-ID failed: AI systems=%d, bindings=%d",
			len(got.AISystems), len(got.AISystems[0].Bindings))
	}
}

func TestGetGovernanceMap_AISystems_GroupedByAISystem_DistinctBindingsPreserved(t *testing.T) {
	// One AI system with TWO distinct bindings: one via process, one
	// via surface (under that process). Both bindings must appear under
	// the same AI system node — dedup is by binding ID, not by AI
	// system ID.
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS")
	f.procs.items = []*process.Process{{ID: "proc-1", BusinessServiceID: "bs-1", Status: "active"}}
	f.surfaces.items = []*surface.DecisionSurface{
		{ID: "surf-1", ProcessID: "proc-1", Status: surface.SurfaceStatusActive, Version: 1},
	}
	f.aiSys.items["ai-1"] = &aisystem.AISystem{ID: "ai-1", Name: "AI", Status: "active"}
	f.aiBind.items = []*aisystem.AISystemBinding{
		{ID: "b-via-proc", AISystemID: "ai-1", ProcessID: "proc-1"},
		{ID: "b-via-surf", AISystemID: "ai-1", SurfaceID: "surf-1"},
	}

	got, _ := f.service().GetGovernanceMap(context.Background(), "bs-1")
	if len(got.AISystems) != 1 {
		t.Fatalf("want 1 AI system, got %d", len(got.AISystems))
	}
	if len(got.AISystems[0].Bindings) != 2 {
		t.Errorf("want 2 bindings under ai-1; got %d", len(got.AISystems[0].Bindings))
	}
	// Deterministic ordering by binding ID.
	if got.AISystems[0].Bindings[0].ID != "b-via-proc" || got.AISystems[0].Bindings[1].ID != "b-via-surf" {
		t.Errorf("binding ordering: %+v", got.AISystems[0].Bindings)
	}
}

func TestGetGovernanceMap_AISystems_AllStatusesIncluded(t *testing.T) {
	// Deprecated AI systems with bindings must remain visible in the map.
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS")
	f.aiSys.items["ai-deprecated"] = &aisystem.AISystem{
		ID: "ai-deprecated", Name: "AI", Status: aisystem.AISystemStatusDeprecated,
	}
	f.aiBind.items = []*aisystem.AISystemBinding{
		{ID: "b-1", AISystemID: "ai-deprecated", BusinessServiceID: "bs-1"},
	}

	got, _ := f.service().GetGovernanceMap(context.Background(), "bs-1")
	if len(got.AISystems) != 1 || got.AISystems[0].System.Status != aisystem.AISystemStatusDeprecated {
		t.Errorf("deprecated AI system filtered out: %+v", got.AISystems)
	}
}

func TestGetGovernanceMap_AISystems_NoActiveVersion_ActiveVersionNil(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS")
	f.aiSys.items["ai-1"] = &aisystem.AISystem{ID: "ai-1", Name: "AI", Status: "active"}
	// No version registered — GetActiveBySystem returns nil.
	f.aiBind.items = []*aisystem.AISystemBinding{
		{ID: "b-1", AISystemID: "ai-1", BusinessServiceID: "bs-1"},
	}

	got, _ := f.service().GetGovernanceMap(context.Background(), "bs-1")
	if len(got.AISystems) != 1 || got.AISystems[0].ActiveVersion != nil {
		t.Errorf("AI system without active version: ActiveVersion should be nil; got %+v",
			got.AISystems[0].ActiveVersion)
	}
}

func TestGetGovernanceMap_AISystems_DanglingBinding_Skipped(t *testing.T) {
	// Binding references an AI system that doesn't exist in the registry.
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS")
	f.aiBind.items = []*aisystem.AISystemBinding{
		{ID: "b-1", AISystemID: "ai-deleted", BusinessServiceID: "bs-1"},
	}

	got, _ := f.service().GetGovernanceMap(context.Background(), "bs-1")
	if len(got.AISystems) != 0 {
		t.Errorf("dangling binding must produce zero AI systems; got %+v", got.AISystems)
	}
}

func TestGetGovernanceMap_AISystems_UnrelatedBindingsExcluded(t *testing.T) {
	// AI system bound to BS-A (via business_service_id) AND to BS-B
	// (via business_service_id). Querying BS-A's map must return only
	// the BS-A binding, not the BS-B one.
	f := newFixture()
	f.bs.items["bs-a"] = makeBS("bs-a", "BS A")
	f.bs.items["bs-b"] = makeBS("bs-b", "BS B")
	f.aiSys.items["ai-multi"] = &aisystem.AISystem{ID: "ai-multi", Name: "Multi", Status: "active"}
	f.aiBind.items = []*aisystem.AISystemBinding{
		{ID: "b-a", AISystemID: "ai-multi", BusinessServiceID: "bs-a"},
		{ID: "b-b", AISystemID: "ai-multi", BusinessServiceID: "bs-b"},
	}

	got, _ := f.service().GetGovernanceMap(context.Background(), "bs-a")
	if len(got.AISystems) != 1 {
		t.Fatalf("want 1 AI system, got %d", len(got.AISystems))
	}
	if len(got.AISystems[0].Bindings) != 1 || got.AISystems[0].Bindings[0].ID != "b-a" {
		t.Errorf("unrelated BS-B binding leaked into BS-A map: %+v",
			got.AISystems[0].Bindings)
	}
}

// ---------------------------------------------------------------------------
// AuthoritySummary — aggregate distinct counts + Coverage
// ---------------------------------------------------------------------------

func TestGetGovernanceMap_AuthoritySummary_DistinctSetsAtEachLevel(t *testing.T) {
	// Two surfaces share one profile (theoretically possible). The
	// aggregate active_profile_count must count it once, not twice.
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS")
	f.procs.items = []*process.Process{{ID: "proc-1", BusinessServiceID: "bs-1", Status: "active"}}
	f.surfaces.items = []*surface.DecisionSurface{
		{ID: "surf-1", ProcessID: "proc-1", Status: surface.SurfaceStatusActive, Version: 1},
		{ID: "surf-2", ProcessID: "proc-1", Status: surface.SurfaceStatusActive, Version: 1},
	}
	// One profile linked to both surfaces.
	f.profiles.items = []*authority.AuthorityProfile{
		{ID: "prof-shared", SurfaceID: "surf-1", Status: authority.ProfileStatusActive},
		{ID: "prof-shared", SurfaceID: "surf-2", Status: authority.ProfileStatusActive},
	}
	f.grants.items = []*authority.AuthorityGrant{
		{ID: "grant-1", ProfileID: "prof-shared", Status: authority.GrantStatusActive, AgentID: "agent-1"},
	}

	got, _ := f.service().GetGovernanceMap(context.Background(), "bs-1")
	if got.AuthoritySummary.SurfaceCount != 2 {
		t.Errorf("SurfaceCount: want 2, got %d", got.AuthoritySummary.SurfaceCount)
	}
	if got.AuthoritySummary.ActiveProfileCount != 1 {
		t.Errorf("ActiveProfileCount: distinct profile across surfaces = 1; got %d",
			got.AuthoritySummary.ActiveProfileCount)
	}
	if got.AuthoritySummary.ActiveGrantCount != 1 {
		t.Errorf("ActiveGrantCount: want 1, got %d", got.AuthoritySummary.ActiveGrantCount)
	}
	if got.AuthoritySummary.ActiveAgentCount != 1 {
		t.Errorf("ActiveAgentCount: want 1, got %d", got.AuthoritySummary.ActiveAgentCount)
	}
}

func TestGetGovernanceMap_Coverage_WithBindingVsWithout(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS")
	f.procs.items = []*process.Process{{ID: "proc-1", BusinessServiceID: "bs-1", Status: "active"}}
	f.surfaces.items = []*surface.DecisionSurface{
		{ID: "surf-with", ProcessID: "proc-1", Status: surface.SurfaceStatusActive, Version: 1},
		{ID: "surf-without", ProcessID: "proc-1", Status: surface.SurfaceStatusActive, Version: 1},
	}
	f.aiSys.items["ai-1"] = &aisystem.AISystem{ID: "ai-1", Name: "AI", Status: "active"}
	f.aiBind.items = []*aisystem.AISystemBinding{
		{ID: "b-1", AISystemID: "ai-1", SurfaceID: "surf-with"},
	}

	got, _ := f.service().GetGovernanceMap(context.Background(), "bs-1")
	if got.Coverage.SurfaceCount != 2 {
		t.Errorf("SurfaceCount: want 2, got %d", got.Coverage.SurfaceCount)
	}
	if got.Coverage.SurfacesWithAIBinding != 1 {
		t.Errorf("SurfacesWithAIBinding: want 1, got %d", got.Coverage.SurfacesWithAIBinding)
	}
	if got.Coverage.SurfacesWithoutAIBinding != 1 {
		t.Errorf("SurfacesWithoutAIBinding: want 1, got %d", got.Coverage.SurfacesWithoutAIBinding)
	}
}

// ---------------------------------------------------------------------------
// Recent decisions — sentinel for the PR 4 deferral
// ---------------------------------------------------------------------------

func TestGetGovernanceMap_RecentDecisions_AlwaysNilInPR4(t *testing.T) {
	// Even with a fully populated service, RecentDecisions stays nil.
	// PR 8 will revisit when the envelope substrate lands.
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS")
	f.procs.items = []*process.Process{{ID: "proc-1", BusinessServiceID: "bs-1", Status: "active"}}

	got, _ := f.service().GetGovernanceMap(context.Background(), "bs-1")
	if got.RecentDecisions != nil {
		t.Errorf("RecentDecisions must be nil in PR 4; got %+v", got.RecentDecisions)
	}
}

// ---------------------------------------------------------------------------
// Deterministic ordering for AI systems
// ---------------------------------------------------------------------------

func TestGetGovernanceMap_AISystems_SortedByIDForDeterministicResponse(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS")
	for _, id := range []string{"ai-z", "ai-a", "ai-m"} {
		f.aiSys.items[id] = &aisystem.AISystem{ID: id, Name: id, Status: "active"}
	}
	f.aiBind.items = []*aisystem.AISystemBinding{
		{ID: "b-z", AISystemID: "ai-z", BusinessServiceID: "bs-1"},
		{ID: "b-a", AISystemID: "ai-a", BusinessServiceID: "bs-1"},
		{ID: "b-m", AISystemID: "ai-m", BusinessServiceID: "bs-1"},
	}

	got, _ := f.service().GetGovernanceMap(context.Background(), "bs-1")
	gotOrder := []string{}
	for _, ai := range got.AISystems {
		gotOrder = append(gotOrder, ai.System.ID)
	}
	want := []string{"ai-a", "ai-m", "ai-z"}
	for i, w := range want {
		if i >= len(gotOrder) || gotOrder[i] != w {
			t.Errorf("AI systems must be sorted by ID; want %v, got %v", want, gotOrder)
			break
		}
	}
}

// timeNow is a stable timestamp helper for tests that need one.
func timeNow() time.Time { return time.Now().UTC() }
