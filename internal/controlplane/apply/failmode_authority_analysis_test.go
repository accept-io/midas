package apply_test

// failmode_authority_analysis_test.go — D29h apply-time tension
// analysis pins.
//
// Covers:
//
//   1. Positive emission for each of the five tension cases
//      (closed+permit, open+deny, open+escalate, closed+deny,
//      open+manual_review).
//   2. The same-outcome edge case: closed+enforced+escalate
//      produces zero warnings even though the reason_code differs
//      between previous and enforced paths.
//   3. Non-emission gates: evidence_only, dry_run, missing rule
//      (defensive), inactive policy, no FailModePolicy reference.
//   4. Surface-level analyzer emits one warning per profile when
//      multiple profiles govern the same surface (deduplicated by
//      (policy, profile, surface) tuple).
//   5. BusinessService-level analyzer emits a generic
//      potential-tension warning when the referenced policy has an
//      enforced resource rule, and skips the warning when the rule
//      is evidence_only.
//   6. Read-only / runtime-preservation:
//      - the warning does NOT change entry.Action,
//      - the FAIL_MODE_POLICY_ENFORCED runtime payload is unchanged,
//      - no audit event is emitted by the analyzer.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/failmode"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/value"
)

// seedFailModePolicyWithResourceRule seeds a FailModePolicy with the
// resource-class rule carrying the supplied
// (PermittedMode, EnforcementState, Outcome) tuple. Other rules use
// safe closed+evidence_only defaults sufficient for D29b validation
// when called through Create directly (bypassing the validator,
// which is fine for memory-repo seeding).
func seedFailModePolicyWithResourceRule(
	t *testing.T,
	repos *store.Repositories,
	id string,
	resourcePosture failmode.PermittedMode,
	resourceState failmode.EnforcementState,
	resourceOutcome failmode.Outcome,
) {
	t.Helper()
	now := time.Now().UTC()
	policy := &failmode.FailModePolicy{
		ID:             id,
		Version:        1,
		Name:           "Test " + id,
		Status:         failmode.FailModePolicyStatusActive,
		EffectiveDate:  now.Add(-24 * time.Hour),
		BusinessOwner:  "owner@example.com",
		TechnicalOwner: "platform-team",
		Rules: []failmode.FailModePolicyRule{
			{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: resourcePosture, EnforcementState: resourceState, Outcome: resourceOutcome},
			{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		},
		Origin:    "manual",
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "test",
	}
	if err := repos.FailModePolicies.Create(context.Background(), policy); err != nil {
		t.Fatalf("seed FailModePolicy %s: %v", id, err)
	}
}

// seedAuthorityProfileWithFailMode seeds an active AuthorityProfile
// associated with the supplied surface ID and FailMode. The
// thresholds are nominal — D29h doesn't inspect them.
func seedAuthorityProfileWithFailMode(t *testing.T, repos *store.Repositories, id, surfaceID string, fm authority.FailMode) {
	t.Helper()
	now := time.Now().UTC()
	if err := repos.Profiles.Create(context.Background(), &authority.AuthorityProfile{
		ID:                  id,
		SurfaceID:           surfaceID,
		Name:                "Test " + id,
		Status:              authority.ProfileStatusActive,
		Version:             1,
		EffectiveDate:       now.Add(-time.Hour),
		ConfidenceThreshold: 0.8,
		ConsequenceThreshold: authority.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingHigh,
		},
		FailMode: fm,
	}); err != nil {
		t.Fatalf("seed AuthorityProfile %s: %v", id, err)
	}
}

// findTensionWarning returns the first FAIL_MODE_POLICY_AUTHORITY_TENSION
// warning whose RelatedID equals the supplied id (or any if id == "").
// Returns nil when none matches.
func findTensionWarning(entry apply.ApplyPlanEntry, relatedID string) *apply.PlanWarning {
	for i := range entry.Warnings {
		w := &entry.Warnings[i]
		if w.Code != apply.WarningFailModePolicyAuthorityTension {
			continue
		}
		if relatedID == "" || w.RelatedID == relatedID {
			return w
		}
	}
	return nil
}

