package apply_test

// drift_ref_test.go — apply-time referential-resolution tests for the
// DriftDefinition Kind. Covers:
//
//   - target referential check rejects unknown target IDs when the
//     relevant repo is wired
//   - target referential check degrades to structural-only when the
//     relevant repo is unavailable
//   - governance_expectation_ref existence-check rejects unknown
//     references when the GovernanceExpectation repo is wired

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/governanceexpectation"
	"github.com/accept-io/midas/internal/store/memory"
)

func TestApply_DriftDefinition_RejectsUnknownTargetID_WhenRepoPresent(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		AISystems:        repos.AISystems,
		DriftDefinitions: repos.DriftDefinitions,
	})

	doc := makeValidDriftDoc("approve-rate-drift").Doc.(types.DriftDefinitionDocument)
	doc.Spec.Target.Kind = "ai_system"
	doc.Spec.Target.ID = "missing-system"
	parsed := parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{parsed})
	if len(plan.Entries) != 1 {
		t.Fatalf("plan entries = %d, want 1", len(plan.Entries))
	}
	e := plan.Entries[0]
	if e.Action != apply.ApplyActionInvalid {
		t.Fatalf("Action: want Invalid (target rejected), got %q", e.Action)
	}
	var found bool
	for _, ve := range e.ValidationErrors {
		if strings.Contains(ve.Message, "missing-system") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("missing target ID not named in validation error; got %+v", e.ValidationErrors)
	}
}

func TestApply_DriftDefinition_DegradesWhenTargetRepoMissing(t *testing.T) {
	repos := memory.NewRepositories()
	// Deliberately build the apply Service WITHOUT the AISystems repo
	// so target referential check should degrade to structural-only.
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		DriftDefinitions: repos.DriftDefinitions,
	})

	doc := makeValidDriftDoc("approve-rate-drift").Doc.(types.DriftDefinitionDocument)
	doc.Spec.Target.Kind = "ai_system"
	doc.Spec.Target.ID = "missing-system"
	parsed := parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{parsed})
	if len(plan.Entries) != 1 {
		t.Fatalf("plan entries = %d, want 1", len(plan.Entries))
	}
	e := plan.Entries[0]
	if e.Action == apply.ApplyActionInvalid {
		t.Errorf("missing-repo case must degrade to structural-only; got Invalid: %+v", e.ValidationErrors)
	}
}

func TestApply_DriftDefinition_RejectsUnknownGovernanceExpectationRef(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		AISystems:              repos.AISystems,
		GovernanceExpectations: repos.GovernanceExpectations,
		DriftDefinitions:       repos.DriftDefinitions,
	})

	// Seed a target so the target check passes.
	seedAISystemForTarget(t, context.Background(), repos, "ai-target")

	doc := makeValidDriftDoc("approve-rate-drift").Doc.(types.DriftDefinitionDocument)
	doc.Spec.Target.Kind = "ai_system"
	doc.Spec.Target.ID = "ai-target"
	doc.Spec.Metrics[0].GovernanceExpectationRef = "missing-expectation"
	parsed := parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{parsed})
	if len(plan.Entries) != 1 {
		t.Fatalf("plan entries = %d, want 1", len(plan.Entries))
	}
	e := plan.Entries[0]
	if e.Action != apply.ApplyActionInvalid {
		t.Fatalf("Action: want Invalid (unknown governance_expectation_ref), got %q", e.Action)
	}
	var found bool
	for _, ve := range e.ValidationErrors {
		if strings.Contains(ve.Message, "missing-expectation") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("unknown governance_expectation_ref not surfaced; got %+v", e.ValidationErrors)
	}
}

func TestApply_DriftDefinition_AcceptsKnownGovernanceExpectationRef(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		AISystems:              repos.AISystems,
		GovernanceExpectations: repos.GovernanceExpectations,
		DriftDefinitions:       repos.DriftDefinitions,
	})
	ctx := context.Background()
	seedAISystemForTarget(t, ctx, repos, "ai-target")

	// Seed an expectation.
	now := time.Now().UTC()
	if err := repos.GovernanceExpectations.Create(ctx, &governanceexpectation.GovernanceExpectation{
		ID:                "exp-x",
		Version:           1,
		ScopeKind:         governanceexpectation.ScopeKindProcess,
		ScopeID:           "proc-x",
		RequiredSurfaceID: "surf-x",
		Name:              "Expectation X",
		Status:            governanceexpectation.ExpectationStatusActive,
		EffectiveDate:     now,
		BusinessOwner:     "owner",
		TechnicalOwner:    "owner",
		ConditionType:     governanceexpectation.ConditionTypeRiskCondition,
		ConditionPayload:  []byte(`{}`),
		CreatedAt:         now,
		UpdatedAt:         now,
		CreatedBy:         "seed",
	}); err != nil {
		t.Fatalf("seed expectation: %v", err)
	}

	doc := makeValidDriftDoc("approve-rate-drift").Doc.(types.DriftDefinitionDocument)
	doc.Spec.Target.Kind = "ai_system"
	doc.Spec.Target.ID = "ai-target"
	doc.Spec.Metrics[0].GovernanceExpectationRef = "exp-x"
	parsed := parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}

	plan := svc.Plan(ctx, []parser.ParsedDocument{parsed})
	if len(plan.Entries) != 1 {
		t.Fatalf("plan entries = %d, want 1", len(plan.Entries))
	}
	e := plan.Entries[0]
	if e.Action != apply.ApplyActionCreate {
		t.Errorf("Action: want Create, got %q (errs: %+v)", e.Action, e.ValidationErrors)
	}
}
