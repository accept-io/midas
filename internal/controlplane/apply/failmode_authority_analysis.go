package apply

// failmode_authority_analysis.go — D29h apply-time tension analysis.
//
// Emits FAIL_MODE_POLICY_AUTHORITY_TENSION advisory warnings when an
// enforced FailModePolicy rule would override the behaviour an
// authority profile's FailMode value would otherwise produce on a
// policy_evaluator_error trigger.
//
// The analysis is read-only: it loads the referenced FailModePolicy,
// inspects its resource-class rule, walks to authority profiles
// governing affected surfaces, predicts the enforced and previous
// outcomes via the shared failmode helper, and emits one warning per
// (policy, profile, surface) tuple where the prediction reveals a
// material delta. Warnings never block apply.
//
// Scope (D29h): only correctness_class=resource rules with
// enforcement_state=enforced on the policy_evaluator_error trigger
// are analysed. evidence_only and dry_run rules produce no warning
// because they do not change runtime outcomes. Inactive policies
// (status != active) and referentially-broken references are handled
// by existing referential-integrity errors and never duplicated as
// warnings.

import (
	"context"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/failmode"
)

// tensionMessage* are the operator-facing message strings for the
// five tension cases enumerated in the D29h brief. Stored as
// constants so tests can pin exact copy and so the call site stays
// terse. Each message is authority-agnostic per approach (A):
// describes the delta in terms of previous-vs-enforced outcome
// words only, without naming authority.FailMode values.
const (
	// Case A: authority.FailMode = closed + enforced permit_with_evidence.
	tensionMessageClosedPermit = "FailModePolicy permits execution where an authority fail-closed profile would escalate on policy evaluator error."

	// Case B: authority.FailMode = open + enforced deny.
	tensionMessageOpenDeny = "FailModePolicy rejects execution where an authority fail-open profile would proceed on policy evaluator error."

	// Case C: authority.FailMode = open + enforced escalate.
	tensionMessageOpenEscalate = "FailModePolicy escalates execution where an authority fail-open profile would proceed on policy evaluator error."

	// Case D: authority.FailMode = closed + enforced deny.
	tensionMessageClosedDeny = "FailModePolicy rejects execution where an authority fail-closed profile would escalate on policy evaluator error."

	// Case E: authority.FailMode = open + enforced manual_review.
	tensionMessageOpenManualReview = "FailModePolicy routes execution to manual-review semantics where an authority fail-open profile would proceed on policy evaluator error."

	// Same-outcome edge case: when the enforced rule's mapped outcome
	// equals the authority-FailMode-driven outcome, NO warning is
	// emitted. The reason_code may differ (the audit chain records
	// this difference), but the runtime outcome is unchanged — the
	// brief explicitly excludes this from D29h's warning surface.
)

// analyzeSurfaceFailModePolicyTension is the Surface plan entry's
// call site. It loads the policy referenced by the surface (which
// has already been validated as active by checkFailModePolicyReference)
// and runs the per-profile tension analyzer against the single
// affected surface. The warning, if emitted, is attached to the
// Surface plan entry so the operator sees the tension on the
// document they are planning.
//
// effectiveAt is the surface's effective_from time used by
// FindActiveAt to resolve the policy version that the surface will
// activate against. When effectiveAt is zero the analyzer falls
// back to FindByID + active-status (matching the resolver and
// checkFailModePolicyReference shape).
//
// Skipped silently when:
//   - policyID is empty (no FailModePolicy reference),
//   - failModePolicyRepo is not wired,
//   - the policy cannot be loaded (referential check already
//     handled this as an error),
//   - the policy has no resource-class rule,
//   - the matching rule is not enforcement_state=enforced.
func (s *Service) analyzeSurfaceFailModePolicyTension(
	ctx context.Context,
	entry *ApplyPlanEntry,
	surfaceID string,
	policyID string,
	effectiveAt time.Time,
) {
	if policyID == "" || s.failModePolicyRepo == nil {
		return
	}
	policy := s.loadActivePolicyForTensionAnalysis(ctx, policyID, effectiveAt)
	if policy == nil {
		return
	}
	rule, ok := failmode.SelectRuleForClass(policy, failmode.CorrectnessClassResource)
	if !ok {
		// Defensive: validator enforces exhaustive class coverage,
		// so a missing resource rule indicates non-validated data.
		// Skip analysis rather than emit a misleading warning.
		return
	}
	s.analyzeFailModePolicyAuthorityTension(ctx, entry, policyID, rule, []surfaceTensionRef{
		{surfaceID: surfaceID, field: "spec.fail_mode_policy_id"},
	})
}

