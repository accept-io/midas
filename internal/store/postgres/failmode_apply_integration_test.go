package postgres

// failmode_apply_integration_test.go — end-to-end FailModePolicy apply
// against real Postgres (D27j-impl-1b). Skipped automatically when
// DATABASE_URL is not set.
//
// These tests pin the contract that a FailModePolicy document lands via
// the apply path with the expected (id, version) row visible in
// fail_mode_policies, that the persisted status is "review" regardless of
// any lifecycle.status declared in the document, and that re-applying the
// same logical id appends a new version.
//
// FailModePolicy has no audit emission in D27j-impl-1b — the future
// approval-endpoint tranche (D27j-impl-1c) will add the four control-audit
// constructors (created, versioned, approved, deprecated) as a coherent
// set. These tests therefore do not assert audit presence.

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/failmode"
)

func makeFMPApplyDoc(id string) parser.ParsedDocument {
	doc := types.FailModePolicyDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindFailModePolicy,
		Metadata:   types.DocumentMetadata{ID: id, Name: "Apply Test " + id},
		Spec: types.FailModePolicySpec{
			Name:           "Apply Test " + id,
			BusinessOwner:  "owner@example.com",
			TechnicalOwner: "platform-team",
			Rules: []types.FailModePolicyRuleSpec{
				{CorrectnessClass: "governance_integrity", PermittedMode: "closed"},
				{CorrectnessClass: "persistence", PermittedMode: "closed"},
				{CorrectnessClass: "input", PermittedMode: "not_applicable"},
				{CorrectnessClass: "resource", PermittedMode: "closed"},
				{CorrectnessClass: "consistency", PermittedMode: "closed"},
			},
		},
	}
	return parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}
}

// fmpApplyServiceForPostgres wires a minimal apply service against the
// Postgres repos. BusinessServices is intentionally included so the
// validation-only fallback at executePlan does not short-circuit the
// apply (the fallback fires when every "core" repo is nil — see
// service.go:2024).
func fmpApplyServiceForPostgres(t *testing.T) (*apply.Service, *FailModePolicyRepo, func()) {
	t.Helper()
	db := openTestDB(t)

	// Pre-clean any rows from a prior failed run.
	cleanupFailModePolicies(t, db)

	cleanup := func() {
		cleanupFailModePolicies(t, db)
		db.Close()
	}
	t.Cleanup(cleanup)

	s, err := NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices: repos.BusinessServices,
		FailModePolicies: repos.FailModePolicies,
		ControlAudit:     repos.ControlAudit,
	})

	repo, err := NewFailModePolicyRepo(db)
	if err != nil {
		t.Fatalf("NewFailModePolicyRepo: %v", err)
	}

	return svc, repo, func() {}
}

func TestApply_FailModePolicy_CreatesReviewRow_Postgres(t *testing.T) {
	svc, repo, _ := fmpApplyServiceForPostgres(t)
	ctx := context.Background()

	result := svc.Apply(ctx, []parser.ParsedDocument{makeFMPApplyDoc("default-fail-mode")}, "operator:fmp-test")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("validation errors: %+v", result.ValidationErrors)
	}
	if result.ApplyErrorCount() != 0 {
		t.Fatalf("apply errors: %+v", result.Results)
	}
	if result.CreatedCount() != 1 {
		t.Fatalf("CreatedCount: want 1, got %d (result=%+v)", result.CreatedCount(), result)
	}

	got, err := repo.FindByID(ctx, "default-fail-mode")
	if err != nil {
		t.Fatalf("FindByID after apply: %v", err)
	}
	if got == nil {
		t.Fatal("FindByID after apply: row missing")
	}
	if got.Status != failmode.FailModePolicyStatusReview {
		t.Errorf("Status: want review (forced), got %q", got.Status)
	}
	if got.Version != 1 {
		t.Errorf("Version: want 1 for first apply, got %d", got.Version)
	}
	if got.CreatedBy != "operator:fmp-test" {
		t.Errorf("CreatedBy: want %q, got %q", "operator:fmp-test", got.CreatedBy)
	}
	if got.ApprovedBy != "" || got.ApprovedAt != nil {
		t.Errorf("approval fields should be empty post-apply; got ApprovedBy=%q ApprovedAt=%v",
			got.ApprovedBy, got.ApprovedAt)
	}
	if len(got.Rules) != 5 {
		t.Errorf("Rules: want 5 entries, got %d", len(got.Rules))
	}
}

func TestApply_FailModePolicy_ForcesReviewEvenWhenLifecycleStatusActive_Postgres(t *testing.T) {
	svc, repo, _ := fmpApplyServiceForPostgres(t)
	ctx := context.Background()

	parsed := makeFMPApplyDoc("default-fail-mode")
	doc := parsed.Doc.(types.FailModePolicyDocument)
	doc.Lifecycle.Status = "active" // valid enum value, must be ignored at persistence
	parsed.Doc = doc

	result := svc.Apply(ctx, []parser.ParsedDocument{parsed}, "operator:fmp-test")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("validation errors: %+v", result.ValidationErrors)
	}
	if result.ApplyErrorCount() != 0 {
		t.Fatalf("apply errors: %+v", result.Results)
	}

	got, err := repo.FindByID(ctx, "default-fail-mode")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil {
		t.Fatal("FindByID: row missing")
	}
	if got.Status != failmode.FailModePolicyStatusReview {
		t.Errorf("Status: want review (forced even with lifecycle.status=active), got %q", got.Status)
	}
}

func TestApply_FailModePolicy_AppendsNewVersionOnUpdate_Postgres(t *testing.T) {
	svc, repo, _ := fmpApplyServiceForPostgres(t)
	ctx := context.Background()

	// First apply — version 1.
	res1 := svc.Apply(ctx, []parser.ParsedDocument{makeFMPApplyDoc("default-fail-mode")}, "operator:fmp-test")
	if res1.ApplyErrorCount() != 0 || res1.ValidationErrorCount() != 0 {
		t.Fatalf("first apply produced errors: %+v", res1)
	}

	// Second apply with mutated description — must create version 2.
	parsed := makeFMPApplyDoc("default-fail-mode")
	doc := parsed.Doc.(types.FailModePolicyDocument)
	doc.Spec.Description = "Updated description for v2"
	parsed.Doc = doc

	res2 := svc.Apply(ctx, []parser.ParsedDocument{parsed}, "operator:fmp-test")
	if res2.ApplyErrorCount() != 0 || res2.ValidationErrorCount() != 0 {
		t.Fatalf("second apply produced errors: %+v", res2)
	}

	versions, err := repo.ListVersions(ctx, "default-fail-mode")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("ListVersions: want 2 entries, got %d", len(versions))
	}
	// ListVersions returns descending: [0] is v2, [1] is v1.
	if versions[0].Version != 2 {
		t.Errorf("latest version: want 2, got %d", versions[0].Version)
	}
	if versions[0].Description != "Updated description for v2" {
		t.Errorf("v2 description: want updated, got %q", versions[0].Description)
	}
	if versions[1].Version != 1 {
		t.Errorf("first version: want 1, got %d", versions[1].Version)
	}
	// Both versions must be review-status (no approval flow yet in -1b).
	for i, v := range versions {
		if v.Status != failmode.FailModePolicyStatusReview {
			t.Errorf("version %d (Version=%d): want review status, got %q", i, v.Version, v.Status)
		}
	}
}
