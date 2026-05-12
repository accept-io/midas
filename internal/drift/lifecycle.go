package drift

import (
	"errors"
	"fmt"
)

// ErrInvalidLifecycleTransition is returned when a requested
// DriftDefinitionStatus change does not follow the lifecycle
// progression. Mirrors governanceexpectation.ErrInvalidLifecycleTransition.
var ErrInvalidLifecycleTransition = errors.New("invalid lifecycle transition")

// allowedDefinitionTransitions enumerates the permitted status
// progressions for a DriftDefinition revision. The graph mirrors
// authority.AuthorityProfile / failmode.FailModePolicy:
//
//	draft      → review | retired
//	review     → active | draft | retired
//	active     → deprecated | retired
//	deprecated → retired
//	retired    → terminal
var allowedDefinitionTransitions = map[DriftDefinitionStatus][]DriftDefinitionStatus{
	DriftDefinitionStatusDraft: {
		DriftDefinitionStatusReview,
		DriftDefinitionStatusRetired,
	},
	DriftDefinitionStatusReview: {
		DriftDefinitionStatusActive,
		DriftDefinitionStatusDraft,
		DriftDefinitionStatusRetired,
	},
	DriftDefinitionStatusActive: {
		DriftDefinitionStatusDeprecated,
		DriftDefinitionStatusRetired,
	},
	DriftDefinitionStatusDeprecated: {
		DriftDefinitionStatusRetired,
	},
	// retired is terminal; no outgoing edges.
}

// CanTransitionTo reports whether a DriftDefinition revision may
// transition from its current Status to next. The companion
// package-level ValidateLifecycleTransition agrees with this method
// on every input.
func (d *DriftDefinition) CanTransitionTo(next DriftDefinitionStatus) bool {
	permitted, ok := allowedDefinitionTransitions[d.Status]
	if !ok {
		return false
	}
	for _, allowed := range permitted {
		if allowed == next {
			return true
		}
	}
	return false
}

// ValidateLifecycleTransition checks whether the requested status
// change is permitted by the DriftDefinition lifecycle model. Returns
// an error wrapping ErrInvalidLifecycleTransition when the transition
// is not permitted. Mirrors governanceexpectation.ValidateLifecycleTransition.
func ValidateLifecycleTransition(from, to DriftDefinitionStatus) error {
	permitted, ok := allowedDefinitionTransitions[from]
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

// RevisionTransitionOpKind discriminates the planned operations
// produced by RevisionTransitionPlan. Pure data; the persistence and
// control-plane tranches consume the plan and actually apply the
// operations.
type RevisionTransitionOpKind string

const (
	// RevisionTransitionOpDeprecate marks a deprecate operation on the
	// prior active revision. Emitted when a new revision is being
	// activated and a prior active revision exists.
	RevisionTransitionOpDeprecate RevisionTransitionOpKind = "deprecate"

	// RevisionTransitionOpActivate marks an activate operation on the
	// next revision.
	RevisionTransitionOpActivate RevisionTransitionOpKind = "activate"
)

// RevisionTransitionOp is one operation in a planned atomic
// revision transition. The (DefinitionID, Version) pair identifies the
// revision the operation targets.
type RevisionTransitionOp struct {
	Kind          RevisionTransitionOpKind
	DefinitionID  string
	Version       int
	TargetStatus  DriftDefinitionStatus
}

// RevisionTransitionPlan returns the operations that a later
// persistence or control-plane tranche must apply atomically when
// activating a new DriftDefinition revision. Pure helper — does NOT
// mutate either argument.
//
// Semantics:
//
//   - When prior is nil OR prior.Status != active, the plan is just
//     [activate(next)].
//
//   - When prior is non-nil AND prior.Status == active, the plan is
//     [deprecate(prior), activate(next)] — the prior active revision
//     must be deprecated as part of the same atomic transition that
//     activates the new revision.
//
//   - When prior and next have different IDs, the plan is still
//     produced (cross-ID transitions are unusual but not invalid at
//     this layer; admission validation is a later-tranche concern).
//
// Drift-1a tests the plan only; no persistence is exercised.
func RevisionTransitionPlan(prior, next *DriftDefinition) []RevisionTransitionOp {
	plan := make([]RevisionTransitionOp, 0, 2)
	if prior != nil && prior.Status == DriftDefinitionStatusActive {
		plan = append(plan, RevisionTransitionOp{
			Kind:         RevisionTransitionOpDeprecate,
			DefinitionID: prior.ID,
			Version:      prior.Version,
			TargetStatus: DriftDefinitionStatusDeprecated,
		})
	}
	if next != nil {
		plan = append(plan, RevisionTransitionOp{
			Kind:         RevisionTransitionOpActivate,
			DefinitionID: next.ID,
			Version:      next.Version,
			TargetStatus: DriftDefinitionStatusActive,
		})
	}
	return plan
}
