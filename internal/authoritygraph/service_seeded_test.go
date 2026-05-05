package authoritygraph

// service_seeded_test.go — seeded full-stack regression for the
// authoritygraph projection. Mirrors
// internal/governancemap/read_service_seeded_test.go: instead of
// stubbing, this test runs bootstrap.SeedDemo against the in-memory
// repos, builds a real *governancemap.ReadService, wires it into a
// real Service, and asserts the typed Phase 2A node data is populated
// from the seed.
//
// The pipeline exercised is:
//
//   bootstrap.SeedDemo → memory repos → governancemap.ReadService.GetGovernanceMap
//                                     → authoritygraph.Service.Project
//                                     → typed-data assertions
//
// This closes the Phase 1 review's "no seeded e2e for authoritygraph"
// risk: a bug in projectServiceView interacting with a real seeded
// shape would surface here loudly.

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/bootstrap"
	"github.com/accept-io/midas/internal/governancemap"
	"github.com/accept-io/midas/internal/store/memory"
)

// newSeededService builds a Service backed by real memory repos seeded
// via bootstrap.SeedDemo. Returned ready for Project calls.
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
	return NewService(governancemap.NewReadService(
		repos.BusinessServices,
		repos.BusinessServiceRelationships,
		repos.BusinessServiceCapabilities,
		repos.Capabilities,
		repos.Processes,
		repos.Surfaces,
		repos.Profiles,
		repos.Grants,
		repos.AISystems,
		repos.AISystemVersions,
		repos.AISystemBindings,
	))
}

