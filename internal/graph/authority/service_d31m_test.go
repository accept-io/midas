package authoritygraph

// service_d31m_test.go — service-level tests for the D31m diagnostic
// summary rollup and per-surface authority posture. Tests follow the
// existing stub-fixture pattern from service_test.go: build a small
// fixture, plant data into the per-reader maps, run Project, and
// assert on the resulting Projection.

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/escalation"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

// firstPosture finds the posture record for surfID in p.SurfacePosture.
// Helper for tests; returns nil when absent.
func firstPosture(p *Projection, surfID string) *SurfaceAuthorityPosture {
	for i := range p.SurfacePosture {
		if p.SurfacePosture[i].Surface.ID == surfID {
			return &p.SurfacePosture[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// DiagnosticSummary
// ---------------------------------------------------------------------------

func TestAuthorityGraph_DiagnosticSummary_Empty(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	prof := makeProfile("prof-1", "surf-1")
	prof.EscalationTargetID = "et-1"
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{prof}
	// Grant must carry at least one capability to avoid the
	// D31i grant_has_no_capabilities warning. Healthy chain → zero
	// diagnostics expected.
	g := makeGrant("grant-1", "prof-1", "agent-1")
	g.Capabilities = []authority.Capability{authority.CapabilityApprove}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{g}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent")
	f.etgts.items["et-1"] = makeEscalationTarget("et-1", "Target")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got.DiagnosticSummary == nil {
		t.Fatal("DiagnosticSummary missing")
	}
	ds := got.DiagnosticSummary
	if ds.Info != 0 || ds.Warning != 0 || ds.Critical != 0 {
		t.Errorf("counts: want 0/0/0, got %d/%d/%d", ds.Info, ds.Warning, ds.Critical)
	}
	if ds.HighestSeverity != HighestSeverityNone {
		t.Errorf("HighestSeverity: want none, got %q", ds.HighestSeverity)
	}
	if ds.ByKind != nil {
		t.Errorf("ByKind must be nil/empty when no diagnostics; got %+v", ds.ByKind)
	}
}

func TestAuthorityGraph_DiagnosticSummary_CountsAndHighestSeverity(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	// Two surfaces: one with no profile (warning), one with profile
	// but no escalation target (info) and an inactive agent (critical).
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{
		makeSurface("surf-bare", "proc-1"),
		makeSurface("surf-mixed", "proc-1"),
	}
	prof := makeProfile("prof-1", "surf-mixed")
	f.profiles.bySurface["surf-mixed"] = []*authority.AuthorityProfile{prof}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-1")}
	suspended := makeAgent("agent-1", "Agent")
	suspended.OperationalState = agent.OperationalStateSuspended
	f.agents.items["agent-1"] = suspended

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	ds := got.DiagnosticSummary
	if ds == nil {
		t.Fatal("DiagnosticSummary missing")
	}
	total := ds.Info + ds.Warning + ds.Critical
	if total != len(got.Diagnostics) {
		t.Errorf("counts sum: %d, want %d (len diagnostics)", total, len(got.Diagnostics))
	}
	if ds.Critical < 1 {
		t.Errorf("Critical: want >= 1 (inactive agent), got %d", ds.Critical)
	}
	if ds.HighestSeverity != HighestSeverityCritical {
		t.Errorf("HighestSeverity: want critical, got %q", ds.HighestSeverity)
	}
}

func TestAuthorityGraph_DiagnosticSummary_ByKind(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{
		makeSurface("surf-a", "proc-1"),
		makeSurface("surf-b", "proc-1"),
	}
	// Both surfaces have profiles without escalation targets → two
	// `profile_has_no_escalation_target` info diagnostics.
	f.profiles.bySurface["surf-a"] = []*authority.AuthorityProfile{makeProfile("prof-a", "surf-a")}
	f.profiles.bySurface["surf-b"] = []*authority.AuthorityProfile{makeProfile("prof-b", "surf-b")}
	f.grants.byProfile["prof-a"] = []*authority.AuthorityGrant{makeGrant("grant-a", "prof-a", "agent-1")}
	f.grants.byProfile["prof-b"] = []*authority.AuthorityGrant{makeGrant("grant-b", "prof-b", "agent-1")}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	ds := got.DiagnosticSummary
	if ds == nil {
		t.Fatal("DiagnosticSummary missing")
	}
	if ds.ByKind == nil {
		t.Fatal("ByKind must be populated when diagnostics present")
	}
	if ds.ByKind[DiagnosticKindProfileHasNoEscalationTarget] != 2 {
		t.Errorf("ByKind[%q]: want 2, got %d",
			DiagnosticKindProfileHasNoEscalationTarget,
			ds.ByKind[DiagnosticKindProfileHasNoEscalationTarget])
	}
}

// ---------------------------------------------------------------------------
// SurfacePosture — happy and gap variants
// ---------------------------------------------------------------------------

func TestAuthorityGraph_SurfacePosture_HealthyComplete(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	prof := makeProfile("prof-1", "surf-1")
	prof.EscalationTargetID = "et-1"
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{prof}
	g := makeGrant("grant-1", "prof-1", "agent-1")
	g.Capabilities = []authority.Capability{authority.CapabilityApprove}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{g}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent")
	f.etgts.items["et-1"] = makeEscalationTarget("et-1", "Target")

	got, _ := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	p := firstPosture(got, "surf-1")
	if p == nil {
		t.Fatal("posture for surf-1 missing")
	}
	if p.AuthorityStatus != AuthorityStatusComplete {
		t.Errorf("AuthorityStatus: want complete, got %q", p.AuthorityStatus)
	}
	if p.ProfileStatus != ProfileStatusCovered {
		t.Errorf("ProfileStatus: want covered, got %q", p.ProfileStatus)
	}
	if p.GrantStatus != GrantStatusCovered {
		t.Errorf("GrantStatus: want covered, got %q", p.GrantStatus)
	}
	if p.AgentStatus != AgentStatusCovered {
		t.Errorf("AgentStatus: want covered, got %q", p.AgentStatus)
	}
	if p.FailModePolicyStatus != FailModePolicyStatusMissing {
		t.Errorf("FailModePolicyStatus: want missing (no policy at all), got %q", p.FailModePolicyStatus)
	}
	if p.EscalationStatus != EscalationStatusTargeted {
		t.Errorf("EscalationStatus: want targeted, got %q", p.EscalationStatus)
	}
	if p.CompletePaths != 1 {
		t.Errorf("CompletePaths: want 1, got %d", p.CompletePaths)
	}
	if p.IncompletePaths != 0 {
		t.Errorf("IncompletePaths: want 0, got %d", p.IncompletePaths)
	}
	if p.HighestSeverity != HighestSeverityNone {
		t.Errorf("HighestSeverity: want none, got %q", p.HighestSeverity)
	}
}

func TestAuthorityGraph_SurfacePosture_SurfaceWithoutProfile_Uncovered(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	// No profiles for surf-1.

	got, _ := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	p := firstPosture(got, "surf-1")
	if p == nil {
		t.Fatal("posture missing")
	}
	if p.AuthorityStatus != AuthorityStatusUncovered {
		t.Errorf("AuthorityStatus: want uncovered, got %q", p.AuthorityStatus)
	}
	if p.ProfileStatus != ProfileStatusMissing {
		t.Errorf("ProfileStatus: want missing, got %q", p.ProfileStatus)
	}
	if p.GrantStatus != GrantStatusMissing {
		t.Errorf("GrantStatus: want missing, got %q", p.GrantStatus)
	}
	if p.AgentStatus != AgentStatusMissing {
		t.Errorf("AgentStatus: want missing, got %q", p.AgentStatus)
	}
	if p.CompletePaths != 0 || p.IncompletePaths != 1 {
		t.Errorf("paths: want 0 complete / 1 incomplete (one surface gap), got %d/%d",
			p.CompletePaths, p.IncompletePaths)
	}
}

func TestAuthorityGraph_SurfacePosture_ProfileWithoutGrant_Incomplete(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	// No grants for prof-1.

	got, _ := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	p := firstPosture(got, "surf-1")
	if p == nil {
		t.Fatal("posture missing")
	}
	if p.AuthorityStatus != AuthorityStatusIncomplete {
		t.Errorf("AuthorityStatus: want incomplete, got %q", p.AuthorityStatus)
	}
	if p.ProfileStatus != ProfileStatusCovered {
		t.Errorf("ProfileStatus: want covered, got %q", p.ProfileStatus)
	}
	if p.GrantStatus != GrantStatusMissing {
		t.Errorf("GrantStatus: want missing, got %q", p.GrantStatus)
	}
	if p.AgentStatus != AgentStatusMissing {
		t.Errorf("AgentStatus: want missing, got %q", p.AgentStatus)
	}
	if p.IncompletePaths < 1 {
		t.Errorf("IncompletePaths: want >= 1, got %d", p.IncompletePaths)
	}
}

func TestAuthorityGraph_SurfacePosture_GrantWithoutAgent_BlockedOrMissing(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	// Grant references an agent id that does not exist in the agent
	// store → critical missing-agent diagnostic → agent_status=blocked.
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-missing")}

	got, _ := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	p := firstPosture(got, "surf-1")
	if p == nil {
		t.Fatal("posture missing")
	}
	if p.AgentStatus != AgentStatusBlocked {
		t.Errorf("AgentStatus: want blocked (critical missing-agent), got %q", p.AgentStatus)
	}
	if p.GrantStatus != GrantStatusCovered {
		t.Errorf("GrantStatus: want covered, got %q", p.GrantStatus)
	}
	if p.CompletePaths != 0 {
		t.Errorf("CompletePaths: want 0, got %d", p.CompletePaths)
	}
	if p.AuthorityStatus != AuthorityStatusIncomplete {
		t.Errorf("AuthorityStatus: want incomplete (no complete path), got %q", p.AuthorityStatus)
	}
}

func TestAuthorityGraph_SurfacePosture_InactiveAgent_BlockedCritical(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-1")}
	suspended := makeAgent("agent-1", "Agent")
	suspended.OperationalState = agent.OperationalStateSuspended
	f.agents.items["agent-1"] = suspended

	got, _ := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	p := firstPosture(got, "surf-1")
	if p == nil {
		t.Fatal("posture missing")
	}
	if p.AgentStatus != AgentStatusBlocked {
		t.Errorf("AgentStatus: want blocked (D31j inactive-agent critical), got %q", p.AgentStatus)
	}
	// Inactive agent still counts as a complete path in node terms.
	if p.CompletePaths != 1 {
		t.Errorf("CompletePaths: want 1 (inactive agent still completes path), got %d", p.CompletePaths)
	}
	if p.HighestSeverity != HighestSeverityCritical {
		t.Errorf("HighestSeverity: want critical, got %q", p.HighestSeverity)
	}
	if p.AuthorityStatus != AuthorityStatusDegraded {
		t.Errorf("AuthorityStatus: want degraded (complete path + critical diag), got %q", p.AuthorityStatus)
	}
}

// ---------------------------------------------------------------------------
// SurfacePosture — fail-mode-policy axis
// ---------------------------------------------------------------------------

func TestAuthorityGraph_SurfacePosture_FailModeOverride(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	surf := makeSurface("surf-1", "proc-1")
	surf.FailModePolicyID = "fmp-override"
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{surf}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-1")}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent")
	f.fmpols.items["fmp-override"] = makePolicy("fmp-override", "Override Policy")

	got, _ := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	p := firstPosture(got, "surf-1")
	if p == nil {
		t.Fatal("posture missing")
	}
	if p.FailModePolicyStatus != FailModePolicyStatusOverride {
		t.Errorf("FailModePolicyStatus: want override, got %q", p.FailModePolicyStatus)
	}
}

func TestAuthorityGraph_SurfacePosture_FailModeInherited(t *testing.T) {
	f := newFixture()
	bs := makeBS("bs-1", "BS One")
	bs.FailModePolicyID = "fmp-default"
	f.bs.items["bs-1"] = bs
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	// Surface has no override.
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-1")}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent")
	f.fmpols.items["fmp-default"] = makePolicy("fmp-default", "Default Policy")

	got, _ := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	p := firstPosture(got, "surf-1")
	if p == nil {
		t.Fatal("posture missing")
	}
	if p.FailModePolicyStatus != FailModePolicyStatusInherited {
		t.Errorf("FailModePolicyStatus: want inherited, got %q", p.FailModePolicyStatus)
	}
}

func TestAuthorityGraph_SurfacePosture_FailModeMissing(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One") // no FailModePolicyID
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-1")}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent")

	got, _ := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	p := firstPosture(got, "surf-1")
	if p == nil {
		t.Fatal("posture missing")
	}
	if p.FailModePolicyStatus != FailModePolicyStatusMissing {
		t.Errorf("FailModePolicyStatus: want missing, got %q", p.FailModePolicyStatus)
	}
}

func TestAuthorityGraph_SurfacePosture_FailModeDangling(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	surf := makeSurface("surf-1", "proc-1")
	surf.FailModePolicyID = "fmp-dangling" // not in f.fmpols.items
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{surf}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-1")}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent")

	got, _ := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	p := firstPosture(got, "surf-1")
	if p == nil {
		t.Fatal("posture missing")
	}
	if p.FailModePolicyStatus != FailModePolicyStatusDangling {
		t.Errorf("FailModePolicyStatus: want dangling, got %q", p.FailModePolicyStatus)
	}
}

