package apply_test

// governanceexpectation_test.go — focused tests for the
// GovernanceExpectation Kind in control-plane apply (#52). Versioning
// and review-forcing parallel Profile; the new piece is the
// Surface↔Process referential check that enforces the issue's headline
// constraint that required_surface_id must belong to the declared
// process.

import (
	"context"
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/governanceexpectation"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/memory"
)

// geApplyService builds an apply.Service wired with the full memory
// RepositorySet — every Kind-specific repo is plumbed so the planner's
// referential checks operate on real persisted state. The seedTestProcess
// helper has already created "test.bs" / "test.cap" / "test.process".
func geApplyService(t *testing.T, repos *store.Repositories) *apply.Service {
	t.Helper()
	return apply.NewServiceWithRepos(apply.RepositorySet{
		Surfaces:                    repos.Surfaces,
		Agents:                      repos.Agents,
		Profiles:                    repos.Profiles,
		Grants:                      repos.Grants,
		Processes:                   repos.Processes,
		Capabilities:                repos.Capabilities,
		BusinessServices:            repos.BusinessServices,
		BusinessServiceCapabilities: repos.BusinessServiceCapabilities,
		GovernanceExpectations:      repos.GovernanceExpectations,
	})
}

// makeSurfaceDoc builds a structurally-valid Surface document referencing
// the given process. Used by tests that need an in-bundle Surface co-
// resident with a GovernanceExpectation.
func makeSurfaceDoc(id, processID string) parser.ParsedDocument {
	doc := types.SurfaceDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindSurface,
		Metadata:   types.DocumentMetadata{ID: id, Name: "Test Surface " + id},
		Spec: types.SurfaceSpec{
			Category:  "test",
			RiskTier:  "low",
			Status:    "active",
			ProcessID: processID,
		},
	}
	return parser.ParsedDocument{Kind: types.KindSurface, ID: id, Doc: doc}
}

// makeProcessDoc builds a structurally-valid Process document.
func makeProcessDoc(id, businessServiceID string) parser.ParsedDocument {
	doc := types.ProcessDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindProcess,
		Metadata:   types.DocumentMetadata{ID: id, Name: "Test Process " + id},
		Spec:       types.ProcessSpec{BusinessServiceID: businessServiceID, Status: "active"},
	}
	return parser.ParsedDocument{Kind: types.KindProcess, ID: id, Doc: doc}
}

// makeGEDoc builds a GovernanceExpectation parsed document. Tests
// override individual fields by mutating the spec before wrapping.
func makeGEDoc(id, scopeID, requiredSurfaceID string) types.GovernanceExpectationDocument {
	return types.GovernanceExpectationDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindGovernanceExpectation,
		Metadata:   types.DocumentMetadata{ID: id, Name: "Expectation " + id},
		Spec: types.GovernanceExpectationSpec{
			ScopeKind:         "process",
			ScopeID:           scopeID,
			RequiredSurfaceID: requiredSurfaceID,
			ConditionType:     "risk_condition",
			BusinessOwner:     "biz",
			TechnicalOwner:    "tech",
		},
	}
}

func wrapGEDoc(doc types.GovernanceExpectationDocument) parser.ParsedDocument {
	return parser.ParsedDocument{Kind: types.KindGovernanceExpectation, ID: doc.Metadata.ID, Doc: doc}
}

// surfDocBundle is the canonical "create the surface that the
// expectation references in this same bundle" fixture.
func surfDocBundle(geID, processID, surfaceID string) []parser.ParsedDocument {
	return []parser.ParsedDocument{
		makeSurfaceDoc(surfaceID, processID),
		wrapGEDoc(makeGEDoc(geID, processID, surfaceID)),
	}
}

// ---------------------------------------------------------------------------
// Versioning + review-forcing
// ---------------------------------------------------------------------------

