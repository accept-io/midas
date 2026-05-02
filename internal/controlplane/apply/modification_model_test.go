package apply_test

// modification_model_test.go — end-to-end tests proving the per-resource-kind
// modification semantics described in docs/control-plane.md § Modification model.
//
// All tests use in-memory repositories so no database is required.

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
	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// Surface: versioned, governed reapply
// ---------------------------------------------------------------------------

// TestModification_Surface_FirstApply_CreatesVersion1 verifies that applying a
// surface for the first time creates version 1 in review status.
func TestModification_Surface_FirstApply_CreatesVersion1(t *testing.T) {
	repos := memory.NewRepositories()
	seedTestProcess(t, repos)
	svc := apply.NewServiceWithRepos(apply.RepositorySet{Surfaces: repos.Surfaces, Processes: alwaysExistsProcessRepo{}})
	ctx := context.Background()

	result := svc.Apply(ctx, []parser.ParsedDocument{modSurfaceDoc("surf-mod-1")}, "ops-user")
	if result.ValidationErrorCount() != 0 || result.CreatedCount() != 1 {
		t.Fatalf("expected 1 created, got errors=%v created=%d", result.ValidationErrors, result.CreatedCount())
	}

	s, err := repos.Surfaces.FindLatestByID(ctx, "surf-mod-1")
	if err != nil || s == nil {
		t.Fatalf("FindLatestByID: %v (err %v)", s, err)
	}
	if s.Version != 1 {
		t.Errorf("expected version 1, got %d", s.Version)
	}
	if s.Status != surface.SurfaceStatusReview {
		t.Errorf("expected status 'review', got %q", s.Status)
	}
}

// TestModification_Surface_Reapply_ActiveVersion_CreatesNewVersion verifies
// that reapplying a surface whose latest version is active creates a new
// version (v2) in review status, while v1 (active) continues to exist.
func TestModification_Surface_Reapply_ActiveVersion_CreatesNewVersion(t *testing.T) {
	repos := memory.NewRepositories()
	seedTestProcess(t, repos)
	svc := apply.NewServiceWithRepos(apply.RepositorySet{Surfaces: repos.Surfaces, Processes: alwaysExistsProcessRepo{}})
	ctx := context.Background()

	// First apply → v1 in review.
	svc.Apply(ctx, []parser.ParsedDocument{modSurfaceDoc("surf-mod-2")}, "ops-user")

	// Manually promote v1 to active so the second apply is not a conflict.
	v1, _ := repos.Surfaces.FindLatestByID(ctx, "surf-mod-2")
	v1Active := *v1
	v1Active.Status = surface.SurfaceStatusActive
	repos.Surfaces.Update(ctx, &v1Active)

	// Second apply → must create v2 in review.
	result := svc.Apply(ctx, []parser.ParsedDocument{modSurfaceDoc("surf-mod-2")}, "ops-user")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("unexpected validation errors: %v", result.ValidationErrors)
	}
	if result.CreatedCount() != 1 {
		t.Errorf("expected 1 created (new version), got %d", result.CreatedCount())
	}
	if result.ConflictCount() != 0 {
		t.Errorf("expected 0 conflicts on reapply of active surface, got %d", result.ConflictCount())
	}

	latest, _ := repos.Surfaces.FindLatestByID(ctx, "surf-mod-2")
	if latest.Version != 2 {
		t.Errorf("expected latest version 2, got %d", latest.Version)
	}
	if latest.Status != surface.SurfaceStatusReview {
		t.Errorf("expected latest status 'review', got %q", latest.Status)
	}

	// v1 must still exist and remain active.
	versions, _ := repos.Surfaces.ListVersions(ctx, "surf-mod-2")
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	// ListVersions returns descending; versions[1] is v1.
	if versions[1].Version != 1 || versions[1].Status != surface.SurfaceStatusActive {
		t.Errorf("v1 should still be active, got version=%d status=%q",
			versions[1].Version, versions[1].Status)
	}
}

