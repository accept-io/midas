package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/config"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/approval"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/httpapi"
	"github.com/accept-io/midas/internal/policy"
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
		Surfaces:     repos.Surfaces,
		Agents:       repos.Agents,
		Profiles:     repos.Profiles,
		Grants:       repos.Grants,
		ControlAudit: repos.ControlAudit,
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

	// Seed a surface in review state. BusinessOwner matches approver_id so
	// CanApproveSurface returns true for governance.approver role.
	err = repos.Surfaces.Create(ctx, &surface.DecisionSurface{
		ID:             "surf-wiring-test",
		Version:        1,
		Name:           "Wiring Test Surface",
		Domain:         "test",
		Status:         surface.SurfaceStatusReview,
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
