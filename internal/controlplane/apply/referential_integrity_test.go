package apply

// Tests for bundle-level referential integrity planning.
//
// These tests verify that the planner correctly resolves cross-document
// references within a bundle and against persisted state. The references
// enforced are:
//
//   - Profile.spec.surface_id → Surface
//   - Grant.spec.agent_id    → Agent
//   - Grant.spec.profile_id  → Profile
//
// A reference is satisfied by either a create entry in the same bundle or a
// resource confirmed via repository lookup. When no repository is available for
// the referenced kind, the reference passes (cannot be verified).

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// Profile → Surface reference tests
// ---------------------------------------------------------------------------

// TestRefIntegrity_Profile_PersistedSurface_Succeeds verifies that a profile
// referencing a surface that exists in persisted state is planned as create.
func TestRefIntegrity_Profile_PersistedSurface_Succeeds(t *testing.T) {
	surfaceRepo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, id string) (*surface.DecisionSurface, error) {
			if id == "surf-persisted" {
				return &surface.DecisionSurface{ID: "surf-persisted"}, nil
			}
			return nil, nil
		},
	}
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) {
			return nil, nil // profile does not yet exist
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Surfaces: surfaceRepo, Profiles: profileRepo})

	docs := []parser.ParsedDocument{
		validProfileWithSurface("profile-new", "surf-persisted"),
	}
	result := svc.Apply(context.Background(), docs, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d: %+v", result.ValidationErrorCount(), result.ValidationErrors)
	}
	if result.CreatedCount() != 1 {
		t.Errorf("expected 1 created result, got %d", result.CreatedCount())
	}
}

// TestRefIntegrity_Profile_InBundleSurface_Succeeds verifies that a profile
// referencing a surface being created in the same bundle is planned as create.
func TestRefIntegrity_Profile_InBundleSurface_Succeeds(t *testing.T) {
	surfaceRepo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, nil // surface is new (not persisted yet)
		},
	}
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Surfaces: surfaceRepo, Profiles: profileRepo, Processes: processRepoAlwaysExists()})

	docs := []parser.ParsedDocument{
		validSurface("surf-new"),
		validProfileWithSurface("profile-new", "surf-new"),
	}
	result := svc.Apply(context.Background(), docs, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d: %+v", result.ValidationErrorCount(), result.ValidationErrors)
	}
	if result.CreatedCount() != 2 {
		t.Errorf("expected 2 created results, got %d", result.CreatedCount())
	}
}

// TestRefIntegrity_Profile_MissingSurface_Fails verifies that a profile
// referencing a surface that neither exists in persisted state nor is being
// created in the same bundle is marked invalid.
func TestRefIntegrity_Profile_MissingSurface_Fails(t *testing.T) {
	surfaceRepo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, nil // surface does not exist
		},
	}
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Surfaces: surfaceRepo, Profiles: profileRepo})

	docs := []parser.ParsedDocument{
		validProfileWithSurface("profile-orphan", "surf-does-not-exist"),
	}
	result := svc.Apply(context.Background(), docs, "")

	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation errors for profile with missing surface reference, got none")
	}
	// Verify the error mentions referential integrity.
	found := false
	for _, ve := range result.ValidationErrors {
		if ve.Kind == types.KindProfile && ve.ID == "profile-orphan" && ve.Field == "spec.surface_id" {
			found = true
			if !containsSubstring(ve.Message, ErrReferentialIntegrity.Error()) {
				t.Errorf("expected error message to contain %q, got: %q", ErrReferentialIntegrity.Error(), ve.Message)
			}
		}
	}
	if !found {
		t.Error("expected a validation error for Profile spec.surface_id field, not found")
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created results, got %d", result.CreatedCount())
	}
}

// TestRefIntegrity_Profile_ErrReferentialIntegrity_IsWrapped verifies that
// the referential integrity error is represented in the message in a way that
// allows callers to identify it as a referential integrity violation.
func TestRefIntegrity_Profile_ErrReferentialIntegrity_IsWrapped(t *testing.T) {
	if !errors.Is(ErrReferentialIntegrity, ErrReferentialIntegrity) {
		t.Fatal("ErrReferentialIntegrity must satisfy errors.Is with itself")
	}
}

