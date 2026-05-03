package httpapi

// End-to-end integration test for the Governance Map read model
// (Epic 1, PR 4, Cluster D). Applies a complete service-led bundle
// (BS + relationship + capability + BSC + process + active surface +
// AI system + version + binding) through the apply path, then queries
// GET /v1/businessservices/{id}/governance-map and asserts every
// section the read service assembles.
//
// Memory-backed: no DATABASE_URL gate. Reuses the existing helpers
// (memory.NewRepositories, apply.NewServiceWithRepos, NewStructuralService,
// performRequest) so no new test infrastructure is introduced.
//
// What this test pins that lower layers do not:
//
//   - The full apply → store → governancemap.ReadService → wire chain
//     produces a coherent map (no nil-map panics, no missing sections).
//   - The recent_decisions field is literally absent from the wire body
//     end-to-end (Step 0.5 deferral; PR 8 hand-off marker).
//   - external_ref renders verbatim through the four-layer pipeline on
//     the BS node when populated by apply.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/governancemap"
	"github.com/accept-io/midas/internal/store/memory"
	"github.com/accept-io/midas/internal/surface"
)

func TestGovernanceMap_E2E_ApplyThenRead(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices:             repos.BusinessServices,
		BusinessServiceRelationships: repos.BusinessServiceRelationships,
		BusinessServiceCapabilities:  repos.BusinessServiceCapabilities,
		Capabilities:                 repos.Capabilities,
		Processes:                    repos.Processes,
		Surfaces:                     repos.Surfaces,
		Profiles:                     repos.Profiles,
		Agents:                       repos.Agents,
		Grants:                       repos.Grants,
		AISystems:                    repos.AISystems,
		AISystemVersions:             repos.AISystemVersions,
		AISystemBindings:             repos.AISystemBindings,
		ControlAudit:                 repos.ControlAudit,
	})

	pinnedVersion := 1
	bundle := []parser.ParsedDocument{
		{Kind: types.KindBusinessService, ID: "bs-gm-a", Doc: types.BusinessServiceDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindBusinessService,
			Metadata: types.DocumentMetadata{ID: "bs-gm-a", Name: "BS A"},
			Spec: types.BusinessServiceSpec{
				ServiceType: "internal", Status: "active",
				ExternalRef: &types.ExternalRefSpec{
					SourceSystem: "github", SourceID: "accept-io/midas",
				},
			},
		}},
		{Kind: types.KindBusinessService, ID: "bs-gm-b", Doc: types.BusinessServiceDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindBusinessService,
			Metadata: types.DocumentMetadata{ID: "bs-gm-b", Name: "BS B"},
			Spec:     types.BusinessServiceSpec{ServiceType: "internal", Status: "active"},
		}},
		{Kind: types.KindBusinessServiceRelationship, ID: "rel-gm", Doc: types.BusinessServiceRelationshipDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindBusinessServiceRelationship,
			Metadata: types.DocumentMetadata{ID: "rel-gm"},
			Spec: types.BusinessServiceRelationshipSpec{
				SourceBusinessServiceID: "bs-gm-a", TargetBusinessServiceID: "bs-gm-b",
				RelationshipType: "depends_on",
			},
		}},
		{Kind: types.KindCapability, ID: "cap-gm", Doc: types.CapabilityDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindCapability,
			Metadata: types.DocumentMetadata{ID: "cap-gm", Name: "Cap"},
			Spec:     types.CapabilitySpec{Status: "active"},
		}},
		{Kind: types.KindBusinessServiceCapability, ID: "bsc-gm", Doc: types.BusinessServiceCapabilityDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindBusinessServiceCapability,
			Metadata: types.DocumentMetadata{ID: "bsc-gm"},
			Spec: types.BusinessServiceCapabilitySpec{
				BusinessServiceID: "bs-gm-a", CapabilityID: "cap-gm",
			},
		}},
		{Kind: types.KindProcess, ID: "proc-gm", Doc: types.ProcessDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindProcess,
			Metadata: types.DocumentMetadata{ID: "proc-gm", Name: "Proc"},
			Spec:     types.ProcessSpec{BusinessServiceID: "bs-gm-a", Status: "active"},
		}},
		{Kind: types.KindSurface, ID: "surf-gm", Doc: types.SurfaceDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindSurface,
			Metadata: types.DocumentMetadata{ID: "surf-gm", Name: "Surface"},
			Spec: types.SurfaceSpec{
				Category: "test", RiskTier: "low",
				Status: "active", ProcessID: "proc-gm",
			},
		}},
		{Kind: types.KindAgent, ID: "agent-gm", Doc: types.AgentDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindAgent,
			Metadata: types.DocumentMetadata{ID: "agent-gm", Name: "Agent"},
			Spec:     types.AgentSpec{Type: "automation", Status: "active"},
		}},
		{Kind: types.KindProfile, ID: "prof-gm", Doc: types.ProfileDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindProfile,
			Metadata: types.DocumentMetadata{ID: "prof-gm", Name: "Profile"},
			Spec: types.ProfileSpec{
				SurfaceID: "surf-gm",
				Authority: types.ProfileAuthority{
					DecisionConfidenceThreshold: 0.5,
					ConsequenceThreshold:        types.ConsequenceThreshold{Type: "monetary", Amount: 100, Currency: "USD"},
				},
				Policy:    types.ProfilePolicy{Reference: "rego://test/v1", FailMode: "open"},
				Lifecycle: types.ProfileLifecycle{Status: "active"},
			},
		}},
		{Kind: types.KindGrant, ID: "grant-gm", Doc: types.GrantDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindGrant,
			Metadata: types.DocumentMetadata{ID: "grant-gm", Name: "Grant"},
			Spec: types.GrantSpec{
				AgentID: "agent-gm", ProfileID: "prof-gm", Status: "active",
				GrantedBy: "team-gm", GrantedAt: "2026-01-01T00:00:00Z",
				EffectiveFrom: "2026-01-01T00:00:00Z",
			},
		}},
		{Kind: types.KindAISystem, ID: "ai-gm", Doc: types.AISystemDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindAISystem,
			Metadata: types.DocumentMetadata{ID: "ai-gm", Name: "AI"},
			Spec:     types.AISystemSpec{Status: "active", Origin: "manual", SystemType: "llm"},
		}},
		{Kind: types.KindAISystemVersion, ID: "aiv-gm", Doc: types.AISystemVersionDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindAISystemVersion,
			Metadata: types.DocumentMetadata{ID: "aiv-gm"},
			Spec: types.AISystemVersionSpec{
				AISystemID: "ai-gm", Version: 1, Status: "active",
				EffectiveFrom: "2026-01-01T00:00:00Z",
			},
		}},
		{Kind: types.KindAISystemBinding, ID: "bind-gm", Doc: types.AISystemBindingDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindAISystemBinding,
			Metadata: types.DocumentMetadata{ID: "bind-gm"},
			Spec: types.AISystemBindingSpec{
				AISystemID: "ai-gm", AISystemVersion: &pinnedVersion,
				SurfaceID: "surf-gm", Role: "primary-evaluator",
			},
		}},
	}

	result := svc.Apply(context.Background(), bundle, "operator:gm-e2e")
	if result.ValidationErrorCount() != 0 || result.ApplyErrorCount() != 0 {
		t.Fatalf("apply errors: validation=%v apply=%v", result.ValidationErrors, result.Results)
	}
	if result.CreatedCount() != len(bundle) {
		t.Fatalf("CreatedCount: want %d, got %d", len(bundle), result.CreatedCount())
	}

	// Apply lands new surfaces and profiles in `review` (regardless of
	// the spec.status value the operator wrote) — the active state is
	// reached through a separate approval flow not under test here.
	// Promote both directly via the repos so the read service's
	// `Status == active` filters surface non-empty sections. This mirrors
	// what an approval endpoint would do post-PR.
	ctx := context.Background()
	surf, _ := repos.Surfaces.FindLatestByID(ctx, "surf-gm")
	surf.Status = surface.SurfaceStatusActive
	if err := repos.Surfaces.Update(ctx, surf); err != nil {
		t.Fatalf("promote surface: %v", err)
	}
	prof, _ := repos.Profiles.FindByID(ctx, "prof-gm")
	prof.Status = authority.ProfileStatusActive
	if err := repos.Profiles.Update(ctx, prof); err != nil {
		t.Fatalf("promote profile: %v", err)
	}

	// Wire the server with a real read service over the same memory aggregate.
	structural := NewStructuralService(repos.Capabilities, repos.Processes, repos.Surfaces).
		WithBusinessServices(repos.BusinessServices).
		WithBusinessServiceRelationships(repos.BusinessServiceRelationships).
		WithAISystems(repos.AISystems, repos.AISystemVersions, repos.AISystemBindings)
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(structural)
	srv.WithGovernanceMap(governancemap.NewReadService(
		repos.BusinessServices, repos.BusinessServiceRelationships,
		repos.BusinessServiceCapabilities, repos.Capabilities,
		repos.Processes, repos.Surfaces, repos.Profiles, repos.Grants,
		repos.AISystems, repos.AISystemVersions, repos.AISystemBindings,
	))

	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-gm-a/governance-map", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}

	// Step 0.5 deferral: recent_decisions must be literally absent from
	// the body (no key, no null). PR 8 will lift this assertion.
	if strings.Contains(rec.Body.String(), "recent_decisions") {
		t.Errorf("recent_decisions key must be absent end-to-end (Step 0.5 deferral)")
	}

	var resp governanceMapResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Business service node — identity + ExternalRef round-trip.
	if resp.BusinessService.ID != "bs-gm-a" || resp.BusinessService.Name != "BS A" {
		t.Errorf("BS identity: %+v", resp.BusinessService)
	}
	if resp.BusinessService.ExternalRef == nil ||
		resp.BusinessService.ExternalRef.SourceSystem != "github" ||
		resp.BusinessService.ExternalRef.SourceID != "accept-io/midas" {
		t.Errorf("BS external_ref: %+v", resp.BusinessService.ExternalRef)
	}

	// Relationships — outgoing populated, target name resolved from BS B.
	if len(resp.Relationships.Outgoing) != 1 {
		t.Fatalf("outgoing: want 1, got %d", len(resp.Relationships.Outgoing))
	}
	out := resp.Relationships.Outgoing[0]
	if out.TargetBusinessServiceID != "bs-gm-b" || out.OtherName != "BS B" || out.RelationshipType != "depends_on" {
		t.Errorf("outgoing relationship: %+v", out)
	}

	// Capabilities + processes filtered by BS link.
	if len(resp.Capabilities) != 1 || resp.Capabilities[0].ID != "cap-gm" {
		t.Errorf("capabilities: %+v", resp.Capabilities)
	}
	if len(resp.Processes) != 1 || resp.Processes[0].ID != "proc-gm" {
		t.Errorf("processes: %+v", resp.Processes)
	}

	// Surfaces — single active surface, AI binding attached, per-surface counts.
	if len(resp.Surfaces) != 1 {
		t.Fatalf("surfaces: want 1, got %d", len(resp.Surfaces))
	}
	s := resp.Surfaces[0]
	if s.ID != "surf-gm" || s.Status != "active" {
		t.Errorf("surface identity: %+v", s)
	}
	if len(s.AIBindings) != 1 || s.AIBindings[0] != "bind-gm" {
		t.Errorf("surface ai_bindings: %+v", s.AIBindings)
	}
	if s.ProfileCount != 1 || s.GrantCount != 1 || s.AgentCount != 1 {
		t.Errorf("surface counts: profile=%d grant=%d agent=%d", s.ProfileCount, s.GrantCount, s.AgentCount)
	}

	// AI systems — discovered via the surface-scoped binding, active version pinned.
	if len(resp.AISystems) != 1 {
		t.Fatalf("ai_systems: want 1, got %d", len(resp.AISystems))
	}
	ai := resp.AISystems[0]
	if ai.ID != "ai-gm" || ai.ActiveVersion == nil || ai.ActiveVersion.Version != 1 {
		t.Errorf("ai system / active version: %+v", ai)
	}
	if len(ai.Bindings) != 1 || ai.Bindings[0].ID != "bind-gm" {
		t.Errorf("ai system bindings: %+v", ai.Bindings)
	}
	if ai.Bindings[0].SurfaceID == nil || *ai.Bindings[0].SurfaceID != "surf-gm" {
		t.Errorf("binding surface_id: %v", ai.Bindings[0].SurfaceID)
	}

	// Authority summary — distinct counts at each level.
	if resp.AuthoritySummary.SurfaceCount != 1 ||
		resp.AuthoritySummary.ActiveProfileCount != 1 ||
		resp.AuthoritySummary.ActiveGrantCount != 1 ||
		resp.AuthoritySummary.ActiveAgentCount != 1 {
		t.Errorf("authority_summary: %+v", resp.AuthoritySummary)
	}

	// Coverage — the surface has an AI binding attached.
	if resp.Coverage.SurfaceCount != 1 ||
		resp.Coverage.SurfacesWithAIBinding != 1 ||
		resp.Coverage.SurfacesWithoutAIBinding != 0 {
		t.Errorf("coverage: %+v", resp.Coverage)
	}
}