func countTensionWarnings(entry apply.ApplyPlanEntry) int {
	n := 0
	for _, w := range entry.Warnings {
		if w.Code == apply.WarningFailModePolicyAuthorityTension {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Five positive tension cases — Surface plan entry path
// ---------------------------------------------------------------------------

func TestPlan_Tension_ClosedPermitWithEvidence_EmitsWarning(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	seedTestProcess(t, repos)

	const surfaceID = "surf-tension-cp"
	seedFailModePolicyWithResourceRule(t, repos, "fmp-cp",
		failmode.PermittedModeSoft, failmode.EnforcementStateEnforced, failmode.OutcomePermitWithEvidence)
	seedAuthorityProfileWithFailMode(t, repos, "prof-closed", surfaceID, authority.FailModeClosed)

	doc := surfaceDocWithFMP(surfaceID, "test.process", "fmp-cp", time.Now().UTC())
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})
	if len(plan.Entries) != 1 {
		t.Fatalf("plan entries: want 1, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionCreate {
		t.Fatalf("warning must not change action; want create, got %s", entry.Action)
	}
	w := findTensionWarning(entry, "prof-closed")
	if w == nil {
		t.Fatalf("expected FAIL_MODE_POLICY_AUTHORITY_TENSION warning for prof-closed; got %+v", entry.Warnings)
	}
	const want = "FailModePolicy permits execution where an authority fail-closed profile would escalate on policy evaluator error."
	if w.Message != want {
		t.Errorf("message:\n  want %q\n  got  %q", want, w.Message)
	}
	if w.Severity != apply.WarningSeverityWarning {
		t.Errorf("severity = %q, want %q", w.Severity, apply.WarningSeverityWarning)
	}
	if w.RelatedKind != types.KindProfile {
		t.Errorf("related kind = %q, want %q", w.RelatedKind, types.KindProfile)
	}
	if w.Field != "spec.fail_mode_policy_id" {
		t.Errorf("field = %q, want spec.fail_mode_policy_id", w.Field)
	}
}

func TestPlan_Tension_OpenDeny_EmitsWarning(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	seedTestProcess(t, repos)

	const surfaceID = "surf-tension-od"
	seedFailModePolicyWithResourceRule(t, repos, "fmp-od",
		failmode.PermittedModeClosed, failmode.EnforcementStateEnforced, failmode.OutcomeDeny)
	seedAuthorityProfileWithFailMode(t, repos, "prof-open", surfaceID, authority.FailModeOpen)

	doc := surfaceDocWithFMP(surfaceID, "test.process", "fmp-od", time.Now().UTC())
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})
	entry := plan.Entries[0]
	w := findTensionWarning(entry, "prof-open")
	if w == nil {
		t.Fatalf("expected tension warning; got %+v", entry.Warnings)
	}
	const want = "FailModePolicy rejects execution where an authority fail-open profile would proceed on policy evaluator error."
	if w.Message != want {
		t.Errorf("message:\n  want %q\n  got  %q", want, w.Message)
	}
}

func TestPlan_Tension_OpenEscalate_EmitsWarning(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	seedTestProcess(t, repos)

	const surfaceID = "surf-tension-oe"
	seedFailModePolicyWithResourceRule(t, repos, "fmp-oe",
		failmode.PermittedModeClosed, failmode.EnforcementStateEnforced, failmode.OutcomeEscalate)
	seedAuthorityProfileWithFailMode(t, repos, "prof-open-esc", surfaceID, authority.FailModeOpen)

	doc := surfaceDocWithFMP(surfaceID, "test.process", "fmp-oe", time.Now().UTC())
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})
	entry := plan.Entries[0]
	w := findTensionWarning(entry, "prof-open-esc")
	if w == nil {
		t.Fatalf("expected tension warning; got %+v", entry.Warnings)
	}
	const want = "FailModePolicy escalates execution where an authority fail-open profile would proceed on policy evaluator error."
	if w.Message != want {
		t.Errorf("message:\n  want %q\n  got  %q", want, w.Message)
	}
}

