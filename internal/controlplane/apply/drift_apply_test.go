package apply_test

// drift_apply_test.go — apply-pipeline integration tests for the
// DriftDefinition Kind (Drift-1c). Validation, mapping, planning, and
// apply-time persistence are exercised end-to-end against the memory
// repository.
//
// Runtime-inert pins: applying a DriftDefinition must NOT create
// DriftSeries, DriftSeriesPoints, or DriftObservations. The bottom of
// this file pins those invariants.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/controlplane/validate"
	"github.com/accept-io/midas/internal/drift"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/memory"
)

// driftApplyService builds an apply.Service wired with the full memory
// RepositorySet. Mirrors fmpApplyService — every repo is wired so the
// executor's validation-only fallback short-circuit does not fire.
func driftApplyService(t *testing.T) (*apply.Service, *store.Repositories) {
	t.Helper()
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Surfaces:                     repos.Surfaces,
		Agents:                       repos.Agents,
		Profiles:                     repos.Profiles,
		Grants:                       repos.Grants,
		Processes:                    repos.Processes,
		Capabilities:                 repos.Capabilities,
		BusinessServices:             repos.BusinessServices,
		BusinessServiceCapabilities:  repos.BusinessServiceCapabilities,
		BusinessServiceRelationships: repos.BusinessServiceRelationships,
		GovernanceExpectations:       repos.GovernanceExpectations,
		AISystems:                    repos.AISystems,
		AISystemVersions:             repos.AISystemVersions,
		AISystemBindings:             repos.AISystemBindings,
		FailModePolicies:             repos.FailModePolicies,
		DriftDefinitions:             repos.DriftDefinitions,
	})
	return svc, repos
}

// makeValidDriftDoc returns a parsed DriftDefinition document
// satisfying every Drift-1c validator constraint. Tests mutate fields
// per scenario. The target kind is decision_surface; surface
// referential checks degrade to structural-only when the apply Service
// has no surface repo wired and the surface ID does not need to
// exist for the test scenario. To avoid referential failures we point
// the target at an existing seeded entity below.
func makeValidDriftDoc(id string) parser.ParsedDocument {
	doc := types.DriftDefinitionDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindDriftDefinition,
		Metadata:   types.DocumentMetadata{ID: id},
		Spec: types.DriftDefinitionSpec{
			Name:           "Drift " + id,
			BusinessOwner:  "risk-governance",
			TechnicalOwner: "platform-governance",
			Target: types.DriftTargetSpec{
				Kind: "ai_system",
				ID:   "non-existent-ai-system-but-not-needed",
			},
			Metrics: []types.DriftMetricSpec{
				{
					MetricID:           "outcome-psi",
					DriftType:          "outcome",
					BaselineStrategy:   "fixed_governed",
					WindowSeconds:      3600,
					Cadence:            "hour",
					ThresholdDirection: "ascending",
					WarningThreshold:   0.10,
					BreachedThreshold:  0.20,
				},
			},
		},
	}
	return parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}
}

func driftCountCreated(res types.ApplyResult, kind string) int {
	n := 0
	for _, r := range res.Results {
		if r.Kind == kind && r.Status == types.ResourceStatusCreated {
			n++
		}
	}
	return n
}

func TestApply_DriftDefinition_CreatesReviewRow(t *testing.T) {
	svc, repos := driftApplyService(t)
	ctx := context.Background()

	// Seed a target ai_system so the referential check does not reject.
	seedAISystemForTarget(t, ctx, repos, "non-existent-ai-system-but-not-needed")

	doc := makeValidDriftDoc("approve-rate-drift")
	if errs := validate.ValidateDocument(doc); len(errs) != 0 {
		t.Fatalf("validation should pass; got %+v", errs)
	}

	res := svc.Apply(ctx, []parser.ParsedDocument{doc}, "alice")
	if got := driftCountCreated(res, types.KindDriftDefinition); got != 1 {
		t.Fatalf("Created DriftDefinition: want 1, got %d (full result: %+v)", got, res.Results)
	}

	got, err := repos.DriftDefinitions.FindByID(ctx, "approve-rate-drift")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil {
		t.Fatal("persisted DriftDefinition missing")
	}
	if got.Status != drift.DriftDefinitionStatusReview {
		t.Errorf("Status: want review (forced), got %q", got.Status)
	}
	if got.Version != 1 {
		t.Errorf("Version: want 1, got %d", got.Version)
	}
	if len(got.Metrics) != 1 {
		t.Errorf("Metrics: want 1, got %d", len(got.Metrics))
	}
}

