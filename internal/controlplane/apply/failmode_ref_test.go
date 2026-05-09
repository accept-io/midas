package apply_test

// failmode_ref_test.go — apply-pipeline integration tests for the
// optional Surface.fail_mode_policy_id and BusinessService.fail_mode_policy_id
// references introduced in D27j-impl-2. Confirms the ref-check helper
// rejects entries that point at missing, review, deprecated, or out-of-window
// policies, and accepts active references.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/failmode"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/memory"
)

// fmpRefService builds a fully-wired apply.Service against memory repos
// (matches fmpApplyService in failmode_test.go but is duplicated here so
// the two test files can be moved/owned independently).
func fmpRefService(t *testing.T) (*apply.Service, *store.Repositories) {
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

// seedFailModePolicy seeds a FailModePolicy directly into the memory repo
// (bypasses apply because apply forces status=review, and most scenarios
// here need an explicit status/window).
func seedFailModePolicy(t *testing.T, repos *store.Repositories, id string, version int, status failmode.FailModePolicyStatus, effective time.Time) {
	t.Helper()
	if err := repos.FailModePolicies.Create(context.Background(), &failmode.FailModePolicy{
		ID:             id,
		Version:        version,
		Name:           "Test " + id,
		Status:         status,
		EffectiveDate:  effective,
		BusinessOwner:  "owner@example.com",
		TechnicalOwner: "platform-team",
		Rules: []failmode.FailModePolicyRule{
			{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed},
			{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed},
			{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable},
			{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: failmode.PermittedModeClosed},
			{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeClosed},
		},
		Origin:    "manual",
		Managed:   true,
		CreatedAt: effective,
		UpdatedAt: effective,
		CreatedBy: "test",
	}); err != nil {
		t.Fatalf("seed FailModePolicy %s/v%d: %v", id, version, err)
	}
}

// surfaceDocWithFMP returns a surface document referencing the given
// FailModePolicy. effectiveFrom is set so FindActiveAt has a window to
// match against.
func surfaceDocWithFMP(id, processID, fmpID string, effectiveFrom time.Time) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindSurface,
		ID:   id,
		Doc: types.SurfaceDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindSurface,
			Metadata:   types.DocumentMetadata{ID: id, Name: "Test Surface " + id},
			Spec: types.SurfaceSpec{
				Category:         "financial",
				RiskTier:         "high",
				Status:           "active",
				ProcessID:        processID,
				FailModePolicyID: fmpID,
				EffectiveFrom:    effectiveFrom,
			},
		},
	}
}

func bsDocWithFMP(id, fmpID string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindBusinessService,
		ID:   id,
		Doc: types.BusinessServiceDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindBusinessService,
			Metadata:   types.DocumentMetadata{ID: id, Name: "Test BS " + id},
			Spec: types.BusinessServiceSpec{
				ServiceType:      "internal",
				Status:           "active",
				FailModePolicyID: fmpID,
			},
		},
	}
}

// findFMPValidationError returns true when the entry has a validation
// error on the named field whose message contains the substring.
func findFMPValidationError(entry apply.ApplyPlanEntry, field, substr string) bool {
	for _, ve := range entry.ValidationErrors {
		if ve.Field == field && strings.Contains(ve.Message, substr) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Surface.fail_mode_policy_id reference scenarios
// ---------------------------------------------------------------------------

func TestSurface_FailModePolicyRef_ActiveAccepted(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	seedTestProcess(t, repos)

	now := time.Now().UTC()
	seedFailModePolicy(t, repos, "fmp-active", 1, failmode.FailModePolicyStatusActive, now.Add(-24*time.Hour))

	doc := surfaceDocWithFMP("surf-fmp-active", "test.process", "fmp-active", now)
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})

	if len(plan.Entries) != 1 {
		t.Fatalf("plan entries: want 1, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action == apply.ApplyActionInvalid {
		t.Errorf("active fail-mode policy reference must be accepted; got action=%s, errors=%v",
			entry.Action, entry.ValidationErrors)
	}
}

func TestSurface_FailModePolicyRef_MissingRejected(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	seedTestProcess(t, repos)

	doc := surfaceDocWithFMP("surf-fmp-missing", "test.process", "fmp-does-not-exist", time.Now().UTC())
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})

	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionInvalid {
		t.Fatalf("missing fail-mode policy reference must be invalid; got %s", entry.Action)
	}
	if !findFMPValidationError(entry, "spec.fail_mode_policy_id", "does not exist") {
		t.Errorf("expected 'does not exist' error on spec.fail_mode_policy_id; got %v", entry.ValidationErrors)
	}
}

