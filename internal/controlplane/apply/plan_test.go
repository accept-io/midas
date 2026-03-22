package apply

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// Typed error tests
// ---------------------------------------------------------------------------

func TestErrInvalidBundle_IsCheckable(t *testing.T) {
	wrapped := errors.New("invalid bundle: bad yaml")
	if !errors.Is(ErrInvalidBundle, ErrInvalidBundle) {
		t.Fatal("ErrInvalidBundle must satisfy errors.Is with itself")
	}
	_ = wrapped
}

func TestApplyBundle_ParseError_WrapsErrInvalidBundle(t *testing.T) {
	svc := NewService()
	_, err := svc.ApplyBundle(context.Background(), []byte("not: valid: yaml: {{{{"), "")
	if err == nil {
		t.Fatal("expected error for unparseable bundle, got nil")
	}
	if !errors.Is(err, ErrInvalidBundle) {
		t.Errorf("expected error to wrap ErrInvalidBundle, got: %v", err)
	}
}

func TestApplyBundle_EmptyBundle_WrapsErrInvalidBundle(t *testing.T) {
	svc := NewService()
	_, err := svc.ApplyBundle(context.Background(), []byte("---"), "")
	if err == nil {
		t.Fatal("expected error for empty bundle, got nil")
	}
	if !errors.Is(err, ErrInvalidBundle) {
		t.Errorf("expected error to wrap ErrInvalidBundle, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ApplyPlan construction tests
// ---------------------------------------------------------------------------

func validSurface(id string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindSurface,
		ID:   id,
		Doc: types.SurfaceDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindSurface,
			Metadata: types.DocumentMetadata{
				ID:   id,
				Name: "Valid Surface",
			},
			Spec: types.SurfaceSpec{
				Category: "financial",
				RiskTier: "high",
				Status:   "active",
			},
		},
	}
}

func invalidSurface(id string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindSurface,
		ID:   id,
		Doc: types.SurfaceDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindSurface,
			Metadata: types.DocumentMetadata{
				ID:   id,
				Name: "", // required — triggers validation failure
			},
			Spec: types.SurfaceSpec{
				Category: "",  // required — triggers validation failure
				RiskTier: "high",
				Status:   "active",
			},
		},
	}
}

func TestBuildApplyPlan_ValidDocs_AllActionCreate(t *testing.T) {
	svc := NewService()
	docs := []parser.ParsedDocument{
		validSurface("surf-1"),
		validSurface("surf-2"),
	}

	plan := svc.buildApplyPlan(context.Background(), docs)

	if len(plan.Entries) != 2 {
		t.Fatalf("expected 2 plan entries, got %d", len(plan.Entries))
	}
	for _, entry := range plan.Entries {
		if entry.Action != ApplyActionCreate {
			t.Errorf("expected action %q for %q, got %q", ApplyActionCreate, entry.ID, entry.Action)
		}
		if len(entry.ValidationErrors) != 0 {
			t.Errorf("expected no validation errors for %q, got %d", entry.ID, len(entry.ValidationErrors))
		}
	}

	if plan.HasInvalid() {
		t.Error("expected HasInvalid() == false for all-valid plan")
	}
}

func TestBuildApplyPlan_InvalidDoc_ActionInvalid(t *testing.T) {
	svc := NewService()
	docs := []parser.ParsedDocument{
		invalidSurface("surf-bad"),
	}

	plan := svc.buildApplyPlan(context.Background(), docs)

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 plan entry, got %d", len(plan.Entries))
	}

	entry := plan.Entries[0]
	if entry.Action != ApplyActionInvalid {
		t.Errorf("expected action %q, got %q", ApplyActionInvalid, entry.Action)
	}
	if len(entry.ValidationErrors) == 0 {
		t.Error("expected validation errors on invalid entry, got none")
	}
	if !plan.HasInvalid() {
		t.Error("expected HasInvalid() == true")
	}
}