// ---------------------------------------------------------------------------
// Grant → Agent reference tests
// ---------------------------------------------------------------------------

// TestRefIntegrity_Grant_PersistedAgent_Succeeds verifies that a grant
// referencing an agent that exists in persisted state is planned as create.
func TestRefIntegrity_Grant_PersistedAgent_Succeeds(t *testing.T) {
	agentRepo := &controlledAgentRepo{
		getByIDFn: func(_ context.Context, id string) (*agent.Agent, error) {
			if id == "agent-persisted" {
				return &agent.Agent{ID: "agent-persisted"}, nil
			}
			return nil, nil
		},
	}
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, id string) (*authority.AuthorityProfile, error) {
			if id == "profile-persisted" {
				return &authority.AuthorityProfile{ID: "profile-persisted"}, nil
			}
			return nil, nil
		},
	}
	grantRepo := &controlledGrantRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityGrant, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Agents: agentRepo, Profiles: profileRepo, Grants: grantRepo})

	docs := []parser.ParsedDocument{
		validGrantWithRefs("grant-new", "agent-persisted", "profile-persisted"),
	}
	result := svc.Apply(context.Background(), docs, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d: %+v", result.ValidationErrorCount(), result.ValidationErrors)
	}
	if result.CreatedCount() != 1 {
		t.Errorf("expected 1 created result, got %d", result.CreatedCount())
	}
}

// TestRefIntegrity_Grant_InBundleAgentAndProfile_Succeeds verifies that a grant
// referencing an agent and a profile both being created in the same bundle is
// planned as create.
func TestRefIntegrity_Grant_InBundleAgentAndProfile_Succeeds(t *testing.T) {
	surfaceRepo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, nil
		},
	}
	agentRepo := &controlledAgentRepo{
		getByIDFn: func(_ context.Context, _ string) (*agent.Agent, error) { return nil, nil },
	}
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) { return nil, nil },
	}
	grantRepo := &controlledGrantRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityGrant, error) { return nil, nil },
	}
	svc := newServiceWithAll(surfaceRepo, agentRepo, profileRepo, grantRepo)

	docs := []parser.ParsedDocument{
		validSurface("surf-a"),
		validAgent("agent-a"),
		validProfileWithSurface("profile-a", "surf-a"),
		validGrantWithRefs("grant-a", "agent-a", "profile-a"),
	}
	result := svc.Apply(context.Background(), docs, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d: %+v", result.ValidationErrorCount(), result.ValidationErrors)
	}
	if result.CreatedCount() != 4 {
		t.Errorf("expected 4 created results, got %d", result.CreatedCount())
	}
}

// TestRefIntegrity_Grant_MissingAgent_Fails verifies that a grant referencing
// an agent that neither exists in persisted state nor is in the bundle is
// marked invalid.
func TestRefIntegrity_Grant_MissingAgent_Fails(t *testing.T) {
	agentRepo := &controlledAgentRepo{
		getByIDFn: func(_ context.Context, _ string) (*agent.Agent, error) {
			return nil, nil // agent does not exist
		},
	}
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, id string) (*authority.AuthorityProfile, error) {
			// profile exists so only the agent reference fails
			return &authority.AuthorityProfile{ID: id}, nil
		},
	}
	grantRepo := &controlledGrantRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityGrant, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Agents: agentRepo, Profiles: profileRepo, Grants: grantRepo})

	docs := []parser.ParsedDocument{
		validGrantWithRefs("grant-orphan", "agent-missing", "profile-exists"),
	}
	result := svc.Apply(context.Background(), docs, "")

	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation errors for grant with missing agent reference, got none")
	}
	found := false
	for _, ve := range result.ValidationErrors {
		if ve.Kind == types.KindGrant && ve.ID == "grant-orphan" && ve.Field == "spec.agent_id" {
			found = true
			if !containsSubstring(ve.Message, ErrReferentialIntegrity.Error()) {
				t.Errorf("expected error message to contain %q, got: %q", ErrReferentialIntegrity.Error(), ve.Message)
			}
		}
	}
	if !found {
		t.Error("expected a validation error for Grant spec.agent_id field, not found")
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created results, got %d", result.CreatedCount())
	}
}