func TestGovernanceExpectation_FirstApply_CreatesVersion1AsReview(t *testing.T) {
	repos := memory.NewRepositories()
	seedTestProcess(t, repos)

	if err := repos.Surfaces.Create(context.Background(), modActiveSurface("test.surface.ge")); err != nil {
		t.Fatalf("seed surface: %v", err)
	}

	svc := geApplyService(t, repos)
	ctx := context.Background()

	doc := makeGEDoc("ge-first", "test.process", "test.surface.ge")
	doc.Spec.Lifecycle.Status = "active" // YAML claims active; mapper must force review
	docs := []parser.ParsedDocument{wrapGEDoc(doc)}

	plan := svc.Plan(ctx, docs)
	if got, want := len(plan.Entries), 1; got != want {
		t.Fatalf("plan entries: want %d, got %d", want, got)
	}
	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionCreate {
		t.Errorf("Action: want create, got %s", entry.Action)
	}
	if entry.CreateKind != apply.CreateKindNew {
		t.Errorf("CreateKind: want new, got %s", entry.CreateKind)
	}
	if entry.NewVersion != 1 {
		t.Errorf("NewVersion: want 1, got %d", entry.NewVersion)
	}

	result := svc.Apply(ctx, docs, "actor-test")
	if result.CreatedCount() != 1 {
		t.Fatalf("CreatedCount: want 1, got %d", result.CreatedCount())
	}
	if !result.Success() {
		t.Fatal("Apply must succeed")
	}

	// Persisted state must show status=review regardless of YAML status=active.
	got, err := repos.GovernanceExpectations.FindByID(ctx, "ge-first")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil {
		t.Fatal("expectation not persisted")
	}
	if got.Version != 1 {
		t.Errorf("Version: want 1, got %d", got.Version)
	}
	if got.Status != governanceexpectation.ExpectationStatusReview {
		t.Errorf("Status: want review (forced), got %s", got.Status)
	}
	if got.CreatedBy != "actor-test" {
		t.Errorf("CreatedBy: want actor-test, got %q", got.CreatedBy)
	}
}

func TestGovernanceExpectation_ReApply_CreatesNewReviewVersion(t *testing.T) {
	repos := memory.NewRepositories()
	seedTestProcess(t, repos)
	if err := repos.Surfaces.Create(context.Background(), modActiveSurface("test.surface.ge")); err != nil {
		t.Fatalf("seed surface: %v", err)
	}

	svc := geApplyService(t, repos)
	ctx := context.Background()
	docs := []parser.ParsedDocument{wrapGEDoc(makeGEDoc("ge-reapply", "test.process", "test.surface.ge"))}

	// First apply.
	if got := svc.Apply(ctx, docs, "actor-1"); got.CreatedCount() != 1 {
		t.Fatalf("first apply CreatedCount: want 1, got %d", got.CreatedCount())
	}

	// Re-apply. Plan must show CreateKindNewVersion + NewVersion=2.
	plan := svc.Plan(ctx, docs)
	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionCreate {
		t.Errorf("re-apply Action: want create, got %s", entry.Action)
	}
	if entry.CreateKind != apply.CreateKindNewVersion {
		t.Errorf("re-apply CreateKind: want new_version, got %s", entry.CreateKind)
	}
	if entry.NewVersion != 2 {
		t.Errorf("re-apply NewVersion: want 2, got %d", entry.NewVersion)
	}
	if !strings.Contains(entry.Message, "version 1") || !strings.Contains(entry.Message, "version 2") {
		t.Errorf("re-apply Message must name both versions; got %q", entry.Message)
	}

	if got := svc.Apply(ctx, docs, "actor-2"); got.CreatedCount() != 1 {
		t.Fatalf("re-apply CreatedCount: want 1, got %d", got.CreatedCount())
	}

	// The persisted latest version must now be 2 and still in review.
	latest, err := repos.GovernanceExpectations.FindByID(ctx, "ge-reapply")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if latest.Version != 2 {
		t.Errorf("latest Version: want 2, got %d", latest.Version)
	}
	if latest.Status != governanceexpectation.ExpectationStatusReview {
		t.Errorf("latest Status: want review, got %s", latest.Status)
	}
}

// ---------------------------------------------------------------------------
// Same-bundle references
// ---------------------------------------------------------------------------