func TestBuildApplyPlan_MixedDocs_CorrectActions(t *testing.T) {
	svc := NewService()
	docs := []parser.ParsedDocument{
		validSurface("surf-ok"),
		invalidSurface("surf-bad"),
	}

	plan := svc.buildApplyPlan(context.Background(), docs)

	if len(plan.Entries) != 2 {
		t.Fatalf("expected 2 plan entries, got %d", len(plan.Entries))
	}

	byID := make(map[string]ApplyPlanEntry)
	for _, e := range plan.Entries {
		byID[e.ID] = e
	}

	if byID["surf-ok"].Action != ApplyActionCreate {
		t.Errorf("expected surf-ok to have action %q, got %q", ApplyActionCreate, byID["surf-ok"].Action)
	}
	if byID["surf-bad"].Action != ApplyActionInvalid {
		t.Errorf("expected surf-bad to have action %q, got %q", ApplyActionInvalid, byID["surf-bad"].Action)
	}
}

func TestBuildApplyPlan_DocumentIndexIsOneBased(t *testing.T) {
	svc := NewService()
	docs := []parser.ParsedDocument{
		validSurface("surf-a"),
		validSurface("surf-b"),
	}

	plan := svc.buildApplyPlan(context.Background(), docs)

	for i, entry := range plan.Entries {
		expected := i + 1
		if entry.DocumentIndex != expected {
			t.Errorf("entry %d: expected DocumentIndex %d, got %d", i, expected, entry.DocumentIndex)
		}
	}
}

func TestBuildApplyPlan_ValidationErrors_PreserveFieldInfo(t *testing.T) {
	svc := NewService()
	docs := []parser.ParsedDocument{invalidSurface("surf-bad")}

	plan := svc.buildApplyPlan(context.Background(), docs)

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}

	entry := plan.Entries[0]
	fields := make(map[string]bool)
	for _, ve := range entry.ValidationErrors {
		fields[ve.Field] = true
	}

	if !fields["metadata.name"] {
		t.Error("expected validation error with Field='metadata.name', not found")
	}
	if !fields["spec.category"] {
		t.Error("expected validation error with Field='spec.category', not found")
	}
}

// ---------------------------------------------------------------------------
// Planner/executor split: observable results match Apply
// ---------------------------------------------------------------------------

func TestExecutePlan_ValidPlan_ProducesCreatedResults(t *testing.T) {
	svc := NewService()
	docs := []parser.ParsedDocument{
		validSurface("surf-1"),
		validSurface("surf-2"),
	}

	plan := svc.buildApplyPlan(context.Background(), docs)
	result := svc.executePlan(context.Background(), plan, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d", result.ValidationErrorCount())
	}
	if result.CreatedCount() != 2 {
		t.Errorf("expected 2 created results, got %d", result.CreatedCount())
	}
}

func TestExecutePlan_InvalidPlan_ReturnsValidationErrorsOnly(t *testing.T) {
	svc := NewService()
	docs := []parser.ParsedDocument{invalidSurface("surf-bad")}

	plan := svc.buildApplyPlan(context.Background(), docs)
	result := svc.executePlan(context.Background(), plan, "")

	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation errors, got none")
	}
	if result.TotalCount() != 0 {
		t.Fatalf("expected no resource results, got %d", result.TotalCount())
	}
}

func TestApplyMatchesBuildPlanThenExecutePlan(t *testing.T) {
	svc := NewService()
	docs := []parser.ParsedDocument{
		validSurface("surf-1"),
		validSurface("surf-2"),
	}

	directResult := svc.Apply(context.Background(), docs, "")

	plan := svc.buildApplyPlan(context.Background(), docs)
	splitResult := svc.executePlan(context.Background(), plan, "")

	if directResult.CreatedCount() != splitResult.CreatedCount() {
		t.Errorf("Apply() and buildApplyPlan+executePlan() disagree: created %d vs %d",
			directResult.CreatedCount(), splitResult.CreatedCount())
	}
	if directResult.ValidationErrorCount() != splitResult.ValidationErrorCount() {
		t.Errorf("Apply() and buildApplyPlan+executePlan() disagree: validation errors %d vs %d",
			directResult.ValidationErrorCount(), splitResult.ValidationErrorCount())
	}
}

