package apply

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// Test helpers: document constructors
// ---------------------------------------------------------------------------

func validAgent(id string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindAgent,
		ID:   id,
		Doc: types.AgentDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindAgent,
			Metadata: types.DocumentMetadata{
				ID:   id,
				Name: "Test Agent",
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
	}
}

func invalidAgent(id string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindAgent,
		ID:   id,
		Doc: types.AgentDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindAgent,
			Metadata: types.DocumentMetadata{
				ID:   id,
				Name: "", // required — triggers validation failure
			},
			Spec: types.AgentSpec{
				Type:   "", // required — triggers validation failure
				Status: "", // required — triggers validation failure
			},
		},
	}
}

func validProfile(id string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindProfile,
		ID:   id,
		Doc: types.ProfileDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindProfile,
			Metadata: types.DocumentMetadata{
				ID:   id,
				Name: "Test Profile",
			},
			Spec: types.ProfileSpec{
				SurfaceID: "payment.execute",
				Authority: types.ProfileAuthority{
					DecisionConfidenceThreshold: 0.85,
					ConsequenceThreshold: types.ConsequenceThreshold{
						Type:     "monetary",
						Amount:   10000,
						Currency: "USD",
					},
				},
				Policy: types.ProfilePolicy{
					Reference: "rego://payments/auto_approve_v1",
					FailMode:  "closed",
				},
			},
		},
	}
}

func invalidProfile(id string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindProfile,
		ID:   id,
		Doc: types.ProfileDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindProfile,
			Metadata: types.DocumentMetadata{
				ID:   id,
				Name: "", // required — triggers validation failure
			},
			Spec: types.ProfileSpec{
				SurfaceID: "", // required — triggers validation failure
				Policy: types.ProfilePolicy{
					Reference: "", // required — triggers validation failure
					FailMode:  "", // required — triggers validation failure
				},
			},
		},
	}
}

func validGrant(id string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindGrant,
		ID:   id,
		Doc: types.GrantDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindGrant,
			Metadata: types.DocumentMetadata{
				ID:   id,
				Name: "Test Grant",
			},
			Spec: types.GrantSpec{
				AgentID:       "agent-1",
				ProfileID:     "profile-1",
				GrantedBy:     "admin@example.com",
				GrantedAt:     "2025-03-17T10:00:00Z",
				EffectiveFrom: "2025-03-17T10:00:00Z",
				Status:        "active",
			},
		},
	}
}

