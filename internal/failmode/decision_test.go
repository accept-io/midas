package failmode_test

// decision_test.go — D29h parity pin.
//
// The shared mapping helper ComputeFailModePolicyDecisionFromOutcome
// is used by:
//
//   1. D29e dry-run computation (decision-package wrapper preserves
//      divergence calculation),
//   2. D29f runtime enforcement (the orchestrator calls the helper
//      with FailModeOpen-continuation as fallback),
//   3. D29h control-plane apply-time tension analyzer (the analyzer
//      calls the helper with the same FailModeOpen-continuation
//      fallback to predict the enforced runtime outcome).
//
// This test pins the mapping table for every Axis C outcome and
// every authority.FailMode value, guaranteeing that the apply-time
// warnings D29h emits describe the actual runtime behaviour D29f
// produces.
//
// If this test drifts, warnings become misinformation. Fix the
// mapping helper first, then this test.

import (
	"testing"

	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/failmode"
)

// TestParity_ComputeFailModePolicyDecisionFromOutcome enumerates
// every (Axis C outcome, fallbackOutcome) pair and pins the
// expected (eval.Outcome, eval.ReasonCode) result. The two
// fallback values mirror the load-bearing call sites:
//
//   - eval.OutcomeAccept: D29f's FailModeOpen continuation. D29h's
//     analyzer uses the same fallback when predicting enforced
//     runtime outcomes.
//   - eval.OutcomeEscalate: a contrarian fallback used to confirm
//     the helper actually consults its parameter for
//     permit_with_evidence (and only permit_with_evidence).
func TestParity_ComputeFailModePolicyDecisionFromOutcome(t *testing.T) {
	cases := []struct {
		name            string
		ruleOutcome     failmode.Outcome
		fallbackOutcome eval.Outcome
		wantOutcome     eval.Outcome
		wantReason      eval.ReasonCode
	}{
		// Deny — always reject; fallbackOutcome is ignored.
		{"deny_fallback_accept", failmode.OutcomeDeny, eval.OutcomeAccept, eval.OutcomeReject, eval.ReasonFailModePolicyDenied},
		{"deny_fallback_escalate", failmode.OutcomeDeny, eval.OutcomeEscalate, eval.OutcomeReject, eval.ReasonFailModePolicyDenied},

		// Escalate — always escalate; fallbackOutcome is ignored.
		{"escalate_fallback_accept", failmode.OutcomeEscalate, eval.OutcomeAccept, eval.OutcomeEscalate, eval.ReasonFailModePolicyEscalated},
		{"escalate_fallback_escalate", failmode.OutcomeEscalate, eval.OutcomeEscalate, eval.OutcomeEscalate, eval.ReasonFailModePolicyEscalated},

		// PermitWithEvidence — fallbackOutcome is the result. This
		// is the only outcome whose mapping depends on fallback.
		{"permit_fallback_accept", failmode.OutcomePermitWithEvidence, eval.OutcomeAccept, eval.OutcomeAccept, eval.ReasonFailModePolicyPermitWithEvidence},
		{"permit_fallback_escalate", failmode.OutcomePermitWithEvidence, eval.OutcomeEscalate, eval.OutcomeEscalate, eval.ReasonFailModePolicyPermitWithEvidence},

		// ManualReview — always escalate (eval.OutcomeManualReview
		// does not exist as of D29h). The reason code preserves
		// operator intent.
		{"manual_review_fallback_accept", failmode.OutcomeManualReview, eval.OutcomeAccept, eval.OutcomeEscalate, eval.ReasonFailModePolicyManualReview},
		{"manual_review_fallback_escalate", failmode.OutcomeManualReview, eval.OutcomeEscalate, eval.OutcomeEscalate, eval.ReasonFailModePolicyManualReview},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rule := failmode.FailModePolicyRule{
				CorrectnessClass: failmode.CorrectnessClassResource,
				PermittedMode:    failmode.PermittedModeClosed,
				EnforcementState: failmode.EnforcementStateEnforced,
				Outcome:          c.ruleOutcome,
			}
			gotOutcome, gotReason := failmode.ComputeFailModePolicyDecisionFromOutcome(rule, c.fallbackOutcome)
			if gotOutcome != c.wantOutcome {
				t.Errorf("outcome: want %q, got %q", c.wantOutcome, gotOutcome)
			}
			if gotReason != c.wantReason {
				t.Errorf("reason: want %q, got %q", c.wantReason, gotReason)
			}
		})
	}
}