// ---------------------------------------------------------------------------
// ApplyPlan helper method tests
// ---------------------------------------------------------------------------

func TestApplyPlan_HasInvalid(t *testing.T) {
	plan := ApplyPlan{}
	if plan.HasInvalid() {
		t.Error("expected HasInvalid() == false for empty plan")
	}

	plan.Entries = append(plan.Entries, ApplyPlanEntry{Action: ApplyActionCreate})
	if plan.HasInvalid() {
		t.Error("expected HasInvalid() == false when all entries are create")
	}

	plan.Entries = append(plan.Entries, ApplyPlanEntry{Action: ApplyActionInvalid})
	if !plan.HasInvalid() {
		t.Error("expected HasInvalid() == true after adding invalid entry")
	}
}

func TestApplyPlan_HasConflict(t *testing.T) {
	plan := ApplyPlan{}
	if plan.HasConflict() {
		t.Error("expected HasConflict() == false for empty plan")
	}

	plan.Entries = append(plan.Entries, ApplyPlanEntry{Action: ApplyActionCreate})
	if plan.HasConflict() {
		t.Error("expected HasConflict() == false when all entries are create")
	}

	plan.Entries = append(plan.Entries, ApplyPlanEntry{Action: ApplyActionConflict})
	if !plan.HasConflict() {
		t.Error("expected HasConflict() == true after adding conflict entry")
	}
}

// ---------------------------------------------------------------------------
// Repository-backed surface planning tests
// ---------------------------------------------------------------------------

// controlledSurfaceRepo is a minimal SurfaceRepository stub whose behavior is
// set per-test via function fields. Only the methods used by buildApplyPlan
// need implementations; all others panic to catch accidental calls.
type controlledSurfaceRepo struct {
	findLatestFn func(ctx context.Context, id string) (*surface.DecisionSurface, error)
	createCalled int
}

func (r *controlledSurfaceRepo) FindLatestByID(ctx context.Context, id string) (*surface.DecisionSurface, error) {
	return r.findLatestFn(ctx, id)
}

func (r *controlledSurfaceRepo) Create(ctx context.Context, s *surface.DecisionSurface) error {
	r.createCalled++
	return nil
}

func (r *controlledSurfaceRepo) Update(_ context.Context, _ *surface.DecisionSurface) error {
	panic("Update called unexpectedly in surface planner test")
}
func (r *controlledSurfaceRepo) FindByIDVersion(_ context.Context, _ string, _ int) (*surface.DecisionSurface, error) {
	panic("FindByIDVersion called unexpectedly in surface planner test")
}
func (r *controlledSurfaceRepo) FindActiveAt(_ context.Context, _ string, _ time.Time) (*surface.DecisionSurface, error) {
	panic("FindActiveAt called unexpectedly in surface planner test")
}
func (r *controlledSurfaceRepo) ListVersions(_ context.Context, _ string) ([]*surface.DecisionSurface, error) {
	panic("ListVersions called unexpectedly in surface planner test")
}
func (r *controlledSurfaceRepo) ListAll(_ context.Context) ([]*surface.DecisionSurface, error) {
	panic("ListAll called unexpectedly in surface planner test")
}
func (r *controlledSurfaceRepo) ListByStatus(_ context.Context, _ surface.SurfaceStatus) ([]*surface.DecisionSurface, error) {
	panic("ListByStatus called unexpectedly in surface planner test")
}
func (r *controlledSurfaceRepo) ListByDomain(_ context.Context, _ string) ([]*surface.DecisionSurface, error) {
	panic("ListByDomain called unexpectedly in surface planner test")
}
func (r *controlledSurfaceRepo) Search(_ context.Context, _ surface.SearchCriteria) ([]*surface.DecisionSurface, error) {
	panic("Search called unexpectedly in surface planner test")
}