func TestApply_DriftDefinition_AppendsNewVersionOnReapply(t *testing.T) {
	svc, repos := driftApplyService(t)
	ctx := context.Background()
	seedAISystemForTarget(t, ctx, repos, "non-existent-ai-system-but-not-needed")

	_ = svc.Apply(ctx, []parser.ParsedDocument{makeValidDriftDoc("approve-rate-drift")}, "alice")

	res := svc.Apply(ctx, []parser.ParsedDocument{makeValidDriftDoc("approve-rate-drift")}, "alice")
	if got := driftCountCreated(res, types.KindDriftDefinition); got != 1 {
		t.Errorf("re-apply Created: want 1, got %d", got)
	}

	versions, err := repos.DriftDefinitions.ListVersions(ctx, "approve-rate-drift")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Errorf("ListVersions: want 2, got %d", len(versions))
	}
	if versions[0].Version != 2 {
		t.Errorf("latest version: want 2, got %d", versions[0].Version)
	}
}

func TestApply_DriftDefinition_PlanReportsNewVersionForExistingID(t *testing.T) {
	svc, repos := driftApplyService(t)
	ctx := context.Background()
	seedAISystemForTarget(t, ctx, repos, "non-existent-ai-system-but-not-needed")

	_ = svc.Apply(ctx, []parser.ParsedDocument{makeValidDriftDoc("approve-rate-drift")}, "alice")

	plan := svc.Plan(ctx, []parser.ParsedDocument{makeValidDriftDoc("approve-rate-drift")})
	if len(plan.Entries) != 1 {
		t.Fatalf("Plan.Entries: want 1, got %d", len(plan.Entries))
	}
	e := plan.Entries[0]
	if e.Action != apply.ApplyActionCreate {
		t.Errorf("Action: want Create, got %q", e.Action)
	}
	if e.CreateKind != apply.CreateKindNewVersion {
		t.Errorf("CreateKind: want NewVersion, got %q", e.CreateKind)
	}
	if e.NewVersion != 2 {
		t.Errorf("NewVersion: want 2, got %d", e.NewVersion)
	}
}

func TestApply_DriftDefinition_RejectsMetricMutationOnPinnedVersion(t *testing.T) {
	svc, repos := driftApplyService(t)
	ctx := context.Background()
	seedAISystemForTarget(t, ctx, repos, "non-existent-ai-system-but-not-needed")

	// Persist version 1 with the original metric set.
	_ = svc.Apply(ctx, []parser.ParsedDocument{makeValidDriftDoc("approve-rate-drift")}, "alice")

	// Re-apply with a mutated metric (still threshold-coherent so the
	// document validator passes) AND lifecycle.version pointed at v1.
	// The planner's metric-immutability check is what should reject.
	mutated := makeValidDriftDoc("approve-rate-drift").Doc.(types.DriftDefinitionDocument)
	mutated.Spec.Metrics[0].WarningThreshold = 0.05
	mutated.Lifecycle.Version = 1
	parsedMutated := parser.ParsedDocument{Kind: mutated.Kind, ID: mutated.Metadata.ID, Doc: mutated}

	plan := svc.Plan(ctx, []parser.ParsedDocument{parsedMutated})
	if len(plan.Entries) != 1 {
		t.Fatalf("Plan.Entries: want 1, got %d", len(plan.Entries))
	}
	e := plan.Entries[0]
	if e.Action != apply.ApplyActionInvalid {
		t.Errorf("Action: want Invalid, got %q", e.Action)
	}
	var found bool
	for _, ve := range e.ValidationErrors {
		if strings.Contains(ve.Message, "DriftDefinition metrics are immutable within a revision; create a new version.") {
			found = true
			if !strings.Contains(ve.Message, "outcome-psi") {
				t.Errorf("error must include changed metric_id; got %q", ve.Message)
			}
			break
		}
	}
	if !found {
		t.Errorf("metric-immutability error missing; got %+v", e.ValidationErrors)
	}
}