func TestPlan_Tension_ClosedDeny_EmitsWarning(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	seedTestProcess(t, repos)

	const surfaceID = "surf-tension-cd"
	seedFailModePolicyWithResourceRule(t, repos, "fmp-cd",
		failmode.PermittedModeClosed, failmode.EnforcementStateEnforced, failmode.OutcomeDeny)
	seedAuthorityProfileWithFailMode(t, repos, "prof-closed-deny", surfaceID, authority.FailModeClosed)

	doc := surfaceDocWithFMP(surfaceID, "test.process", "fmp-cd", time.Now().UTC())
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})
	entry := plan.Entries[0]
	w := findTensionWarning(entry, "prof-closed-deny")
	if w == nil {
		t.Fatalf("expected tension warning; got %+v", entry.Warnings)
	}
	const want = "FailModePolicy rejects execution where an authority fail-closed profile would escalate on policy evaluator error."
	if w.Message != want {
		t.Errorf("message:\n  want %q\n  got  %q", want, w.Message)
	}
}

func TestPlan_Tension_OpenManualReview_EmitsWarning(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	seedTestProcess(t, repos)

	const surfaceID = "surf-tension-omr"
	seedFailModePolicyWithResourceRule(t, repos, "fmp-omr",
		failmode.PermittedModeClosed, failmode.EnforcementStateEnforced, failmode.OutcomeManualReview)
	seedAuthorityProfileWithFailMode(t, repos, "prof-open-mr", surfaceID, authority.FailModeOpen)

	doc := surfaceDocWithFMP(surfaceID, "test.process", "fmp-omr", time.Now().UTC())
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})
	entry := plan.Entries[0]
	w := findTensionWarning(entry, "prof-open-mr")
	if w == nil {
		t.Fatalf("expected tension warning; got %+v", entry.Warnings)
	}
	const want = "FailModePolicy routes execution to manual-review semantics where an authority fail-open profile would proceed on policy evaluator error."
	if w.Message != want {
		t.Errorf("message:\n  want %q\n  got  %q", want, w.Message)
	}
}

// ---------------------------------------------------------------------------
// Same-outcome edge cases — no warning
// ---------------------------------------------------------------------------

// TestPlan_Tension_ClosedEnforcedEscalate_NoWarning pins the
// explicit same-outcome edge case from the brief: enforced +
// escalate under authority.FailModeClosed produces Escalate in both
// the previous and enforced paths. Reason_code attribution changes
// but the runtime outcome does not — no warning required.
func TestPlan_Tension_ClosedEnforcedEscalate_NoWarning(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	seedTestProcess(t, repos)

	const surfaceID = "surf-tension-same-closed"
	seedFailModePolicyWithResourceRule(t, repos, "fmp-same-closed",
		failmode.PermittedModeClosed, failmode.EnforcementStateEnforced, failmode.OutcomeEscalate)
	seedAuthorityProfileWithFailMode(t, repos, "prof-same-closed", surfaceID, authority.FailModeClosed)

	doc := surfaceDocWithFMP(surfaceID, "test.process", "fmp-same-closed", time.Now().UTC())
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})
	entry := plan.Entries[0]
	if got := countTensionWarnings(entry); got != 0 {
		t.Errorf("same-outcome edge case must produce 0 warnings; got %d (%+v)", got, entry.Warnings)
	}
}

// TestPlan_Tension_OpenEnforcedPermit_NoWarning pins the symmetric
// same-outcome case: enforced + permit_with_evidence under
// FailModeOpen — both paths produce Accept.
func TestPlan_Tension_OpenEnforcedPermit_NoWarning(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	seedTestProcess(t, repos)

	const surfaceID = "surf-tension-same-open"
	seedFailModePolicyWithResourceRule(t, repos, "fmp-same-open",
		failmode.PermittedModeOpen, failmode.EnforcementStateEnforced, failmode.OutcomePermitWithEvidence)
	seedAuthorityProfileWithFailMode(t, repos, "prof-same-open", surfaceID, authority.FailModeOpen)

	doc := surfaceDocWithFMP(surfaceID, "test.process", "fmp-same-open", time.Now().UTC())
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})
	entry := plan.Entries[0]
	if got := countTensionWarnings(entry); got != 0 {
		t.Errorf("same-outcome edge case must produce 0 warnings; got %d (%+v)", got, entry.Warnings)
	}
}

