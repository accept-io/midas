package httpapi

// authority_graph_governance_map_contract_test.go — Phase 2A
// cross-endpoint contract test.
//
// Two endpoints share the same underlying read engine
// (governancemap.ReadService.GetGovernanceMap):
//
//   GET /v1/businessservices/{id}/governance-map
//   GET /v1/authority-graph?view=service&id={id}&depth=5
//
// This test asserts they agree on the structural and numeric content
// the governance map computes — entity counts and the typed coverage /
// authority counts surfaced as Phase 2A typed-data on the synthetic
// nodes. A regression that touches the read service without updating
// the projection (or vice versa) surfaces here loudly.
//
// Memory-backed: seeded via bootstrap.SeedDemo, no DATABASE_URL gate.
// The fixture is the canonical Consumer Lending demo so the asserted
// counts are stable across the project's lifecycle.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/accept-io/midas/internal/authoritygraph"
	"github.com/accept-io/midas/internal/bootstrap"
	"github.com/accept-io/midas/internal/governancemap"
	"github.com/accept-io/midas/internal/store/memory"
)

// newSeededContractServer builds a memory-mode Server with the
// governance-map read service AND the authoritygraph service wired
// against the same shared ReadService instance, mirroring the
// production main.go composition. The store is seeded via
// bootstrap.SeedDemo.
func newSeededContractServer(t *testing.T) *Server {
	t.Helper()
	store := memory.NewStore()
	repos, err := store.Repositories()
	if err != nil {
		t.Fatalf("memory.NewStore().Repositories(): %v", err)
	}
	if err := bootstrap.SeedDemo(context.Background(), repos); err != nil {
		t.Fatalf("bootstrap.SeedDemo: %v", err)
	}
	gmap := governancemap.NewReadService(
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
	)
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil)
	srv.WithStructural(NewStructuralService(repos.Capabilities, repos.Processes, repos.Surfaces).
		WithBusinessServices(repos.BusinessServices).
		WithBusinessServiceRelationships(repos.BusinessServiceRelationships).
		WithBusinessServiceCapabilities(repos.BusinessServiceCapabilities).
		WithAISystems(repos.AISystems, repos.AISystemVersions, repos.AISystemBindings))
	srv.WithGovernanceMap(gmap)
	srv.WithAuthorityGraph(authoritygraph.NewServiceWithReaders(authoritygraph.Readers{GovernanceMap: gmap}))
	return srv
}

// gmapResponseShape is a narrow read-model of the governance-map
// response, restricted to the fields this contract test compares
// against the projection. Mirrors the wire JSON shape verbatim.
type gmapResponseShape struct {
	BusinessService struct {
		ID              string `json:"id"`
		ServiceType     string `json:"service_type"`
		RegulatoryScope string `json:"regulatory_scope"`
	} `json:"business_service"`
	Relationships struct {
		Outgoing []map[string]any `json:"outgoing"`
		Incoming []map[string]any `json:"incoming"`
	} `json:"relationships"`
	Capabilities []map[string]any `json:"capabilities"`
	Processes    []map[string]any `json:"processes"`
	Surfaces     []map[string]any `json:"surfaces"`
	AISystems    []struct {
		ID            string `json:"id"`
		ActiveVersion *struct {
			Version int    `json:"version"`
			Status  string `json:"status"`
		} `json:"active_version"`
		Bindings []struct {
			ID          string `json:"id"`
			Role        string `json:"role"`
			Description string `json:"description"`
		} `json:"bindings"`
	} `json:"ai_systems"`
	AuthoritySummary struct {
		SurfaceCount       int `json:"surface_count"`
		ActiveProfileCount int `json:"active_profile_count"`
		ActiveGrantCount   int `json:"active_grant_count"`
		ActiveAgentCount   int `json:"active_agent_count"`
	} `json:"authority_summary"`
	Coverage struct {
		SurfaceCount                int `json:"surface_count"`
		SurfacesWithDirectAIBinding int `json:"surfaces_with_direct_ai_binding"`
		SurfacesWithScopedAIBinding int `json:"surfaces_with_scoped_ai_binding"`
		SurfacesWithNoAIBinding     int `json:"surfaces_with_no_ai_binding"`
	} `json:"coverage"`
}