// analyzeBusinessServiceFailModePolicyTension is the
// BusinessService plan entry's call site. The apply-side
// interface has no list-surfaces-by-business-service method
// today, so the analyzer cannot enumerate affected authority
// profiles. Instead it emits a single potential-tension warning
// naming the BS + policy + rule shape when the referenced policy
// has an enforced resource rule.
//
// The warning is intentionally generic: operators are directed to
// audit authority profiles for surfaces governed by this BS before
// approving the apply. Per-surface analysis (with named profiles)
// runs on the Surface plan path where the surface→profile walk
// uses ListBySurface.
//
// Skipped silently when:
//   - policyID is empty,
//   - failModePolicyRepo is not wired,
//   - the policy cannot be loaded,
//   - the policy has no resource-class rule,
//   - the matching rule is not enforcement_state=enforced.
func (s *Service) analyzeBusinessServiceFailModePolicyTension(
	ctx context.Context,
	entry *ApplyPlanEntry,
	businessServiceID string,
	policyID string,
) {
	if policyID == "" || s.failModePolicyRepo == nil {
		return
	}
	// BusinessService has no effective_from at the document layer;
	// the ref-check uses FindByID + active-status. Mirror that
	// behaviour here.
	policy := s.loadActivePolicyForTensionAnalysis(ctx, policyID, time.Time{})
	if policy == nil {
		return
	}
	rule, ok := failmode.SelectRuleForClass(policy, failmode.CorrectnessClassResource)
	if !ok {
		return
	}
	if rule.EnforcementState != failmode.EnforcementStateEnforced {
		return
	}
	// Emit a single generic potential-tension warning. The exact
	// affected profiles are not enumerated here; operators run a
	// per-surface plan to see named-profile deltas.
	message := fmt.Sprintf(
		"FailModePolicy %q is enforced for the resource correctness class. Authority profiles governing surfaces under business service %q may be overridden on policy evaluator error; audit affected profiles before approval.",
		policyID, businessServiceID,
	)
	entry.AddWarning(PlanWarning{
		Code:        WarningFailModePolicyAuthorityTension,
		Severity:    WarningSeverityWarning,
		Message:     message,
		Field:       "spec.fail_mode_policy_id",
		RelatedKind: types.KindFailModePolicy,
		RelatedID:   policyID,
	})
}

// loadActivePolicyForTensionAnalysis resolves the policy at the
// given effective time. When effectiveAt is zero it falls back to
// FindByID and asserts active status, matching
// checkFailModePolicyReference's shape. Returns nil on any error
// or non-active result — the analyzer never blocks plans on its
// own lookup failures.
func (s *Service) loadActivePolicyForTensionAnalysis(
	ctx context.Context,
	policyID string,
	effectiveAt time.Time,
) *failmode.FailModePolicy {
	if effectiveAt.IsZero() {
		latest, err := s.failModePolicyRepo.FindByID(ctx, policyID)
		if err != nil || latest == nil {
			return nil
		}
		if latest.Status != failmode.FailModePolicyStatusActive {
			return nil
		}
		return latest
	}
	policy, err := s.failModePolicyRepo.FindActiveAt(ctx, policyID, effectiveAt)
	if err != nil {
		return nil
	}
	return policy
}