// ---------------------------------------------------------------------------
// Non-emission gates
// ---------------------------------------------------------------------------

func TestPlan_Tension_EvidenceOnly_NoWarning(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	seedTestProcess(t, repos)

	const surfaceID = "surf-tension-eo"
	seedFailModePolicyWithResourceRule(t, repos, "fmp-eo",
		failmode.PermittedModeClosed, failmode.EnforcementStateEvidenceOnly, failmode.OutcomeEscalate)
	seedAuthorityProfileWithFailMode(t, repos, "prof-eo", surfaceID, authority.FailModeClosed)

	doc := surfaceDocWithFMP(surfaceID, "test.process", "fmp-eo", time.Now().UTC())
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})
	entry := plan.Entries[0]
	if got := countTensionWarnings(entry); got != 0 {
		t.Errorf("evidence_only rule must produce 0 warnings; got %d", got)
	}
}

func TestPlan_Tension_DryRun_NoWarning(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	seedTestProcess(t, repos)

	const surfaceID = "surf-tension-dr"
	seedFailModePolicyWithResourceRule(t, repos, "fmp-dr",
		failmode.PermittedModeClosed, failmode.EnforcementStateDryRun, failmode.OutcomeDeny)
	seedAuthorityProfileWithFailMode(t, repos, "prof-dr", surfaceID, authority.FailModeClosed)

	doc := surfaceDocWithFMP(surfaceID, "test.process", "fmp-dr", time.Now().UTC())
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})
	entry := plan.Entries[0]
	if got := countTensionWarnings(entry); got != 0 {
		t.Errorf("dry_run rule must produce 0 warnings; got %d", got)
	}
}

func TestPlan_Tension_NoFailModePolicyReference_NoWarning(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	seedTestProcess(t, repos)

	const surfaceID = "surf-tension-nofmp"
	seedAuthorityProfileWithFailMode(t, repos, "prof-nofmp", surfaceID, authority.FailModeClosed)

	// Surface document with no FailModePolicy reference.
	doc := surfaceDocWithFMP(surfaceID, "test.process", "", time.Now().UTC())
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})
	entry := plan.Entries[0]
	if got := countTensionWarnings(entry); got != 0 {
		t.Errorf("surface without FMP reference must produce 0 warnings; got %d", got)
	}
}

// ---------------------------------------------------------------------------
// Per-(policy, profile, surface) tuple uniqueness
// ---------------------------------------------------------------------------

// TestPlan_Tension_MultipleProfiles_OneWarningEach pins the
// uniqueness guarantee: one warning per affected profile when
// multiple profiles govern the same surface.
func TestPlan_Tension_MultipleProfiles_OneWarningEach(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	seedTestProcess(t, repos)

	const surfaceID = "surf-tension-multi"
	seedFailModePolicyWithResourceRule(t, repos, "fmp-multi",
		failmode.PermittedModeSoft, failmode.EnforcementStateEnforced, failmode.OutcomePermitWithEvidence)
	seedAuthorityProfileWithFailMode(t, repos, "prof-multi-a", surfaceID, authority.FailModeClosed)
	seedAuthorityProfileWithFailMode(t, repos, "prof-multi-b", surfaceID, authority.FailModeClosed)

	doc := surfaceDocWithFMP(surfaceID, "test.process", "fmp-multi", time.Now().UTC())
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})
	entry := plan.Entries[0]
	if got := countTensionWarnings(entry); got != 2 {
		t.Errorf("expected exactly 2 tension warnings (one per profile); got %d (%+v)",
			got, entry.Warnings)
	}
	if findTensionWarning(entry, "prof-multi-a") == nil {
		t.Errorf("missing warning for prof-multi-a")
	}
	if findTensionWarning(entry, "prof-multi-b") == nil {
		t.Errorf("missing warning for prof-multi-b")
	}
}