// TestBuildApplyPlan_SurfaceNotFound_ActionCreate verifies that a surface with
// no persisted version is planned as create.
func TestBuildApplyPlan_SurfaceNotFound_ActionCreate(t *testing.T) {
	repo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, nil // no existing version
		},
	}
	svc := NewServiceWithRepo(repo)

	plan := svc.buildApplyPlan(context.Background(), []parser.ParsedDocument{validSurface("surf-new")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Action != ApplyActionCreate {
		t.Errorf("expected action %q for new surface, got %q", ApplyActionCreate, plan.Entries[0].Action)
	}
}

// TestBuildApplyPlan_SurfaceInReview_ActionConflict verifies that a surface
// whose latest persisted version is in review status is planned as a conflict.
// Applying again while a review is pending would create an ambiguous governance
// state.
func TestBuildApplyPlan_SurfaceInReview_ActionConflict(t *testing.T) {
	repo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return &surface.DecisionSurface{
				ID:      "surf-review",
				Version: 1,
				Status:  surface.SurfaceStatusReview,
			}, nil
		},
	}
	svc := NewServiceWithRepo(repo)

	plan := svc.buildApplyPlan(context.Background(), []parser.ParsedDocument{validSurface("surf-review")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionConflict {
		t.Errorf("expected action %q for review-status surface, got %q", ApplyActionConflict, entry.Action)
	}
	if entry.Message == "" {
		t.Error("expected non-empty conflict message")
	}
	if !plan.HasConflict() {
		t.Error("expected HasConflict() == true")
	}
}

// TestBuildApplyPlan_SurfaceActive_ActionCreate verifies that a surface whose
// latest persisted version is active is planned as create. The versioning model
// intends a new governed version in this case.
func TestBuildApplyPlan_SurfaceActive_ActionCreate(t *testing.T) {
	repo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return &surface.DecisionSurface{
				ID:      "surf-active",
				Version: 2,
				Status:  surface.SurfaceStatusActive,
			}, nil
		},
	}
	svc := NewServiceWithRepo(repo)

	plan := svc.buildApplyPlan(context.Background(), []parser.ParsedDocument{validSurface("surf-active")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Action != ApplyActionCreate {
		t.Errorf("expected action %q for active surface, got %q", ApplyActionCreate, plan.Entries[0].Action)
	}
}

// TestBuildApplyPlan_SurfaceRepoError_ActionInvalid verifies that a repository
// error during planning produces an invalid entry rather than proceeding with
// an incorrect action.
func TestBuildApplyPlan_SurfaceRepoError_ActionInvalid(t *testing.T) {
	repo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, fmt.Errorf("database unavailable")
		},
	}
	svc := NewServiceWithRepo(repo)

	plan := svc.buildApplyPlan(context.Background(), []parser.ParsedDocument{validSurface("surf-err")})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionInvalid {
		t.Errorf("expected action %q on repo error, got %q", ApplyActionInvalid, entry.Action)
	}
	if len(entry.ValidationErrors) == 0 {
		t.Error("expected validation errors to carry the repo error message")
	}
}

// ---------------------------------------------------------------------------
// Executor behavior for conflict and unchanged entries
// ---------------------------------------------------------------------------