// TestSeeded_ConsumerLending_TypedDataPopulated asserts that every
// Phase 2A typed-data block carries values for the seeded Consumer
// Lending business service:
//
//   - coverage carries the four count fields (with at least one
//     non-zero — the seed has at least one direct AI binding on
//     surf-v2-id-verify after Phase 9's inheritance work)
//   - authority_summary carries the four count fields with the
//     seed's documented values (1 profile, 1 grant, 1 agent under
//     the surf-v2-id-verify path)
//   - decision_surface nodes carry profile / grant / agent counts
//     and AI binding ID arrays
//   - ai_system_binding nodes carry scope_kind / scope_id /
//     scope_label / ai_system_id
func TestSeeded_ConsumerLending_TypedDataPopulated(t *testing.T) {
	const bsID = "bs-consumer-lending"
	svc := newSeededService(t)

	p, err := svc.Project(context.Background(), ViewService, bsID, MaxDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	var coverageNode *Node
	var summaryNode *Node
	var idVerifyNode *Node
	var anyBindingNode *Node

	for i := range p.Nodes {
		n := &p.Nodes[i]
		switch n.Kind {
		case NodeKindCoverage:
			coverageNode = n
		case NodeKindAuthoritySummary:
			summaryNode = n
		case NodeKindDecisionSurface:
			if n.ID == "surf-v2-id-verify" {
				idVerifyNode = n
			}
		case NodeKindAISystemBinding:
			// Capture any seeded binding for scope-typed-data assertions.
			if anyBindingNode == nil {
				anyBindingNode = n
			}
		}
	}

	// 1. Coverage typed data.
	if coverageNode == nil || coverageNode.Coverage == nil {
		t.Fatal("coverage node or typed data missing")
	}
	cv := coverageNode.Coverage
	if cv.SurfaceCount <= 0 {
		t.Errorf("coverage.surface_count: want > 0, got %d", cv.SurfaceCount)
	}
	// The classification must be self-consistent.
	if cv.SurfacesWithDirectAIBinding+cv.SurfacesWithScopedAIBinding+cv.SurfacesWithNoAIBinding != cv.SurfaceCount {
		t.Errorf("coverage classification not self-consistent: direct=%d scoped=%d none=%d total=%d",
			cv.SurfacesWithDirectAIBinding, cv.SurfacesWithScopedAIBinding,
			cv.SurfacesWithNoAIBinding, cv.SurfaceCount)
	}

	// 2. Authority summary typed data.
	if summaryNode == nil || summaryNode.AuthoritySummary == nil {
		t.Fatal("authority_summary node or typed data missing")
	}
	as := summaryNode.AuthoritySummary
	if as.SurfaceCount != cv.SurfaceCount {
		t.Errorf("authority_summary.surface_count (%d) and coverage.surface_count (%d) must match",
			as.SurfaceCount, cv.SurfaceCount)
	}
	// Phase 9 seed pins: Consumer Lending has profile-v2-onboarding +
	// grant-v2-onboarding + agent-v2-evaluator under surf-v2-id-verify,
	// giving exactly one of each on this BS map.
	if as.ActiveProfileCount != 1 {
		t.Errorf("authority_summary.active_profile_count: want 1 (Phase 9 seed), got %d", as.ActiveProfileCount)
	}
	if as.ActiveGrantCount != 1 {
		t.Errorf("authority_summary.active_grant_count: want 1 (Phase 9 seed), got %d", as.ActiveGrantCount)
	}
	if as.ActiveAgentCount != 1 {
		t.Errorf("authority_summary.active_agent_count: want 1 (Phase 9 seed), got %d", as.ActiveAgentCount)
	}

	// 3. Decision surface typed data on the canonical onboarding surface.
	if idVerifyNode == nil || idVerifyNode.DecisionSurface == nil {
		t.Fatal("decision_surface surf-v2-id-verify or typed data missing")
	}
	ds := idVerifyNode.DecisionSurface
	if ds.ID != "surf-v2-id-verify" {
		t.Errorf("decision_surface.id: want surf-v2-id-verify, got %q", ds.ID)
	}
	if ds.Version <= 0 {
		t.Errorf("decision_surface.version: want > 0, got %d", ds.Version)
	}
	if ds.ProcessID == "" {
		t.Error("decision_surface.process_id must be populated")
	}
	if ds.AIBindingIDs == nil {
		t.Error("decision_surface.ai_binding_ids must be non-nil (possibly empty)")
	}
	if ds.InheritedAIBindingIDs == nil {
		t.Error("decision_surface.inherited_ai_binding_ids must be non-nil (possibly empty)")
	}
	// Phase 9 seed pin: surf-v2-id-verify has exactly one active profile.
	if ds.ProfileCount != 1 {
		t.Errorf("surf-v2-id-verify.profile_count: want 1 (Phase 9 seed), got %d", ds.ProfileCount)
	}
	if ds.GrantCount != 1 {
		t.Errorf("surf-v2-id-verify.grant_count: want 1 (Phase 9 seed), got %d", ds.GrantCount)
	}
	if ds.AgentCount != 1 {
		t.Errorf("surf-v2-id-verify.agent_count: want 1 (Phase 9 seed), got %d", ds.AgentCount)
	}

	// 4. AI binding scope typed data — assertion is conditional on
	// the seed including a binding for Consumer Lending. As of this
	// phase, bootstrap.SeedDemo seeds no AI bindings (AI systems and
	// bindings are reserved for the apply-driven flow), so this block
	// runs only if a future seed extension adds them. Detailed
	// binding-typed-data coverage lives in the stub-fed
	// TestService_FullDemo_AllKindsAndEdges which DOES seed a binding.
	if anyBindingNode != nil && anyBindingNode.AISystemBinding != nil {
		bd := anyBindingNode.AISystemBinding
		if bd.ID == "" {
			t.Error("ai_system_binding.id must be populated")
		}
		if bd.AISystemID == "" {
			t.Error("ai_system_binding.ai_system_id must be populated")
		}
		if bd.ScopeKind == "" {
			t.Error("ai_system_binding.scope_kind must be populated (Phase 2A typed data)")
		}
		if bd.ScopeID == "" {
			t.Error("ai_system_binding.scope_id must be populated (Phase 2A typed data)")
		}
		if bd.ScopeLabel == "" {
			t.Error("ai_system_binding.scope_label must be populated (Phase 2A typed data)")
		}
		switch bd.ScopeKind {
		case NodeKindDecisionSurface, NodeKindProcess, NodeKindCapability, NodeKindBusinessService:
			// OK
		default:
			t.Errorf("ai_system_binding.scope_kind: must be one of decision_surface/process/capability/business_service, got %q", bd.ScopeKind)
		}
	}

	// 5. Business service root carries typed data.
	var bsNode *Node
	for i := range p.Nodes {
		if p.Nodes[i].Kind == NodeKindBusinessService && p.Nodes[i].ID == bsID {
			bsNode = &p.Nodes[i]
			break
		}
	}
	if bsNode == nil || bsNode.BusinessService == nil {
		t.Fatal("root business_service typed data missing")
	}
	if bsNode.BusinessService.ID != bsID {
		t.Errorf("business_service.id: want %s, got %s", bsID, bsNode.BusinessService.ID)
	}
	if bsNode.BusinessService.Status == "" {
		t.Error("business_service.status must be populated from the seed")
	}
}

// TestSeeded_ExclusiveTypedDataPerKind pins the Phase 2A invariant
// that exactly one typed-data pointer is populated per node, matched
// to its kind. A regression that populated multiple pointers (or the
// wrong one) would surface here.
func TestSeeded_ExclusiveTypedDataPerKind(t *testing.T) {
	svc := newSeededService(t)
	p, err := svc.Project(context.Background(), ViewService, "bs-consumer-lending", MaxDepth)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	for _, n := range p.Nodes {
		populated := 0
		matched := false

		check := func(slot string, ok bool) {
			if !ok {
				return
			}
			populated++
			if slot == n.Kind {
				matched = true
			} else {
				t.Errorf("node %s/%s: typed data for wrong kind %q is populated",
					n.Kind, n.ID, slot)
			}
		}
		check(NodeKindBusinessService, n.BusinessService != nil)
		check(NodeKindRelatedBusinessService, n.RelatedBusinessService != nil)
		check(NodeKindCapability, n.Capability != nil)
		check(NodeKindProcess, n.Process != nil)
		check(NodeKindDecisionSurface, n.DecisionSurface != nil)
		check(NodeKindAISystem, n.AISystem != nil)
		check(NodeKindAISystemBinding, n.AISystemBinding != nil)
		check(NodeKindAuthoritySummary, n.AuthoritySummary != nil)
		check(NodeKindCoverage, n.Coverage != nil)

		if populated != 1 {
			t.Errorf("node %s/%s: want exactly 1 typed-data slot populated, got %d",
				n.Kind, n.ID, populated)
		}
		if !matched {
			t.Errorf("node %s/%s: no typed-data slot matches kind", n.Kind, n.ID)
		}
	}
}