// TestParity_UnknownOutcomeReturnsFallback pins the defensive
// fallthrough: an unknown configured outcome (which the D29b
// validator rejects on apply) returns the fallback outcome and an
// empty reason code. The runtime helper preserves the
// pre-enforcement decision in this case.
func TestParity_UnknownOutcomeReturnsFallback(t *testing.T) {
	rule := failmode.FailModePolicyRule{
		CorrectnessClass: failmode.CorrectnessClassResource,
		PermittedMode:    failmode.PermittedModeClosed,
		EnforcementState: failmode.EnforcementStateEnforced,
		Outcome:          failmode.Outcome("bogus_unknown_outcome"),
	}
	gotOutcome, gotReason := failmode.ComputeFailModePolicyDecisionFromOutcome(rule, eval.OutcomeAccept)
	if gotOutcome != eval.OutcomeAccept {
		t.Errorf("unknown outcome must return fallback %q; got %q", eval.OutcomeAccept, gotOutcome)
	}
	if gotReason != "" {
		t.Errorf("unknown outcome must return empty reason; got %q", gotReason)
	}
}

// TestParity_RuntimeMatrix pins the full D29f enforcement matrix:
// for every (Axis C outcome, authority.FailMode) pair, the predicted
// enforced outcome the apply-time analyzer would record must match
// the runtime outcome D29f's orchestrator would apply. The
// orchestrator's enforced call site passes eval.OutcomeAccept as
// fallback (the FailModeOpen continuation), regardless of profile
// FailMode. This test pins that contract from the helper side.
//
// The previous-outcome side of the matrix is pinned by the
// control-plane apply test
// TestPlan_FailModePolicyAuthorityTension_PreviousOutcomeMatchesRuntime
// in internal/controlplane/apply (the apply test has access to the
// previousOutcomeFromFailMode helper that the analyzer uses).
func TestParity_RuntimeMatrix(t *testing.T) {
	// The runtime enforced fallback is always eval.OutcomeAccept,
	// from orchestrator.go's call site:
	//   failmode.ComputeFailModePolicyDecisionFromOutcome(rule, eval.OutcomeAccept)
	const runtimeFallback = eval.OutcomeAccept

	matrix := []struct {
		ruleOutcome     failmode.Outcome
		wantEnforced    eval.Outcome
		wantEnforcedRC  eval.ReasonCode
	}{
		{failmode.OutcomeDeny, eval.OutcomeReject, eval.ReasonFailModePolicyDenied},
		{failmode.OutcomeEscalate, eval.OutcomeEscalate, eval.ReasonFailModePolicyEscalated},
		{failmode.OutcomePermitWithEvidence, eval.OutcomeAccept, eval.ReasonFailModePolicyPermitWithEvidence},
		{failmode.OutcomeManualReview, eval.OutcomeEscalate, eval.ReasonFailModePolicyManualReview},
	}
	for _, m := range matrix {
		t.Run(string(m.ruleOutcome), func(t *testing.T) {
			rule := failmode.FailModePolicyRule{
				CorrectnessClass: failmode.CorrectnessClassResource,
				PermittedMode:    failmode.PermittedModeClosed,
				EnforcementState: failmode.EnforcementStateEnforced,
				Outcome:          m.ruleOutcome,
			}
			gotOutcome, gotReason := failmode.ComputeFailModePolicyDecisionFromOutcome(rule, runtimeFallback)
			if gotOutcome != m.wantEnforced {
				t.Errorf("enforced outcome for rule.Outcome=%q: want %q, got %q",
					m.ruleOutcome, m.wantEnforced, gotOutcome)
			}
			if gotReason != m.wantEnforcedRC {
				t.Errorf("enforced reason for rule.Outcome=%q: want %q, got %q",
					m.ruleOutcome, m.wantEnforcedRC, gotReason)
			}
		})
	}
}
