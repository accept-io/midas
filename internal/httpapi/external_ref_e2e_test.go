package httpapi

// End-to-end integration test for the ExternalRef substrate (Epic 1,
// PR 3, Cluster D). Applies a bundle whose every entity carries a
// populated external_ref, then queries each entity via its HTTP read
// endpoint and asserts the values surface verbatim on the wire.
//
// The test pins the four-layer canonicalisation contract end-to-end:
// apply mapper → storage (memory) → HTTP wire. UTC normalisation on
// LastSyncedAt is preserved across all four boundaries. Reuses
// existing helpers (memory.NewRepositories, apply.NewServiceWithRepos,
// NewStructuralService, performRequest) so no new test infrastructure.

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

func TestExternalRef_E2E_ApplyThenRead(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices:             repos.BusinessServices,
		BusinessServiceRelationships: repos.BusinessServiceRelationships,
		AISystems:                    repos.AISystems,
		AISystemVersions:             repos.AISystemVersions,
		AISystemBindings:             repos.AISystemBindings,
		ControlAudit:                 repos.ControlAudit,
	})

	// extSpec is the canonical fixture; every document in the bundle
	// carries it. Round-tripping the same values via every endpoint
	// confirms each entity wires the helper correctly.
	extSpec := func(sourceID string) *types.ExternalRefSpec {
		return &types.ExternalRefSpec{
			SourceSystem:  "github",
			SourceID:      sourceID,
			SourceURL:     "https://github.com/accept-io/" + sourceID,
			SourceVersion: "v1.2.0",
			LastSyncedAt:  "2026-04-30T11:00:00+02:00", // UTC: 09:00
		}
	}

	v := 1
	bundle := []parser.ParsedDocument{
		{Kind: types.KindBusinessService, ID: "bs-ext-e2e-a", Doc: types.BusinessServiceDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindBusinessService,
			Metadata: types.DocumentMetadata{ID: "bs-ext-e2e-a", Name: "BS A"},
			Spec: types.BusinessServiceSpec{
				ServiceType: "internal", Status: "active",
				ExternalRef: extSpec("ext-bs"),
			},
		}},
		{Kind: types.KindBusinessService, ID: "bs-ext-e2e-b", Doc: types.BusinessServiceDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindBusinessService,
			Metadata: types.DocumentMetadata{ID: "bs-ext-e2e-b", Name: "BS B"},
			Spec:     types.BusinessServiceSpec{ServiceType: "internal", Status: "active"},
		}},
		{Kind: types.KindBusinessServiceRelationship, ID: "rel-ext-e2e", Doc: types.BusinessServiceRelationshipDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindBusinessServiceRelationship,
			Metadata: types.DocumentMetadata{ID: "rel-ext-e2e"},
			Spec: types.BusinessServiceRelationshipSpec{
				SourceBusinessServiceID: "bs-ext-e2e-a", TargetBusinessServiceID: "bs-ext-e2e-b",
				RelationshipType: "depends_on",
				ExternalRef:      extSpec("ext-rel"),
			},
		}},
		{Kind: types.KindAISystem, ID: "ai-ext-e2e", Doc: types.AISystemDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindAISystem,
			Metadata: types.DocumentMetadata{ID: "ai-ext-e2e", Name: "AI"},
			Spec: types.AISystemSpec{
				Status: "active", Origin: "manual",
				ExternalRef: extSpec("ext-ai"),
			},
		}},
		{Kind: types.KindAISystemVersion, ID: "aiv-ext-e2e", Doc: types.AISystemVersionDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindAISystemVersion,
			Metadata: types.DocumentMetadata{ID: "aiv-ext-e2e"},
			Spec: types.AISystemVersionSpec{
				AISystemID: "ai-ext-e2e", Version: 1, Status: "active",
				EffectiveFrom: "2026-04-15T00:00:00Z",
				ExternalRef:   extSpec("ext-aiv"),
			},
		}},
		{Kind: types.KindAISystemBinding, ID: "bind-ext-e2e", Doc: types.AISystemBindingDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindAISystemBinding,
			Metadata: types.DocumentMetadata{ID: "bind-ext-e2e"},
			Spec: types.AISystemBindingSpec{
				AISystemID: "ai-ext-e2e", AISystemVersion: &v,
				BusinessServiceID: "bs-ext-e2e-a",
				ExternalRef:       extSpec("ext-bind"),
			},
		}},
	}

	result := svc.Apply(context.Background(), bundle, "operator:ext-e2e")
	if result.ValidationErrorCount() != 0 || result.ApplyErrorCount() != 0 {
		t.Fatalf("apply errors: validation=%v apply=%v", result.ValidationErrors, result.Results)
	}
	if result.CreatedCount() != 6 {
		t.Fatalf("CreatedCount: want 6, got %d", result.CreatedCount())
	}

	// Build a server backed by the same memory aggregate.
	structural := NewStructuralService(repos.Capabilities, repos.Processes, repos.Surfaces).
		WithBusinessServices(repos.BusinessServices).
		WithBusinessServiceRelationships(repos.BusinessServiceRelationships).
		WithAISystems(repos.AISystems, repos.AISystemVersions, repos.AISystemBindings)
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(structural)

	// assertExtRef pins the wire-format invariants for the round-tripped
	// ExternalRef: every populated field flows through verbatim, and the
	// LastSyncedAt timestamp is normalised to UTC (the input fixture is
	// in +02:00; the wire must read 09:00Z).
	assertExtRef := func(t *testing.T, label string, got *externalRefResponse, wantSourceID string) {
		t.Helper()
		if got == nil {
			t.Fatalf("%s: ExternalRef nil on the wire", label)
		}
		if got.SourceSystem != "github" || got.SourceID != wantSourceID {
			t.Errorf("%s: system/id mismatch: %+v", label, got)
		}
		if got.SourceVersion != "v1.2.0" {
			t.Errorf("%s: source_version: %q", label, got.SourceVersion)
		}
		if got.LastSyncedAt == nil || *got.LastSyncedAt != "2026-04-30T09:00:00Z" {
			t.Errorf("%s: last_synced_at not UTC-normalised: %v", label, got.LastSyncedAt)
		}
	}

	// GET /v1/businessservices/{id} — populated external_ref.
	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-ext-e2e-a", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("BS detail: %d %s", rec.Code, rec.Body.String())
	}
	var bs businessServiceResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &bs)
	assertExtRef(t, "BusinessService", bs.ExternalRef, "ext-bs")

	// GET /v1/businessservices/{id}/relationships — outgoing[0].external_ref.
	rec = performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-ext-e2e-a/relationships", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("BSR list: %d", rec.Code)
	}
	var rels businessServiceRelationshipsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &rels)
	if len(rels.Outgoing) != 1 {
		t.Fatalf("outgoing: want 1, got %d", len(rels.Outgoing))
	}
	assertExtRef(t, "BSR", rels.Outgoing[0].ExternalRef, "ext-rel")

	// GET /v1/aisystems/{id} — populated external_ref.
	rec = performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-ext-e2e", nil)
	var sys aiSystemResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &sys)
	assertExtRef(t, "AISystem", sys.ExternalRef, "ext-ai")

	// GET /v1/aisystems/{id}/versions/{version} — populated external_ref.
	rec = performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-ext-e2e/versions/1", nil)
	var ver aiSystemVersionResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &ver)
	assertExtRef(t, "AISystemVersion", ver.ExternalRef, "ext-aiv")

	// GET /v1/aisystems/{id}/bindings — bindings[0].external_ref.
	rec = performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-ext-e2e/bindings", nil)
	var binds aiSystemBindingsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &binds)
	if len(binds.Bindings) != 1 {
		t.Fatalf("bindings: want 1, got %d", len(binds.Bindings))
	}
	assertExtRef(t, "AISystemBinding", binds.Bindings[0].ExternalRef, "ext-bind")
}