// TestModification_Surface_Reapply_ReviewVersion_Conflict verifies that
// reapplying while the latest version is already in review returns conflict.
func TestModification_Surface_Reapply_ReviewVersion_Conflict(t *testing.T) {
	repos := memory.NewRepositories()
	seedTestProcess(t, repos)
	svc := apply.NewServiceWithRepos(apply.RepositorySet{Surfaces: repos.Surfaces, Processes: alwaysExistsProcessRepo{}})
	ctx := context.Background()

	// First apply → v1 in review.
	svc.Apply(ctx, []parser.ParsedDocument{modSurfaceDoc("surf-mod-3")}, "ops-user")

	// Reapply without approving → conflict because v1 is still in review.
	result := svc.Apply(ctx, []parser.ParsedDocument{modSurfaceDoc("surf-mod-3")}, "ops-user")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("unexpected validation errors: %v", result.ValidationErrors)
	}
	if result.ConflictCount() != 1 {
		t.Errorf("expected 1 conflict when latest version is in review, got %d", result.ConflictCount())
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created when latest version is in review, got %d", result.CreatedCount())
	}

	// Only one version must exist — the conflict must not have written a second version.
	versions, _ := repos.Surfaces.ListVersions(ctx, "surf-mod-3")
	if len(versions) != 1 {
		t.Errorf("expected 1 version after conflict, got %d", len(versions))
	}
}

// ---------------------------------------------------------------------------
// Profile: versioned lineage
// ---------------------------------------------------------------------------

// TestModification_Profile_Reapply_CreatesNewVersion verifies that reapplying
// a profile whose logical ID already exists creates a new version rather than
// conflicting or overwriting.
func TestModification_Profile_Reapply_CreatesNewVersion(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Surfaces: repos.Surfaces,
		Profiles: repos.Profiles,
	})
	ctx := context.Background()

	// Seed the referenced process and surface so referential integrity passes.
	seedTestProcess(t, repos)
	repos.Surfaces.Create(ctx, modActiveSurface("surf-prof-mod"))

	// First apply → v1.
	result1 := svc.Apply(ctx, []parser.ParsedDocument{modProfileDoc("prof-mod-1", "surf-prof-mod")}, "ops")
	if result1.ValidationErrorCount() != 0 || result1.CreatedCount() != 1 {
		t.Fatalf("first apply: errors=%v created=%d", result1.ValidationErrors, result1.CreatedCount())
	}

	// Second apply → v2 (not a conflict).
	result2 := svc.Apply(ctx, []parser.ParsedDocument{modProfileDoc("prof-mod-1", "surf-prof-mod")}, "ops")
	if result2.ValidationErrorCount() != 0 {
		t.Fatalf("second apply: unexpected errors: %v", result2.ValidationErrors)
	}
	if result2.CreatedCount() != 1 {
		t.Errorf("expected 1 created (new version), got %d", result2.CreatedCount())
	}
	if result2.ConflictCount() != 0 {
		t.Errorf("expected 0 conflicts for profile reapply, got %d", result2.ConflictCount())
	}

	// Verify both versions exist.
	versions, _ := repos.Profiles.ListVersions(ctx, "prof-mod-1")
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	// Descending order: versions[0]=v2, versions[1]=v1.
	if versions[0].Version != 2 || versions[1].Version != 1 {
		t.Errorf("expected [v2,v1], got [v%d,v%d]", versions[0].Version, versions[1].Version)
	}
}

// ---------------------------------------------------------------------------
// Agent: immutable create-once
// ---------------------------------------------------------------------------

// TestModification_Agent_Reapply_Conflict verifies that reapplying an agent
// whose ID already exists returns conflict (agents are immutable once created).
func TestModification_Agent_Reapply_Conflict(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{Agents: repos.Agents})
	ctx := context.Background()

	// First apply → created.
	result1 := svc.Apply(ctx, []parser.ParsedDocument{modAgentDoc("agent-mod-1")}, "ops")
	if result1.ValidationErrorCount() != 0 || result1.CreatedCount() != 1 {
		t.Fatalf("first apply: errors=%v created=%d", result1.ValidationErrors, result1.CreatedCount())
	}

	// Second apply → conflict.
	result2 := svc.Apply(ctx, []parser.ParsedDocument{modAgentDoc("agent-mod-1")}, "ops")
	if result2.ValidationErrorCount() != 0 {
		t.Fatalf("second apply: unexpected errors: %v", result2.ValidationErrors)
	}
	if result2.ConflictCount() != 1 {
		t.Errorf("expected 1 conflict for agent reapply, got %d", result2.ConflictCount())
	}
	if result2.CreatedCount() != 0 {
		t.Errorf("expected 0 created for agent reapply, got %d", result2.CreatedCount())
	}
}

