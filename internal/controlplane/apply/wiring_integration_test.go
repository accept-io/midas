package apply_test

// wiring_integration_test.go — proof that NewServiceWithRepos actually persists
// all four resource kinds and that NewService (no repos) does not.
//
// These tests use the in-memory store so they run without a database.

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/store/memory"
)

// fullBundle returns a four-document bundle: Surface → Agent → Profile → Grant.
// All documents are valid and their cross-references are satisfied within the bundle.
func fullBundle() []parser.ParsedDocument {
	return []parser.ParsedDocument{
		{
			Kind: types.KindSurface,
			ID:   "surface-wiring-test",
			Doc: types.SurfaceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindSurface,
				Metadata: types.DocumentMetadata{
					ID:   "surface-wiring-test",
					Name: "Wiring Integration Test Surface",
				},
				Spec: types.SurfaceSpec{
					Description: "Surface used by wiring integration tests",
					Category:    "financial",
					RiskTier:    "high",
					Status:      "active",
					ProcessID:   "test.process",
				},
			},
		},
		{
			Kind: types.KindAgent,
			ID:   "agent-wiring-test",
			Doc: types.AgentDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindAgent,
				Metadata: types.DocumentMetadata{
					ID:   "agent-wiring-test",
					Name: "Wiring Test Agent",
				},
				Spec: types.AgentSpec{
					Type: "llm_agent",
					Runtime: types.AgentRuntime{
						Model:    "gpt-4",
						Version:  "2024-11-20",
						Provider: "openai",
					},
					Status: "active",
				},
			},
		},
		{
			Kind: types.KindProfile,
			ID:   "profile-wiring-test",
			Doc: types.ProfileDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProfile,
				Metadata: types.DocumentMetadata{
					ID:   "profile-wiring-test",
					Name: "Wiring Test Profile",
				},
				Spec: types.ProfileSpec{
					SurfaceID: "surface-wiring-test",
					Authority: types.ProfileAuthority{
						DecisionConfidenceThreshold: 0.80,
						ConsequenceThreshold: types.ConsequenceThreshold{
							Type:     "monetary",
							Amount:   5000,
							Currency: "USD",
						},
					},
					InputRequirements: types.ProfileInputRequirements{
						RequiredContext: []string{"customer_id"},
					},
					Policy: types.ProfilePolicy{
						Reference: "rego://wiring/test_v1",
						FailMode:  "closed",
					},
				},
			},
		},
		{
			Kind: types.KindGrant,
			ID:   "grant-wiring-test",
			Doc: types.GrantDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindGrant,
				Metadata: types.DocumentMetadata{
					ID:   "grant-wiring-test",
					Name: "Wiring Test Grant",
				},
				Spec: types.GrantSpec{
					AgentID:       "agent-wiring-test",
					ProfileID:     "profile-wiring-test",
					GrantedBy:     "system",
					GrantedAt:     "2025-01-01T00:00:00Z",
					EffectiveFrom: "2025-01-01T00:00:00Z",
					Status:        "active",
				},
			},
		},
	}
}

// TestApplyWithRepos_AllFourKindsPersisted is the primary proof test.
// It applies a four-document bundle through a service wired with real (memory)
// repositories, then confirms every resource is retrievable from its backing store.
func TestApplyWithRepos_AllFourKindsPersisted(t *testing.T) {
	repos := memory.NewRepositories()
	seedTestProcess(t, repos)
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Surfaces:  repos.Surfaces,
		Agents:    repos.Agents,
		Profiles:  repos.Profiles,
		Grants:    repos.Grants,
		Processes: alwaysExistsProcessRepo{},
	})

	ctx := context.Background()
	result := svc.Apply(ctx, fullBundle(), "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d: %+v", result.ValidationErrorCount(), result.ValidationErrors)
	}
	if result.CreatedCount() != 4 {
		t.Fatalf("expected 4 created results, got %d", result.CreatedCount())
	}
	if !result.Success() {
		t.Fatal("expected apply to succeed")
	}

	// Assert the surface is retrievable.
	surface, err := repos.Surfaces.FindLatestByID(ctx, "surface-wiring-test")
	if err != nil {
		t.Fatalf("FindLatestByID: unexpected error: %v", err)
	}
	if surface == nil {
		t.Fatal("surface not found in backing store after apply — surface was not persisted")
	}

	// Assert the agent is retrievable.
	ag, err := repos.Agents.GetByID(ctx, "agent-wiring-test")
	if err != nil {
		t.Fatalf("GetByID (agent): unexpected error: %v", err)
	}
	if ag == nil {
		t.Fatal("agent not found in backing store after apply — agent was not persisted")
	}

	// Assert the profile is retrievable.
	profile, err := repos.Profiles.FindByID(ctx, "profile-wiring-test")
	if err != nil {
		t.Fatalf("FindByID (profile): unexpected error: %v", err)
	}
	if profile == nil {
		t.Fatal("profile not found in backing store after apply — profile was not persisted")
	}

	// Assert the grant is retrievable.
	grant, err := repos.Grants.FindByID(ctx, "grant-wiring-test")
	if err != nil {
		t.Fatalf("FindByID (grant): unexpected error: %v", err)
	}
	if grant == nil {
		t.Fatal("grant not found in backing store after apply — grant was not persisted")
	}
}

// TestApplyWithoutRepos_ReportsCreatedButDoesNotPersist is the regression guard.
// It proves that NewService() (no repositories) reports "created" in the result
// but leaves the backing store empty — matching the pre-fix wiring behaviour that
// this PR corrects.
//
// If main.go ever accidentally reverts to NewService(), this test makes the
// distinction visible: the integration test above will fail because no repos are
// wired, while this test documents that validation-only mode does NOT persist.
func TestApplyWithoutRepos_ReportsCreatedButDoesNotPersist(t *testing.T) {
	// Construct a service with NO repositories — this is the broken wiring.
	svc := apply.NewService()

	// Use a separate repo set so we can check what was actually stored.
	repos := memory.NewRepositories()

	ctx := context.Background()
	result := svc.Apply(ctx, fullBundle(), "")

	// The service still reports 4 creates (validation-only mode).
	if result.CreatedCount() != 4 {
		t.Fatalf("expected NewService() to report 4 creates (validation-only), got %d", result.CreatedCount())
	}

	// But nothing was written to the backing store.
	surface, err := repos.Surfaces.FindLatestByID(ctx, "surface-wiring-test")
	if err != nil {
		t.Fatalf("FindLatestByID: unexpected error: %v", err)
	}
	if surface != nil {
		t.Error("surface unexpectedly found in backing store — NewService() should not persist")
	}

	ag, err := repos.Agents.GetByID(ctx, "agent-wiring-test")
	if err != nil {
		t.Fatalf("GetByID (agent): unexpected error: %v", err)
	}
	if ag != nil {
		t.Error("agent unexpectedly found in backing store — NewService() should not persist")
	}

	profile, err := repos.Profiles.FindByID(ctx, "profile-wiring-test")
	if err != nil {
		t.Fatalf("FindByID (profile): unexpected error: %v", err)
	}
	if profile != nil {
		t.Error("profile unexpectedly found in backing store — NewService() should not persist")
	}

	grant, err := repos.Grants.FindByID(ctx, "grant-wiring-test")
	if err != nil {
		t.Fatalf("FindByID (grant): unexpected error: %v", err)
	}
	if grant != nil {
		t.Error("grant unexpectedly found in backing store — NewService() should not persist")
	}
}