// analyzeFailModePolicyAuthorityTension emits one warning per
// affected (policy, profile, surface) tuple when the supplied
// FailModePolicy resource-class rule has enforcement_state=enforced
// and predicts a runtime outcome that differs from what authority's
// FailMode would have produced on a policy_evaluator_error trigger.
//
// Preconditions enforced by the caller:
//   - the resolved policy is active,
//   - the rule slice has been resolved to the resource-class rule
//     with axis defaults applied (via failmode.SelectRuleForClass),
//   - the surfaces argument lists the surface IDs known to reference
//     this policy at plan time.
//
// Repository dependencies:
//   - profileRepo.ListBySurface — to walk from a surface to the
//     authority profiles governing it. Skipped when the profile repo
//     is not wired; in that case the analyzer emits no warnings (the
//     existing referential-integrity check will have rejected the
//     plan earlier if profiles are required).
//
// The analyzer deduplicates by (fail_mode_policy_id,
// authority_profile_id, surface_id) so a profile shared across
// multiple in-bundle paths produces a single warning.
//
// Returns the number of warnings emitted, primarily for tests that
// pin "exactly N warnings emitted on this fixture."
func (s *Service) analyzeFailModePolicyAuthorityTension(
	ctx context.Context,
	entry *ApplyPlanEntry,
	policyID string,
	rule failmode.FailModePolicyRule,
	surfaces []surfaceTensionRef,
) int {
	// Gate (a): rule must be enforced. evidence_only and dry_run
	// rules never produce a warning.
	if rule.EnforcementState != failmode.EnforcementStateEnforced {
		return 0
	}
	// Gate (b): rule must apply to the resource correctness class.
	// D29h scopes to the policy_evaluator_error trigger which maps
	// to this class via internal/decision/failure_class.go.
	if rule.CorrectnessClass != failmode.CorrectnessClassResource {
		return 0
	}
	if s.profileRepo == nil {
		return 0
	}

	// Compute the enforced outcome once — it depends only on the
	// rule, not on the per-profile authority FailMode. The
	// FailModeOpen-continuation fallback (eval.OutcomeAccept,
	// eval.ReasonWithinAuthority) matches the runtime helper's
	// behaviour exactly; see internal/failmode/decision.go.
	enforcedOutcome, enforcedReason := failmode.ComputeFailModePolicyDecisionFromOutcome(rule, eval.OutcomeAccept)

	seen := make(map[string]struct{})
	emitted := 0
	for _, surfaceRef := range surfaces {
		profiles, err := s.profileRepo.ListBySurface(ctx, surfaceRef.surfaceID)
		if err != nil || len(profiles) == 0 {
			// A repository error here cannot fail the plan — the
			// analyzer is advisory. Silently skip; the next plan
			// run after the repo recovers will surface any warning.
			continue
		}
		for _, prof := range profiles {
			if prof == nil {
				continue
			}
			dedupKey := policyID + "\x00" + prof.ID + "\x00" + surfaceRef.surfaceID
			if _, dup := seen[dedupKey]; dup {
				continue
			}
			seen[dedupKey] = struct{}{}

			previousOutcome, previousReason := previousOutcomeFromFailMode(prof.FailMode)
			if enforcedOutcome == previousOutcome {
				// Same-outcome edge case (e.g. closed + enforced
				// escalate produces escalate in both paths, only
				// reason_code differs). The brief explicitly
				// excludes this from the warning surface.
				continue
			}
			message := tensionMessageForCase(prof.FailMode, rule.Outcome)
			if message == "" {
				// Defensive: an unknown rule.Outcome would have
				// been rejected by the D29b validator. Skip rather
				// than emit a warning with no message.
				continue
			}
			entry.AddWarning(PlanWarning{
				Code:        WarningFailModePolicyAuthorityTension,
				Severity:    WarningSeverityWarning,
				Message:     message,
				Field:       surfaceRef.field,
				RelatedKind: types.KindProfile,
				RelatedID:   prof.ID,
			})
			emitted++
			// enforcedReason and previousReason are computed but
			// not surfaced on the wire today — the parity test in
			// internal/failmode/decision_test.go pins their
			// equality with the runtime path so a future tranche
			// adding them to the warning detail line cannot drift.
			_ = enforcedReason
			_ = previousReason
		}
	}
	return emitted
}

// surfaceTensionRef describes one surface known to reference the
// FailModePolicy being analysed. The field path is reported on the
// emitted warning so the operator can locate the source of the
// tension within the planned document.
type surfaceTensionRef struct {
	surfaceID string
	field     string
}

// previousOutcomeFromFailMode returns the (outcome, reason_code)
// pair that the runtime orchestrator would produce on a
// policy_evaluator_error trigger when no FailModePolicy
// enforcement applies, parameterised by authority.FailMode. This
// mirrors the dispatch in
// internal/decision/orchestrator.go::evaluatePolicy: FailModeOpen
// returns ("", "", true, nil) which the orchestrator falls through
// to Accept/WithinAuthority; FailModeClosed returns
// (Escalate, POLICY_ERROR, true, nil) directly.
//
// The parity test in internal/failmode/decision_test.go ensures
// these values stay aligned with the runtime path.
func previousOutcomeFromFailMode(fm authority.FailMode) (eval.Outcome, eval.ReasonCode) {
	switch fm {
	case authority.FailModeClosed:
		return eval.OutcomeEscalate, eval.ReasonPolicyError
	case authority.FailModeOpen:
		return eval.OutcomeAccept, eval.ReasonWithinAuthority
	}
	// Unknown FailMode — defensive fallthrough. The authority
	// validator rejects unknown values on apply, so this branch is
	// unreachable under healthy data.
	return eval.OutcomeAccept, eval.ReasonWithinAuthority
}

// tensionMessageForCase returns the exact operator-facing message
// for one of the five tension cases. Returns "" for case
// combinations that should not warn (the caller pre-checks the
// same-outcome edge case).
//
// Cases are case-sensitive: the brief specifies authority.FailMode
// and rule.Outcome pair → message. Combinations not listed below
// fall into the no-warning bucket either because outcome matches
// the previous path or because the rule.Outcome is structurally
// invalid (validator rejection).
func tensionMessageForCase(fm authority.FailMode, ruleOutcome failmode.Outcome) string {
	switch fm {
	case authority.FailModeClosed:
		switch ruleOutcome {
		case failmode.OutcomePermitWithEvidence:
			return tensionMessageClosedPermit
		case failmode.OutcomeDeny:
			return tensionMessageClosedDeny
		}
	case authority.FailModeOpen:
		switch ruleOutcome {
		case failmode.OutcomeDeny:
			return tensionMessageOpenDeny
		case failmode.OutcomeEscalate:
			return tensionMessageOpenEscalate
		case failmode.OutcomeManualReview:
			return tensionMessageOpenManualReview
		}
	}
	// All other combinations either produce the same outcome as
	// authority would (no warning) or are invalid under the D29b
	// validator (unreachable).
	return ""
}