// ---------------------------------------------------------------------------
// Grant: immutable create-once
// ---------------------------------------------------------------------------

// TestModification_Grant_Reapply_Conflict verifies that reapplying a grant
// whose ID already exists returns conflict (grants are immutable once created).
func TestModification_Grant_Reapply_Conflict(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Surfaces: repos.Surfaces,
		Agents:   repos.Agents,
		Profiles: repos.Profiles,
		Grants:   repos.Grants,
	})
	ctx := context.Background()

	// Seed the referenced process and surface so the profile's referential integrity passes.
	seedTestProcess(t, repos)
	repos.Surfaces.Create(ctx, modActiveSurface("surf-grant-mod"))

	// First apply: agent + profile + grant in a single bundle so cross-references resolve.
	firstDocs := []parser.ParsedDocument{
		modAgentDoc("agent-gmod"),
		modProfileDoc("prof-gmod", "surf-grant-mod"),
		modGrantDoc("grant-mod-1", "agent-gmod", "prof-gmod"),
	}
	result1 := svc.Apply(ctx, firstDocs, "ops")
	if result1.ValidationErrorCount() != 0 {
		t.Fatalf("first apply: unexpected errors: %v", result1.ValidationErrors)
	}
	if result1.CreatedCount() != 3 {
		t.Fatalf("expected 3 created, got %d", result1.CreatedCount())
	}

	// Second apply of just the grant → conflict.
	grantOnly := []parser.ParsedDocument{modGrantDoc("grant-mod-1", "agent-gmod", "prof-gmod")}
	result2 := svc.Apply(ctx, grantOnly, "ops")
	if result2.ValidationErrorCount() != 0 {
		t.Fatalf("second apply: unexpected errors: %v", result2.ValidationErrors)
	}
	if result2.ConflictCount() != 1 {
		t.Errorf("expected 1 conflict for grant reapply, got %d", result2.ConflictCount())
	}
	if result2.CreatedCount() != 0 {
		t.Errorf("expected 0 created for grant reapply, got %d", result2.CreatedCount())
	}
}

// ---------------------------------------------------------------------------
// Runtime resolution: latest ≠ active during a governance review cycle
// ---------------------------------------------------------------------------

// TestModification_Surface_LatestVsActive_DistinctDuringReviewCycle proves that
// FindLatestByID and FindActiveAt return different versions when a new version
// is in review — the governance invariant that keeps evaluations stable.
func TestModification_Surface_LatestVsActive_DistinctDuringReviewCycle(t *testing.T) {
	repos := memory.NewRepositories()
	seedTestProcess(t, repos)
	svc := apply.NewServiceWithRepos(apply.RepositorySet{Surfaces: repos.Surfaces, Processes: alwaysExistsProcessRepo{}})
	ctx := context.Background()

	// First apply → v1 in review.
	svc.Apply(ctx, []parser.ParsedDocument{modSurfaceDoc("surf-mod-4")}, "ops")

	// Promote v1 to active.
	v1, _ := repos.Surfaces.FindLatestByID(ctx, "surf-mod-4")
	v1Active := *v1
	v1Active.Status = surface.SurfaceStatusActive
	repos.Surfaces.Update(ctx, &v1Active)

	// Capture v1's effective time before the second apply.
	v1EffectiveFrom := v1Active.EffectiveFrom

	// Second apply → v2 in review.
	svc.Apply(ctx, []parser.ParsedDocument{modSurfaceDoc("surf-mod-4")}, "ops")

	// FindLatestByID → v2 (review).
	latest, _ := repos.Surfaces.FindLatestByID(ctx, "surf-mod-4")
	if latest == nil || latest.Version != 2 {
		t.Errorf("FindLatestByID: expected version 2, got %v", latest)
	}
	if latest.Status != surface.SurfaceStatusReview {
		t.Errorf("FindLatestByID: expected status 'review', got %q", latest.Status)
	}

	// FindActiveAt → v1 (active). The evaluation window is just after v1's effective date.
	evalTime := v1EffectiveFrom.Add(time.Second)
	active, err := repos.Surfaces.FindActiveAt(ctx, "surf-mod-4", evalTime)
	if err != nil {
		t.Fatalf("FindActiveAt: %v", err)
	}
	if active == nil {
		t.Fatal("FindActiveAt: expected v1 (active), got nil — evaluation would break")
	}
	if active.Version != 1 {
		t.Errorf("FindActiveAt: expected version 1, got %d", active.Version)
	}
}