// ---------------------------------------------------------------------------
// SurfacePosture — escalation axis
// ---------------------------------------------------------------------------

func TestAuthorityGraph_SurfacePosture_EscalationTargeted(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	prof := makeProfile("prof-1", "surf-1")
	prof.EscalationTargetID = "et-1"
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{prof}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-1")}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent")
	f.etgts.items["et-1"] = makeEscalationTarget("et-1", "Target")

	got, _ := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	p := firstPosture(got, "surf-1")
	if p == nil {
		t.Fatal("posture missing")
	}
	if p.EscalationStatus != EscalationStatusTargeted {
		t.Errorf("EscalationStatus: want targeted, got %q", p.EscalationStatus)
	}
}

func TestAuthorityGraph_SurfacePosture_EscalationNotTargeted(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	// Profile has empty EscalationTargetID.
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-1")}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent")

	got, _ := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	p := firstPosture(got, "surf-1")
	if p == nil {
		t.Fatal("posture missing")
	}
	if p.EscalationStatus != EscalationStatusNotTargeted {
		t.Errorf("EscalationStatus: want not_targeted, got %q", p.EscalationStatus)
	}
}

func TestAuthorityGraph_SurfacePosture_EscalationDangling(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	prof := makeProfile("prof-1", "surf-1")
	prof.EscalationTargetID = "et-missing"
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{prof}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-1")}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent")
	// et-missing intentionally not in f.etgts.items.

	got, _ := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	p := firstPosture(got, "surf-1")
	if p == nil {
		t.Fatal("posture missing")
	}
	if p.EscalationStatus != EscalationStatusDangling {
		t.Errorf("EscalationStatus: want dangling, got %q", p.EscalationStatus)
	}
}

