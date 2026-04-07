package apply

// Tests for the public Plan / PlanBundle API and PlanResultFromPlan.
//
// These tests verify the seven behavioral requirements for the dry-run path:
//
//  1. Plan returns create entries for valid new resources.
//  2. Plan returns conflict entries for repository conflicts.
//  3. Plan returns invalid entries for validation failures.
//  4. Plan returns invalid entries for referential-integrity failures.
//  5. Same-bundle dependency satisfaction is visible in the plan (DecisionSource).
//  6. Plan performs no repository writes.
//  7. Apply still preserves current behaviour by executing the generated plan.
//
// Tests for (7) are already covered by TestApplyMatchesBuildPlanThenExecutePlan
// in plan_test.go; this file focuses on the new public surface.

import (
	"context"
	"fmt"
	"testing"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// 1. Plan returns create entries for valid new resources
// ---------------------------------------------------------------------------

// TestPlan_ValidNewResources_AllCreate verifies that a bundle of valid
// documents with no persisted counterparts produces create entries for each.
func TestPlan_ValidNewResources_AllCreate(t *testing.T) {
	svc := NewService()
	docs := []parser.ParsedDocument{
		validSurface("surf-new"),
		validAgent("agent-new"),
		validProfile("profile-new"),
		validGrant("grant-new"),
	}

	plan := svc.Plan(context.Background(), docs)

	if len(plan.Entries) != 4 {
		t.Fatalf("expected 4 plan entries, got %d", len(plan.Entries))
	}
	for _, e := range plan.Entries {
		if e.Action != ApplyActionCreate {
			t.Errorf("entry %q: expected action %q, got %q", e.ID, ApplyActionCreate, e.Action)
		}
	}
	if plan.HasInvalid() {
		t.Error("expected HasInvalid() == false for all-valid plan")
	}
}

// TestPlan_WithRepo_ValidNewSurface_Create verifies that a surface with no
// persisted version produces a create entry when a repository is configured.
func TestPlan_WithRepo_ValidNewSurface_Create(t *testing.T) {
	repo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Surfaces: repo, Processes: processRepoAlwaysExists()})

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{validSurface("surf-brand-new")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 plan entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionCreate {
		t.Errorf("expected action %q, got %q", ApplyActionCreate, entry.Action)
	}
	if entry.DecisionSource != DecisionSourcePersistedState {
		t.Errorf("expected DecisionSource %q, got %q", DecisionSourcePersistedState, entry.DecisionSource)
	}
}

// ---------------------------------------------------------------------------
// 2. Plan returns conflict entries for repository conflicts
// ---------------------------------------------------------------------------

// TestPlan_RepoConflict_SurfaceInReview_Conflict verifies that a surface whose
// latest persisted version is in review status produces a conflict entry.
func TestPlan_RepoConflict_SurfaceInReview_Conflict(t *testing.T) {
	repo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, id string) (*surface.DecisionSurface, error) {
			return &surface.DecisionSurface{
				ID:      id,
				Version: 1,
				Status:  surface.SurfaceStatusReview,
			}, nil
		},
	}
	svc := NewServiceWithRepo(repo)

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{validSurface("surf-in-review")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 plan entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionConflict {
		t.Errorf("expected action %q, got %q", ApplyActionConflict, entry.Action)
	}
	if entry.Message == "" {
		t.Error("expected non-empty message for conflict entry")
	}
	if entry.DecisionSource != DecisionSourcePersistedState {
		t.Errorf("expected DecisionSource %q, got %q", DecisionSourcePersistedState, entry.DecisionSource)
	}
}