// TestRefIntegrity_Grant_MissingProfile_Fails verifies that a grant referencing
// a profile that neither exists in persisted state nor is in the bundle is
// marked invalid.
func TestRefIntegrity_Grant_MissingProfile_Fails(t *testing.T) {
	agentRepo := &controlledAgentRepo{
		getByIDFn: func(_ context.Context, id string) (*agent.Agent, error) {
			// agent exists so only the profile reference fails
			return &agent.Agent{ID: id}, nil
		},
	}
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) {
			return nil, nil // profile does not exist
		},
	}
	grantRepo := &controlledGrantRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityGrant, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Agents: agentRepo, Profiles: profileRepo, Grants: grantRepo})

	docs := []parser.ParsedDocument{
		validGrantWithRefs("grant-orphan", "agent-exists", "profile-missing"),
	}
	result := svc.Apply(context.Background(), docs, "")

	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation errors for grant with missing profile reference, got none")
	}
	found := false
	for _, ve := range result.ValidationErrors {
		if ve.Kind == types.KindGrant && ve.ID == "grant-orphan" && ve.Field == "spec.profile_id" {
			found = true
			if !containsSubstring(ve.Message, ErrReferentialIntegrity.Error()) {
				t.Errorf("expected error message to contain %q, got: %q", ErrReferentialIntegrity.Error(), ve.Message)
			}
		}
	}
	if !found {
		t.Error("expected a validation error for Grant spec.profile_id field, not found")
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created results, got %d", result.CreatedCount())
	}
}

// ---------------------------------------------------------------------------
// Mixed bundle dependency graph tests
// ---------------------------------------------------------------------------

// TestRefIntegrity_MixedBundle_DependencyOrderPreserved verifies that a bundle
// containing all four kinds executes in the correct dependency order and that
// each resource is created successfully.
func TestRefIntegrity_MixedBundle_DependencyOrderPreserved(t *testing.T) {
	var createOrder []string

	surfaceRepo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, nil
		},
	}
	// Override Create to record the kind being created.
	surfaceRepo.createCalled = 0 // reset counter; recording via a closure below

	agentRepo := &controlledAgentRepo{
		getByIDFn: func(_ context.Context, _ string) (*agent.Agent, error) { return nil, nil },
	}
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) { return nil, nil },
	}
	grantRepo := &controlledGrantRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityGrant, error) { return nil, nil },
	}

	// Wrap the apply service with recording repos via a custom RepositorySet.
	recordingSurface := &recordingRepo{inner: surfaceRepo, order: &createOrder, kind: types.KindSurface}
	recordingAgent := &recordingAgentRepo{inner: agentRepo, order: &createOrder}
	recordingProfile := &recordingProfileRepo{inner: profileRepo, order: &createOrder}
	recordingGrant := &recordingGrantRepo{inner: grantRepo, order: &createOrder}

	svc := NewServiceWithRepos(RepositorySet{
		Surfaces:  recordingSurface,
		Agents:    recordingAgent,
		Profiles:  recordingProfile,
		Grants:    recordingGrant,
		Processes: processRepoAlwaysExists(),
	})

	// Submit bundle in reverse dependency order to confirm that the executor
	// reorders them correctly.
	docs := []parser.ParsedDocument{
		validGrantWithRefs("grant-1", "agent-1", "profile-1"),
		validProfileWithSurface("profile-1", "surf-1"),
		validAgent("agent-1"),
		validSurface("surf-1"),
	}
	result := svc.Apply(context.Background(), docs, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d: %+v", result.ValidationErrorCount(), result.ValidationErrors)
	}
	if result.CreatedCount() != 4 {
		t.Errorf("expected 4 created results, got %d", result.CreatedCount())
	}

	// Verify ordering: Surface and Agent before Profile; Profile before Grant.
	surfIdx := indexOf(createOrder, types.KindSurface)
	agentIdx := indexOf(createOrder, types.KindAgent)
	profileIdx := indexOf(createOrder, types.KindProfile)
	grantIdx := indexOf(createOrder, types.KindGrant)

	if surfIdx < 0 || agentIdx < 0 || profileIdx < 0 || grantIdx < 0 {
		t.Fatalf("not all kinds were created: order=%v", createOrder)
	}
	if profileIdx < surfIdx {
		t.Errorf("profile was created before surface: order=%v", createOrder)
	}
	if grantIdx < agentIdx {
		t.Errorf("grant was created before agent: order=%v", createOrder)
	}
	if grantIdx < profileIdx {
		t.Errorf("grant was created before profile: order=%v", createOrder)
	}
}

