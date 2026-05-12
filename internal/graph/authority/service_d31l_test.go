package authoritygraph

// service_d31l_test.go — projection tests for the D31l escalation-
// target additions. Pins:
//
//   - profile_escalates_to edge + escalation_target node emission
//   - dedupe of shared targets across multiple profiles
//   - info diagnostic for profiles with empty EscalationTargetID
//   - warning diagnostic for profiles with dangling EscalationTargetID
//   - propagation of repo errors (FindActiveAt → projector error)
//   - clear registration error when EscalationTargets is unwired but
//     a profile carries a non-empty target id
//   - summary rollups (count + with/without/dangling lists)
//   - depth=4 (default) includes target; depth=2 hides target nodes
//     while preserving Summary

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/escalation"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

// firstNodeOfKind returns a pointer to the first emitted node of the
// given kind, or nil if absent.
func firstNodeOfKind(nodes []Node, kind string) *Node {
	for i := range nodes {
		if nodes[i].Kind == kind {
			return &nodes[i]
		}
	}
	return nil
}

// firstEdgeOfKind returns a pointer to the first emitted edge of
// the given kind, or nil if absent.
func firstEdgeOfKind(edges []Edge, kind string) *Edge {
	for i := range edges {
		if edges[i].Kind == kind {
			return &edges[i]
		}
	}
	return nil
}

func TestAuthorityGraph_Project_ProfileWithEscalationTarget(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	prof := makeProfile("prof-1", "surf-1")
	prof.EscalationTargetID = "et-approver"
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{prof}
	f.etgts.items["et-approver"] = makeEscalationTarget("et-approver", "Governance Approver")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	tgtNode := firstNodeOfKind(got.Nodes, NodeKindEscalationTarget)
	if tgtNode == nil {
		t.Fatalf("expected escalation_target node; got %+v", got.Nodes)
	}
	if tgtNode.ID != "et-approver" {
		t.Errorf("escalation_target node id: want et-approver, got %q", tgtNode.ID)
	}
	if tgtNode.EscalationTarget == nil {
		t.Errorf("escalation_target node missing typed data")
	} else {
		if tgtNode.EscalationTarget.Kind != "role" {
			t.Errorf("typed data kind: want role, got %q", tgtNode.EscalationTarget.Kind)
		}
		if tgtNode.EscalationTarget.Status != "active" {
			t.Errorf("typed data status: want active, got %q", tgtNode.EscalationTarget.Status)
		}
		if tgtNode.EscalationTarget.Handle == "" {
			t.Errorf("typed data handle must be non-empty")
		}
	}

	edge := firstEdgeOfKind(got.Edges, EdgeKindProfileEscalatesTo)
	if edge == nil {
		t.Fatalf("expected profile_escalates_to edge; got %+v", got.Edges)
	}
	if edge.Src.Kind != NodeKindAuthorityProfile || edge.Src.ID != "prof-1" {
		t.Errorf("edge src: %+v", edge.Src)
	}
	if edge.Dst.Kind != NodeKindEscalationTarget || edge.Dst.ID != "et-approver" {
		t.Errorf("edge dst: %+v", edge.Dst)
	}

	// Profile typed data should carry the configured target id.
	profNode := firstNodeOfKind(got.Nodes, NodeKindAuthorityProfile)
	if profNode == nil || profNode.AuthorityProfile == nil {
		t.Fatal("profile node missing")
	}
	if profNode.AuthorityProfile.EscalationTargetID != "et-approver" {
		t.Errorf("AuthorityProfileData.EscalationTargetID: want %q, got %q",
			"et-approver", profNode.AuthorityProfile.EscalationTargetID)
	}
}

func TestAuthorityGraph_Project_ProfileWithSharedEscalationTarget_DedupesTargetNode(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	// Two surfaces, each with its own profile, both targeting the
	// same escalation target id.
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{
		makeSurface("surf-a", "proc-1"),
		makeSurface("surf-b", "proc-1"),
	}
	pa := makeProfile("prof-a", "surf-a")
	pa.EscalationTargetID = "et-shared"
	pb := makeProfile("prof-b", "surf-b")
	pb.EscalationTargetID = "et-shared"
	f.profiles.bySurface["surf-a"] = []*authority.AuthorityProfile{pa}
	f.profiles.bySurface["surf-b"] = []*authority.AuthorityProfile{pb}
	f.etgts.items["et-shared"] = makeEscalationTarget("et-shared", "Shared Approver")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	if c := countKind(got.Nodes, NodeKindEscalationTarget); c != 1 {
		t.Errorf("escalation_target node count: want 1 (deduped), got %d", c)
	}
	if c := countEdgeKind(got.Edges, EdgeKindProfileEscalatesTo); c != 2 {
		t.Errorf("profile_escalates_to edge count: want 2 (one per profile), got %d", c)
	}
	if got.Summary == nil {
		t.Fatal("Summary missing")
	}
	if got.Summary.EscalationTargetCount != 1 {
		t.Errorf("EscalationTargetCount: want 1, got %d", got.Summary.EscalationTargetCount)
	}
	if got.Summary.ProfilesWithEscalationTarget != 2 {
		t.Errorf("ProfilesWithEscalationTarget: want 2, got %d", got.Summary.ProfilesWithEscalationTarget)
	}
}

