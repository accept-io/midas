package drift

import (
	"errors"
	"testing"
)

// TestValidateLifecycleTransition_Table covers every (from, to) pair
// across the five DriftDefinitionStatus values. Permitted edges:
//
//	draft      → review | retired
//	review     → active | draft | retired
//	active     → deprecated | retired
//	deprecated → retired
//	retired    → terminal (no outgoing)
//
// All other transitions must fail with ErrInvalidLifecycleTransition.
func TestValidateLifecycleTransition_Table(t *testing.T) {
	all := []DriftDefinitionStatus{
		DriftDefinitionStatusDraft,
		DriftDefinitionStatusReview,
		DriftDefinitionStatusActive,
		DriftDefinitionStatusDeprecated,
		DriftDefinitionStatusRetired,
	}

	permitted := map[DriftDefinitionStatus]map[DriftDefinitionStatus]bool{
		DriftDefinitionStatusDraft: {
			DriftDefinitionStatusReview:  true,
			DriftDefinitionStatusRetired: true,
		},
		DriftDefinitionStatusReview: {
			DriftDefinitionStatusActive:  true,
			DriftDefinitionStatusDraft:   true,
			DriftDefinitionStatusRetired: true,
		},
		DriftDefinitionStatusActive: {
			DriftDefinitionStatusDeprecated: true,
			DriftDefinitionStatusRetired:    true,
		},
		DriftDefinitionStatusDeprecated: {
			DriftDefinitionStatusRetired: true,
		},
		// retired: no outgoing edges.
	}

	for _, from := range all {
		for _, to := range all {
			err := ValidateLifecycleTransition(from, to)
			expected := permitted[from][to]
			if expected && err != nil {
				t.Errorf("ValidateLifecycleTransition(%q, %q) = %v, want nil (permitted)", from, to, err)
			}
			if !expected && err == nil {
				t.Errorf("ValidateLifecycleTransition(%q, %q) = nil, want error (forbidden)", from, to)
			}
			if !expected && err != nil && !errors.Is(err, ErrInvalidLifecycleTransition) {
				t.Errorf("ValidateLifecycleTransition(%q, %q) error must wrap ErrInvalidLifecycleTransition; got %v", from, to, err)
			}
		}
	}
}

// TestDriftDefinition_CanTransitionTo_AgreesWithValidator pins that
// the method on DriftDefinition and the package-level helper agree on
// every (from, to) pair.
func TestDriftDefinition_CanTransitionTo_AgreesWithValidator(t *testing.T) {
	all := []DriftDefinitionStatus{
		DriftDefinitionStatusDraft,
		DriftDefinitionStatusReview,
		DriftDefinitionStatusActive,
		DriftDefinitionStatusDeprecated,
		DriftDefinitionStatusRetired,
	}

	for _, from := range all {
		d := &DriftDefinition{Status: from}
		for _, to := range all {
			methodSays := d.CanTransitionTo(to)
			validatorSays := ValidateLifecycleTransition(from, to) == nil
			if methodSays != validatorSays {
				t.Errorf("disagreement for (%q → %q): method=%v validator=%v",
					from, to, methodSays, validatorSays)
			}
		}
	}
}

// TestRetired_HasNoOutgoingTransitions pins the terminal posture of
// the retired status.
func TestRetired_HasNoOutgoingTransitions(t *testing.T) {
	for _, to := range []DriftDefinitionStatus{
		DriftDefinitionStatusDraft,
		DriftDefinitionStatusReview,
		DriftDefinitionStatusActive,
		DriftDefinitionStatusDeprecated,
		DriftDefinitionStatusRetired,
	} {
		err := ValidateLifecycleTransition(DriftDefinitionStatusRetired, to)
		if err == nil {
			t.Errorf("retired must not transition to %q, but ValidateLifecycleTransition returned nil", to)
		}
	}
}
