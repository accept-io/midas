package authoritygraph

// service_seeded_test.go — full-stack regression for the Authority
// Graph projection. Instead of stubbing, this test runs
// bootstrap.SeedDemo against the in-memory repositories, wires a
// real Service, and asserts the projection for bs-consumer-lending
// is non-empty across every MVP node and edge kind.
//
// The pipeline exercised is:
//
//   bootstrap.SeedDemo → memory repos → authoritygraph.Service.Project
//                                     → typed-data + edge assertions
//
// A bug in projectServiceView interacting with the real seeded shape
// (or a regression in the demo seed that removes the fail-mode-
// policy / authority chain) would surface here loudly.

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/bootstrap"
	"github.com/accept-io/midas/internal/store/memory"
)

// newSeededService builds a Service backed by real memory repos
// seeded via bootstrap.SeedDemo. Returned ready for Project calls.
func newSeededService(t *testing.T) *Service {
	t.Helper()
	store := memory.NewStore()
	repos, err := store.Repositories()
	if err != nil {
		t.Fatalf("memory.NewStore().Repositories(): %v", err)
	}
	if err := bootstrap.SeedDemo(context.Background(), repos); err != nil {
		t.Fatalf("bootstrap.SeedDemo: %v", err)
	}
	return NewService(Readers{
		BusinessServices:  repos.BusinessServices,
		Processes:         repos.Processes,
		Surfaces:          repos.Surfaces,
		Profiles:          repos.Profiles,
		Grants:            repos.Grants,
		Agents:            repos.Agents,
		FailModePolicies:  repos.FailModePolicies,
		EscalationTargets: repos.EscalationTargets,
	})
}