// TestExecutePlan_ConflictEntry_NoCreateCall verifies that a conflict entry in
// the plan does not trigger any persistence call. The executor must record a
// conflict result and leave the repository untouched.
func TestExecutePlan_ConflictEntry_NoCreateCall(t *testing.T) {
	repo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepo(repo)

	// Build a plan manually with a conflict entry.
	plan := ApplyPlan{
		Entries: []ApplyPlanEntry{
			{
				Kind:    types.KindSurface,
				ID:      "surf-conflict",
				Action:  ApplyActionConflict,
				Message: "pending review exists",
				Doc:     validSurface("surf-conflict"),
			},
		},
	}

	result := svc.executePlan(context.Background(), plan, "")

	if repo.createCalled != 0 {
		t.Errorf("expected Create to not be called for conflict entry, called %d times", repo.createCalled)
	}
	if result.ConflictCount() != 1 {
		t.Errorf("expected 1 conflict result, got %d", result.ConflictCount())
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created results, got %d", result.CreatedCount())
	}
}

// TestExecutePlan_UnchangedEntry_NoCreateCall verifies that an unchanged entry
// in the plan does not trigger any persistence call. The executor records an
// unchanged result and skips the repository.
func TestExecutePlan_UnchangedEntry_NoCreateCall(t *testing.T) {
	repo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepo(repo)

	plan := ApplyPlan{
		Entries: []ApplyPlanEntry{
			{
				Kind:   types.KindSurface,
				ID:     "surf-unchanged",
				Action: ApplyActionUnchanged,
				Doc:    validSurface("surf-unchanged"),
			},
		},
	}

	result := svc.executePlan(context.Background(), plan, "")

	if repo.createCalled != 0 {
		t.Errorf("expected Create to not be called for unchanged entry, called %d times", repo.createCalled)
	}
	if result.UnchangedCount() != 1 {
		t.Errorf("expected 1 unchanged result, got %d", result.UnchangedCount())
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created results, got %d", result.CreatedCount())
	}
}

// TestApply_ConflictSurface_ReturnsConflictResult verifies that the full Apply
// path surfaces a conflict result (not an error) when the planner detects a
// pending review.
func TestApply_ConflictSurface_ReturnsConflictResult(t *testing.T) {
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

	result := svc.Apply(context.Background(), []parser.ParsedDocument{validSurface("surf-pending")}, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d", result.ValidationErrorCount())
	}
	if result.ConflictCount() != 1 {
		t.Errorf("expected 1 conflict result, got %d", result.ConflictCount())
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created results, got %d", result.CreatedCount())
	}
	if result.Success() {
		t.Error("expected Apply to not be successful when a conflict exists")
	}
}

// TestApply_NewSurface_WithRepo_ReturnsCreated verifies that the full Apply
// path creates a new surface when no persisted version exists.
func TestApply_NewSurface_WithRepo_ReturnsCreated(t *testing.T) {
	repo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, nil // no existing version
		},
	}
	svc := NewServiceWithRepo(repo)

	result := svc.Apply(context.Background(), []parser.ParsedDocument{validSurface("surf-brand-new")}, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d", result.ValidationErrorCount())
	}
	if result.CreatedCount() != 1 {
		t.Errorf("expected 1 created result, got %d", result.CreatedCount())
	}
	if !result.Success() {
		t.Error("expected Apply to succeed for new surface")
	}
}

// TestApply_InvalidBundle_WithRepo_StillRejectsAll verifies that invalid
// bundles are rejected in full even when a repository is configured. No
// repository calls should occur.
func TestApply_InvalidBundle_WithRepo_StillRejectsAll(t *testing.T) {
	findCalled := 0
	repo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			findCalled++
			return nil, nil
		},
	}
	svc := NewServiceWithRepo(repo)

	result := svc.Apply(context.Background(), []parser.ParsedDocument{invalidSurface("surf-bad")}, "")

	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation errors for invalid bundle")
	}
	if result.TotalCount() != 0 {
		t.Errorf("expected 0 total results for invalid bundle, got %d", result.TotalCount())
	}
	// The planner marks the doc invalid from validation before calling the repo.
	// Verify that no repo call occurred for the invalid document.
	if findCalled != 0 {
		t.Errorf("expected FindLatestByID to not be called for invalid doc, called %d times", findCalled)
	}
}