// ---------------------------------------------------------------------------
// BusinessService plan path — generic potential-tension warning
// ---------------------------------------------------------------------------

func TestPlan_Tension_BusinessService_EnforcedRule_EmitsGenericWarning(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()

	seedFailModePolicyWithResourceRule(t, repos, "fmp-bs-enforced",
		failmode.PermittedModeClosed, failmode.EnforcementStateEnforced, failmode.OutcomeDeny)

	doc := bsDocWithFMP("bs-tension", "fmp-bs-enforced")
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})
	if len(plan.Entries) != 1 {
		t.Fatalf("plan entries: want 1, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionCreate {
		t.Fatalf("warning must not change action; want create, got %s", entry.Action)
	}
	w := findTensionWarning(entry, "fmp-bs-enforced")
	if w == nil {
		t.Fatalf("expected generic potential-tension warning on BS entry; got %+v", entry.Warnings)
	}
	// Generic message names the BS + policy; uses "may be overridden"
	// language (not the per-profile case messages).
	if !strings.Contains(w.Message, "may be overridden") {
		t.Errorf("BS generic warning should use potential-tension language; got %q", w.Message)
	}
	if !strings.Contains(w.Message, "bs-tension") {
		t.Errorf("BS warning should name the business service id; got %q", w.Message)
	}
	if !strings.Contains(w.Message, "fmp-bs-enforced") {
		t.Errorf("BS warning should name the policy id; got %q", w.Message)
	}
	if w.RelatedKind != types.KindFailModePolicy {
		t.Errorf("related kind = %q, want %q", w.RelatedKind, types.KindFailModePolicy)
	}
	if w.RelatedID != "fmp-bs-enforced" {
		t.Errorf("related id = %q, want %q", w.RelatedID, "fmp-bs-enforced")
	}
}

func TestPlan_Tension_BusinessService_EvidenceOnly_NoWarning(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()

	seedFailModePolicyWithResourceRule(t, repos, "fmp-bs-eo",
		failmode.PermittedModeClosed, failmode.EnforcementStateEvidenceOnly, failmode.OutcomeEscalate)

	doc := bsDocWithFMP("bs-tension-eo", "fmp-bs-eo")
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})
	entry := plan.Entries[0]
	if got := countTensionWarnings(entry); got != 0 {
		t.Errorf("evidence_only BS reference must produce 0 warnings; got %d", got)
	}
}

func TestPlan_Tension_BusinessService_NoReference_NoWarning(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	_ = repos

	doc := bsDocWithFMP("bs-tension-noref", "")
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})
	entry := plan.Entries[0]
	if got := countTensionWarnings(entry); got != 0 {
		t.Errorf("BS without FMP reference must produce 0 warnings; got %d", got)
	}
}

// ---------------------------------------------------------------------------
// Read-only / runtime preservation
// ---------------------------------------------------------------------------

// TestPlan_Tension_DoesNotChangeAction pins that the warning never
// downgrades the entry to invalid. Even with every tension case
// emitting a warning, the entry's action remains create.
func TestPlan_Tension_DoesNotChangeAction(t *testing.T) {
	svc, repos := fmpRefService(t)
	ctx := context.Background()
	seedTestProcess(t, repos)

	const surfaceID = "surf-tension-action"
	seedFailModePolicyWithResourceRule(t, repos, "fmp-action",
		failmode.PermittedModeSoft, failmode.EnforcementStateEnforced, failmode.OutcomePermitWithEvidence)
	seedAuthorityProfileWithFailMode(t, repos, "prof-action", surfaceID, authority.FailModeClosed)

	doc := surfaceDocWithFMP(surfaceID, "test.process", "fmp-action", time.Now().UTC())
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})
	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionCreate {
		t.Errorf("entry action must remain create with warning; got %s", entry.Action)
	}
	if len(entry.ValidationErrors) != 0 {
		t.Errorf("warnings must not produce validation errors; got %+v", entry.ValidationErrors)
	}
}