func TestAuthorityGraph_Project_ProfileWithNoEscalationTarget_DiagnosticInfo(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	// Profile with EscalationTargetID empty.
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindEscalationTarget) != 0 {
		t.Errorf("escalation_target node count: want 0, got %d", countKind(got.Nodes, NodeKindEscalationTarget))
	}
	if countEdgeKind(got.Edges, EdgeKindProfileEscalatesTo) != 0 {
		t.Errorf("profile_escalates_to edge count: want 0, got %d", countEdgeKind(got.Edges, EdgeKindProfileEscalatesTo))
	}
	var found bool
	for _, d := range got.Diagnostics {
		if d.Kind != DiagnosticKindProfileHasNoEscalationTarget {
			continue
		}
		found = true
		if d.Severity != DiagnosticSeverityInfo {
			t.Errorf("profile_has_no_escalation_target severity: want info, got %q", d.Severity)
		}
		if len(d.NodeRefs) == 0 || d.NodeRefs[0].ID != "prof-1" {
			t.Errorf("diagnostic refs wrong: %+v", d.NodeRefs)
		}
	}
	if !found {
		t.Errorf("expected profile_has_no_escalation_target diagnostic; got %+v", got.Diagnostics)
	}
	// Summary reflects the profile in profiles_without_escalation_target.
	if got.Summary == nil || len(got.Summary.ProfilesWithoutEscalationTarget) != 1 {
		t.Errorf("Summary.ProfilesWithoutEscalationTarget: want 1 entry; got %+v", got.Summary)
	}
}

func TestAuthorityGraph_Project_ProfileWithDanglingEscalationTarget_DiagnosticWarning(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	prof := makeProfile("prof-1", "surf-1")
	prof.EscalationTargetID = "et-missing"
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{prof}
	// f.etgts has no et-missing → FindActiveAt returns (nil, nil).

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindEscalationTarget) != 0 {
		t.Errorf("dangling target must not produce an escalation_target node")
	}
	if countEdgeKind(got.Edges, EdgeKindProfileEscalatesTo) != 0 {
		t.Errorf("dangling target must not produce a profile_escalates_to edge")
	}

	var found bool
	for _, d := range got.Diagnostics {
		if d.Kind != DiagnosticKindEscalationTargetReferenceDangling {
			continue
		}
		found = true
		if d.Severity != DiagnosticSeverityWarning {
			t.Errorf("escalation_target_reference_dangling severity: want warning, got %q", d.Severity)
		}
	}
	if !found {
		t.Errorf("expected escalation_target_reference_dangling diagnostic; got %+v", got.Diagnostics)
	}
	if got.Summary == nil || len(got.Summary.ProfilesWithDanglingEscalationTarget) != 1 {
		t.Errorf("Summary.ProfilesWithDanglingEscalationTarget: want 1 entry; got %+v", got.Summary)
	}
}

func TestAuthorityGraph_Project_EscalationTargetRepoError_ReturnsError(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	prof := makeProfile("prof-1", "surf-1")
	prof.EscalationTargetID = "et-explodes"
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{prof}
	f.etgts.err = errors.New("boom")

	_, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err == nil {
		t.Fatal("expected projection error when escalation resolver fails")
	}
	if !errors.Is(err, f.etgts.err) {
		t.Errorf("expected wrapped resolver error; got %v", err)
	}
}

func TestAuthorityGraph_Project_MissingEscalationTargetReader_ReturnsRegistrationError(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	prof := makeProfile("prof-1", "surf-1")
	prof.EscalationTargetID = "et-orphaned"
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{prof}

	// Construct readers with EscalationTargets nil. NewServiceWithClock
	// still registers ViewService because the resolver is optional at
	// construction — the missing-reader check fires at projection time
	// only when a profile actually carries a non-empty
	// EscalationTargetID.
	r := f.readers()
	r.EscalationTargets = nil
	svc := NewServiceWithClock(r, func() time.Time { return fixedNow })
	_, err := svc.Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err == nil {
		t.Fatal("expected ErrEscalationTargetReaderNotConfigured")
	}
	if !errors.Is(err, ErrEscalationTargetReaderNotConfigured) {
		t.Errorf("expected ErrEscalationTargetReaderNotConfigured chain; got %v", err)
	}
}

func TestAuthorityGraph_Project_MissingEscalationTargetReader_AndEmptyTargetID_NoError(t *testing.T) {
	// When EscalationTargets is nil but no profile carries an
	// EscalationTargetID, the projection should succeed and emit
	// only the info diagnostic for the missing target.
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}

	r := f.readers()
	r.EscalationTargets = nil
	svc := NewServiceWithClock(r, func() time.Time { return fixedNow })
	got, err := svc.Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if !hasDiagnostic(got.Diagnostics, DiagnosticKindProfileHasNoEscalationTarget) {
		t.Errorf("expected info diagnostic even when EscalationTargets is unwired; got %+v", got.Diagnostics)
	}
}

