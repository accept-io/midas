package httpapi

// End-to-end integration test for the AI System Registration substrate
// (Epic 1, PR 2, Cluster D). Applies a bundle (BS + AISystem +
// AISystemVersion + AISystemBinding) through the apply path, then
// queries the read endpoints to verify the bundle landed and is
// observable through the HTTP layer.
//
// Memory-backed: no DATABASE_URL gate. Reuses existing helpers
// (memory.NewRepositories, apply.NewServiceWithRepos, NewStructuralService,
// performRequest) so no new test infrastructure is introduced.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/store/memory"
)

func TestAISystem_EndToEnd_ApplyThenRead(t *testing.T) {
	// 1. Memory-backed repositories — wires every FK validator that
	// production memory mode uses.
	repos := memory.NewRepositories()

	// 2. Apply service over the same repository aggregate.
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices: repos.BusinessServices,
		AISystems:        repos.AISystems,
		AISystemVersions: repos.AISystemVersions,
		AISystemBindings: repos.AISystemBindings,
		ControlAudit:     repos.ControlAudit,
	})

	// 3. Bundle: BS → AISystem → AISystemVersion → AISystemBinding
	// (apply order tier 0 → 10 → 11 → 12).
	pinnedVersion := 1
	bundle := []parser.ParsedDocument{
		{
			Kind: types.KindBusinessService, ID: "bs-e2e",
			Doc: types.BusinessServiceDocument{
				APIVersion: types.APIVersionV1, Kind: types.KindBusinessService,
				Metadata: types.DocumentMetadata{ID: "bs-e2e", Name: "E2E BS"},
				Spec:     types.BusinessServiceSpec{ServiceType: "internal", Status: "active"},
			},
		},
		{
			Kind: types.KindAISystem, ID: "ai-e2e",
			Doc: types.AISystemDocument{
				APIVersion: types.APIVersionV1, Kind: types.KindAISystem,
				Metadata: types.DocumentMetadata{ID: "ai-e2e", Name: "E2E AISystem"},
				Spec: types.AISystemSpec{
					Description: "End-to-end test system",
					Status:      "active", Origin: "manual", SystemType: "llm",
				},
			},
		},
		{
			Kind: types.KindAISystemVersion, ID: "aiv-e2e-v1",
			Doc: types.AISystemVersionDocument{
				APIVersion: types.APIVersionV1, Kind: types.KindAISystemVersion,
				Metadata: types.DocumentMetadata{ID: "aiv-e2e-v1"},
				Spec: types.AISystemVersionSpec{
					AISystemID:    "ai-e2e",
					Version:       1,
					Status:        "active",
					EffectiveFrom: "2026-04-15T00:00:00Z",
					ReleaseLabel:  "e2e-r1",
				},
			},
		},
		{
			Kind: types.KindAISystemBinding, ID: "bind-e2e",
			Doc: types.AISystemBindingDocument{
				APIVersion: types.APIVersionV1, Kind: types.KindAISystemBinding,
				Metadata: types.DocumentMetadata{ID: "bind-e2e"},
				Spec: types.AISystemBindingSpec{
					AISystemID:        "ai-e2e",
					AISystemVersion:   &pinnedVersion,
					BusinessServiceID: "bs-e2e",
					Role:              "primary-evaluator",
				},
			},
		},
	}

	result := svc.Apply(context.Background(), bundle, "operator:e2e")
	if result.ValidationErrorCount() != 0 || result.ApplyErrorCount() != 0 {
		t.Fatalf("apply errors: validation=%v, apply=%v", result.ValidationErrors, result.Results)
	}
	if result.CreatedCount() != 4 {
		t.Fatalf("CreatedCount: want 4, got %d", result.CreatedCount())
	}

	// 4. Server wired to the same memory repo aggregate via StructuralService.
	structural := NewStructuralService(repos.Capabilities, repos.Processes, repos.Surfaces).
		WithBusinessServices(repos.BusinessServices).
		WithAISystems(repos.AISystems, repos.AISystemVersions, repos.AISystemBindings)
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(structural)

	// 5. Read-side assertions. Each call exercises a different handler
	// (list, detail, sub-list, version-detail, binding-list) against the
	// just-applied state.

	// GET /v1/aisystems → bundle's AISystem visible.
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/v1/aisystems: %d %s", rec.Code, rec.Body.String())
	}
	var listResp aiSystemsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(listResp.AISystems) != 1 || listResp.AISystems[0].ID != "ai-e2e" {
		t.Errorf("list: expected [ai-e2e], got %+v", listResp.AISystems)
	}

	// GET /v1/aisystems/ai-e2e → status active, system_type llm.
	rec = performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-e2e", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/v1/aisystems/{id}: %d", rec.Code)
	}
	var detail aiSystemResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &detail)
	if detail.Status != "active" || detail.SystemType != "llm" || detail.CreatedBy != "operator:e2e" {
		t.Errorf("detail: %+v", detail)
	}

	// GET /v1/aisystems/ai-e2e/versions → v1 visible, status honoured.
	rec = performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-e2e/versions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/v1/aisystems/{id}/versions: %d", rec.Code)
	}
	var vList aiSystemVersionsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &vList)
	if len(vList.Versions) != 1 || vList.Versions[0].Version != 1 || vList.Versions[0].Status != "active" {
		t.Errorf("versions: %+v", vList.Versions)
	}

	// GET /v1/aisystems/ai-e2e/versions/1 → release_label round-trips.
	rec = performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-e2e/versions/1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/v1/aisystems/{id}/versions/{version}: %d", rec.Code)
	}
	var ver aiSystemVersionResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &ver)
	if ver.ReleaseLabel != "e2e-r1" {
		t.Errorf("version detail release_label: %q", ver.ReleaseLabel)
	}

	// GET /v1/aisystems/ai-e2e/bindings → binding visible with pinned version.
	rec = performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-e2e/bindings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/v1/aisystems/{id}/bindings: %d", rec.Code)
	}
	var bList aiSystemBindingsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &bList)
	if len(bList.Bindings) != 1 {
		t.Fatalf("bindings: want 1, got %d", len(bList.Bindings))
	}
	b := bList.Bindings[0]
	if b.ID != "bind-e2e" || b.AISystemVersion == nil || *b.AISystemVersion != 1 {
		t.Errorf("binding: %+v", b)
	}
	if b.BusinessServiceID == nil || *b.BusinessServiceID != "bs-e2e" {
		t.Errorf("binding business_service_id: %v", b.BusinessServiceID)
	}
	if b.Role != "primary-evaluator" {
		t.Errorf("binding role: %q", b.Role)
	}
}