// ---------------------------------------------------------------------------
// SurfacePosture — severity precedence on authority_status
// ---------------------------------------------------------------------------

func TestAuthorityGraph_SurfacePosture_InfoDiagnosticDoesNotDegradeCompleteSurface(t *testing.T) {
	// A profile_has_no_escalation_target diagnostic is INFO. The
	// surface has a complete path. authority_status must be complete,
	// NOT degraded.
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	// Grant must have capabilities so the only diagnostic is the
	// info-severity profile_has_no_escalation_target.
	g := makeGrant("grant-1", "prof-1", "agent-1")
	g.Capabilities = []authority.Capability{authority.CapabilityApprove}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{g}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent")

	got, _ := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	p := firstPosture(got, "surf-1")
	if p == nil {
		t.Fatal("posture missing")
	}
	if p.HighestSeverity != HighestSeverityInfo {
		t.Errorf("HighestSeverity: want info, got %q", p.HighestSeverity)
	}
	if p.AuthorityStatus != AuthorityStatusComplete {
		t.Errorf("AuthorityStatus: info diagnostic must not degrade; want complete, got %q", p.AuthorityStatus)
	}
}

func TestAuthorityGraph_SurfacePosture_WarningDiagnosticDegradesCompleteSurface(t *testing.T) {
	// A grant_has_no_capabilities diagnostic is WARNING. The surface
	// has a complete path. authority_status must be degraded.
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	prof := makeProfile("prof-1", "surf-1")
	prof.EscalationTargetID = "et-1"
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{prof}
	// Grant with no capabilities triggers warning diagnostic.
	g := makeGrant("grant-1", "prof-1", "agent-1")
	g.Capabilities = nil
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{g}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent")
	f.etgts.items["et-1"] = makeEscalationTarget("et-1", "Target")

	got, _ := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	p := firstPosture(got, "surf-1")
	if p == nil {
		t.Fatal("posture missing")
	}
	if p.HighestSeverity != HighestSeverityWarning {
		t.Errorf("HighestSeverity: want warning, got %q", p.HighestSeverity)
	}
	if p.AuthorityStatus != AuthorityStatusDegraded {
		t.Errorf("AuthorityStatus: warning must degrade; want degraded, got %q", p.AuthorityStatus)
	}
}