func TestGovernanceExpectation_SameBundleProcessAndSurface_PlanCreate(t *testing.T) {
	repos := memory.NewRepositories()
	seedTestProcess(t, repos) // seeds test.bs

	svc := geApplyService(t, repos)
	ctx := context.Background()

	// Create a fresh process and surface within the same bundle as the
	// expectation; references resolve via the bundle pre-pass map.
	docs := []parser.ParsedDocument{
		makeProcessDoc("proc-same-bundle", "test.bs"),
		makeSurfaceDoc("surf-same-bundle", "proc-same-bundle"),
		wrapGEDoc(makeGEDoc("ge-same-bundle", "proc-same-bundle", "surf-same-bundle")),
	}

	plan := svc.Plan(ctx, docs)
	for _, e := range plan.Entries {
		if e.Action != apply.ApplyActionCreate {
			t.Errorf("entry %s/%s: want create, got %s (errors=%+v message=%q)",
				e.Kind, e.ID, e.Action, e.ValidationErrors, e.Message)
		}
	}

	result := svc.Apply(ctx, docs, "")
	if result.CreatedCount() != 3 {
		t.Errorf("CreatedCount: want 3, got %d", result.CreatedCount())
	}
}

// ---------------------------------------------------------------------------
// Negative referential paths
// ---------------------------------------------------------------------------

func TestGovernanceExpectation_MissingProcess_Invalidates(t *testing.T) {
	repos := memory.NewRepositories()
	seedTestProcess(t, repos)
	if err := repos.Surfaces.Create(context.Background(), modActiveSurface("test.surface.ge")); err != nil {
		t.Fatalf("seed surface: %v", err)
	}
	svc := geApplyService(t, repos)
	ctx := context.Background()

	// scope_id points at a process that doesn't exist anywhere.
	doc := makeGEDoc("ge-no-proc", "proc-does-not-exist", "test.surface.ge")
	docs := []parser.ParsedDocument{wrapGEDoc(doc)}

	plan := svc.Plan(ctx, docs)
	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionInvalid {
		t.Fatalf("Action: want invalid, got %s", entry.Action)
	}
	if !hasErrOnField(entry.ValidationErrors, "spec.scope_id") {
		t.Errorf("missing-process error must target spec.scope_id; got %+v", entry.ValidationErrors)
	}

	if got := svc.Apply(ctx, docs, ""); got.CreatedCount() != 0 {
		t.Errorf("CreatedCount: want 0 (invalid blocks bundle), got %d", got.CreatedCount())
	}
}

func TestGovernanceExpectation_MissingSurface_Invalidates(t *testing.T) {
	repos := memory.NewRepositories()
	seedTestProcess(t, repos)
	svc := geApplyService(t, repos)
	ctx := context.Background()

	// required_surface_id points at a Surface that doesn't exist.
	docs := []parser.ParsedDocument{
		wrapGEDoc(makeGEDoc("ge-no-surf", "test.process", "surf-does-not-exist")),
	}

	plan := svc.Plan(ctx, docs)
	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionInvalid {
		t.Fatalf("Action: want invalid, got %s", entry.Action)
	}
	if !hasErrOnField(entry.ValidationErrors, "spec.required_surface_id") {
		t.Errorf("missing-surface error must target spec.required_surface_id; got %+v", entry.ValidationErrors)
	}

	if got := svc.Apply(ctx, docs, ""); got.CreatedCount() != 0 {
		t.Errorf("CreatedCount: want 0 (invalid blocks bundle), got %d", got.CreatedCount())
	}
}

