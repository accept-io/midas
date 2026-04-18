package apply

import (
	"context"
	"errors"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

// controlledProcessRepo is a test double for ProcessRepository. The planner
// now reads Status through GetByID to emit advisory warnings, so
// getByIDFn takes precedence when set. For backward-compatibility, tests
// that only set existsFn still work: GetByID synthesises a default
// active process from the existsFn result.
type controlledProcessRepo struct {
	existsFn  func(ctx context.Context, id string) (bool, error)
	getByIDFn func(ctx context.Context, id string) (*process.Process, error)
}

func (r *controlledProcessRepo) Exists(ctx context.Context, id string) (bool, error) {
	if r.existsFn != nil {
		return r.existsFn(ctx, id)
	}
	return false, nil
}

func (r *controlledProcessRepo) GetByID(ctx context.Context, id string) (*process.Process, error) {
	if r.getByIDFn != nil {
		return r.getByIDFn(ctx, id)
	}
	if r.existsFn == nil {
		return nil, nil
	}
	exists, err := r.existsFn(ctx, id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	return &process.Process{ID: id, Status: "active"}, nil
}

func (r *controlledProcessRepo) Create(_ context.Context, _ *process.Process) error {
	return nil
}

// surfaceDocWithProcessID returns a minimal valid surface document with the
// given process_id set. All required fields are present.
func surfaceDocWithProcessID(id, processID string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindSurface,
		ID:   id,
		Doc: types.SurfaceDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindSurface,
			Metadata: types.DocumentMetadata{
				ID:   id,
				Name: "Test Surface",
			},
			Spec: types.SurfaceSpec{
				Category:  "financial",
				RiskTier:  "high",
				Status:    "active",
				ProcessID: processID,
			},
		},
	}
}

// processRepoAlwaysExists returns a ProcessRepository test double that reports
// every process ID as existing. Use this in tests that exercise the surface
// apply path but do not need to test process-existence logic specifically.
func processRepoAlwaysExists() *controlledProcessRepo {
	return &controlledProcessRepo{
		existsFn: func(_ context.Context, _ string) (bool, error) {
			return true, nil
		},
	}
}

// stubSurfaceRepo returns nil (no existing version) for any lookup.
func stubSurfaceRepoNew() *controlledSurfaceRepo {
	return &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, nil
		},
	}
}

// ---------------------------------------------------------------------------
// Surface-process linkage tests
// ---------------------------------------------------------------------------

// assertProcessIDRequired is a helper that verifies a surface plan entry is
// rejected with a required error on spec.process_id. The error may originate
// from the validation phase (DecisionSourceValidation) or the planning phase
// (DecisionSourcePersistedState); both produce Field: "spec.process_id" with
// a message containing "required".
func assertProcessIDRequired(t *testing.T, plan ApplyPlan) {
	t.Helper()
	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionInvalid {
		t.Errorf("want action=invalid, got %s; errors: %v", entry.Action, entry.ValidationErrors)
		return
	}
	for _, ve := range entry.ValidationErrors {
		if ve.Field == "spec.process_id" {
			return
		}
	}
	t.Errorf("expected validation error on spec.process_id, got: %v", entry.ValidationErrors)
}

// TestSurface_ProcessID_Empty_Rejected verifies that an empty process_id is
// rejected (I-1: every Surface must belong to a Process).
func TestSurface_ProcessID_Empty_Rejected(t *testing.T) {
	svc := &Service{surfaceRepo: stubSurfaceRepoNew()}
	plan := svc.Plan(context.Background(), []parser.ParsedDocument{
		surfaceDocWithProcessID("surf-empty-proc", ""),
	})
	assertProcessIDRequired(t, plan)
}

// TestSurface_ProcessID_Whitespace_Rejected verifies that a whitespace-only
// process_id is treated as absent and rejected.
func TestSurface_ProcessID_Whitespace_Rejected(t *testing.T) {
	svc := &Service{surfaceRepo: stubSurfaceRepoNew()}
	plan := svc.Plan(context.Background(), []parser.ParsedDocument{
		surfaceDocWithProcessID("surf-ws-proc", "   "),
	})
	assertProcessIDRequired(t, plan)
}

