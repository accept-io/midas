package bootstrap_test

// apply_wiring_test.go — regression guard for control-plane apply wiring.
//
// Proves that constructing an apply service with real repositories actually
// persists all four resource kinds (Surface, Agent, Profile, Grant).  This test
// lives in the bootstrap package because that is where the application wiring
// decisions are made; if main.go ever reverts to apply.NewService() the
// integration test in internal/controlplane/apply will catch it, but this file
// adds an additional guard at the bootstrap boundary so reviewers see the
// failure in the most visible location.

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/store/memory"
)

// alwaysExistsProcessRepo is a test double for apply.ProcessRepository that
// reports every process ID as existing.
type alwaysExistsProcessRepo struct{}

func (alwaysExistsProcessRepo) Exists(_ context.Context, _ string) (bool, error) {
	return true, nil
}

func (alwaysExistsProcessRepo) GetByID(_ context.Context, id string) (*process.Process, error) {
	return &process.Process{ID: id, Status: "active"}, nil
}

func (alwaysExistsProcessRepo) Create(_ context.Context, _ *process.Process) error {
	return nil
}

// TestApplyServiceWiring_WithRepos_PersistsAllKinds verifies that the apply
// service construction used in main.go (NewServiceWithRepos) actually writes
// each resource kind to the backing store.  A service constructed with no
// repositories (NewService) would produce the same apply result but leave the
// store empty — the assertions on the store confirm the distinction.
func TestApplyServiceWiring_WithRepos_PersistsAllKinds(t *testing.T) {
	repos := memory.NewRepositories()

	// Seed the process referenced by the surface fixture so the memory-store
	// structural integrity check (G-12) passes.
	seedCtx := context.Background()
	now := time.Now().UTC()
	if err := repos.Capabilities.Create(seedCtx, &capability.Capability{
		ID: "test.cap", Name: "Test Cap", Status: "active", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed capability: %v", err)
	}
	if err := repos.Processes.Create(seedCtx, &process.Process{
		ID: "test.process", Name: "Test Process", CapabilityID: "test.cap",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed process: %v", err)
	}

	// This is the constructor that main.go must use.
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Surfaces:  repos.Surfaces,
		Agents:    repos.Agents,
		Profiles:  repos.Profiles,
		Grants:    repos.Grants,
		Processes: alwaysExistsProcessRepo{},
	})

	docs := []parser.ParsedDocument{
		{
			Kind: types.KindSurface,
			ID:   "surface-bootstrap-guard",
			Doc: types.SurfaceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindSurface,
				Metadata:   types.DocumentMetadata{ID: "surface-bootstrap-guard", Name: "Bootstrap Guard Surface"},
				Spec: types.SurfaceSpec{
					Description: "Bootstrap wiring guard surface",
					Category:    "financial",
					RiskTier:    "medium",
					Status:      "active",
					ProcessID:   "test.process",
				},
			},
		},
		{
			Kind: types.KindAgent,
			ID:   "agent-bootstrap-guard",
			Doc: types.AgentDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindAgent,
				Metadata:   types.DocumentMetadata{ID: "agent-bootstrap-guard", Name: "Bootstrap Guard Agent"},
				Spec: types.AgentSpec{
					Type:    "llm_agent",
					Runtime: types.AgentRuntime{Model: "gpt-4", Version: "2024-11-20", Provider: "openai"},
					Status:  "active",
				},
			},
		},
		{
			Kind: types.KindProfile,
			ID:   "profile-bootstrap-guard",
			Doc: types.ProfileDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProfile,
				Metadata:   types.DocumentMetadata{ID: "profile-bootstrap-guard", Name: "Bootstrap Guard Profile"},
				Spec: types.ProfileSpec{
					SurfaceID: "surface-bootstrap-guard",
					Authority: types.ProfileAuthority{
						DecisionConfidenceThreshold: 0.75,
						ConsequenceThreshold:        types.ConsequenceThreshold{Type: "monetary", Amount: 1000, Currency: "USD"},
					},
					Policy: types.ProfilePolicy{Reference: "rego://bootstrap/guard_v1", FailMode: "closed"},
				},
			},
		},
		{
			Kind: types.KindGrant,
			ID:   "grant-bootstrap-guard",
			Doc: types.GrantDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindGrant,
				Metadata:   types.DocumentMetadata{ID: "grant-bootstrap-guard", Name: "Bootstrap Guard Grant"},
				Spec: types.GrantSpec{
					AgentID:       "agent-bootstrap-guard",
					ProfileID:     "profile-bootstrap-guard",
					GrantedBy:     "system",
					GrantedAt:     "2025-01-01T00:00:00Z",
					EffectiveFrom: "2025-01-01T00:00:00Z",
					Status:        "active",
				},
			},
		},
	}

	ctx := context.Background()
	result := svc.Apply(ctx, docs, "")

	if !result.Success() {
		t.Fatalf("apply failed; validation errors: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 4 {
		t.Fatalf("expected 4 resources created, got %d", result.CreatedCount())
	}

	// Confirm each resource is actually in the backing store.
	if s, _ := repos.Surfaces.FindLatestByID(ctx, "surface-bootstrap-guard"); s == nil {
		t.Error("surface not persisted — apply service is not wired with repositories")
	}
	if a, _ := repos.Agents.GetByID(ctx, "agent-bootstrap-guard"); a == nil {
		t.Error("agent not persisted — apply service is not wired with repositories")
	}
	if p, _ := repos.Profiles.FindByID(ctx, "profile-bootstrap-guard"); p == nil {
		t.Error("profile not persisted — apply service is not wired with repositories")
	}
	if g, _ := repos.Grants.FindByID(ctx, "grant-bootstrap-guard"); g == nil {
		t.Error("grant not persisted — apply service is not wired with repositories")
	}
}
