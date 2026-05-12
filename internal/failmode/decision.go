package failmode

// decision.go — D29h-lifted shared mapping helper.
//
// This file relocates the pure outcome-mapping helper that was
// previously local to internal/decision/orchestrator.go (introduced
// in D29e and shared with D29f). Lifting it into internal/failmode
// lets both the runtime orchestrator and the control-plane apply
// analyzer use the same mapping table, eliminating the risk of
// drift between D29h's apply-time warnings and the actual runtime
// behaviour they describe.
//
// Layering: this file imports internal/eval (a low-level types
// package). internal/eval imports internal/value only, so no
// cycle is introduced. The failmode package's existing constraint
// — no dependency on internal/decision — is preserved: this lift
// strictly inverts the previous dependency direction (decision
// now imports failmode for the helper rather than declaring it).

import (
	"github.com/accept-io/midas/internal/eval"
)

// ComputeFailModePolicyDecisionFromOutcome is the shared pure mapping
// helper used by D29e dry-run computation, D29f enforcement, and
// D29h apply-time tension analysis.
//
// It maps a FailModePolicyRule's configured Outcome into the
// (eval.Outcome, eval.ReasonCode) pair MIDAS would apply if the
// rule were enforced. For OutcomePermitWithEvidence the caller-
// supplied fallbackOutcome is returned — permit-with-evidence
// means "proceed as if no policy were enforced", so the outcome
// the orchestrator would have produced without enforcement is the
// right value.
//
// The function is pure: no accumulator, repository, or
// orchestrator state access. Sharing this helper between dry-run,
// enforced, and the control-plane analyser guarantees all three
// produce identical mappings — the only difference is what each
// caller passes for fallbackOutcome:
//
//   - D29e (dry-run): passes the actual outcome the orchestrator
//     is about to apply under authority.FailMode. The dry-run
//     event records the would-be-no-op behaviour for
//     permit_with_evidence.
//   - D29f (enforced): passes the FailModeOpen-continuation
//     outcome (eval.OutcomeAccept / WITHIN_AUTHORITY). Enforced
//     permit_with_evidence always lands on the proceed-with-
//     evidence path regardless of profile FailMode — this is the
//     documented operator-surprise case where an enforced
//     FailModePolicy overrides FailModeClosed.
//   - D29h (apply-time analysis): passes the same FailModeOpen-
//     continuation outcome as D29f when predicting the enforced
//     outcome; passes the per-profile authority.FailMode-derived
//     outcome when predicting the previous outcome.
//
// Mapping table (D29e §3 / D29f §3 / D29h §3, identical for all):
//
//	configured Outcome           → outcome              reason_code
//	─────────────────────────────────────────────────────────────────
//	OutcomeDeny                  → eval.OutcomeReject   FAIL_MODE_POLICY_DENIED
//	OutcomeEscalate              → eval.OutcomeEscalate FAIL_MODE_POLICY_ESCALATED
//	OutcomePermitWithEvidence    → fallbackOutcome      FAIL_MODE_POLICY_PERMIT_WITH_EVIDENCE
//	OutcomeManualReview          → eval.OutcomeEscalate FAIL_MODE_POLICY_MANUAL_REVIEW
//
// ManualReview maps to eval.OutcomeEscalate because
// eval.OutcomeManualReview does not exist; adding it would have
// wire-contract implications outside the runtime tranches' scope.
// ReasonFailModePolicyManualReview is preserved so operator
// intent is recorded even when the runtime outcome enum cannot
// express it.
//
// An unknown configured outcome (which the D29b validator
// rejects on apply) returns the fallback pair defensively — the
// caller continues with the pre-enforcement decision.
func ComputeFailModePolicyDecisionFromOutcome(
	rule FailModePolicyRule,
	fallbackOutcome eval.Outcome,
) (eval.Outcome, eval.ReasonCode) {
	switch rule.Outcome {
	case OutcomeDeny:
		return eval.OutcomeReject, eval.ReasonFailModePolicyDenied
	case OutcomeEscalate:
		return eval.OutcomeEscalate, eval.ReasonFailModePolicyEscalated
	case OutcomePermitWithEvidence:
		return fallbackOutcome, eval.ReasonFailModePolicyPermitWithEvidence
	case OutcomeManualReview:
		return eval.OutcomeEscalate, eval.ReasonFailModePolicyManualReview
	}
	// Defensive: D29b validator rejects unknown outcomes, so this
	// branch is unreachable under healthy data. The fallback pair
	// preserves the pre-enforcement decision.
	return fallbackOutcome, ""
}
