package failmode

import (
	"testing"
)

// TestCanTransitionTo pins the FailModePolicy lifecycle transitions per
// D27j-impl-1a. The transition map mirrors the authority/control artefact
// posture used by AuthorityProfile and DecisionSurface.
func TestCanTransitionTo(t *testing.T) {
	cases := []struct {
		from FailModePolicyStatus
		to   FailModePolicyStatus
		want bool
	}{
		// draft → review|retired only
		{FailModePolicyStatusDraft, FailModePolicyStatusReview, true},
		{FailModePolicyStatusDraft, FailModePolicyStatusRetired, true},
		{FailModePolicyStatusDraft, FailModePolicyStatusActive, false},
		{FailModePolicyStatusDraft, FailModePolicyStatusDeprecated, false},
		{FailModePolicyStatusDraft, FailModePolicyStatusDraft, false},

		// review → active|draft|retired
		{FailModePolicyStatusReview, FailModePolicyStatusActive, true},
		{FailModePolicyStatusReview, FailModePolicyStatusDraft, true},
		{FailModePolicyStatusReview, FailModePolicyStatusRetired, true},
		{FailModePolicyStatusReview, FailModePolicyStatusDeprecated, false},
		{FailModePolicyStatusReview, FailModePolicyStatusReview, false},

		// active → deprecated|retired
		{FailModePolicyStatusActive, FailModePolicyStatusDeprecated, true},
		{FailModePolicyStatusActive, FailModePolicyStatusRetired, true},
		{FailModePolicyStatusActive, FailModePolicyStatusDraft, false},
		{FailModePolicyStatusActive, FailModePolicyStatusReview, false},
		{FailModePolicyStatusActive, FailModePolicyStatusActive, false},

		// deprecated → retired only
		{FailModePolicyStatusDeprecated, FailModePolicyStatusRetired, true},
		{FailModePolicyStatusDeprecated, FailModePolicyStatusActive, false},
		{FailModePolicyStatusDeprecated, FailModePolicyStatusDraft, false},
		{FailModePolicyStatusDeprecated, FailModePolicyStatusReview, false},
		{FailModePolicyStatusDeprecated, FailModePolicyStatusDeprecated, false},

		// retired is terminal
		{FailModePolicyStatusRetired, FailModePolicyStatusActive, false},
		{FailModePolicyStatusRetired, FailModePolicyStatusDraft, false},
		{FailModePolicyStatusRetired, FailModePolicyStatusReview, false},
		{FailModePolicyStatusRetired, FailModePolicyStatusDeprecated, false},
		{FailModePolicyStatusRetired, FailModePolicyStatusRetired, false},

		// unknown source state
		{FailModePolicyStatus(""), FailModePolicyStatusReview, false},
		{FailModePolicyStatus("garbage"), FailModePolicyStatusActive, false},
	}

	for _, tc := range cases {
		p := &FailModePolicy{Status: tc.from}
		got := p.CanTransitionTo(tc.to)
		if got != tc.want {
			t.Errorf("from=%q to=%q: want %v, got %v", tc.from, tc.to, tc.want, got)
		}
	}
}