// TestContract_AuthorityGraph_AgreesWithGovernanceMap is the headline
// Phase 2A regression: the two endpoints must produce structurally
// equivalent content for the same root.
func TestContract_AuthorityGraph_AgreesWithGovernanceMap(t *testing.T) {
	const bsID = "bs-consumer-lending"
	srv := newSeededContractServer(t)

	// Fetch governance map.
	gmRec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/"+bsID+"/governance-map", nil)
	if gmRec.Code != http.StatusOK {
		t.Fatalf("governance-map: want 200, got %d: %s", gmRec.Code, gmRec.Body.String())
	}
	var gm gmapResponseShape
	if err := json.Unmarshal(gmRec.Body.Bytes(), &gm); err != nil {
		t.Fatalf("decode governance-map: %v", err)
	}

	// Fetch authority graph at MaxDepth so every node is visible.
	agRec := performRequest(t, srv, http.MethodGet, "/v1/authority-graph?view=service&id="+bsID+"&depth=5", nil)
	if agRec.Code != http.StatusOK {
		t.Fatalf("authority-graph: want 200, got %d: %s", agRec.Code, agRec.Body.String())
	}
	var ag authoritygraph.Projection
	if err := json.Unmarshal(agRec.Body.Bytes(), &ag); err != nil {
		t.Fatalf("decode authority-graph: %v", err)
	}

	// Count nodes by kind for the projection.
	nodeCount := map[string]int{}
	var coverageNode *authoritygraph.Node
	var summaryNode *authoritygraph.Node
	for i := range ag.Nodes {
		n := &ag.Nodes[i]
		nodeCount[n.Kind]++
		switch n.Kind {
		case authoritygraph.NodeKindCoverage:
			coverageNode = n
		case authoritygraph.NodeKindAuthoritySummary:
			summaryNode = n
		}
	}

	// 0. Relationship parity (Phase 2B Step 6). The projection emits
	//    one related_business_service node per related-BS id (deduped
	//    when the same id appears in both directions); the
	//    outgoing/incoming GMAP arrays each correspond to a populated
	//    sub-row pointer on the typed data. Counts therefore agree on
	//    each side independently — projection's outgoing-row count
	//    equals gmap.Relationships.Outgoing length, and same for
	//    incoming.
	projOutgoing := 0
	projIncoming := 0
	for i := range ag.Nodes {
		n := &ag.Nodes[i]
		if n.Kind != authoritygraph.NodeKindRelatedBusinessService || n.RelatedBusinessService == nil {
			continue
		}
		if n.RelatedBusinessService.Outgoing != nil {
			projOutgoing++
		}
		if n.RelatedBusinessService.Incoming != nil {
			projIncoming++
		}
	}
	if got, want := projOutgoing, len(gm.Relationships.Outgoing); got != want {
		t.Errorf("relationships.outgoing count: gmap=%d, projection=%d", want, got)
	}
	if got, want := projIncoming, len(gm.Relationships.Incoming); got != want {
		t.Errorf("relationships.incoming count: gmap=%d, projection=%d", want, got)
	}

	// 1. Capability count parity.
	if got, want := nodeCount[authoritygraph.NodeKindCapability], len(gm.Capabilities); got != want {
		t.Errorf("capability count: gmap=%d, projection=%d", want, got)
	}

	// 2. Process count parity.
	if got, want := nodeCount[authoritygraph.NodeKindProcess], len(gm.Processes); got != want {
		t.Errorf("process count: gmap=%d, projection=%d", want, got)
	}

	// 3. Decision surface count parity.
	if got, want := nodeCount[authoritygraph.NodeKindDecisionSurface], len(gm.Surfaces); got != want {
		t.Errorf("decision_surface count: gmap=%d, projection=%d", want, got)
	}

	// 4. AI system count parity (deduped by id).
	gmAISystemIDs := map[string]struct{}{}
	for _, s := range gm.AISystems {
		gmAISystemIDs[s.ID] = struct{}{}
	}
	if got, want := nodeCount[authoritygraph.NodeKindAISystem], len(gmAISystemIDs); got != want {
		t.Errorf("ai_system count: gmap=%d, projection=%d", want, got)
	}

	// 5. AI binding count parity (sum across AI systems).
	totalGmapBindings := 0
	for _, s := range gm.AISystems {
		totalGmapBindings += len(s.Bindings)
	}
	if got, want := nodeCount[authoritygraph.NodeKindAISystemBinding], totalGmapBindings; got != want {
		t.Errorf("ai_system_binding count: gmap=%d, projection=%d", want, got)
	}

	// 6. AuthoritySummary counts parity (typed data on the synthetic node).
	if summaryNode == nil || summaryNode.AuthoritySummary == nil {
		t.Fatalf("authority_summary node missing or has nil typed data")
	}
	as := summaryNode.AuthoritySummary
	if as.SurfaceCount != gm.AuthoritySummary.SurfaceCount {
		t.Errorf("authority_summary.surface_count: gmap=%d, projection=%d",
			gm.AuthoritySummary.SurfaceCount, as.SurfaceCount)
	}
	if as.ActiveProfileCount != gm.AuthoritySummary.ActiveProfileCount {
		t.Errorf("authority_summary.active_profile_count: gmap=%d, projection=%d",
			gm.AuthoritySummary.ActiveProfileCount, as.ActiveProfileCount)
	}
	if as.ActiveGrantCount != gm.AuthoritySummary.ActiveGrantCount {
		t.Errorf("authority_summary.active_grant_count: gmap=%d, projection=%d",
			gm.AuthoritySummary.ActiveGrantCount, as.ActiveGrantCount)
	}
	if as.ActiveAgentCount != gm.AuthoritySummary.ActiveAgentCount {
		t.Errorf("authority_summary.active_agent_count: gmap=%d, projection=%d",
			gm.AuthoritySummary.ActiveAgentCount, as.ActiveAgentCount)
	}

	// 7. Coverage counts parity (typed data on the synthetic node).
	if coverageNode == nil || coverageNode.Coverage == nil {
		t.Fatalf("coverage node missing or has nil typed data")
	}
	cv := coverageNode.Coverage
	if cv.SurfaceCount != gm.Coverage.SurfaceCount {
		t.Errorf("coverage.surface_count: gmap=%d, projection=%d",
			gm.Coverage.SurfaceCount, cv.SurfaceCount)
	}
	if cv.SurfacesWithDirectAIBinding != gm.Coverage.SurfacesWithDirectAIBinding {
		t.Errorf("coverage.surfaces_with_direct_ai_binding: gmap=%d, projection=%d",
			gm.Coverage.SurfacesWithDirectAIBinding, cv.SurfacesWithDirectAIBinding)
	}
	if cv.SurfacesWithScopedAIBinding != gm.Coverage.SurfacesWithScopedAIBinding {
		t.Errorf("coverage.surfaces_with_scoped_ai_binding: gmap=%d, projection=%d",
			gm.Coverage.SurfacesWithScopedAIBinding, cv.SurfacesWithScopedAIBinding)
	}
	if cv.SurfacesWithNoAIBinding != gm.Coverage.SurfacesWithNoAIBinding {
		t.Errorf("coverage.surfaces_with_no_ai_binding: gmap=%d, projection=%d",
			gm.Coverage.SurfacesWithNoAIBinding, cv.SurfacesWithNoAIBinding)
	}

	// 8. Coverage classification self-consistency: direct + scoped + none == surface_count.
	sum := cv.SurfacesWithDirectAIBinding + cv.SurfacesWithScopedAIBinding + cv.SurfacesWithNoAIBinding
	if sum != cv.SurfaceCount {
		t.Errorf("coverage classification: direct(%d) + scoped(%d) + none(%d) = %d, want surface_count=%d",
			cv.SurfacesWithDirectAIBinding, cv.SurfacesWithScopedAIBinding,
			cv.SurfacesWithNoAIBinding, sum, cv.SurfaceCount)
	}
}

