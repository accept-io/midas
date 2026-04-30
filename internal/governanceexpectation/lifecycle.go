package governanceexpectation

import (
	"errors"
	"fmt"
)

// ErrInvalidLifecycleTransition is returned when a requested status
// change does not follow the defined GovernanceExpectation lifecycle
// progression.
var ErrInvalidLifecycleTransition = errors.New("invalid lifecycle transition")

// allowedTransitions defines the complete set of permitted status
// changes for a GovernanceExpectation. Mirrors
// AuthorityProfile.CanTransitionTo:
//
//	review → active        (approval)
//	active → deprecated    (supersession)
//
// No retirement transitions, no draft↔review editing path, no
// deprecated → retired path. Deprecated expectations are kept
// indefinitely as historical record. The status constants for draft
// and retired exist in governanceexpectation.go for schema-CHECK
// alignment with other Kinds, but are not reachable through this
// transition graph.
var allowedTransitions = map[ExpectationStatus][]ExpectationStatus{
	ExpectationStatusReview: {
		ExpectationStatusActive,
	},
	ExpectationStatusActive: {
		ExpectationStatusDeprecated,
	},
	// draft, deprecated, retired have no outgoing transitions.
}

// ValidateLifecycleTransition checks whether the requested status
// change is permitted by the GovernanceExpectation lifecycle model.
// Returns an error wrapping ErrInvalidLifecycleTransition when the
// transition is not in the allowed set.
//
// The companion (*GovernanceExpectation).CanTransitionTo method agrees
// with this function on every input — see
// TestGovernanceExpectation_CanTransitionTo_AgreesWithValidator.
func ValidateLifecycleTransition(from, to ExpectationStatus) error {
	permitted, ok := allowedTransitions[from]
	if !ok {
		return fmt.Errorf("%w: no outgoing transitions from %q", ErrInvalidLifecycleTransition, from)
	}
	for _, allowed := range permitted {
		if allowed == to {
			return nil
		}
	}
	return fmt.Errorf("%w: %s → %s is not a valid progression", ErrInvalidLifecycleTransition, from, to)
}
