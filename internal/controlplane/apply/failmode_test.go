package apply_test

// failmode_test.go — apply-pipeline integration tests for the
// FailModePolicy Kind (D27j-impl-1b). Validation, mapping, planning, and
// apply-time persistence are exercised end-to-end against the memory
// repository. The Postgres counterpart lives at
// internal/store/postgres/failmode_apply_integration_test.go.

import (
	"context"
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/controlplane/validate"
	"github.com/accept-io/midas/internal/failmode"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/memory"
)

// fmpApplyService builds an apply.Service wired with the full memory
// RepositorySet. Wiring every repo (rather than just FailModePolicies) is
// required because executePlan's validation-only fallback at
// service.go:2024 short-circuits any apply where every "core" repo is
// nil — a partially-wired Service would silently report Created without
// persisting the row.
func fmpApplyService(t *testing.T) (*apply.Service, *store.Repositories) {
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
	})
	return svc, repos
}

// makeValidFMPDoc returns a parsed FailModePolicy document satisfying every
// closed-only validator constraint. Tests mutate fields per scenario.
func makeValidFMPDoc(id string) parser.ParsedDocument {
	doc := types.FailModePolicyDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindFailModePolicy,
		Metadata:   types.DocumentMetadata{ID: id, Name: "Test Policy " + id},
		Spec: types.FailModePolicySpec{
			Name:           "Test Policy " + id,
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

func TestApply_FailModePolicy_CreatesReviewRow_Memory(t *testing.T) {
	svc, repos := fmpApplyService(t)
	ctx := context.Background()

	doc := makeValidFMPDoc("default-fail-mode")
	if errs := validate.ValidateDocument(doc); len(errs) != 0 {
		t.Fatalf("validation should pass; got %+v", errs)
	}

	res := svc.Apply(ctx, []parser.ParsedDocument{doc}, "user-1")
	created := countCreated(res, types.KindFailModePolicy)
	if created != 1 {
		t.Errorf("Created FailModePolicy: want 1, got %d (full result: %+v)", created, res.Results)
	}

	got, err := repos.FailModePolicies.FindByID(ctx, "default-fail-mode")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil {
		t.Fatal("FindByID: persisted row missing")
	}
	if got.Status != failmode.FailModePolicyStatusReview {
		t.Errorf("Status: want review (forced), got %q", got.Status)
	}
	if got.Version != 1 {
		t.Errorf("Version: want 1, got %d", got.Version)
	}
	if got.CreatedBy != "user-1" {
		t.Errorf("CreatedBy: want %q, got %q", "user-1", got.CreatedBy)
	}
	if got.ApprovedBy != "" || got.ApprovedAt != nil {
		t.Errorf("approval fields should remain empty on apply; got ApprovedBy=%q ApprovedAt=%v",
			got.ApprovedBy, got.ApprovedAt)
	}
}

// TestApply_FailModePolicy_ForcesReviewEvenWhenLifecycleStatusActive pins
// the user-flagged invariant that an explicit lifecycle.status of "active"
// in the document does not override the persisted "review" status.
func TestApply_FailModePolicy_ForcesReviewEvenWhenLifecycleStatusActive(t *testing.T) {
	svc, repos := fmpApplyService(t)
	ctx := context.Background()

	parsed := makeValidFMPDoc("default-fail-mode")
	doc := parsed.Doc.(types.FailModePolicyDocument)
	doc.Lifecycle.Status = "active" // valid enum value, must not influence persistence
	parsed.Doc = doc

	if errs := validate.ValidateDocument(parsed); len(errs) != 0 {
		t.Fatalf("validation should pass; got %+v", errs)
	}

	_ = svc.Apply(ctx, []parser.ParsedDocument{parsed}, "user-1")

	got, err := repos.FailModePolicies.FindByID(ctx, "default-fail-mode")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Status != failmode.FailModePolicyStatusReview {
		t.Errorf("Status: want review (forced even when lifecycle.status=active), got %q", got.Status)
	}
}

// D29b replacements — the deleted RejectsSoftMode / RejectsOpenMode
// tests are replaced by positive admittance checks plus a forbidden-
// combination test.

func TestApply_FailModePolicy_AcceptsSoftMode_EvidenceOnly(t *testing.T) {
	parsed := makeValidFMPDoc("default-fail-mode")
	doc := parsed.Doc.(types.FailModePolicyDocument)
	for i := range doc.Spec.Rules {
		if doc.Spec.Rules[i].CorrectnessClass == "resource" {
			doc.Spec.Rules[i].PermittedMode = "soft"
			doc.Spec.Rules[i].EnforcementState = "evidence_only"
		}
	}
	parsed.Doc = doc

	errs := validate.ValidateDocument(parsed)
	if len(errs) != 0 {
		t.Errorf("apply validate must accept soft + evidence_only; got %+v", errs)
	}
}

func TestApply_FailModePolicy_AcceptsOpenMode_EvidenceOnly(t *testing.T) {
	parsed := makeValidFMPDoc("default-fail-mode")
	doc := parsed.Doc.(types.FailModePolicyDocument)
	for i := range doc.Spec.Rules {
		if doc.Spec.Rules[i].CorrectnessClass == "resource" {
			doc.Spec.Rules[i].PermittedMode = "open"
			doc.Spec.Rules[i].EnforcementState = "evidence_only"
		}
	}
	parsed.Doc = doc

	errs := validate.ValidateDocument(parsed)
	if len(errs) != 0 {
		t.Errorf("apply validate must accept open + evidence_only; got %+v", errs)
	}
}

func TestApply_FailModePolicy_RejectsForbiddenCombination_ClosedEnforcedPermitWithEvidence(t *testing.T) {
	parsed := makeValidFMPDoc("default-fail-mode")
	doc := parsed.Doc.(types.FailModePolicyDocument)
	for i := range doc.Spec.Rules {
		if doc.Spec.Rules[i].CorrectnessClass == "resource" {
			// closed + enforced + permit_with_evidence is forbidden:
			// closed posture cannot produce a relax outcome under
			// active enforcement.
			doc.Spec.Rules[i].PermittedMode = "closed"
			doc.Spec.Rules[i].EnforcementState = "enforced"
			doc.Spec.Rules[i].Outcome = "permit_with_evidence"
		}
	}
	parsed.Doc = doc

	errs := validate.ValidateDocument(parsed)
	if len(errs) == 0 {
		t.Fatal("expected validation errors for closed + enforced + permit_with_evidence; got none")
	}
	found := false
	for _, e := range errs {
		if e.Field == "spec.rules" && strings.Contains(e.Message, "not permitted") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'not permitted' validator error; got %+v", errs)
	}
}

func TestApply_FailModePolicy_AppendsNewVersionOnUpdate_Memory(t *testing.T) {
	svc, repos := fmpApplyService(t)
	ctx := context.Background()

	// First apply — version 1.
	_ = svc.Apply(ctx, []parser.ParsedDocument{makeValidFMPDoc("default-fail-mode")}, "user-1")

	// Second apply with mutated description — must create version 2, not
	// conflict, not overwrite.
	parsed := makeValidFMPDoc("default-fail-mode")
	doc := parsed.Doc.(types.FailModePolicyDocument)
	doc.Spec.Description = "Updated description for v2"
	parsed.Doc = doc

	_ = svc.Apply(ctx, []parser.ParsedDocument{parsed}, "user-1")

	versions, err := repos.FailModePolicies.ListVersions(ctx, "default-fail-mode")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("ListVersions: want 2 entries, got %d", len(versions))
	}
	// ListVersions returns descending, so [0] is the latest.
	if versions[0].Version != 2 {
		t.Errorf("latest version: want 2, got %d", versions[0].Version)
	}
	if versions[0].Description != "Updated description for v2" {
		t.Errorf("v2 description: want updated, got %q", versions[0].Description)
	}
	if versions[0].Status != failmode.FailModePolicyStatusReview {
		t.Errorf("v2 status: want review, got %q", versions[0].Status)
	}
}

// countCreated returns the number of Created results of the given kind in
// the ApplyResult.
func countCreated(res types.ApplyResult, kind string) int {
	n := 0
	for _, r := range res.Results {
		if r.Kind == kind && r.Status == types.ResourceStatusCreated {
			n++
		}
	}
	return n
}

// TestApply_FailModePolicy_PlanReportsNewVersionForExistingID exercises the
// planner's first-create vs new-version branch via the public Plan API.
func TestApply_FailModePolicy_PlanReportsNewVersionForExistingID(t *testing.T) {
	svc, _ := fmpApplyService(t)
	ctx := context.Background()

	_ = svc.Apply(ctx, []parser.ParsedDocument{makeValidFMPDoc("default-fail-mode")}, "user-1")

	plan := svc.Plan(ctx, []parser.ParsedDocument{makeValidFMPDoc("default-fail-mode")})
	if len(plan.Entries) != 1 {
		t.Fatalf("Plan.Entries: want 1, got %d", len(plan.Entries))
	}
	e := plan.Entries[0]
	if e.Action != apply.ApplyActionCreate {
		t.Errorf("Action: want Create, got %q", e.Action)
	}
	if e.NewVersion != 2 {
		t.Errorf("NewVersion: want 2, got %d", e.NewVersion)
	}
	if e.CreateKind != apply.CreateKindNewVersion {
		t.Errorf("CreateKind: want NewVersion, got %q", e.CreateKind)
	}
}