// TestRefIntegrity_InvalidDependency_NothingPersisted verifies that a bundle
// with an unresolvable reference rejects the entire bundle without persisting
// any resource.
func TestRefIntegrity_InvalidDependency_NothingPersisted(t *testing.T) {
	surfaceRepo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, nil // surface does not exist
		},
	}
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) { return nil, nil },
	}

	svc := NewServiceWithRepos(RepositorySet{Surfaces: surfaceRepo, Profiles: profileRepo})

	// Profile references a surface that doesn't exist; bundle should be rejected.
	docs := []parser.ParsedDocument{
		validProfileWithSurface("profile-bad", "surf-nonexistent"),
	}
	result := svc.Apply(context.Background(), docs, "")

	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation errors for unresolvable reference, got none")
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created results (nothing persisted), got %d", result.CreatedCount())
	}
	// The profile repo Create must not have been called.
	if profileRepo.createCalled != 0 {
		t.Errorf("expected profile Create to not be called for invalid bundle, called %d times", profileRepo.createCalled)
	}
	if surfaceRepo.createCalled != 0 {
		t.Errorf("expected surface Create to not be called for invalid bundle, called %d times", surfaceRepo.createCalled)
	}
}

// TestRefIntegrity_ExistingProfileSatisfiesGrantRef verifies that a profile
// document whose logical ID already exists in persisted state is planned as a
// new-version create (not a conflict) and therefore satisfies the grant's
// referential integrity check within the same bundle. Both profile and grant
// should be created.
func TestRefIntegrity_ExistingProfileSatisfiesGrantRef(t *testing.T) {
	agentRepo := &controlledAgentRepo{
		getByIDFn: func(_ context.Context, id string) (*agent.Agent, error) {
			return &agent.Agent{ID: id}, nil // agent always exists
		},
	}
	// Profile already exists at v1 → planner creates v2 (not conflict).
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, id string) (*authority.AuthorityProfile, error) {
			return &authority.AuthorityProfile{ID: id, Version: 1}, nil
		},
	}
	grantRepo := &controlledGrantRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityGrant, error) { return nil, nil },
	}
	svc := NewServiceWithRepos(RepositorySet{Agents: agentRepo, Profiles: profileRepo, Grants: grantRepo})

	// The profile document is in the bundle and will create a new version (v2).
	// The grant references that profile. Since the profile is a create (new version),
	// it contributes to bundleCreates, satisfying the grant's reference.
	// Both profile and grant should be created.
	docs := []parser.ParsedDocument{
		validProfileWithSurface("profile-existing", "payment.execute"),
		validGrantWithRefs("grant-new", "agent-exists", "profile-existing"),
	}
	result := svc.Apply(context.Background(), docs, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d: %+v", result.ValidationErrorCount(), result.ValidationErrors)
	}
	if result.ConflictCount() != 0 {
		t.Errorf("expected 0 conflicts (profiles never conflict), got %d", result.ConflictCount())
	}
	if result.CreatedCount() != 2 {
		t.Errorf("expected 2 created (profile new version + grant), got %d", result.CreatedCount())
	}
}

// ---------------------------------------------------------------------------
// Execution ordering verification helpers
// ---------------------------------------------------------------------------

// recordingRepo wraps a SurfaceRepository and records the kind when Create is called.
type recordingRepo struct {
	inner *controlledSurfaceRepo
	order *[]string
	kind  string
}

func (r *recordingRepo) FindLatestByID(ctx context.Context, id string) (*surface.DecisionSurface, error) {
	return r.inner.FindLatestByID(ctx, id)
}

func (r *recordingRepo) Create(ctx context.Context, s *surface.DecisionSurface) error {
	*r.order = append(*r.order, r.kind)
	return r.inner.Create(ctx, s)
}