func TestApply_DriftDefinition_NewVersionWithMetricChanges_Accepted(t *testing.T) {
	svc, repos := driftApplyService(t)
	ctx := context.Background()
	seedAISystemForTarget(t, ctx, repos, "non-existent-ai-system-but-not-needed")

	_ = svc.Apply(ctx, []parser.ParsedDocument{makeValidDriftDoc("approve-rate-drift")}, "alice")

	// Re-apply with a mutated metric and NO lifecycle.version pin.
	mutated := makeValidDriftDoc("approve-rate-drift").Doc.(types.DriftDefinitionDocument)
	mutated.Spec.Metrics[0].WarningThreshold = 0.05
	parsedMutated := parser.ParsedDocument{Kind: mutated.Kind, ID: mutated.Metadata.ID, Doc: mutated}

	res := svc.Apply(ctx, []parser.ParsedDocument{parsedMutated}, "alice")
	if got := driftCountCreated(res, types.KindDriftDefinition); got != 1 {
		t.Errorf("Created: want 1, got %d", got)
	}
	versions, _ := repos.DriftDefinitions.ListVersions(ctx, "approve-rate-drift")
	if len(versions) != 2 {
		t.Errorf("ListVersions: want 2, got %d", len(versions))
	}
	if versions[0].Metrics[0].WarningThreshold != 0.05 {
		t.Errorf("latest revision metrics not the mutated value; got %v", versions[0].Metrics[0].WarningThreshold)
	}
}

// Runtime-inert pin: applying a DriftDefinition must NOT cause any
// DriftSeries, DriftSeriesPoint, or DriftObservation to be created.
// Detection / aggregation / observation emission are scoped to later
// tranches (Drift-3a/b/c).
func TestApply_DriftDefinition_RuntimeInert(t *testing.T) {
	svc, repos := driftApplyService(t)
	ctx := context.Background()
	seedAISystemForTarget(t, ctx, repos, "non-existent-ai-system-but-not-needed")

	_ = svc.Apply(ctx, []parser.ParsedDocument{makeValidDriftDoc("approve-rate-drift")}, "alice")

	// Series, points, and observations must remain absent.
	bySeries, err := repos.DriftSeries.ListByDefinition(ctx, "approve-rate-drift")
	if err != nil {
		t.Fatalf("ListByDefinition: %v", err)
	}
	if len(bySeries) != 0 {
		t.Errorf("DriftSeries created by apply (must be runtime-inert); got %d", len(bySeries))
	}
	byObs, err := repos.DriftObservations.ListByDefinition(ctx, "approve-rate-drift")
	if err != nil {
		t.Fatalf("Observations ListByDefinition: %v", err)
	}
	if len(byObs) != 0 {
		t.Errorf("DriftObservation created by apply; got %d", len(byObs))
	}
}

// seedAISystemForTarget inserts a minimal AISystem directly via the
// store repository so the referential check on a target.kind ==
// "ai_system" passes without going through the AISystem apply
// pipeline. The drift apply pipeline's AISystemRepository.Exists is
// the resolver used by drift_ref_check.
func seedAISystemForTarget(t *testing.T, ctx context.Context, repos *store.Repositories, id string) {
	t.Helper()
	if repos.AISystems == nil {
		return
	}
	now := time.Now().UTC()
	if err := repos.AISystems.Create(ctx, &aisystem.AISystem{
		ID:         id,
		Name:       "Seed AI " + id,
		Owner:      "platform",
		Vendor:     "internal",
		SystemType: "internal",
		Status:     "active",
		Origin:     "manual",
		Managed:    true,
		CreatedAt:  now,
		UpdatedAt:  now,
		CreatedBy:  "seed",
	}); err != nil {
		t.Fatalf("seedAISystemForTarget Create: %v", err)
	}
}