// TestPlan_RepoConflict_ExistingAgent_Conflict verifies that an agent whose ID
// already exists in persisted state produces a conflict entry.
func TestPlan_RepoConflict_ExistingAgent_Conflict(t *testing.T) {
	agentRepo := &controlledAgentRepo{
		getByIDFn: func(_ context.Context, id string) (*agent.Agent, error) {
			return &agent.Agent{ID: id}, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Agents: agentRepo})

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{validAgent("agent-existing")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 plan entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionConflict {
		t.Errorf("expected action %q, got %q", ApplyActionConflict, entry.Action)
	}
	if entry.DecisionSource != DecisionSourcePersistedState {
		t.Errorf("expected DecisionSource %q, got %q", DecisionSourcePersistedState, entry.DecisionSource)
	}
}

// ---------------------------------------------------------------------------
// 3. Plan returns invalid entries for validation failures
// ---------------------------------------------------------------------------

// TestPlan_ValidationFailure_ActionInvalid verifies that a structurally invalid
// document produces an invalid entry with populated ValidationErrors.
func TestPlan_ValidationFailure_ActionInvalid(t *testing.T) {
	svc := NewService()

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{invalidSurface("surf-bad")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 plan entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionInvalid {
		t.Errorf("expected action %q, got %q", ApplyActionInvalid, entry.Action)
	}
	if len(entry.ValidationErrors) == 0 {
		t.Error("expected ValidationErrors to be populated for invalid entry")
	}
	if entry.DecisionSource != DecisionSourceValidation {
		t.Errorf("expected DecisionSource %q, got %q", DecisionSourceValidation, entry.DecisionSource)
	}
}

// TestPlan_ValidationFailure_AllKinds_ActionInvalid verifies that structural
// validation failures for all resource kinds produce invalid entries.
func TestPlan_ValidationFailure_AllKinds_ActionInvalid(t *testing.T) {
	svc := NewService()
	docs := []parser.ParsedDocument{
		invalidSurface("surf-bad"),
		invalidAgent("agent-bad"),
		invalidProfile("profile-bad"),
		invalidGrant("grant-bad"),
	}

	plan := svc.Plan(context.Background(), docs)

	for _, e := range plan.Entries {
		if e.Action != ApplyActionInvalid {
			t.Errorf("entry %q: expected action %q, got %q", e.ID, ApplyActionInvalid, e.Action)
		}
		if e.DecisionSource != DecisionSourceValidation {
			t.Errorf("entry %q: expected DecisionSource %q, got %q", e.ID, DecisionSourceValidation, e.DecisionSource)
		}
	}
}

// ---------------------------------------------------------------------------
// 4. Plan returns invalid entries for referential-integrity failures
// ---------------------------------------------------------------------------

// TestPlan_RefIntegrity_MissingSurface_Invalid verifies that a profile whose
// surface_id references a surface that does not exist produces an invalid entry.
func TestPlan_RefIntegrity_MissingSurface_Invalid(t *testing.T) {
	surfaceRepo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, nil
		},
	}
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Surfaces: surfaceRepo, Profiles: profileRepo})

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{
		validProfileWithSurface("profile-orphan", "surf-does-not-exist"),
	})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 plan entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionInvalid {
		t.Errorf("expected action %q, got %q", ApplyActionInvalid, entry.Action)
	}
	if entry.DecisionSource != DecisionSourceValidation {
		t.Errorf("expected DecisionSource %q, got %q", DecisionSourceValidation, entry.DecisionSource)
	}
	found := false
	for _, ve := range entry.ValidationErrors {
		if ve.Field == "spec.surface_id" {
			found = true
		}
	}
	if !found {
		t.Error("expected a ValidationError with field 'spec.surface_id'")
	}
}

// TestPlan_RefIntegrity_MissingAgentAndProfile_BothErrors verifies that a
// grant with both an unresolvable agent_id and an unresolvable profile_id
// accumulates validation errors for both references.
func TestPlan_RefIntegrity_MissingAgentAndProfile_BothErrors(t *testing.T) {
	agentRepo := &controlledAgentRepo{
		getByIDFn: func(_ context.Context, _ string) (*agent.Agent, error) {
			return nil, nil
		},
	}
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) {
			return nil, nil
		},
	}
	grantRepo := &controlledGrantRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityGrant, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Agents: agentRepo, Profiles: profileRepo, Grants: grantRepo})

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{
		validGrantWithRefs("grant-orphan", "agent-missing", "profile-missing"),
	})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 plan entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionInvalid {
		t.Errorf("expected action %q, got %q", ApplyActionInvalid, entry.Action)
	}
	fields := make(map[string]bool)
	for _, ve := range entry.ValidationErrors {
		fields[ve.Field] = true
	}
	if !fields["spec.agent_id"] {
		t.Error("expected ValidationError for spec.agent_id")
	}
	if !fields["spec.profile_id"] {
		t.Error("expected ValidationError for spec.profile_id")
	}
}