func TestAuthorityGraph_Summary_EscalationTargetFields(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{
		makeSurface("surf-a", "proc-1"),
		makeSurface("surf-b", "proc-1"),
		makeSurface("surf-c", "proc-1"),
	}
	resolved := makeProfile("prof-resolved", "surf-a")
	resolved.EscalationTargetID = "et-real"
	dangling := makeProfile("prof-dangling", "surf-b")
	dangling.EscalationTargetID = "et-ghost"
	empty := makeProfile("prof-empty", "surf-c")
	f.profiles.bySurface["surf-a"] = []*authority.AuthorityProfile{resolved}
	f.profiles.bySurface["surf-b"] = []*authority.AuthorityProfile{dangling}
	f.profiles.bySurface["surf-c"] = []*authority.AuthorityProfile{empty}
	f.etgts.items["et-real"] = makeEscalationTarget("et-real", "Real Target")
	// et-ghost not present → dangling.

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	sm := got.Summary
	if sm == nil {
		t.Fatal("Summary missing")
	}
	if sm.EscalationTargetCount != 1 {
		t.Errorf("EscalationTargetCount: want 1, got %d", sm.EscalationTargetCount)
	}
	if sm.ProfilesWithEscalationTarget != 1 {
		t.Errorf("ProfilesWithEscalationTarget: want 1, got %d", sm.ProfilesWithEscalationTarget)
	}
	if len(sm.ProfilesWithoutEscalationTarget) != 1 {
		t.Errorf("ProfilesWithoutEscalationTarget len: want 1, got %d", len(sm.ProfilesWithoutEscalationTarget))
	}
	if len(sm.ProfilesWithDanglingEscalationTarget) != 1 {
		t.Errorf("ProfilesWithDanglingEscalationTarget len: want 1, got %d", len(sm.ProfilesWithDanglingEscalationTarget))
	}
}

func TestAuthorityGraph_DepthDefault_IncludesEscalationTarget(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	prof := makeProfile("prof-1", "surf-1")
	prof.EscalationTargetID = "et-approver"
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{prof}
	f.etgts.items["et-approver"] = makeEscalationTarget("et-approver", "Approver")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindEscalationTarget) != 1 {
		t.Errorf("default depth (%d) must include escalation_target; got %d", DefaultDepth, countKind(got.Nodes, NodeKindEscalationTarget))
	}
	if countEdgeKind(got.Edges, EdgeKindProfileEscalatesTo) != 1 {
		t.Errorf("default depth must include profile_escalates_to; got %d", countEdgeKind(got.Edges, EdgeKindProfileEscalatesTo))
	}
}

func TestAuthorityGraph_DepthTwo_HidesEscalationTargetButSummaryStillReports(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	prof := makeProfile("prof-1", "surf-1")
	prof.EscalationTargetID = "et-approver"
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{prof}
	f.etgts.items["et-approver"] = makeEscalationTarget("et-approver", "Approver")

	// Depth 2 from the BS root: BS (0) → Surface (1) → Profile (2).
	// EscalationTarget is at depth 3 (Profile→Target), so it must
	// be filtered from nodes/edges. Summary is computed from the
	// pre-depth-filter projection and must still reflect the target.
	got, err := f.service().Project(context.Background(), ViewService, "bs-1", 2)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if countKind(got.Nodes, NodeKindEscalationTarget) != 0 {
		t.Errorf("depth=2 must hide escalation_target nodes; got %d", countKind(got.Nodes, NodeKindEscalationTarget))
	}
	if countEdgeKind(got.Edges, EdgeKindProfileEscalatesTo) != 0 {
		t.Errorf("depth=2 must hide profile_escalates_to edges; got %d", countEdgeKind(got.Edges, EdgeKindProfileEscalatesTo))
	}
	if got.Summary == nil {
		t.Fatal("Summary missing")
	}
	if got.Summary.EscalationTargetCount != 1 {
		t.Errorf("Summary.EscalationTargetCount must describe pre-depth projection; got %d", got.Summary.EscalationTargetCount)
	}
	if got.Summary.ProfilesWithEscalationTarget != 1 {
		t.Errorf("Summary.ProfilesWithEscalationTarget must describe pre-depth projection; got %d", got.Summary.ProfilesWithEscalationTarget)
	}
}

// TestAuthorityGraph_NodeKindConstants_D31l pins the new node kind
// constant value and asserts it joins the canonical seven.
func TestAuthorityGraph_NodeKindConstants_D31l(t *testing.T) {
	if NodeKindEscalationTarget != "escalation_target" {
		t.Errorf("NodeKindEscalationTarget: want %q, got %q", "escalation_target", NodeKindEscalationTarget)
	}
}

// TestAuthorityGraph_EdgeKindConstants_D31l pins the new edge kind.
func TestAuthorityGraph_EdgeKindConstants_D31l(t *testing.T) {
	if EdgeKindProfileEscalatesTo != "profile_escalates_to" {
		t.Errorf("EdgeKindProfileEscalatesTo: want %q, got %q", "profile_escalates_to", EdgeKindProfileEscalatesTo)
	}
}

// keep escalation-package import live via the typed fixture
var _ = escalation.KindRole
