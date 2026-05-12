package authoritygraph

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
	"github.com/accept-io/midas/internal/value"
)

// TestAuthorityGraph_GrantData_CapabilitiesAndConstraints_Emitted
// pins that the AuthorityGrant typed-data slot carries the grant's
// Capabilities and Constraints onto the wire shape.
func TestAuthorityGraph_GrantData_CapabilitiesAndConstraints_Emitted(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}

	g := makeGrant("grant-1", "prof-1", "agent-1")
	g.Capabilities = []authority.Capability{
		authority.CapabilityRecommend,
		authority.CapabilityApprove,
		authority.CapabilityStop,
	}
	g.Constraints = []authority.Constraint{
		{Kind: authority.ConstraintKindConfidenceThresholdMin, MinConfidence: 0.85},
		{Kind: authority.ConstraintKindHumanOnly},
	}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{g}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent One")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	var emitted *AuthorityGrantData
	for _, n := range got.Nodes {
		if n.Kind == NodeKindAuthorityGrant && n.ID == "grant-1" {
			emitted = n.AuthorityGrant
		}
	}
	if emitted == nil {
		t.Fatal("grant node not emitted")
	}
	if len(emitted.Capabilities) != 3 {
		t.Errorf("capabilities len: want 3, got %d (%v)", len(emitted.Capabilities), emitted.Capabilities)
	}
	wantCaps := map[string]bool{"recommend": true, "approve": true, "stop": true}
	for _, c := range emitted.Capabilities {
		if !wantCaps[c] {
			t.Errorf("unexpected capability %q on wire", c)
		}
	}
	if len(emitted.Constraints) != 2 {
		t.Fatalf("constraints len: want 2, got %d (%v)", len(emitted.Constraints), emitted.Constraints)
	}
	if emitted.Constraints[0].Kind != "confidence_threshold_min" || emitted.Constraints[0].MinConfidence == nil || *emitted.Constraints[0].MinConfidence != 0.85 {
		t.Errorf("Constraints[0]: want confidence_threshold_min/0.85; got %+v", emitted.Constraints[0])
	}
	if emitted.Constraints[1].Kind != "human_only" {
		t.Errorf("Constraints[1].Kind: want human_only; got %q", emitted.Constraints[1].Kind)
	}
}

// TestAuthorityGraph_GrantWithoutCapabilities_EmitsWarning pins the
// new D31i diagnostic: an emitted grant carrying zero Capabilities
// produces a warning.
func TestAuthorityGraph_GrantWithoutCapabilities_EmitsWarning(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	// Grant carries NO capabilities.
	g := makeGrant("grant-empty-caps", "prof-1", "agent-1")
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{g}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent One")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	found := false
	for _, d := range got.Diagnostics {
		if d.Kind != DiagnosticKindGrantHasNoCapabilities {
			continue
		}
		found = true
		if d.Severity != DiagnosticSeverityWarning {
			t.Errorf("grant_has_no_capabilities severity: want warning, got %q", d.Severity)
		}
		if len(d.NodeRefs) == 0 || d.NodeRefs[0].ID != "grant-empty-caps" {
			t.Errorf("grant_has_no_capabilities NodeRefs wrong: %+v", d.NodeRefs)
		}
	}
	if !found {
		t.Errorf("expected grant_has_no_capabilities diagnostic; got %+v", got.Diagnostics)
	}
	// Summary should list the grant in GrantsWithoutCapabilities.
	if got.Summary == nil || len(got.Summary.GrantsWithoutCapabilities) != 1 {
		t.Fatalf("Summary.GrantsWithoutCapabilities expected 1 entry; got %+v", got.Summary)
	}
	if got.Summary.GrantsWithoutCapabilities[0].ID != "grant-empty-caps" {
		t.Errorf("Summary.GrantsWithoutCapabilities[0]: want grant-empty-caps, got %q", got.Summary.GrantsWithoutCapabilities[0].ID)
	}
}

// TestAuthorityGraph_Summary_GrantsWithStopCapability pins that
// Summary.GrantsWithStopCapability counts emitted grants carrying the
// stop capability — once per grant even when reached via multiple
// profiles is not a concern at MVP (one grant ↔ one profile id).
func TestAuthorityGraph_Summary_GrantsWithStopCapability(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{
		makeSurface("surf-a", "proc-1"),
		makeSurface("surf-b", "proc-1"),
	}
	f.profiles.bySurface["surf-a"] = []*authority.AuthorityProfile{makeProfile("prof-a", "surf-a")}
	f.profiles.bySurface["surf-b"] = []*authority.AuthorityProfile{makeProfile("prof-b", "surf-b")}

	gA := makeGrant("grant-a", "prof-a", "agent-1")
	gA.Capabilities = []authority.Capability{authority.CapabilityApprove, authority.CapabilityStop}
	gB := makeGrant("grant-b", "prof-b", "agent-1")
	gB.Capabilities = []authority.Capability{authority.CapabilityApprove}

	f.grants.byProfile["prof-a"] = []*authority.AuthorityGrant{gA}
	f.grants.byProfile["prof-b"] = []*authority.AuthorityGrant{gB}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got.Summary == nil {
		t.Fatal("Summary nil")
	}
	if got.Summary.GrantsWithStopCapability != 1 {
		t.Errorf("GrantsWithStopCapability: want 1, got %d", got.Summary.GrantsWithStopCapability)
	}
}