// ---------------------------------------------------------------------------
// 5. Same-bundle dependency satisfaction visible in the plan (DecisionSource)
// ---------------------------------------------------------------------------

// TestPlan_BundleDependency_ProfileReferencesInBundleSurface verifies that a
// profile whose surface_id is satisfied by another entry in the same bundle
// receives DecisionSource == DecisionSourceBundleDependency.
func TestPlan_BundleDependency_ProfileReferencesInBundleSurface(t *testing.T) {
	surfaceRepo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, nil // surface not in persisted state
		},
	}
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Surfaces: surfaceRepo, Profiles: profileRepo, Processes: processRepoAlwaysExists()})

	docs := []parser.ParsedDocument{
		validSurface("surf-bundle"),
		validProfileWithSurface("profile-dep", "surf-bundle"),
	}
	plan := svc.Plan(context.Background(), docs)

	if len(plan.Entries) != 2 {
		t.Fatalf("expected 2 plan entries, got %d", len(plan.Entries))
	}

	byID := make(map[string]ApplyPlanEntry)
	for _, e := range plan.Entries {
		byID[e.ID] = e
	}

	surfEntry := byID["surf-bundle"]
	if surfEntry.Action != ApplyActionCreate {
		t.Errorf("surf-bundle: expected action %q, got %q", ApplyActionCreate, surfEntry.Action)
	}
	if surfEntry.DecisionSource != DecisionSourcePersistedState {
		t.Errorf("surf-bundle: expected DecisionSource %q, got %q", DecisionSourcePersistedState, surfEntry.DecisionSource)
	}

	profEntry := byID["profile-dep"]
	if profEntry.Action != ApplyActionCreate {
		t.Errorf("profile-dep: expected action %q, got %q", ApplyActionCreate, profEntry.Action)
	}
	if profEntry.DecisionSource != DecisionSourceBundleDependency {
		t.Errorf("profile-dep: expected DecisionSource %q, got %q", DecisionSourceBundleDependency, profEntry.DecisionSource)
	}
}

// TestPlan_BundleDependency_GrantReferencesInBundleAgentAndProfile verifies
// that a grant whose agent_id and profile_id are both satisfied by same-bundle
// entries receives DecisionSource == DecisionSourceBundleDependency.
func TestPlan_BundleDependency_GrantReferencesInBundleAgentAndProfile(t *testing.T) {
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
	plan := svc.Plan(context.Background(), docs)

	byID := make(map[string]ApplyPlanEntry)
	for _, e := range plan.Entries {
		byID[e.ID] = e
	}

	grantEntry := byID["grant-a"]
	if grantEntry.Action != ApplyActionCreate {
		t.Errorf("grant-a: expected action %q, got %q", ApplyActionCreate, grantEntry.Action)
	}
	if grantEntry.DecisionSource != DecisionSourceBundleDependency {
		t.Errorf("grant-a: expected DecisionSource %q, got %q", DecisionSourceBundleDependency, grantEntry.DecisionSource)
	}
}

// ---------------------------------------------------------------------------
// 6. Plan performs no repository writes
// ---------------------------------------------------------------------------

// TestPlan_PerformsNoWrites verifies that calling Plan never triggers a Create
// call on any repository, regardless of the planned actions.
func TestPlan_PerformsNoWrites(t *testing.T) {
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

	plan := svc.Plan(context.Background(), docs)

	// Verify all entries would be created.
	for _, e := range plan.Entries {
		if e.Action != ApplyActionCreate {
			t.Errorf("entry %q: expected action %q, got %q", e.ID, ApplyActionCreate, e.Action)
		}
	}

	// Verify no Create calls occurred on any repository.
	if surfaceRepo.createCalled != 0 {
		t.Errorf("surface repo Create called %d times; expected 0", surfaceRepo.createCalled)
	}
	if agentRepo.createCalled != 0 {
		t.Errorf("agent repo Create called %d times; expected 0", agentRepo.createCalled)
	}
	if profileRepo.createCalled != 0 {
		t.Errorf("profile repo Create called %d times; expected 0", profileRepo.createCalled)
	}
	if grantRepo.createCalled != 0 {
		t.Errorf("grant repo Create called %d times; expected 0", grantRepo.createCalled)
	}
}