// ---------------------------------------------------------------------------
// Document and domain-object constructors for this test file
// ---------------------------------------------------------------------------

// alwaysExistsProcessRepo is a test double for apply.ProcessRepository that
// reports every process ID as existing. Shared across the apply_test package.
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

// alwaysExistsCapabilityRepo is a test double for apply.CapabilityRepository that
// reports every capability ID as existing. Shared across the apply_test package.
type alwaysExistsCapabilityRepo struct{}

func (alwaysExistsCapabilityRepo) Exists(_ context.Context, _ string) (bool, error) {
	return true, nil
}

func (alwaysExistsCapabilityRepo) GetByID(_ context.Context, id string) (*capability.Capability, error) {
	return &capability.Capability{ID: id, Status: "active"}, nil
}

func (alwaysExistsCapabilityRepo) Create(_ context.Context, _ *capability.Capability) error {
	return nil
}

// modSurfaceDoc builds a valid Surface ParsedDocument for modification model tests.
func modSurfaceDoc(id string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindSurface,
		ID:   id,
		Doc: types.SurfaceDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindSurface,
			Metadata:   types.DocumentMetadata{ID: id, Name: "Mod Test Surface"},
			Spec: types.SurfaceSpec{
				Description: "Modification model test surface",
				Category:    "financial",
				RiskTier:    "high",
				Status:      "active",
				ProcessID:   "test.process",
			},
		},
	}
}

// modAgentDoc builds a valid Agent ParsedDocument for modification model tests.
func modAgentDoc(id string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindAgent,
		ID:   id,
		Doc: types.AgentDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindAgent,
			Metadata:   types.DocumentMetadata{ID: id, Name: "Mod Test Agent"},
			Spec: types.AgentSpec{
				Type: "llm_agent",
				Runtime: types.AgentRuntime{
					Model:    "test-model",
					Version:  "1.0",
					Provider: "internal",
				},
				Status: "active",
			},
		},
	}
}

// modProfileDoc builds a valid Profile ParsedDocument for modification model tests.
func modProfileDoc(id, surfaceID string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindProfile,
		ID:   id,
		Doc: types.ProfileDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindProfile,
			Metadata:   types.DocumentMetadata{ID: id, Name: "Mod Test Profile"},
			Spec: types.ProfileSpec{
				SurfaceID: surfaceID,
				Authority: types.ProfileAuthority{
					DecisionConfidenceThreshold: 0.85,
					ConsequenceThreshold: types.ConsequenceThreshold{
						Type:     "monetary",
						Amount:   1000,
						Currency: "GBP",
					},
				},
				Policy: types.ProfilePolicy{
					Reference: "rego://mod/test_v1",
					FailMode:  "closed",
				},
			},
		},
	}
}

// modGrantDoc builds a valid Grant ParsedDocument for modification model tests.
func modGrantDoc(id, agentID, profileID string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindGrant,
		ID:   id,
		Doc: types.GrantDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindGrant,
			Metadata:   types.DocumentMetadata{ID: id, Name: "Mod Test Grant"},
			Spec: types.GrantSpec{
				AgentID:       agentID,
				ProfileID:     profileID,
				GrantedBy:     "governance-lead",
				GrantedAt:     "2026-01-01T00:00:00Z",
				EffectiveFrom: "2026-01-01T00:00:00Z",
				Status:        "active",
			},
		},
	}
}

// modActiveSurface builds a minimal active DecisionSurface domain object for
// directly seeding repositories in tests that need a pre-existing active surface
// without going through the apply service.
// modActiveSurface builds an active DecisionSurface that references the
// canonical test.process seeded by seedTestProcess. Tests using this helper
// must call seedTestProcess against the same repos before invoking Create.
func modActiveSurface(id string) *surface.DecisionSurface {
	return &surface.DecisionSurface{
		ID:            id,
		Version:       1,
		Name:          id,
		Status:        surface.SurfaceStatusActive,
		Domain:        "test",
		ProcessID:     "test.process",
		EffectiveFrom: time.Now().UTC().Add(-time.Hour),
	}
}