func TestSurface_FailModePolicyRef_ReviewRejected(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	seedTestProcess(t, repos)

	now := time.Now().UTC()
	seedFailModePolicy(t, repos, "fmp-review", 1, failmode.FailModePolicyStatusReview, now.Add(-24*time.Hour))

	doc := surfaceDocWithFMP("surf-fmp-review", "test.process", "fmp-review", now)
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})

	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionInvalid {
		t.Fatalf("review fail-mode policy reference must be invalid; got %s", entry.Action)
	}
	if !findFMPValidationError(entry, "spec.fail_mode_policy_id", "not active") {
		t.Errorf("expected 'not active' error; got %v", entry.ValidationErrors)
	}
}

func TestSurface_FailModePolicyRef_DeprecatedRejected(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	seedTestProcess(t, repos)

	now := time.Now().UTC()
	seedFailModePolicy(t, repos, "fmp-deprecated", 1, failmode.FailModePolicyStatusDeprecated, now.Add(-24*time.Hour))

	doc := surfaceDocWithFMP("surf-fmp-deprecated", "test.process", "fmp-deprecated", now)
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})

	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionInvalid {
		t.Fatalf("deprecated fail-mode policy reference must be invalid; got %s", entry.Action)
	}
	if !findFMPValidationError(entry, "spec.fail_mode_policy_id", "not active") {
		t.Errorf("expected 'not active' error for deprecated policy; got %v", entry.ValidationErrors)
	}
}

func TestSurface_FailModePolicyRef_EffectiveAfterSurface_Rejected(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	seedTestProcess(t, repos)

	now := time.Now().UTC()
	// Policy is active but its effective_date is AFTER the surface's effective_from.
	seedFailModePolicy(t, repos, "fmp-future", 1, failmode.FailModePolicyStatusActive, now.Add(48*time.Hour))

	doc := surfaceDocWithFMP("surf-fmp-future", "test.process", "fmp-future", now)
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})

	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionInvalid {
		t.Fatalf("future-effective fail-mode policy reference must be invalid; got %s", entry.Action)
	}
	if !findFMPValidationError(entry, "spec.fail_mode_policy_id", "not effective") {
		t.Errorf("expected 'not effective' error; got %v", entry.ValidationErrors)
	}
}

// ---------------------------------------------------------------------------
// BusinessService.fail_mode_policy_id reference scenarios
//
// BusinessService has no document-level effective_from, so the ref-check
// helper falls back to FindByID + active-status. The tests pin that
// fallback behaviour: only an Active policy passes, and the error
// messages name the actual cause (missing vs not-active).
// ---------------------------------------------------------------------------

func TestBusinessService_FailModePolicyRef_ActiveAccepted(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()

	seedFailModePolicy(t, repos, "fmp-bs-active", 1, failmode.FailModePolicyStatusActive, time.Now().UTC().Add(-time.Hour))

	doc := bsDocWithFMP("bs-fmp-active", "fmp-bs-active")
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})

	entry := plan.Entries[0]
	if entry.Action == apply.ApplyActionInvalid {
		t.Errorf("active fail-mode policy reference must be accepted; got action=%s, errors=%v",
			entry.Action, entry.ValidationErrors)
	}
}

func TestBusinessService_FailModePolicyRef_MissingRejected(t *testing.T) {
	svc, _ := fmpRefService(t)
	ctx := context.Background()

	doc := bsDocWithFMP("bs-fmp-missing", "fmp-does-not-exist")
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})

	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionInvalid {
		t.Fatalf("missing fail-mode policy reference must be invalid; got %s", entry.Action)
	}
	if !findFMPValidationError(entry, "spec.fail_mode_policy_id", "does not exist") {
		t.Errorf("expected 'does not exist' error; got %v", entry.ValidationErrors)
	}
}