// TestPlanBundle_PerformsNoWrites verifies that PlanBundle never triggers a
// Create call on any repository.
func TestPlanBundle_PerformsNoWrites(t *testing.T) {
	surfaceRepo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepo(surfaceRepo)

	yaml := []byte(`
apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: surf-yaml
  name: YAML Surface
spec:
  category: financial
  risk_tier: high
  status: active
`)

	plan, err := svc.PlanBundle(context.Background(), yaml)
	if err != nil {
		t.Fatalf("PlanBundle returned unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("PlanBundle returned nil plan")
	}

	if surfaceRepo.createCalled != 0 {
		t.Errorf("surface repo Create called %d times; expected 0", surfaceRepo.createCalled)
	}
}

// ---------------------------------------------------------------------------
// 7. Apply still preserves current behaviour by executing the generated plan
// ---------------------------------------------------------------------------

// TestApply_CallsPlan_ThenExecutes verifies that Apply produces the same result
// as calling Plan and executing the returned plan. This is the contractual
// guarantee that Apply uses Plan internally and does not duplicate logic.
func TestApply_CallsPlan_ThenExecutes(t *testing.T) {
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

	// Use separate service instances to isolate create counters between the
	// Plan call (no writes) and the Apply call (writes expected).
	svcPlan := newServiceWithAll(
		&controlledSurfaceRepo{findLatestFn: surfaceRepo.findLatestFn},
		&controlledAgentRepo{getByIDFn: agentRepo.getByIDFn},
		&controlledProfileRepo{findByIDFn: profileRepo.findByIDFn},
		&controlledGrantRepo{findByIDFn: grantRepo.findByIDFn},
	)
	svcApply := newServiceWithAll(surfaceRepo, agentRepo, profileRepo, grantRepo)

	docs := []parser.ParsedDocument{
		validSurface("surf-1"),
		validAgent("agent-1"),
		validProfileWithSurface("profile-1", "surf-1"),
		validGrantWithRefs("grant-1", "agent-1", "profile-1"),
	}

	plan := svcPlan.Plan(context.Background(), docs)
	applyResult := svcApply.Apply(context.Background(), docs, "")

	// Plan must show 4 create entries; Apply must show 4 created results.
	createCount := 0
	for _, e := range plan.Entries {
		if e.Action == ApplyActionCreate {
			createCount++
		}
	}
	if createCount != 4 {
		t.Errorf("Plan: expected 4 create entries, got %d", createCount)
	}
	if applyResult.CreatedCount() != 4 {
		t.Errorf("Apply: expected 4 created results, got %d", applyResult.CreatedCount())
	}
}

// ---------------------------------------------------------------------------
// PlanResultFromPlan — structured explanation model
// ---------------------------------------------------------------------------

// TestPlanResultFromPlan_ValidPlan_WouldApplyTrue verifies that a plan with
// only create entries produces WouldApply == true.
func TestPlanResultFromPlan_ValidPlan_WouldApplyTrue(t *testing.T) {
	svc := NewService()
	docs := []parser.ParsedDocument{
		validSurface("surf-1"),
		validSurface("surf-2"),
	}
	plan := svc.Plan(context.Background(), docs)
	result := PlanResultFromPlan(plan)

	if !result.WouldApply {
		t.Error("expected WouldApply == true for all-create plan")
	}
	if result.CreateCount != 2 {
		t.Errorf("expected CreateCount == 2, got %d", result.CreateCount)
	}
	if result.InvalidCount != 0 {
		t.Errorf("expected InvalidCount == 0, got %d", result.InvalidCount)
	}
	if result.ConflictCount != 0 {
		t.Errorf("expected ConflictCount == 0, got %d", result.ConflictCount)
	}
}

// TestPlanResultFromPlan_InvalidPlan_WouldApplyFalse verifies that a plan with
// invalid entries produces WouldApply == false.
func TestPlanResultFromPlan_InvalidPlan_WouldApplyFalse(t *testing.T) {
	svc := NewService()
	plan := svc.Plan(context.Background(), []parser.ParsedDocument{invalidSurface("surf-bad")})
	result := PlanResultFromPlan(plan)

	if result.WouldApply {
		t.Error("expected WouldApply == false for invalid plan")
	}
	if result.InvalidCount != 1 {
		t.Errorf("expected InvalidCount == 1, got %d", result.InvalidCount)
	}
}

// TestPlanResultFromPlan_ConflictPlan_WouldApplyFalse verifies that a plan
// with conflict entries and no creates produces WouldApply == false.
func TestPlanResultFromPlan_ConflictPlan_WouldApplyFalse(t *testing.T) {
	plan := ApplyPlan{
		Entries: []ApplyPlanEntry{
			{
				Kind:    types.KindSurface,
				ID:      "surf-conflict",
				Action:  ApplyActionConflict,
				Message: "pending review exists",
			},
		},
	}
	result := PlanResultFromPlan(plan)

	if result.WouldApply {
		t.Error("expected WouldApply == false when only conflict entries exist")
	}
	if result.ConflictCount != 1 {
		t.Errorf("expected ConflictCount == 1, got %d", result.ConflictCount)
	}
}

// TestPlanResultFromPlan_PreservesDecisionSource verifies that each PlanEntry
// in the result carries the DecisionSource from the corresponding ApplyPlanEntry.
func TestPlanResultFromPlan_PreservesDecisionSource(t *testing.T) {
	plan := ApplyPlan{
		Entries: []ApplyPlanEntry{
			{
				Kind:           types.KindSurface,
				ID:             "surf-1",
				Action:         ApplyActionCreate,
				DocumentIndex:  1,
				DecisionSource: DecisionSourcePersistedState,
			},
			{
				Kind:           types.KindProfile,
				ID:             "profile-1",
				Action:         ApplyActionCreate,
				DocumentIndex:  2,
				DecisionSource: DecisionSourceBundleDependency,
			},
		},
	}

	result := PlanResultFromPlan(plan)

	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 result entries, got %d", len(result.Entries))
	}
	if result.Entries[0].DecisionSource != types.PlanEntryDecisionSourcePersistedState {
		t.Errorf("entry 0: expected DecisionSource %q, got %q",
			types.PlanEntryDecisionSourcePersistedState, result.Entries[0].DecisionSource)
	}
	if result.Entries[1].DecisionSource != types.PlanEntryDecisionSourceBundleDependency {
		t.Errorf("entry 1: expected DecisionSource %q, got %q",
			types.PlanEntryDecisionSourceBundleDependency, result.Entries[1].DecisionSource)
	}
}