func TestGovernanceExpectation_SurfaceBelongsToDifferentProcess_Invalidates(t *testing.T) {
	repos := memory.NewRepositories()
	seedTestProcess(t, repos)

	// Seed a Surface that belongs to test.process. The expectation will
	// declare a different scope process, so the cross-check fails.
	if err := repos.Surfaces.Create(context.Background(), modActiveSurface("surf-mismatch")); err != nil {
		t.Fatalf("seed surface: %v", err)
	}

	svc := geApplyService(t, repos)
	ctx := context.Background()

	// Create another process so scope_id resolves but is not the
	// process that surf-mismatch belongs to.
	docs := []parser.ParsedDocument{
		makeProcessDoc("proc-other", "test.bs"),
		wrapGEDoc(makeGEDoc("ge-mismatch", "proc-other", "surf-mismatch")),
	}

	plan := svc.Plan(ctx, docs)
	geEntry := findEntry(plan.Entries, types.KindGovernanceExpectation, "ge-mismatch")
	if geEntry == nil {
		t.Fatal("expected a GovernanceExpectation entry")
	}
	if geEntry.Action != apply.ApplyActionInvalid {
		t.Fatalf("Action: want invalid, got %s", geEntry.Action)
	}
	field := findFieldErr(geEntry.ValidationErrors, "spec.required_surface_id")
	if field == nil {
		t.Fatalf("expected error on spec.required_surface_id; got %+v", geEntry.ValidationErrors)
	}
	for _, want := range []string{"surf-mismatch", "test.process", "proc-other", "must belong to the declared process"} {
		if !strings.Contains(field.Message, want) {
			t.Errorf("error message must mention %q; got %q", want, field.Message)
		}
	}
}

func TestGovernanceExpectation_SameBundleSurfaceBelongsToDifferentProcess_Invalidates(t *testing.T) {
	repos := memory.NewRepositories()
	seedTestProcess(t, repos)
	svc := geApplyService(t, repos)
	ctx := context.Background()

	// Two co-resident processes; the surface belongs to proc-a, but the
	// expectation declares scope proc-b. The mismatch must be detected
	// from the bundle pre-pass without a repository round-trip.
	docs := []parser.ParsedDocument{
		makeProcessDoc("proc-a", "test.bs"),
		makeProcessDoc("proc-b", "test.bs"),
		makeSurfaceDoc("surf-a", "proc-a"),
		wrapGEDoc(makeGEDoc("ge-bundle-mismatch", "proc-b", "surf-a")),
	}

	plan := svc.Plan(ctx, docs)
	entry := findEntry(plan.Entries, types.KindGovernanceExpectation, "ge-bundle-mismatch")
	if entry == nil {
		t.Fatal("expected a GovernanceExpectation entry")
	}
	if entry.Action != apply.ApplyActionInvalid {
		t.Fatalf("Action: want invalid, got %s", entry.Action)
	}
	field := findFieldErr(entry.ValidationErrors, "spec.required_surface_id")
	if field == nil {
		t.Fatalf("expected error on spec.required_surface_id; got %+v", entry.ValidationErrors)
	}
	for _, want := range []string{"surf-a", "proc-a", "proc-b"} {
		if !strings.Contains(field.Message, want) {
			t.Errorf("error message must mention %q; got %q", want, field.Message)
		}
	}
}

// ---------------------------------------------------------------------------
// Validation rejections (scope_kind allow-list)
// ---------------------------------------------------------------------------

func TestGovernanceExpectation_ScopeKind_BusinessService_RejectedByValidate(t *testing.T) {
	repos := memory.NewRepositories()
	seedTestProcess(t, repos)
	svc := geApplyService(t, repos)
	ctx := context.Background()

	doc := makeGEDoc("ge-bs", "test.bs", "test.surface.ge")
	doc.Spec.ScopeKind = "business_service"
	docs := []parser.ParsedDocument{wrapGEDoc(doc)}

	plan := svc.Plan(ctx, docs)
	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionInvalid {
		t.Fatalf("Action: want invalid, got %s", entry.Action)
	}
	field := findFieldErr(entry.ValidationErrors, "spec.scope_kind")
	if field == nil {
		t.Fatalf("expected error on spec.scope_kind; got %+v", entry.ValidationErrors)
	}
	if !strings.Contains(field.Message, "not supported") {
		t.Errorf("error must explain the apply-side scoping limitation; got %q", field.Message)
	}
}