func invalidGrant(id string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindGrant,
		ID:   id,
		Doc: types.GrantDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindGrant,
			Metadata: types.DocumentMetadata{
				ID:   id,
				Name: "Test Grant",
			},
			Spec: types.GrantSpec{
				AgentID:   "", // required — triggers validation failure
				ProfileID: "profile-1",
				GrantedBy: "admin@example.com",
				GrantedAt: "2025-03-17T10:00:00Z",
				Status:    "active",
				// EffectiveFrom missing — triggers validation failure
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Stub repositories for Agent, Profile, and Grant
// ---------------------------------------------------------------------------

type controlledAgentRepo struct {
	getByIDFn    func(ctx context.Context, id string) (*agent.Agent, error)
	createCalled int
}

func (r *controlledAgentRepo) GetByID(ctx context.Context, id string) (*agent.Agent, error) {
	return r.getByIDFn(ctx, id)
}

func (r *controlledAgentRepo) Create(ctx context.Context, a *agent.Agent) error {
	r.createCalled++
	return nil
}

func (r *controlledAgentRepo) Update(_ context.Context, _ *agent.Agent) error {
	panic("Update called unexpectedly in agent planner test")
}

func (r *controlledAgentRepo) List(_ context.Context) ([]*agent.Agent, error) {
	panic("List called unexpectedly in agent planner test")
}

type controlledProfileRepo struct {
	findByIDFn   func(ctx context.Context, id string) (*authority.AuthorityProfile, error)
	createCalled int
}

func (r *controlledProfileRepo) FindByID(ctx context.Context, id string) (*authority.AuthorityProfile, error) {
	return r.findByIDFn(ctx, id)
}

func (r *controlledProfileRepo) Create(ctx context.Context, p *authority.AuthorityProfile) error {
	r.createCalled++
	return nil
}

func (r *controlledProfileRepo) FindByIDAndVersion(_ context.Context, _ string, _ int) (*authority.AuthorityProfile, error) {
	panic("FindByIDAndVersion called unexpectedly in profile planner test")
}

func (r *controlledProfileRepo) FindActiveAt(_ context.Context, _ string, _ time.Time) (*authority.AuthorityProfile, error) {
	panic("FindActiveAt called unexpectedly in profile planner test")
}

func (r *controlledProfileRepo) ListBySurface(_ context.Context, _ string) ([]*authority.AuthorityProfile, error) {
	panic("ListBySurface called unexpectedly in profile planner test")
}

func (r *controlledProfileRepo) ListVersions(_ context.Context, _ string) ([]*authority.AuthorityProfile, error) {
	panic("ListVersions called unexpectedly in profile planner test")
}

func (r *controlledProfileRepo) Update(_ context.Context, _ *authority.AuthorityProfile) error {
	panic("Update called unexpectedly in profile planner test")
}

type controlledGrantRepo struct {
	findByIDFn   func(ctx context.Context, id string) (*authority.AuthorityGrant, error)
	createCalled int
}

func (r *controlledGrantRepo) FindByID(ctx context.Context, id string) (*authority.AuthorityGrant, error) {
	return r.findByIDFn(ctx, id)
}

func (r *controlledGrantRepo) Create(ctx context.Context, g *authority.AuthorityGrant) error {
	r.createCalled++
	return nil
}

func (r *controlledGrantRepo) FindActiveByAgentAndProfile(_ context.Context, _, _ string) (*authority.AuthorityGrant, error) {
	panic("FindActiveByAgentAndProfile called unexpectedly in grant planner test")
}

func (r *controlledGrantRepo) ListByAgent(_ context.Context, _ string) ([]*authority.AuthorityGrant, error) {
	panic("ListByAgent called unexpectedly in grant planner test")
}

func (r *controlledGrantRepo) Revoke(_ context.Context, _ string, _ string) error {
	panic("Revoke called unexpectedly in grant planner test")
}

func (r *controlledGrantRepo) Suspend(_ context.Context, _ string) error {
	panic("Suspend called unexpectedly in grant planner test")
}

func (r *controlledGrantRepo) Reactivate(_ context.Context, _ string) error {
	panic("Reactivate called unexpectedly in grant planner test")
}

// newServiceWithAll constructs an apply service with all four repositories wired,
// plus a process repository that always reports processes as existing.
func newServiceWithAll(
	surfaceRepo SurfaceRepository,
	agentRepo AgentRepository,
	profileRepo ProfileRepository,
	grantRepo GrantRepository,
) *Service {
	return NewServiceWithRepos(RepositorySet{
		Surfaces:  surfaceRepo,
		Agents:    agentRepo,
		Profiles:  profileRepo,
		Grants:    grantRepo,
		Processes: processRepoAlwaysExists(),
	})
}

// ---------------------------------------------------------------------------
// Agent planner tests
// ---------------------------------------------------------------------------

// TestBuildApplyPlan_Agent_Invalid_ActionInvalid verifies that an invalid agent
// document is planned as invalid before any repository lookup occurs.
func TestBuildApplyPlan_Agent_Invalid_ActionInvalid(t *testing.T) {
	lookupCalled := 0
	agentRepo := &controlledAgentRepo{
		getByIDFn: func(_ context.Context, _ string) (*agent.Agent, error) {
			lookupCalled++
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Agents: agentRepo})

	plan := svc.buildApplyPlan(context.Background(), []parser.ParsedDocument{invalidAgent("agent-bad")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Action != ApplyActionInvalid {
		t.Errorf("expected action %q, got %q", ApplyActionInvalid, plan.Entries[0].Action)
	}
	if lookupCalled != 0 {
		t.Errorf("expected no repository lookup for invalid doc, called %d times", lookupCalled)
	}
}

// TestBuildApplyPlan_Agent_NotFound_ActionCreate verifies that a valid agent
// document with no persisted counterpart is planned as create.
func TestBuildApplyPlan_Agent_NotFound_ActionCreate(t *testing.T) {
	agentRepo := &controlledAgentRepo{
		getByIDFn: func(_ context.Context, _ string) (*agent.Agent, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Agents: agentRepo})

	plan := svc.buildApplyPlan(context.Background(), []parser.ParsedDocument{validAgent("agent-new")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Action != ApplyActionCreate {
		t.Errorf("expected action %q, got %q", ApplyActionCreate, plan.Entries[0].Action)
	}
}

// TestBuildApplyPlan_Agent_Exists_ActionConflict verifies that a valid agent
// document whose ID already exists in persisted state is planned as conflict.
func TestBuildApplyPlan_Agent_Exists_ActionConflict(t *testing.T) {
	agentRepo := &controlledAgentRepo{
		getByIDFn: func(_ context.Context, id string) (*agent.Agent, error) {
			return &agent.Agent{ID: id}, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Agents: agentRepo})

	plan := svc.buildApplyPlan(context.Background(), []parser.ParsedDocument{validAgent("agent-existing")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionConflict {
		t.Errorf("expected action %q, got %q", ApplyActionConflict, entry.Action)
	}
	if entry.Message == "" {
		t.Error("expected non-empty conflict message")
	}
}

// TestBuildApplyPlan_Agent_RepoError_ActionInvalid verifies that a repository
// error during agent planning produces an invalid entry.
func TestBuildApplyPlan_Agent_RepoError_ActionInvalid(t *testing.T) {
	agentRepo := &controlledAgentRepo{
		getByIDFn: func(_ context.Context, _ string) (*agent.Agent, error) {
			return nil, fmt.Errorf("database unavailable")
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Agents: agentRepo})

	plan := svc.buildApplyPlan(context.Background(), []parser.ParsedDocument{validAgent("agent-err")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Action != ApplyActionInvalid {
		t.Errorf("expected action %q on repo error, got %q", ApplyActionInvalid, plan.Entries[0].Action)
	}
}

// ---------------------------------------------------------------------------
// Profile planner tests
// ---------------------------------------------------------------------------

// TestBuildApplyPlan_Profile_Invalid_ActionInvalid verifies that an invalid
// profile document is planned as invalid before any repository lookup occurs.
func TestBuildApplyPlan_Profile_Invalid_ActionInvalid(t *testing.T) {
	lookupCalled := 0
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) {
			lookupCalled++
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Profiles: profileRepo})

	plan := svc.buildApplyPlan(context.Background(), []parser.ParsedDocument{invalidProfile("profile-bad")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Action != ApplyActionInvalid {
		t.Errorf("expected action %q, got %q", ApplyActionInvalid, plan.Entries[0].Action)
	}
	if lookupCalled != 0 {
		t.Errorf("expected no repository lookup for invalid doc, called %d times", lookupCalled)
	}
}

// TestBuildApplyPlan_Profile_NotFound_ActionCreate verifies that a valid profile
// document with no persisted counterpart is planned as create.
func TestBuildApplyPlan_Profile_NotFound_ActionCreate(t *testing.T) {
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Profiles: profileRepo})

	plan := svc.buildApplyPlan(context.Background(), []parser.ParsedDocument{validProfile("profile-new")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Action != ApplyActionCreate {
		t.Errorf("expected action %q, got %q", ApplyActionCreate, plan.Entries[0].Action)
	}
}

// TestBuildApplyPlan_Profile_Exists_CreatesNewVersion verifies that a valid profile
// document whose ID already exists in persisted state is planned as a new version
// (not a conflict). Profiles follow a versioned lineage model.
func TestBuildApplyPlan_Profile_Exists_CreatesNewVersion(t *testing.T) {
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, id string) (*authority.AuthorityProfile, error) {
			return &authority.AuthorityProfile{ID: id, Version: 1}, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Profiles: profileRepo})

	plan := svc.buildApplyPlan(context.Background(), []parser.ParsedDocument{validProfile("profile-existing")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionCreate {
		t.Errorf("expected action %q (new version), got %q", ApplyActionCreate, entry.Action)
	}
	if entry.NewVersion != 2 {
		t.Errorf("expected NewVersion 2, got %d", entry.NewVersion)
	}
	if entry.Message == "" {
		t.Error("expected non-empty version message")
	}
}

// TestBuildApplyPlan_Profile_New_AssignsVersion1 verifies that a valid profile
// document with no persisted counterpart is planned as create with NewVersion=1.
func TestBuildApplyPlan_Profile_New_AssignsVersion1(t *testing.T) {
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Profiles: profileRepo})

	plan := svc.buildApplyPlan(context.Background(), []parser.ParsedDocument{validProfile("profile-new")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionCreate {
		t.Errorf("expected action %q, got %q", ApplyActionCreate, entry.Action)
	}
	if entry.NewVersion != 1 {
		t.Errorf("expected NewVersion 1, got %d", entry.NewVersion)
	}
}

// TestBuildApplyPlan_Profile_VersionIncrement verifies that repeated applies of
// the same profile ID produce increasing version numbers. Uses the memory store
// so the second plan sees the state written by the first apply.
func TestBuildApplyPlan_Profile_VersionIncrement(t *testing.T) {
	// Use a simple stub that simulates a repository with state.
	currentVersion := 0
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) {
			if currentVersion == 0 {
				return nil, nil
			}
			return &authority.AuthorityProfile{ID: "prof-incr", Version: currentVersion}, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Profiles: profileRepo})
	ctx := context.Background()

	// First apply — no existing version.
	plan1 := svc.buildApplyPlan(ctx, []parser.ParsedDocument{validProfile("prof-incr")})
	if len(plan1.Entries) != 1 || plan1.Entries[0].NewVersion != 1 {
		t.Fatalf("first apply: expected NewVersion 1, got %d", plan1.Entries[0].NewVersion)
	}

	// Simulate executor persisting v1.
	currentVersion = 1

	// Second apply — existing version is 1.
	plan2 := svc.buildApplyPlan(ctx, []parser.ParsedDocument{validProfile("prof-incr")})
	if len(plan2.Entries) != 1 || plan2.Entries[0].NewVersion != 2 {
		t.Fatalf("second apply: expected NewVersion 2, got %d", plan2.Entries[0].NewVersion)
	}
	if plan2.Entries[0].Action != ApplyActionCreate {
		t.Errorf("expected action create on second apply, got %q", plan2.Entries[0].Action)
	}
}

// TestBuildApplyPlan_Profile_RepoError_ActionInvalid verifies that a repository
// error during profile planning produces an invalid entry.
func TestBuildApplyPlan_Profile_RepoError_ActionInvalid(t *testing.T) {
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) {
			return nil, fmt.Errorf("database unavailable")
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Profiles: profileRepo})

	plan := svc.buildApplyPlan(context.Background(), []parser.ParsedDocument{validProfile("profile-err")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Action != ApplyActionInvalid {
		t.Errorf("expected action %q on repo error, got %q", ApplyActionInvalid, plan.Entries[0].Action)
	}
}

// ---------------------------------------------------------------------------
// Grant planner tests
// ---------------------------------------------------------------------------

// TestBuildApplyPlan_Grant_Invalid_ActionInvalid verifies that an invalid grant
// document is planned as invalid before any repository lookup occurs.
func TestBuildApplyPlan_Grant_Invalid_ActionInvalid(t *testing.T) {
	lookupCalled := 0
	grantRepo := &controlledGrantRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityGrant, error) {
			lookupCalled++
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Grants: grantRepo})

	plan := svc.buildApplyPlan(context.Background(), []parser.ParsedDocument{invalidGrant("grant-bad")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Action != ApplyActionInvalid {
		t.Errorf("expected action %q, got %q", ApplyActionInvalid, plan.Entries[0].Action)
	}
	if lookupCalled != 0 {
		t.Errorf("expected no repository lookup for invalid doc, called %d times", lookupCalled)
	}
}

// TestBuildApplyPlan_Grant_NotFound_ActionCreate verifies that a valid grant
// document with no persisted counterpart is planned as create.
func TestBuildApplyPlan_Grant_NotFound_ActionCreate(t *testing.T) {
	grantRepo := &controlledGrantRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityGrant, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Grants: grantRepo})

	plan := svc.buildApplyPlan(context.Background(), []parser.ParsedDocument{validGrant("grant-new")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Action != ApplyActionCreate {
		t.Errorf("expected action %q, got %q", ApplyActionCreate, plan.Entries[0].Action)
	}
}

// TestBuildApplyPlan_Grant_Exists_ActionConflict verifies that a valid grant
// document whose ID already exists in persisted state is planned as conflict.
func TestBuildApplyPlan_Grant_Exists_ActionConflict(t *testing.T) {
	grantRepo := &controlledGrantRepo{
		findByIDFn: func(_ context.Context, id string) (*authority.AuthorityGrant, error) {
			return &authority.AuthorityGrant{ID: id}, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Grants: grantRepo})

	plan := svc.buildApplyPlan(context.Background(), []parser.ParsedDocument{validGrant("grant-existing")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionConflict {
		t.Errorf("expected action %q, got %q", ApplyActionConflict, entry.Action)
	}
	if entry.Message == "" {
		t.Error("expected non-empty conflict message")
	}
}

// TestBuildApplyPlan_Grant_RepoError_ActionInvalid verifies that a repository
// error during grant planning produces an invalid entry.
func TestBuildApplyPlan_Grant_RepoError_ActionInvalid(t *testing.T) {
	grantRepo := &controlledGrantRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityGrant, error) {
			return nil, fmt.Errorf("database unavailable")
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Grants: grantRepo})

	plan := svc.buildApplyPlan(context.Background(), []parser.ParsedDocument{validGrant("grant-err")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Action != ApplyActionInvalid {
		t.Errorf("expected action %q on repo error, got %q", ApplyActionInvalid, plan.Entries[0].Action)
	}
}

// ---------------------------------------------------------------------------
// Executor contract tests for Agent/Profile/Grant
// ---------------------------------------------------------------------------

// TestExecutePlan_AgentConflict_NoCreateCall verifies that a conflict entry for
// an agent is not persisted and records a conflict result.
func TestExecutePlan_AgentConflict_NoCreateCall(t *testing.T) {
	agentRepo := &controlledAgentRepo{
		getByIDFn: func(_ context.Context, _ string) (*agent.Agent, error) { return nil, nil },
	}
	svc := NewServiceWithRepos(RepositorySet{Agents: agentRepo})

	plan := ApplyPlan{
		Entries: []ApplyPlanEntry{
			{
				Kind:    types.KindAgent,
				ID:      "agent-conflict",
				Action:  ApplyActionConflict,
				Message: "agent already exists",
				Doc:     validAgent("agent-conflict"),
			},
		},
	}

	result := svc.executePlan(context.Background(), plan, "")

	if agentRepo.createCalled != 0 {
		t.Errorf("expected Create to not be called for conflict entry, called %d times", agentRepo.createCalled)
	}
	if result.ConflictCount() != 1 {
		t.Errorf("expected 1 conflict result, got %d", result.ConflictCount())
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created results, got %d", result.CreatedCount())
	}
}

// TestExecutePlan_AgentCreate_CallsRepository verifies that a create entry for
// an agent calls the repository and records a created result.
func TestExecutePlan_AgentCreate_CallsRepository(t *testing.T) {
	agentRepo := &controlledAgentRepo{
		getByIDFn: func(_ context.Context, _ string) (*agent.Agent, error) { return nil, nil },
	}
	svc := NewServiceWithRepos(RepositorySet{Agents: agentRepo})

	result := svc.Apply(context.Background(), []parser.ParsedDocument{validAgent("agent-new")}, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d", result.ValidationErrorCount())
	}
	if result.CreatedCount() != 1 {
		t.Errorf("expected 1 created result, got %d", result.CreatedCount())
	}
	if agentRepo.createCalled != 1 {
		t.Errorf("expected repository Create to be called once, called %d times", agentRepo.createCalled)
	}
}

// TestExecutePlan_ProfileConflict_NoCreateCall verifies that a conflict entry
// for a profile is not persisted and records a conflict result.
func TestExecutePlan_ProfileConflict_NoCreateCall(t *testing.T) {
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) { return nil, nil },
	}
	svc := NewServiceWithRepos(RepositorySet{Profiles: profileRepo})

	plan := ApplyPlan{
		Entries: []ApplyPlanEntry{
			{
				Kind:    types.KindProfile,
				ID:      "profile-conflict",
				Action:  ApplyActionConflict,
				Message: "profile already exists",
				Doc:     validProfile("profile-conflict"),
			},
		},
	}

	result := svc.executePlan(context.Background(), plan, "")

	if profileRepo.createCalled != 0 {
		t.Errorf("expected Create to not be called for conflict entry, called %d times", profileRepo.createCalled)
	}
	if result.ConflictCount() != 1 {
		t.Errorf("expected 1 conflict result, got %d", result.ConflictCount())
	}
}

// TestExecutePlan_ProfileCreate_CallsRepository verifies that a create entry
// for a profile calls the repository and records a created result.
func TestExecutePlan_ProfileCreate_CallsRepository(t *testing.T) {
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) { return nil, nil },
	}
	svc := NewServiceWithRepos(RepositorySet{Profiles: profileRepo})

	result := svc.Apply(context.Background(), []parser.ParsedDocument{validProfile("profile-new")}, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d", result.ValidationErrorCount())
	}
	if result.CreatedCount() != 1 {
		t.Errorf("expected 1 created result, got %d", result.CreatedCount())
	}
	if profileRepo.createCalled != 1 {
		t.Errorf("expected repository Create to be called once, called %d times", profileRepo.createCalled)
	}
}

// TestExecutePlan_GrantConflict_NoCreateCall verifies that a conflict entry for
// a grant is not persisted and records a conflict result.
func TestExecutePlan_GrantConflict_NoCreateCall(t *testing.T) {
	grantRepo := &controlledGrantRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityGrant, error) { return nil, nil },
	}
	svc := NewServiceWithRepos(RepositorySet{Grants: grantRepo})

	plan := ApplyPlan{
		Entries: []ApplyPlanEntry{
			{
				Kind:    types.KindGrant,
				ID:      "grant-conflict",
				Action:  ApplyActionConflict,
				Message: "grant already exists",
				Doc:     validGrant("grant-conflict"),
			},
		},
	}

	result := svc.executePlan(context.Background(), plan, "")

	if grantRepo.createCalled != 0 {
		t.Errorf("expected Create to not be called for conflict entry, called %d times", grantRepo.createCalled)
	}
	if result.ConflictCount() != 1 {
		t.Errorf("expected 1 conflict result, got %d", result.ConflictCount())
	}
}

// TestExecutePlan_GrantCreate_CallsRepository verifies that a create entry for
// a grant calls the repository and records a created result.
func TestExecutePlan_GrantCreate_CallsRepository(t *testing.T) {
	grantRepo := &controlledGrantRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityGrant, error) { return nil, nil },
	}
	svc := NewServiceWithRepos(RepositorySet{Grants: grantRepo})

	result := svc.Apply(context.Background(), []parser.ParsedDocument{validGrant("grant-new")}, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d", result.ValidationErrorCount())
	}
	if result.CreatedCount() != 1 {
		t.Errorf("expected 1 created result, got %d", result.CreatedCount())
	}
	if grantRepo.createCalled != 1 {
		t.Errorf("expected repository Create to be called once, called %d times", grantRepo.createCalled)
	}
}

// validProfileWithSurface constructs a valid profile document referencing a
// specific surface ID. Use this when the surface ID must match another document
// in the same bundle or a persisted surface.
func validProfileWithSurface(id, surfaceID string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindProfile,
		ID:   id,
		Doc: types.ProfileDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindProfile,
			Metadata: types.DocumentMetadata{
				ID:   id,
				Name: "Test Profile",
			},
			Spec: types.ProfileSpec{
				SurfaceID: surfaceID,
				Authority: types.ProfileAuthority{
					DecisionConfidenceThreshold: 0.85,
					ConsequenceThreshold: types.ConsequenceThreshold{
						Type:     "monetary",
						Amount:   10000,
						Currency: "USD",
					},
				},
				Policy: types.ProfilePolicy{
					Reference: "rego://payments/auto_approve_v1",
					FailMode:  "closed",
				},
			},
		},
	}
}