// TestSurface_ProcessID_Missing_Rejected verifies that a surface document with
// no process_id key (zero value) is rejected.
func TestSurface_ProcessID_Missing_Rejected(t *testing.T) {
	svc := &Service{surfaceRepo: stubSurfaceRepoNew()}
	// surfaceDocWithProcessID("", ...) is not the missing case; here we build a
	// document where ProcessID is simply not set (Go zero value = "").
	doc := parser.ParsedDocument{
		Kind: types.KindSurface,
		ID:   "surf-missing-proc",
		Doc: types.SurfaceDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindSurface,
			Metadata:   types.DocumentMetadata{ID: "surf-missing-proc", Name: "Missing Proc Surface"},
			Spec: types.SurfaceSpec{
				Category: "financial",
				RiskTier: "high",
				Status:   "active",
				// ProcessID intentionally absent (zero value)
			},
		},
	}
	plan := svc.Plan(context.Background(), []parser.ParsedDocument{doc})
	assertProcessIDRequired(t, plan)
}

// TestSurface_ProcessID_Missing_RejectsAtValidationPhase verifies that a missing
// process_id is caught in the validation phase (DecisionSourceValidation), not
// deferred to planning. This is the G-4 alignment test.
func TestSurface_ProcessID_Missing_RejectsAtValidationPhase(t *testing.T) {
	// No surfaceRepo configured: if the error came from the planner's
	// checkProcessExists, it would require a processRepo. Since neither repo is
	// set, any rejection must originate from the validator.
	svc := &Service{}
	doc := parser.ParsedDocument{
		Kind: types.KindSurface,
		ID:   "surf-validate-phase",
		Doc: types.SurfaceDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindSurface,
			Metadata:   types.DocumentMetadata{ID: "surf-validate-phase", Name: "Validate Phase Test"},
			Spec: types.SurfaceSpec{
				Category: "financial",
				RiskTier: "high",
				Status:   "active",
				// ProcessID intentionally absent
			},
		},
	}
	plan := svc.Plan(context.Background(), []parser.ParsedDocument{doc})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionInvalid {
		t.Errorf("want action=invalid, got %s", entry.Action)
	}
	if entry.DecisionSource != DecisionSourceValidation {
		t.Errorf("want DecisionSource=%s (validation phase), got %s — process_id must be rejected before planning",
			DecisionSourceValidation, entry.DecisionSource)
	}
	found := false
	for _, ve := range entry.ValidationErrors {
		if ve.Field == "spec.process_id" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected validation error on spec.process_id, got: %v", entry.ValidationErrors)
	}
}

// TestSurface_WithProcessID_ProcessExists_Creates verifies that a surface
// referencing an existing process is accepted.
func TestSurface_WithProcessID_ProcessExists_Creates(t *testing.T) {
	svc := &Service{
		surfaceRepo: stubSurfaceRepoNew(),
		processRepo: &controlledProcessRepo{
			existsFn: func(_ context.Context, id string) (bool, error) {
				if id == "payments.limits-v1" {
					return true, nil
				}
				return false, nil
			},
		},
	}

	docs := []parser.ParsedDocument{surfaceDocWithProcessID("surf-with-proc", "payments.limits-v1")}
	plan := svc.Plan(context.Background(), docs)

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionCreate {
		t.Errorf("want action=create, got %s; errors: %v", entry.Action, entry.ValidationErrors)
	}
}

// TestSurface_WithProcessID_ProcessNotFound_Invalid verifies that a surface
// referencing a non-existent process is marked invalid.
func TestSurface_WithProcessID_ProcessNotFound_Invalid(t *testing.T) {
	svc := &Service{
		surfaceRepo: stubSurfaceRepoNew(),
		processRepo: &controlledProcessRepo{
			existsFn: func(_ context.Context, _ string) (bool, error) {
				return false, nil
			},
		},
	}

	docs := []parser.ParsedDocument{surfaceDocWithProcessID("surf-bad-proc", "nonexistent.process")}
	plan := svc.Plan(context.Background(), docs)

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionInvalid {
		t.Errorf("want action=invalid, got %s", entry.Action)
	}
	found := false
	for _, ve := range entry.ValidationErrors {
		if ve.Field == "spec.process_id" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected validation error on spec.process_id, got: %v", entry.ValidationErrors)
	}
}

// TestSurface_WithProcessID_RepoError_Invalid verifies that a process repo
// error causes the surface entry to be marked invalid.
func TestSurface_WithProcessID_RepoError_Invalid(t *testing.T) {
	svc := &Service{
		surfaceRepo: stubSurfaceRepoNew(),
		processRepo: &controlledProcessRepo{
			existsFn: func(_ context.Context, _ string) (bool, error) {
				return false, errors.New("db connection error")
			},
		},
	}

	docs := []parser.ParsedDocument{surfaceDocWithProcessID("surf-err-proc", "some.process")}
	plan := svc.Plan(context.Background(), docs)

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionInvalid {
		t.Errorf("want action=invalid on repo error, got %s", entry.Action)
	}
}