func TestGovernanceExpectation_ScopeKind_Capability_RejectedByValidate(t *testing.T) {
	repos := memory.NewRepositories()
	seedTestProcess(t, repos)
	svc := geApplyService(t, repos)
	ctx := context.Background()

	doc := makeGEDoc("ge-cap", "test.cap", "test.surface.ge")
	doc.Spec.ScopeKind = "capability"
	docs := []parser.ParsedDocument{wrapGEDoc(doc)}

	plan := svc.Plan(ctx, docs)
	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionInvalid {
		t.Fatalf("Action: want invalid, got %s", entry.Action)
	}
	if findFieldErr(entry.ValidationErrors, "spec.scope_kind") == nil {
		t.Fatalf("expected error on spec.scope_kind; got %+v", entry.ValidationErrors)
	}
}

// ---------------------------------------------------------------------------
// Authz denial
// ---------------------------------------------------------------------------

// authorizerThatDeniesGE permits every Kind except GovernanceExpectation.
// Used to prove the apply.KindAuthorizer denial path emits the new
// permission name.
func authorizerThatDeniesGE(kind string) (bool, string) {
	if kind == types.KindGovernanceExpectation {
		return false, "governanceexpectation:write"
	}
	return true, ""
}

func TestGovernanceExpectation_AuthzDenial_NamesPermission(t *testing.T) {
	repos := memory.NewRepositories()
	seedTestProcess(t, repos)
	if err := repos.Surfaces.Create(context.Background(), modActiveSurface("test.surface.ge")); err != nil {
		t.Fatalf("seed surface: %v", err)
	}
	svc := geApplyService(t, repos)

	ctx := apply.WithKindAuthorizer(context.Background(), authorizerThatDeniesGE)
	docs := []parser.ParsedDocument{wrapGEDoc(makeGEDoc("ge-denied", "test.process", "test.surface.ge"))}

	plan := svc.Plan(ctx, docs)
	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionInvalid {
		t.Fatalf("Action: want invalid, got %s", entry.Action)
	}
	if !anyErrContains(entry.ValidationErrors, "governanceexpectation:write") {
		t.Errorf("authz denial error must name governanceexpectation:write; got %+v", entry.ValidationErrors)
	}

	if got := svc.Apply(ctx, docs, ""); got.CreatedCount() != 0 {
		t.Errorf("denied bundle must not persist; got CreatedCount=%d", got.CreatedCount())
	}
}

// ---------------------------------------------------------------------------
// Multi-doc: invalid expectation blocks the rest of the bundle
// ---------------------------------------------------------------------------

func TestGovernanceExpectation_InvalidEntry_BlocksWholeBundle(t *testing.T) {
	repos := memory.NewRepositories()
	seedTestProcess(t, repos)
	svc := geApplyService(t, repos)
	ctx := context.Background()

	// Bundle: a valid Surface, plus a GovernanceExpectation referencing
	// a non-existent surface. The invalid entry must block any persistence.
	docs := []parser.ParsedDocument{
		makeSurfaceDoc("surf-blocked-bundle", "test.process"),
		wrapGEDoc(makeGEDoc("ge-blocked", "test.process", "surf-does-not-exist")),
	}

	result := svc.Apply(ctx, docs, "")
	if result.CreatedCount() != 0 {
		t.Errorf("invalid GE must block whole bundle; got CreatedCount=%d", result.CreatedCount())
	}
	// Surface must not have been persisted.
	if s, _ := repos.Surfaces.FindLatestByID(ctx, "surf-blocked-bundle"); s != nil {
		t.Error("surface must not persist when bundle contains an invalid GovernanceExpectation")
	}
}

// ---------------------------------------------------------------------------
// Helpers (test-local)
// ---------------------------------------------------------------------------

func findEntry(entries []apply.ApplyPlanEntry, kind, id string) *apply.ApplyPlanEntry {
	for i := range entries {
		if entries[i].Kind == kind && entries[i].ID == id {
			return &entries[i]
		}
	}
	return nil
}

func hasErrOnField(errs []types.ValidationError, field string) bool {
	for _, e := range errs {
		if e.Field == field {
			return true
		}
	}
	return false
}

func findFieldErr(errs []types.ValidationError, field string) *types.ValidationError {
	for i := range errs {
		if errs[i].Field == field {
			return &errs[i]
		}
	}
	return nil
}