func TestBusinessService_FailModePolicyRef_ReviewRejected(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()

	seedFailModePolicy(t, repos, "fmp-bs-review", 1, failmode.FailModePolicyStatusReview, time.Now().UTC().Add(-time.Hour))

	doc := bsDocWithFMP("bs-fmp-review", "fmp-bs-review")
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})

	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionInvalid {
		t.Fatalf("review fail-mode policy reference must be invalid; got %s", entry.Action)
	}
	if !findFMPValidationError(entry, "spec.fail_mode_policy_id", "not active") {
		t.Errorf("expected 'not active' error; got %v", entry.ValidationErrors)
	}
}

func TestBusinessService_FailModePolicyRef_DeprecatedRejected(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()

	seedFailModePolicy(t, repos, "fmp-bs-deprecated", 1, failmode.FailModePolicyStatusDeprecated, time.Now().UTC().Add(-time.Hour))

	doc := bsDocWithFMP("bs-fmp-deprecated", "fmp-bs-deprecated")
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})

	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionInvalid {
		t.Fatalf("deprecated fail-mode policy reference must be invalid; got %s", entry.Action)
	}
	if !findFMPValidationError(entry, "spec.fail_mode_policy_id", "not active") {
		t.Errorf("expected 'not active' error for deprecated policy; got %v", entry.ValidationErrors)
	}
}

// TestBusinessService_FailModePolicyRef_AppliedReviewSameBatch_Rejected
// pins the same-batch posture: a FailModePolicy applied in the SAME bundle
// lands as Review (forced by the failmode mapper), so any Surface or
// BusinessService in that bundle that references it must be rejected
// during the same Plan run. The fix is operator workflow — apply the
// policy first, approve it, then apply the BusinessService.
func TestBusinessService_FailModePolicyRef_AppliedReviewSameBatch_Rejected(t *testing.T) {
	svc, _ := fmpRefService(t)
	ctx := context.Background()

	// Apply the policy first (it lands as review per the lifecycle invariant).
	policyDoc := makeValidFMPDoc("fmp-same-batch")
	if res := svc.Apply(ctx, []parser.ParsedDocument{policyDoc}, "user-1"); !res.Success() {
		t.Fatalf("seed apply of policy: %+v", res.ValidationErrors)
	}

	// Then plan a BusinessService referencing it — must be rejected because
	// the policy is in review status.
	bsDoc := bsDocWithFMP("bs-same-batch", "fmp-same-batch")
	plan := svc.Plan(ctx, []parser.ParsedDocument{bsDoc})

	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionInvalid {
		t.Errorf("review-status policy from same batch must be rejected; got %s", entry.Action)
	}
}

// TestBusinessService_FailModePolicyRef_NoRepoConfigured_Rejected pins the
// "validation unavailable" error path: a non-empty reference with no
// FailModePolicyRepository wired must reject rather than silently accept.
func TestBusinessService_FailModePolicyRef_NoRepoConfigured_Rejected(t *testing.T) {
	// Build a Service with NO FailModePolicies repo but enough other repos
	// that the BS planner runs.
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices: noopBSRepo{},
		// FailModePolicies intentionally nil
	})
	ctx := context.Background()

	doc := bsDocWithFMP("bs-no-repo", "fmp-something")
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})

	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionInvalid {
		t.Fatalf("missing FailModePolicyRepository must invalidate ref check; got %s", entry.Action)
	}
	if !findFMPValidationError(entry, "spec.fail_mode_policy_id", "validation unavailable") {
		t.Errorf("expected 'validation unavailable' error; got %v", entry.ValidationErrors)
	}
}

// noopBSRepo satisfies BusinessServiceRepository with empty implementations
// for the no-repo-configured test.
type noopBSRepo struct{}

func (noopBSRepo) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (noopBSRepo) GetByID(_ context.Context, _ string) (*businessservice.BusinessService, error) {
	return nil, nil
}
func (noopBSRepo) Create(_ context.Context, _ *businessservice.BusinessService) error { return nil }
func (noopBSRepo) Update(_ context.Context, _ *businessservice.BusinessService) error { return nil }
func (noopBSRepo) List(_ context.Context) ([]*businessservice.BusinessService, error) {
	return nil, nil
}