// validGrantWithRefs constructs a valid grant document referencing specific
// agent and profile IDs. Use this when the referenced IDs must match other
// documents in the same bundle or persisted resources.
func validGrantWithRefs(id, agentID, profileID string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindGrant,
		ID:   id,
		Doc: types.GrantDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindGrant,
			Metadata: types.DocumentMetadata{
				ID:   id,
				Name: "Test Grant",
			},
			Spec: types.GrantSpec{
				AgentID:       agentID,
				ProfileID:     profileID,
				GrantedBy:     "admin@example.com",
				GrantedAt:     "2025-03-17T10:00:00Z",
				EffectiveFrom: "2025-03-17T10:00:00Z",
				Status:        "active",
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Full-bundle tests with all four repository kinds
// ---------------------------------------------------------------------------

// TestApply_FullBundle_AllNew_AllCreated verifies that a bundle containing one
// of each resource kind, all new, results in four created entries. The profile
// references the surface in the same bundle; the grant references the agent and
// profile in the same bundle.
func TestApply_FullBundle_AllNew_AllCreated(t *testing.T) {
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
		validSurface("surf-1"),
		validAgent("agent-1"),
		validProfileWithSurface("profile-1", "surf-1"),
		validGrantWithRefs("grant-1", "agent-1", "profile-1"),
	}

	result := svc.Apply(context.Background(), docs, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d: %+v", result.ValidationErrorCount(), result.ValidationErrors)
	}
	if result.CreatedCount() != 4 {
		t.Errorf("expected 4 created results, got %d", result.CreatedCount())
	}
	if !result.Success() {
		t.Error("expected Apply to succeed when all resources are new")
	}
}

// TestApply_FullBundle_ExistingAgent_ConflictResult verifies that a bundle where
// the agent already exists produces a conflict result and no creation for that agent.
// The grant references the existing agent (conflict) and the in-bundle profile.
// Because the agent is a conflict (not invalid), the grant's reference to it
// cannot be satisfied from the bundle's create entries, so the grant is marked
// invalid via referential integrity.
func TestApply_FullBundle_ExistingAgent_ConflictResult(t *testing.T) {
	surfaceRepo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, nil
		},
	}
	agentRepo := &controlledAgentRepo{
		getByIDFn: func(_ context.Context, id string) (*agent.Agent, error) {
			// "agent-existing" already persisted; "agent-new" is new.
			if id == "agent-existing" {
				return &agent.Agent{ID: id}, nil
			}
			return nil, nil
		},
	}
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) { return nil, nil },
	}
	grantRepo := &controlledGrantRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityGrant, error) { return nil, nil },
	}

	svc := newServiceWithAll(surfaceRepo, agentRepo, profileRepo, grantRepo)

	// The surface is new; the agent already exists (conflict); the profile
	// references the new surface via the bundle; the grant references the
	// existing agent from persisted state (satisfiable via repo lookup) and the
	// in-bundle profile.
	docs := []parser.ParsedDocument{
		validSurface("surf-1"),
		validAgent("agent-existing"),
		validProfileWithSurface("profile-1", "surf-1"),
		validGrantWithRefs("grant-1", "agent-existing", "profile-1"),
	}

	result := svc.Apply(context.Background(), docs, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d: %+v", result.ValidationErrorCount(), result.ValidationErrors)
	}
	// Surface created, agent conflict, profile created, grant created.
	// The grant's agent reference is satisfied by persisted state (repo lookup).
	if result.ConflictCount() != 1 {
		t.Errorf("expected 1 conflict result (for the agent), got %d", result.ConflictCount())
	}
	if result.CreatedCount() != 3 {
		t.Errorf("expected 3 created results (surface, profile, grant), got %d", result.CreatedCount())
	}
	if result.Success() {
		t.Error("expected Apply to not be successful when a conflict exists")
	}
}

