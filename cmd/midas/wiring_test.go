package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/config"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/approval"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/httpapi"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

// TestProductionWiring_ApproveSurface_EndpointIsWired verifies that when the
// server is constructed using the same composition path as production (memory
// mode), the POST /v1/controlplane/surfaces/{id}/approve endpoint is properly
// wired and does not return 501 Not Implemented.
//
// This test was added after discovering that cmd/midas/main.go passed nil for
// the approval service, causing every approve call to return 501. The existing
// handler tests all injected the service via mock, so they never caught the
// production wiring gap.
func TestProductionWiring_ApproveSurface_EndpointIsWired(t *testing.T) {
	ctx := context.Background()

	repos, repoStore, outboxRepo, cleanup, _, err := buildRepositories(ctx, config.StoreConfig{Backend: "memory"})
	if err != nil {
		t.Fatalf("buildRepositories: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	orchestrator, err := decision.NewOrchestrator(repoStore, policy.NoOpPolicyEvaluator{}, nil)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}

	applyService := apply.NewServiceWithRepos(apply.RepositorySet{
		Surfaces:            repos.Surfaces,
		Agents:              repos.Agents,
		Profiles:            repos.Profiles,
		Grants:              repos.Grants,
		ControlAudit:        repos.ControlAudit,
		Processes:           repos.Processes,
		Capabilities:        repos.Capabilities,
		BusinessServices:    repos.BusinessServices,
		ProcessCapabilities: repos.ProcessCapabilities,
	})

	// This is the service that was previously nil in main.go.
	approvalSvc := approval.NewServiceWithProfileAndOutbox(
		repos.Surfaces,
		repos.Profiles,
		approval.DefaultPolicy(),
		outboxRepo,
		repos.ControlAudit,
	)

	srv := httpapi.NewServerFull(
		orchestrator,
		applyService,
		approvalSvc,
		httpapi.NewIntrospectionServiceFull(repos.Surfaces, repos.Profiles, repos.Agents, repos.Grants),
		nil,
		nil,
	)

	// Seed the capability and process referenced by the surface below.
	// The memory store enforces Surface → Process referential integrity.
	now := time.Now()
	if err := repos.Capabilities.Create(ctx, &capability.Capability{
		ID: "cap-wiring-test", Name: "Wiring Cap", Status: "active",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed capability: %v", err)
	}
	if err := repos.Processes.Create(ctx, &process.Process{
		ID: "proc-wiring-test", Name: "Wiring Process", CapabilityID: "cap-wiring-test",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed process: %v", err)
	}

	// Seed a surface in review state. BusinessOwner matches approver_id so
	// CanApproveSurface returns true for governance.approver role.
	err = repos.Surfaces.Create(ctx, &surface.DecisionSurface{
		ID:             "surf-wiring-test",
		Version:        1,
		Name:           "Wiring Test Surface",
		Domain:         "test",
		Status:         surface.SurfaceStatusReview,
		ProcessID:      "proc-wiring-test",
		BusinessOwner:  "approver-a",
		TechnicalOwner: "unassigned",
		EffectiveFrom:  time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("seed surface: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"approver_id":   "approver-a",
		"approver_name": "Approver A",
	})
	req := httptest.NewRequest(http.MethodPost,
		"/v1/controlplane/surfaces/surf-wiring-test/approve",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	// 501 means the approval service is not wired — this is the regression to prevent.
	if rec.Code == http.StatusNotImplemented {
		t.Fatalf("approval endpoint returned 501 Not Implemented: approval service is not wired in the production path")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		SurfaceID  string `json:"surface_id"`
		Status     string `json:"status"`
		ApprovedBy string `json:"approved_by"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "active" {
		t.Errorf("expected status active, got %q", resp.Status)
	}
	if resp.ApprovedBy != "approver-a" {
		t.Errorf("expected approved_by approver-a, got %q", resp.ApprovedBy)
	}
}

// TestProductionWiring_StructuralApply_ReposAreWired verifies that the apply
// service receives the full structural repository set in memory mode, so that
// structural document kinds (Capability, BusinessService, etc.) are persisted
// rather than silently dropped to validation-only.
//
// This test was added after discovering that apply.NewServiceWithRepos was
// called without Processes, Capabilities, BusinessServices, or ProcessCapabilities,
// causing structural apply operations to degrade silently without error.
func TestProductionWiring_StructuralApply_ReposAreWired(t *testing.T) {
	ctx := context.Background()

	repos, _, _, cleanup, _, err := buildRepositories(ctx, config.StoreConfig{Backend: "memory"})
	if err != nil {
		t.Fatalf("buildRepositories: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Verify structural repos are non-nil before wiring — if Phase 1 regressed
	// these would be nil and the apply service would silently validate-only.
	if repos.Capabilities == nil {
		t.Fatal("repos.Capabilities is nil in memory mode")
	}
	if repos.Processes == nil {
		t.Fatal("repos.Processes is nil in memory mode")
	}
	if repos.BusinessServices == nil {
		t.Fatal("repos.BusinessServices is nil in memory mode")
	}
	if repos.ProcessCapabilities == nil {
		t.Fatal("repos.ProcessCapabilities is nil in memory mode")
	}

	applyService := apply.NewServiceWithRepos(apply.RepositorySet{
		Surfaces:            repos.Surfaces,
		Agents:              repos.Agents,
		Profiles:            repos.Profiles,
		Grants:              repos.Grants,
		ControlAudit:        repos.ControlAudit,
		Processes:           repos.Processes,
		Capabilities:        repos.Capabilities,
		BusinessServices:    repos.BusinessServices,
		ProcessCapabilities: repos.ProcessCapabilities,
	})

	// Apply a Capability document and verify it is persisted — not just planned.
	// If the Capabilities repo is nil in the apply service, the executor's
	// validation-only guard short-circuits and nothing is written.
	result := applyService.Apply(ctx, []parser.ParsedDocument{
		{
			Kind: types.KindCapability,
			ID:   "cap-wiring-check",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-wiring-check", Name: "Wiring Check Capability"},
				Spec:       types.CapabilitySpec{Status: "active"},
			},
		},
	}, "test-actor")

	if len(result.Results) != 1 {
		t.Fatalf("apply result: want 1 entry, got %d", len(result.Results))
	}
	entry := result.Results[0]
	if entry.Status != types.ResourceStatusCreated {
		t.Fatalf("apply entry status: want %q, got %q (message: %s)", types.ResourceStatusCreated, entry.Status, entry.Message)
	}

	got, err := repos.Capabilities.GetByID(ctx, "cap-wiring-check")
	if err != nil {
		t.Fatalf("GetByID after apply: %v", err)
	}
	if got == nil {
		t.Fatal("Capability was not persisted: apply service structural repos are not wired")
	}
	if got.ID != "cap-wiring-check" {
		t.Errorf("persisted capability ID: want %q, got %q", "cap-wiring-check", got.ID)
	}
}