// TestContract_AuthorityGraph_Phase2BFieldParity asserts that the
// five Phase 2B-added typed-data fields on the authority-graph
// projection agree with the same fields on the governance-map
// response for the same seeded fixture:
//
//   - business_service.service_type
//   - business_service.regulatory_scope
//   - ai_system_binding.role
//   - ai_system_binding.description
//   - ai_system.active_version_status (mapped from
//     gmap.ai_systems[].active_version.status)
//
// Each assertion compares only when the gmap reports a value or a
// pointer is set on the gmap side; "absent on gmap" trivially agrees
// with "absent on projection" and is not asserted to make the test
// resilient to a seed that does not happen to populate every field.
// The seeded fixture is the canonical Consumer Lending demo, which
// pins service_type and regulatory_scope on the BS.
func TestContract_AuthorityGraph_Phase2BFieldParity(t *testing.T) {
	const bsID = "bs-consumer-lending"
	srv := newSeededContractServer(t)

	gmRec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/"+bsID+"/governance-map", nil)
	if gmRec.Code != http.StatusOK {
		t.Fatalf("governance-map: want 200, got %d: %s", gmRec.Code, gmRec.Body.String())
	}
	var gm gmapResponseShape
	if err := json.Unmarshal(gmRec.Body.Bytes(), &gm); err != nil {
		t.Fatalf("decode governance-map: %v", err)
	}

	agRec := performRequest(t, srv, http.MethodGet, "/v1/authority-graph?view=service&id="+bsID+"&depth=5", nil)
	if agRec.Code != http.StatusOK {
		t.Fatalf("authority-graph: want 200, got %d: %s", agRec.Code, agRec.Body.String())
	}
	var ag authoritygraph.Projection
	if err := json.Unmarshal(agRec.Body.Bytes(), &ag); err != nil {
		t.Fatalf("decode authority-graph: %v", err)
	}

	// Index projection nodes by (kind, id).
	var rootBSNode *authoritygraph.Node
	bindingsByID := map[string]*authoritygraph.AISystemBindingData{}
	aiSystemsByID := map[string]*authoritygraph.AISystemData{}
	for i := range ag.Nodes {
		n := &ag.Nodes[i]
		switch n.Kind {
		case authoritygraph.NodeKindBusinessService:
			if n.ID == bsID {
				rootBSNode = n
			}
		case authoritygraph.NodeKindAISystemBinding:
			if n.AISystemBinding != nil {
				bindingsByID[n.AISystemBinding.ID] = n.AISystemBinding
			}
		case authoritygraph.NodeKindAISystem:
			if n.AISystem != nil {
				aiSystemsByID[n.AISystem.ID] = n.AISystem
			}
		}
	}

	// 1. business_service.service_type / regulatory_scope parity.
	if rootBSNode == nil || rootBSNode.BusinessService == nil {
		t.Fatalf("root business_service typed data missing on projection")
	}
	if got, want := rootBSNode.BusinessService.ServiceType, gm.BusinessService.ServiceType; got != want {
		t.Errorf("business_service.service_type: gmap=%q, projection=%q", want, got)
	}
	if got, want := rootBSNode.BusinessService.RegulatoryScope, gm.BusinessService.RegulatoryScope; got != want {
		t.Errorf("business_service.regulatory_scope: gmap=%q, projection=%q", want, got)
	}

	// 2. AI binding role / description parity for every gmap binding.
	for _, sys := range gm.AISystems {
		for _, b := range sys.Bindings {
			pb, ok := bindingsByID[b.ID]
			if !ok {
				t.Errorf("binding %s present on gmap but missing from projection", b.ID)
				continue
			}
			if pb.Role != b.Role {
				t.Errorf("binding %s role: gmap=%q, projection=%q", b.ID, b.Role, pb.Role)
			}
			if pb.Description != b.Description {
				t.Errorf("binding %s description: gmap=%q, projection=%q", b.ID, b.Description, pb.Description)
			}
		}
	}

	// 3. AI system active_version status parity (only when gmap has
	// an active version).
	for _, sys := range gm.AISystems {
		if sys.ActiveVersion == nil {
			continue
		}
		ps, ok := aiSystemsByID[sys.ID]
		if !ok {
			t.Errorf("ai_system %s present on gmap but missing from projection", sys.ID)
			continue
		}
		if ps.ActiveVersionStatus != sys.ActiveVersion.Status {
			t.Errorf("ai_system %s active_version_status: gmap=%q, projection=%q",
				sys.ID, sys.ActiveVersion.Status, ps.ActiveVersionStatus)
		}
	}
}