// ---------------------------------------------------------------------------
// Depth semantics — D31m must preserve pre-depth Summary/Diagnostics/
// DiagnosticSummary/SurfacePosture.
// ---------------------------------------------------------------------------

func TestAuthorityGraph_SurfacePosture_DepthZeroPreservesPostureAndDiagnosticSummary(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	prof := makeProfile("prof-1", "surf-1")
	prof.EscalationTargetID = "et-1"
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{prof}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{makeGrant("grant-1", "prof-1", "agent-1")}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent")
	f.etgts.items["et-1"] = makeEscalationTarget("et-1", "Target")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", 0)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	// At depth=0 only the root BS node remains in Nodes.
	if got.DiagnosticSummary == nil {
		t.Error("depth=0 must preserve DiagnosticSummary")
	}
	if len(got.SurfacePosture) != 1 {
		t.Errorf("depth=0 must preserve SurfacePosture (full pre-depth set); got %d entries", len(got.SurfacePosture))
	}
	if firstPosture(got, "surf-1") == nil {
		t.Error("depth=0 must include surf-1 posture even though the surface node is filtered")
	}
}

// ---------------------------------------------------------------------------
// Diagnostic-association deterministic sort
// ---------------------------------------------------------------------------

func TestAuthorityGraph_SurfacePosture_DiagnosticKindsSortedDeterministically(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	// Profile with empty escalation target (info) + grant with no
	// capabilities (warning) + inactive agent (critical) →
	// diagnostic_kinds must come back sorted alphabetically.
	prof := makeProfile("prof-1", "surf-1")
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{prof}
	g := makeGrant("grant-1", "prof-1", "agent-1")
	g.Capabilities = nil
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{g}
	a := makeAgent("agent-1", "Agent")
	a.OperationalState = agent.OperationalStateSuspended
	f.agents.items["agent-1"] = a

	got, _ := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	p := firstPosture(got, "surf-1")
	if p == nil {
		t.Fatal("posture missing")
	}
	if len(p.DiagnosticKinds) < 2 {
		t.Fatalf("expected multiple diagnostic kinds; got %+v", p.DiagnosticKinds)
	}
	// Verify alphabetical sort.
	for i := 1; i < len(p.DiagnosticKinds); i++ {
		if p.DiagnosticKinds[i-1] >= p.DiagnosticKinds[i] {
			t.Errorf("diagnostic_kinds not sorted ascending: %v", p.DiagnosticKinds)
			break
		}
	}
}

// Keep escalation import live.
var _ = escalation.KindRole