// ---------------------------------------------------------------------------
// Apply-level coverage (not just plan)
// ---------------------------------------------------------------------------

// TestApply_WithProcessID_ProcessExists_Succeeds verifies that Apply (not just Plan)
// succeeds end-to-end when the process exists.
func TestApply_WithProcessID_ProcessExists_Succeeds(t *testing.T) {
	svc := &Service{
		surfaceRepo: stubSurfaceRepoNew(),
		processRepo: &controlledProcessRepo{
			existsFn: func(_ context.Context, _ string) (bool, error) {
				return true, nil
			},
		},
	}

	docs := []parser.ParsedDocument{surfaceDocWithProcessID("surf-apply-ok", "payments.v1")}
	result := svc.Apply(context.Background(), docs, "tester")

	if result.ValidationErrorCount() != 0 {
		t.Errorf("want no validation errors, got %d: %v", result.ValidationErrorCount(), result.ValidationErrors)
	}
	if !result.Success() {
		t.Error("want apply to succeed")
	}
}

// TestApply_WithProcessID_ProcessNotFound_Rejected verifies that Apply rejects
// a surface with an unknown process_id and makes no persistence calls.
func TestApply_WithProcessID_ProcessNotFound_Rejected(t *testing.T) {
	surfRepo := stubSurfaceRepoNew()

	svc := &Service{
		surfaceRepo: surfRepo,
		processRepo: &controlledProcessRepo{
			existsFn: func(_ context.Context, _ string) (bool, error) {
				return false, nil
			},
		},
	}

	docs := []parser.ParsedDocument{surfaceDocWithProcessID("surf-apply-reject", "nonexistent.proc")}
	result := svc.Apply(context.Background(), docs, "tester")

	if result.ValidationErrorCount() == 0 {
		t.Error("want validation errors for unknown process_id, got none")
	}
	if result.Success() {
		t.Error("want apply to fail for unknown process_id")
	}
	if surfRepo.createCalled != 0 {
		t.Errorf("want no persistence calls, got %d", surfRepo.createCalled)
	}
}

// TestApply_NoProcessID_Rejected verifies that Apply rejects a surface with no
// process_id (I-1: every Surface must belong to a Process).
func TestApply_NoProcessID_Rejected(t *testing.T) {
	svc := &Service{
		surfaceRepo: stubSurfaceRepoNew(),
		processRepo: &controlledProcessRepo{
			existsFn: func(_ context.Context, _ string) (bool, error) {
				// Required-field check runs before repo lookup; should never reach here.
				t.Error("Exists called unexpectedly — required-field check should have fired first")
				return false, nil
			},
		},
	}

	docs := []parser.ParsedDocument{surfaceDocWithProcessID("surf-apply-no-proc", "")}
	result := svc.Apply(context.Background(), docs, "tester")

	if result.ValidationErrorCount() == 0 {
		t.Error("want validation error for missing process_id, got none")
	}
	if result.Success() {
		t.Error("want apply to fail for missing process_id")
	}
}

// TestApply_WithProcessID_NoProcessRepo_Rejected verifies that Apply rejects
// a surface with non-empty process_id when no ProcessRepository is configured.
func TestApply_WithProcessID_NoProcessRepo_Rejected(t *testing.T) {
	svc := &Service{
		surfaceRepo: stubSurfaceRepoNew(),
		// processRepo intentionally nil
	}

	docs := []parser.ParsedDocument{surfaceDocWithProcessID("surf-apply-no-repo", "some.process")}
	result := svc.Apply(context.Background(), docs, "tester")

	if result.ValidationErrorCount() == 0 {
		t.Error("want validation errors when process repo not configured, got none")
	}
	if result.Success() {
		t.Error("want apply to fail when process repo not configured")
	}
}

// TestSurface_WithProcessID_NoProcessRepo_Invalid verifies that when a surface
// specifies a non-empty process_id but no ProcessRepository is configured,
// the entry is rejected with a clear error rather than silently accepted.
func TestSurface_WithProcessID_NoProcessRepo_Invalid(t *testing.T) {
	svc := &Service{
		surfaceRepo: stubSurfaceRepoNew(),
		// processRepo intentionally nil
	}

	docs := []parser.ParsedDocument{surfaceDocWithProcessID("surf-no-repo", "any.process")}
	plan := svc.Plan(context.Background(), docs)

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != ApplyActionInvalid {
		t.Errorf("want action=invalid when process repo not configured, got %s; errors: %v", entry.Action, entry.ValidationErrors)
	}
	found := false
	for _, ve := range entry.ValidationErrors {
		if ve.Field == "spec.process_id" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected validation error on spec.process_id, got: %v", entry.ValidationErrors)
	}
}