// TestAuthorityGraph_Summary_GrantsWithConstraints pins the
// constraint-presence counter increments per emitted grant carrying
// at least one Constraint.
func TestAuthorityGraph_Summary_GrantsWithConstraints(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{
		makeSurface("surf-a", "proc-1"),
		makeSurface("surf-b", "proc-1"),
	}
	f.profiles.bySurface["surf-a"] = []*authority.AuthorityProfile{makeProfile("prof-a", "surf-a")}
	f.profiles.bySurface["surf-b"] = []*authority.AuthorityProfile{makeProfile("prof-b", "surf-b")}

	gWithConstraint := makeGrant("grant-c", "prof-a", "agent-1")
	gWithConstraint.Capabilities = []authority.Capability{authority.CapabilityApprove}
	gWithConstraint.Constraints = []authority.Constraint{
		{Kind: authority.ConstraintKindAIOnly},
	}
	gNoConstraint := makeGrant("grant-nc", "prof-b", "agent-1")
	gNoConstraint.Capabilities = []authority.Capability{authority.CapabilityApprove}

	f.grants.byProfile["prof-a"] = []*authority.AuthorityGrant{gWithConstraint}
	f.grants.byProfile["prof-b"] = []*authority.AuthorityGrant{gNoConstraint}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got.Summary == nil {
		t.Fatal("Summary nil")
	}
	if got.Summary.GrantsWithConstraints != 1 {
		t.Errorf("GrantsWithConstraints: want 1, got %d", got.Summary.GrantsWithConstraints)
	}
}

// TestAuthorityGraph_ConstraintData_TimeWindow_Emitted pins the
// time_window constraint variant's wire shape (start_time + end_time
// as RFC3339 strings).
func TestAuthorityGraph_ConstraintData_TimeWindow_Emitted(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 1, 17, 0, 0, 0, time.UTC)
	g := makeGrant("grant-tw", "prof-1", "agent-1")
	g.Capabilities = []authority.Capability{authority.CapabilityApprove}
	g.Constraints = []authority.Constraint{
		{Kind: authority.ConstraintKindTimeWindow, StartTime: start, EndTime: end},
	}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{g}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	var grant *AuthorityGrantData
	for _, n := range got.Nodes {
		if n.Kind == NodeKindAuthorityGrant {
			grant = n.AuthorityGrant
		}
	}
	if grant == nil || len(grant.Constraints) != 1 {
		t.Fatalf("expected one constraint on emitted grant; got %+v", grant)
	}
	cd := grant.Constraints[0]
	if cd.Kind != "time_window" {
		t.Errorf("Kind: want time_window, got %q", cd.Kind)
	}
	if cd.StartTime == nil || *cd.StartTime != "2026-06-01T09:00:00Z" {
		t.Errorf("StartTime: want 2026-06-01T09:00:00Z, got %v", cd.StartTime)
	}
	if cd.EndTime == nil || *cd.EndTime != "2026-06-01T17:00:00Z" {
		t.Errorf("EndTime: want 2026-06-01T17:00:00Z, got %v", cd.EndTime)
	}
}

// TestAuthorityGraph_ConstraintData_ConsequenceThresholdMax_Emitted
// pins the consequence_threshold_max variant carries the typed
// AuthorityGraphConsequenceThreshold payload.
func TestAuthorityGraph_ConstraintData_ConsequenceThresholdMax_Emitted(t *testing.T) {
	f := newFixture()
	f.bs.items["bs-1"] = makeBS("bs-1", "BS One")
	f.procs.byBS["bs-1"] = []*process.Process{makeProc("proc-1", "bs-1")}
	f.surfs.byProcess["proc-1"] = []*surface.DecisionSurface{makeSurface("surf-1", "proc-1")}
	f.profiles.bySurface["surf-1"] = []*authority.AuthorityProfile{makeProfile("prof-1", "surf-1")}
	g := makeGrant("grant-ct", "prof-1", "agent-1")
	g.Capabilities = []authority.Capability{authority.CapabilityApprove}
	g.Constraints = []authority.Constraint{{
		Kind: authority.ConstraintKindConsequenceThresholdMax,
		MaxConsequence: authority.Consequence{
			Type: value.ConsequenceTypeMonetary, Amount: 100, Currency: "USD",
		},
	}}
	f.grants.byProfile["prof-1"] = []*authority.AuthorityGrant{g}
	f.agents.items["agent-1"] = makeAgent("agent-1", "Agent")

	got, err := f.service().Project(context.Background(), ViewService, "bs-1", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	var grant *AuthorityGrantData
	for _, n := range got.Nodes {
		if n.Kind == NodeKindAuthorityGrant {
			grant = n.AuthorityGrant
		}
	}
	if grant == nil || len(grant.Constraints) != 1 {
		t.Fatalf("expected one constraint; got %+v", grant)
	}
	cd := grant.Constraints[0]
	if cd.MaxConsequence == nil {
		t.Fatal("MaxConsequence missing")
	}
	if cd.MaxConsequence.Type != "monetary" {
		t.Errorf("MaxConsequence.Type: want monetary, got %q", cd.MaxConsequence.Type)
	}
	if cd.MaxConsequence.Amount != 100 {
		t.Errorf("MaxConsequence.Amount: want 100, got %v", cd.MaxConsequence.Amount)
	}
}