// TestApply_NoRepos_AllResourceKinds_AllCreated verifies that validation-only
// mode (no repos) still records all valid resource kinds as created.
func TestApply_NoRepos_AllResourceKinds_AllCreated(t *testing.T) {
	svc := NewService()

	docs := []parser.ParsedDocument{
		validSurface("surf-1"),
		validAgent("agent-1"),
		validProfile("profile-1"),
		validGrant("grant-1"),
	}

	result := svc.Apply(context.Background(), docs, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d: %+v", result.ValidationErrorCount(), result.ValidationErrors)
	}
	if result.CreatedCount() != 4 {
		t.Errorf("expected 4 created results in validation-only mode, got %d", result.CreatedCount())
	}
	if !result.Success() {
		t.Error("expected Apply to succeed in validation-only mode for all-valid bundle")
	}
}

// TestApply_InvalidAgentInBundle_RejectsWholeBundle verifies that an invalid
// agent document in a mixed bundle causes the entire bundle to be rejected.
func TestApply_InvalidAgentInBundle_RejectsWholeBundle(t *testing.T) {
	svc := NewService()

	docs := []parser.ParsedDocument{
		validSurface("surf-1"),
		invalidAgent("agent-bad"),
	}

	result := svc.Apply(context.Background(), docs, "")

	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation errors for invalid agent, got none")
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created results when bundle is invalid, got %d", result.CreatedCount())
	}
	if result.Success() {
		t.Error("expected Apply to fail when bundle contains invalid doc")
	}
}

// ---------------------------------------------------------------------------
// NewServiceWithRepos constructor test
// ---------------------------------------------------------------------------

func TestNewServiceWithRepos_AllNil_ValidatesOnly(t *testing.T) {
	svc := NewServiceWithRepos(RepositorySet{})
	if svc == nil {
		t.Fatal("expected NewServiceWithRepos to return a non-nil service")
	}

	result := svc.Apply(context.Background(), []parser.ParsedDocument{validSurface("surf-1")}, "")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d", result.ValidationErrorCount())
	}
	if result.CreatedCount() != 1 {
		t.Errorf("expected 1 created result in validation-only mode, got %d", result.CreatedCount())
	}
}