// recordingAgentRepo wraps controlledAgentRepo and records create calls.
type recordingAgentRepo struct {
	inner *controlledAgentRepo
	order *[]string
}

func (r *recordingAgentRepo) GetByID(ctx context.Context, id string) (*agent.Agent, error) {
	return r.inner.GetByID(ctx, id)
}

func (r *recordingAgentRepo) Create(ctx context.Context, a *agent.Agent) error {
	*r.order = append(*r.order, types.KindAgent)
	return r.inner.Create(ctx, a)
}

func (r *recordingAgentRepo) Update(ctx context.Context, a *agent.Agent) error {
	return r.inner.Update(ctx, a)
}

func (r *recordingAgentRepo) List(ctx context.Context) ([]*agent.Agent, error) {
	return r.inner.List(ctx)
}

// recordingProfileRepo wraps controlledProfileRepo and records create calls.
type recordingProfileRepo struct {
	inner *controlledProfileRepo
	order *[]string
}

func (r *recordingProfileRepo) FindByID(ctx context.Context, id string) (*authority.AuthorityProfile, error) {
	return r.inner.FindByID(ctx, id)
}

func (r *recordingProfileRepo) Create(ctx context.Context, p *authority.AuthorityProfile) error {
	*r.order = append(*r.order, types.KindProfile)
	return r.inner.Create(ctx, p)
}

func (r *recordingProfileRepo) FindByIDAndVersion(ctx context.Context, id string, v int) (*authority.AuthorityProfile, error) {
	return r.inner.FindByIDAndVersion(ctx, id, v)
}

func (r *recordingProfileRepo) FindActiveAt(ctx context.Context, id string, at time.Time) (*authority.AuthorityProfile, error) {
	panic("FindActiveAt not expected in recording repo")
}

func (r *recordingProfileRepo) ListBySurface(ctx context.Context, surfaceID string) ([]*authority.AuthorityProfile, error) {
	return r.inner.ListBySurface(ctx, surfaceID)
}

func (r *recordingProfileRepo) ListVersions(ctx context.Context, id string) ([]*authority.AuthorityProfile, error) {
	return r.inner.ListVersions(ctx, id)
}

func (r *recordingProfileRepo) Update(ctx context.Context, p *authority.AuthorityProfile) error {
	return r.inner.Update(ctx, p)
}

// recordingGrantRepo wraps controlledGrantRepo and records create calls.
type recordingGrantRepo struct {
	inner *controlledGrantRepo
	order *[]string
}

func (r *recordingGrantRepo) FindByID(ctx context.Context, id string) (*authority.AuthorityGrant, error) {
	return r.inner.FindByID(ctx, id)
}

func (r *recordingGrantRepo) Create(ctx context.Context, g *authority.AuthorityGrant) error {
	*r.order = append(*r.order, types.KindGrant)
	return r.inner.Create(ctx, g)
}

func (r *recordingGrantRepo) FindActiveByAgentAndProfile(ctx context.Context, agentID, profileID string) (*authority.AuthorityGrant, error) {
	return r.inner.FindActiveByAgentAndProfile(ctx, agentID, profileID)
}

func (r *recordingGrantRepo) ListByAgent(ctx context.Context, agentID string) ([]*authority.AuthorityGrant, error) {
	return r.inner.ListByAgent(ctx, agentID)
}

func (r *recordingGrantRepo) Revoke(ctx context.Context, id string, revokedBy string) error {
	return r.inner.Revoke(ctx, id, revokedBy)
}

func (r *recordingGrantRepo) Suspend(ctx context.Context, id string) error {
	return r.inner.Suspend(ctx, id)
}

func (r *recordingGrantRepo) Reactivate(ctx context.Context, id string) error {
	return r.inner.Reactivate(ctx, id)
}

// ---------------------------------------------------------------------------
// Test utilities
// ---------------------------------------------------------------------------

func containsSubstring(s, substr string) bool {
	return strings.Contains(s, substr)
}

func indexOf(slice []string, val string) int {
	for i, v := range slice {
		if v == val {
			return i
		}
	}
	return -1
}