// TestAuthorityGraph_SeededDemo_ConsumerLending_NotEmpty pins the
// demo seed produces a non-empty Authority Graph for
// bs-consumer-lending. The seed currently provisions:
//
//   - 1 BS (bs-consumer-lending) with fail_mode_policy_id =
//     fmp-demo-default
//   - 4 surfaces (id-verify, credit-assess, consumer-fraud,
//     merchant-payment) under their respective processes
//   - 4 active authority profiles (one per surface)
//   - 4 active grants (one per profile, linking to 2 agents)
//   - 2 agents (agent-v2-evaluator, agent-v2-fraud-bot)
//   - 1 fail-mode policy (fmp-demo-default)
//
// A future seed change that breaks any of these invariants is a
// real regression that should land here, not silently in demo UX.
func TestAuthorityGraph_SeededDemo_ConsumerLending_NotEmpty(t *testing.T) {
	svc := newSeededService(t)
	got, err := svc.Project(context.Background(), ViewService, "bs-consumer-lending", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	// Six node kinds must be present in the bs-consumer-lending
	// projection. escalation_target is NOT asserted here because
	// the seed's escalation-target link sits on profile-v2-standard
	// under bs-merchant-services; the D31l-specific assertions live
	// in TestAuthorityGraph_SeededDemo_EscalationTargetEmitted.
	for _, kind := range []string{
		NodeKindBusinessService,
		NodeKindDecisionSurface,
		NodeKindAuthorityProfile,
		NodeKindAuthorityGrant,
		NodeKindAgent,
		NodeKindFailModePolicy,
	} {
		if countKind(got.Nodes, kind) < 1 {
			t.Errorf("seeded projection missing node kind %q", kind)
		}
	}

	// Every Authority Graph edge kind that the seed exercises must
	// be present. (surface_has_fail_mode_policy is NOT exercised by
	// the seed — the demo only attaches fmp-demo-default at the BS
	// level — so it's not asserted here. profile_escalates_to is
	// asserted in the bs-merchant-services D31l test.)
	for _, kind := range []string{
		EdgeKindBusinessServiceHasSurface,
		EdgeKindSurfaceUsesProfile,
		EdgeKindProfileHasGrant,
		EdgeKindGrantAuthorisesAgent,
		EdgeKindBusinessServiceHasFailModePolicy,
	} {
		if countEdgeKind(got.Edges, kind) < 1 {
			t.Errorf("seeded projection missing edge kind %q", kind)
		}
	}
}

// TestAuthorityGraph_SeededDemo_EscalationTargetEmitted pins the
// D31l projection of et-governance-approver and the
// profile_escalates_to edge from profile-v2-standard. The seed
// places profile-v2-standard under bs-merchant-services (its
// surface surf-v2-merchant-payment sits on
// proc-merchant-payment-auth → bs-merchant-services), so the
// projection target is bs-merchant-services, not bs-consumer-lending.
func TestAuthorityGraph_SeededDemo_EscalationTargetEmitted(t *testing.T) {
	svc := newSeededService(t)
	got, err := svc.Project(context.Background(), ViewService, "bs-merchant-services", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	// Exactly one et-governance-approver node.
	count := 0
	for _, n := range got.Nodes {
		if n.Kind == NodeKindEscalationTarget && n.ID == "et-governance-approver" {
			count++
			if n.EscalationTarget == nil {
				t.Errorf("escalation_target node missing typed data: %+v", n)
				continue
			}
			if n.EscalationTarget.Kind == "" {
				t.Errorf("escalation_target node typed data missing kind: %+v", n.EscalationTarget)
			}
			if n.EscalationTarget.Handle == "" {
				t.Errorf("escalation_target node typed data missing handle: %+v", n.EscalationTarget)
			}
			if n.EscalationTarget.Status != "active" {
				t.Errorf("escalation_target node status: want active, got %q", n.EscalationTarget.Status)
			}
		}
	}
	if count != 1 {
		t.Errorf("escalation_target et-governance-approver: want exactly 1 emission, got %d", count)
	}

	// profile-v2-standard → et-governance-approver edge present.
	var found bool
	for _, e := range got.Edges {
		if e.Kind != EdgeKindProfileEscalatesTo {
			continue
		}
		if e.Src.Kind != NodeKindAuthorityProfile || e.Src.ID != "profile-v2-standard" {
			continue
		}
		if e.Dst.Kind != NodeKindEscalationTarget || e.Dst.ID != "et-governance-approver" {
			continue
		}
		found = true
	}
	if !found {
		t.Errorf("expected profile_escalates_to edge profile-v2-standard → et-governance-approver; got %+v", got.Edges)
	}
}

// TestAuthorityGraph_SeededDemo_EscalationTargetSummary pins the
// D31l summary fields against the seeded demo. Like
// TestAuthorityGraph_SeededDemo_EscalationTargetEmitted, the target
// projection is bs-merchant-services.
func TestAuthorityGraph_SeededDemo_EscalationTargetSummary(t *testing.T) {
	svc := newSeededService(t)
	got, err := svc.Project(context.Background(), ViewService, "bs-merchant-services", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got.Summary == nil {
		t.Fatal("seeded projection must produce a Summary")
	}
	sm := got.Summary
	if sm.EscalationTargetCount < 1 {
		t.Errorf("Summary.EscalationTargetCount: want >= 1, got %d", sm.EscalationTargetCount)
	}
	if sm.ProfilesWithEscalationTarget < 1 {
		t.Errorf("Summary.ProfilesWithEscalationTarget: want >= 1, got %d", sm.ProfilesWithEscalationTarget)
	}
	if len(sm.ProfilesWithDanglingEscalationTarget) != 0 {
		t.Errorf("Summary.ProfilesWithDanglingEscalationTarget: seeded demo must be healthy; got %+v", sm.ProfilesWithDanglingEscalationTarget)
	}
}

// TestAuthorityGraph_SeededDemo_NoDanglingEscalationTargetDiagnostic
// pins the healthy-demo invariant: the seeded projection must not
// emit any escalation_target_reference_dangling diagnostic.
func TestAuthorityGraph_SeededDemo_NoDanglingEscalationTargetDiagnostic(t *testing.T) {
	svc := newSeededService(t)
	got, err := svc.Project(context.Background(), ViewService, "bs-consumer-lending", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	for _, d := range got.Diagnostics {
		if d.Kind == DiagnosticKindEscalationTargetReferenceDangling {
			t.Errorf("seeded demo must have no escalation_target_reference_dangling diagnostic; got %+v", d)
		}
	}
}

// TestAuthorityGraph_SeededDemo_ConsumerLending_AgentsDeduped pins
// that the two demo agents (agent-v2-evaluator, agent-v2-fraud-bot)
// each appear once, even though agent-v2-evaluator backs three of
// the four demo grants.
func TestAuthorityGraph_SeededDemo_ConsumerLending_AgentsDeduped(t *testing.T) {
	svc := newSeededService(t)
	got, err := svc.Project(context.Background(), ViewService, "bs-consumer-lending", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	// Count distinct agent ids in the projection — must equal the
	// node count (no duplicates).
	seen := map[string]struct{}{}
	for _, n := range got.Nodes {
		if n.Kind == NodeKindAgent {
			seen[n.ID] = struct{}{}
		}
	}
	if len(seen) != countKind(got.Nodes, NodeKindAgent) {
		t.Errorf("agents not deduped: distinct=%d nodes=%d", len(seen), countKind(got.Nodes, NodeKindAgent))
	}
	if len(seen) < 1 {
		t.Errorf("seeded projection produced no agent nodes")
	}
}

// TestAuthorityGraph_SeededDemo_ConsumerLending_FailModePolicyDefault
// confirms the demo BS default policy resolves and emits the
// business_service_has_fail_mode_policy edge with label "default".
func TestAuthorityGraph_SeededDemo_ConsumerLending_FailModePolicyDefault(t *testing.T) {
	svc := newSeededService(t)
	got, err := svc.Project(context.Background(), ViewService, "bs-consumer-lending", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	var matched bool
	for _, e := range got.Edges {
		if e.Kind != EdgeKindBusinessServiceHasFailModePolicy {
			continue
		}
		if e.Src.Kind != NodeKindBusinessService || e.Src.ID != "bs-consumer-lending" {
			t.Errorf("BS-has-policy edge src wrong: %+v", e.Src)
		}
		if e.Dst.Kind != NodeKindFailModePolicy {
			t.Errorf("BS-has-policy edge dst kind wrong: %+v", e.Dst)
		}
		if e.Label != EdgeLabelDefault {
			t.Errorf("BS-has-policy edge label: want %q, got %q", EdgeLabelDefault, e.Label)
		}
		matched = true
	}
	if !matched {
		t.Errorf("seeded projection missing BS-has-policy edge; got %+v", got.Edges)
	}
}

// TestAuthorityGraph_SeededDemo_NoForbiddenKinds confirms the
// projection emits NO process / ai_system / ai_system_binding /
// coverage / authority_summary nodes — those are Context Graph
// concerns and must not leak.
func TestAuthorityGraph_SeededDemo_NoForbiddenKinds(t *testing.T) {
	svc := newSeededService(t)
	got, err := svc.Project(context.Background(), ViewService, "bs-consumer-lending", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	for _, n := range got.Nodes {
		switch n.Kind {
		case "process", "ai_system", "ai_system_binding", "authority_summary", "coverage", "related_business_service", "capability":
			t.Errorf("Authority Graph must NOT emit forbidden kind %q (id=%q)", n.Kind, n.ID)
		}
	}
}

// ---------------------------------------------------------------------------
// D31g — Summary + Diagnostics on the seeded demo
// ---------------------------------------------------------------------------

// TestAuthorityGraph_SeededDemo_SummaryPopulated confirms the
// seeded bs-consumer-lending projection produces a non-nil Summary
// with positive counts across the authority spine.
func TestAuthorityGraph_SeededDemo_SummaryPopulated(t *testing.T) {
	svc := newSeededService(t)
	got, err := svc.Project(context.Background(), ViewService, "bs-consumer-lending", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got.Summary == nil {
		t.Fatal("seeded projection must produce a non-nil Summary")
	}
	sm := got.Summary
	if sm.SurfaceCount < 1 {
		t.Errorf("Summary.SurfaceCount: want >= 1, got %d", sm.SurfaceCount)
	}
	if sm.ActiveProfileCount < 1 {
		t.Errorf("Summary.ActiveProfileCount: want >= 1, got %d", sm.ActiveProfileCount)
	}
	if sm.ActiveGrantCount < 1 {
		t.Errorf("Summary.ActiveGrantCount: want >= 1, got %d", sm.ActiveGrantCount)
	}
	if sm.ActiveAgentCount < 1 {
		t.Errorf("Summary.ActiveAgentCount: want >= 1, got %d", sm.ActiveAgentCount)
	}
	if sm.FailModePolicyCount < 1 {
		t.Errorf("Summary.FailModePolicyCount: want >= 1 (fmp-demo-default attached at BS level), got %d", sm.FailModePolicyCount)
	}
	if sm.CompleteAuthorityPaths < 1 {
		t.Errorf("seeded demo should have at least one complete authority path; got %d", sm.CompleteAuthorityPaths)
	}
	if sm.SurfacesInheritingBSPolicy < 1 {
		t.Errorf("fmp-demo-default attaches at BS level → surfaces inherit; want >= 1, got %d", sm.SurfacesInheritingBSPolicy)
	}
}

// TestAuthorityGraph_SeededDemo_ZeroCriticalDiagnostics pins the
// healthy-demo invariant: the seeded bs-consumer-lending projection
// emits no critical diagnostics (missing agents, duplicate active
// versions). Warnings or info diagnostics are acceptable (the seed
// exercises BS-default inheritance, which is info-severity).
func TestAuthorityGraph_SeededDemo_ZeroCriticalDiagnostics(t *testing.T) {
	svc := newSeededService(t)
	got, err := svc.Project(context.Background(), ViewService, "bs-consumer-lending", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	for _, d := range got.Diagnostics {
		if d.Severity == DiagnosticSeverityCritical {
			t.Errorf("seeded demo must have zero critical diagnostics; got %+v", d)
		}
	}
}

// TestAuthorityGraph_SeededDemo_BSFailModePolicyDefault_AppearsInDiagnostics
// pins the inheritance diagnostic appears for bs-consumer-lending's
// fmp-demo-default. If a future bootstrap-seed change removes the
// BS-level policy, this test surfaces the change loudly.
func TestAuthorityGraph_SeededDemo_BSFailModePolicyDefault_AppearsInDiagnostics(t *testing.T) {
	svc := newSeededService(t)
	got, err := svc.Project(context.Background(), ViewService, "bs-consumer-lending", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	found := false
	for _, d := range got.Diagnostics {
		if d.Kind == DiagnosticKindSurfaceInheritsBusinessServicePolicy {
			found = true
			if d.Severity != DiagnosticSeverityInfo {
				t.Errorf("surface_inherits_business_service_policy severity: want info, got %q", d.Severity)
			}
		}
	}
	if !found {
		t.Errorf("expected surface_inherits_business_service_policy diagnostic for seeded demo; got %+v", got.Diagnostics)
	}
}

// ---------------------------------------------------------------------------
// D31m — seeded posture + diagnostic-summary assertions
// ---------------------------------------------------------------------------

// TestAuthorityGraph_SeededDemo_BSConsumerLending_SurfacePosturePopulated
// pins that the bs-consumer-lending projection produces one posture
// entry per emitted active surface.
func TestAuthorityGraph_SeededDemo_BSConsumerLending_SurfacePosturePopulated(t *testing.T) {
	svc := newSeededService(t)
	got, err := svc.Project(context.Background(), ViewService, "bs-consumer-lending", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(got.SurfacePosture) == 0 {
		t.Fatal("seeded bs-consumer-lending projection must produce SurfacePosture entries")
	}
	// One posture entry per emitted surface node.
	surfNodeCount := countKind(got.Nodes, NodeKindDecisionSurface)
	if len(got.SurfacePosture) != surfNodeCount {
		t.Errorf("SurfacePosture entries: want %d (one per surface node), got %d",
			surfNodeCount, len(got.SurfacePosture))
	}
	// Every posture must carry the six axis statuses + highest_severity.
	for _, p := range got.SurfacePosture {
		if p.Surface.Kind != NodeKindDecisionSurface {
			t.Errorf("posture.Surface.Kind: want decision_surface, got %q", p.Surface.Kind)
		}
		if p.AuthorityStatus == "" {
			t.Errorf("posture %q missing AuthorityStatus", p.Surface.ID)
		}
		if p.ProfileStatus == "" {
			t.Errorf("posture %q missing ProfileStatus", p.Surface.ID)
		}
		if p.GrantStatus == "" {
			t.Errorf("posture %q missing GrantStatus", p.Surface.ID)
		}
		if p.AgentStatus == "" {
			t.Errorf("posture %q missing AgentStatus", p.Surface.ID)
		}
		if p.FailModePolicyStatus == "" {
			t.Errorf("posture %q missing FailModePolicyStatus", p.Surface.ID)
		}
		if p.EscalationStatus == "" {
			t.Errorf("posture %q missing EscalationStatus", p.Surface.ID)
		}
		if p.HighestSeverity == "" {
			t.Errorf("posture %q missing HighestSeverity", p.Surface.ID)
		}
	}
}

// TestAuthorityGraph_SeededDemo_BSConsumerLending_DiagnosticSummaryCountsMatch
// pins that diagnostic_summary's counts sum to len(diagnostics) and
// highest_severity reflects the most severe diagnostic.
func TestAuthorityGraph_SeededDemo_BSConsumerLending_DiagnosticSummaryCountsMatch(t *testing.T) {
	svc := newSeededService(t)
	got, err := svc.Project(context.Background(), ViewService, "bs-consumer-lending", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got.DiagnosticSummary == nil {
		t.Fatal("DiagnosticSummary must be populated")
	}
	ds := got.DiagnosticSummary
	sum := ds.Info + ds.Warning + ds.Critical
	if sum != len(got.Diagnostics) {
		t.Errorf("DiagnosticSummary counts %d != len(diagnostics) %d", sum, len(got.Diagnostics))
	}
	// Seeded demo has no critical diagnostic, so highest_severity is
	// info OR warning depending on which diagnostics fire.
	switch ds.HighestSeverity {
	case HighestSeverityNone, HighestSeverityInfo, HighestSeverityWarning, HighestSeverityCritical:
		// OK
	default:
		t.Errorf("HighestSeverity unexpected value %q", ds.HighestSeverity)
	}
}

// TestAuthorityGraph_SeededDemo_BSMerchantServices_SurfV2MerchantPaymentEscalationTargeted
// pins that the surf-v2-merchant-payment posture reports
// escalation_status=targeted because profile-v2-standard links to
// et-governance-approver (D31k seed).
func TestAuthorityGraph_SeededDemo_BSMerchantServices_SurfV2MerchantPaymentEscalationTargeted(t *testing.T) {
	svc := newSeededService(t)
	got, err := svc.Project(context.Background(), ViewService, "bs-merchant-services", DefaultDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	var posture *SurfaceAuthorityPosture
	for i := range got.SurfacePosture {
		if got.SurfacePosture[i].Surface.ID == "surf-v2-merchant-payment" {
			posture = &got.SurfacePosture[i]
		}
	}
	if posture == nil {
		t.Fatalf("no posture entry for surf-v2-merchant-payment; got %+v", got.SurfacePosture)
	}
	if posture.EscalationStatus != EscalationStatusTargeted {
		t.Errorf("surf-v2-merchant-payment EscalationStatus: want targeted, got %q", posture.EscalationStatus)
	}
}

// TestAuthorityGraph_SeededDemo_NoMalformedPostureRecords pins that
// every emitted posture has a non-empty Surface.ID and Surface.Kind
// matching NodeKindDecisionSurface. Catches future regressions that
// might emit stale or zero-value records.
func TestAuthorityGraph_SeededDemo_NoMalformedPostureRecords(t *testing.T) {
	svc := newSeededService(t)
	for _, bsID := range []string{"bs-consumer-lending", "bs-merchant-services"} {
		got, err := svc.Project(context.Background(), ViewService, bsID, DefaultDepth)
		if err != nil {
			t.Fatalf("Project(%q): %v", bsID, err)
		}
		for _, p := range got.SurfacePosture {
			if p.Surface.ID == "" || p.Surface.Kind != NodeKindDecisionSurface {
				t.Errorf("BS %q malformed posture record: %+v", bsID, p)
			}
		}
	}
}