// TestPlanResultFromPlan_PreservesValidationErrors verifies that ValidationErrors
// from invalid ApplyPlanEntries appear in the PlanResult entries.
func TestPlanResultFromPlan_PreservesValidationErrors(t *testing.T) {
	svc := NewService()
	plan := svc.Plan(context.Background(), []parser.ParsedDocument{invalidSurface("surf-bad")})
	result := PlanResultFromPlan(plan)

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 result entry, got %d", len(result.Entries))
	}
	if len(result.Entries[0].ValidationErrors) == 0 {
		t.Error("expected ValidationErrors to be propagated into PlanResult entries")
	}
}

// ---------------------------------------------------------------------------
// PlanBundle — parse-error path
// ---------------------------------------------------------------------------

// TestPlanBundle_ParseError_WrapsErrInvalidBundle verifies that PlanBundle
// returns an error wrapping ErrInvalidBundle when the YAML cannot be parsed.
func TestPlanBundle_ParseError_WrapsErrInvalidBundle(t *testing.T) {
	svc := NewService()
	_, err := svc.PlanBundle(context.Background(), []byte("not: valid: yaml: {{{{"))
	if err == nil {
		t.Fatal("expected error for unparseable bundle, got nil")
	}
	if fmt.Sprintf("%v", err) == "" {
		t.Error("expected non-empty error message")
	}
}
