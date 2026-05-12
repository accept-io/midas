package authority

import (
	"errors"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/value"
)

// ConstraintKind enumerates the five discrete constraint families a
// grant may carry. Each constraint kind reads exactly one shape of
// runtime input and produces a deterministic pass/fail decision; the
// constraint set on a grant is the conjunction of all of its
// constraints (all must pass for the grant to authorise the request).
//
// The kind set is fixed and exhaustive at MVP. Adding a new kind
// requires a domain change here, an OpenAPI enum addition, and an
// orchestrator branch.
//
//   - confidence_threshold_min   — request.Confidence must be at
//     least Constraint.MinConfidence. Tightens the profile's own
//     confidence threshold for this specific grant.
//   - consequence_threshold_max  — request consequence must NOT
//     exceed Constraint.MaxConsequence. Tightens the profile's own
//     consequence threshold for this specific grant.
//   - human_only                 — request.Agent.Type must be
//     operator. Use for capabilities that must not be exercised by
//     automated systems.
//   - ai_only                    — request.Agent.Type must be ai.
//     Mirror of human_only; mutually exclusive with it.
//   - time_window                — wall-clock now must lie in
//     [StartTime, EndTime). Tightens the grant's broad EffectiveDate /
//     ExpiresAt window to a narrower in-day or schedule-bounded
//     interval.
type ConstraintKind string

const (
	ConstraintKindConfidenceThresholdMin  ConstraintKind = "confidence_threshold_min"
	ConstraintKindConsequenceThresholdMax ConstraintKind = "consequence_threshold_max"
	ConstraintKindHumanOnly               ConstraintKind = "human_only"
	ConstraintKindAIOnly                  ConstraintKind = "ai_only"
	ConstraintKindTimeWindow              ConstraintKind = "time_window"
)

// ErrInvalidConstraintKind is wrapped for unknown Kind values.
var ErrInvalidConstraintKind = errors.New("invalid constraint kind")

// ErrInvalidConstraint is wrapped for shape-level malformed
// constraints (e.g. confidence_threshold_min with a value outside
// [0,1], time_window with EndTime not after StartTime).
var ErrInvalidConstraint = errors.New("invalid constraint")

// ErrDuplicateConstraintKind is wrapped when more than one
// constraint of the same kind appears in a grant's constraint slice.
// Per-kind uniqueness keeps semantics deterministic (the orchestrator
// never has to decide which of two confidence_threshold_min values
// wins).
var ErrDuplicateConstraintKind = errors.New("duplicate constraint kind")

// ErrConflictingConstraints is wrapped when the constraint set is
// internally inconsistent — currently only the {human_only, ai_only}
// pair, which cannot both pass for any single request.
var ErrConflictingConstraints = errors.New("conflicting constraints")

// Constraint is the typed value object for one grant constraint.
// Exactly the fields relevant to Kind are populated; other fields are
// zero. Mirrors the union-style pattern used by authority.Consequence.
//
// The wire shape (Authority Graph + OpenAPI) carries Kind plus the
// per-kind variant fields with omitempty so a Constraint encodes
// compactly. Persistence stores constraints as JSON via the GrantRepo.
type Constraint struct {
	Kind ConstraintKind

	// confidence_threshold_min variant: minimum allowed
	// request.Confidence. Must be in [0, 1].
	MinConfidence float64

	// consequence_threshold_max variant: the maximum allowed
	// consequence. The Type field must be set; Amount/Currency or
	// RiskRating is populated per the variant.
	MaxConsequence Consequence

	// time_window variant: half-open interval [StartTime, EndTime).
	// Both must be non-zero, and EndTime must be strictly after
	// StartTime.
	StartTime time.Time
	EndTime   time.Time
}

// IsValid reports whether k is one of the five canonical constraint
// kinds.
func (k ConstraintKind) IsValid() bool {
	switch k {
	case ConstraintKindConfidenceThresholdMin,
		ConstraintKindConsequenceThresholdMax,
		ConstraintKindHumanOnly,
		ConstraintKindAIOnly,
		ConstraintKindTimeWindow:
		return true
	default:
		return false
	}
}

// IsValidConstraintKind is the package-level alias for
// ConstraintKind.IsValid.
func IsValidConstraintKind(k ConstraintKind) bool {
	return k.IsValid()
}

// Validate returns nil when the Constraint's shape matches its Kind.
// Used by ValidateConstraints and persistence loaders to reject
// malformed records at the boundary.
func (c Constraint) Validate() error {
	if !c.Kind.IsValid() {
		return fmt.Errorf("%w: %q", ErrInvalidConstraintKind, string(c.Kind))
	}
	switch c.Kind {
	case ConstraintKindConfidenceThresholdMin:
		if c.MinConfidence < 0 || c.MinConfidence > 1 {
			return fmt.Errorf("%w: confidence_threshold_min %.4f out of [0,1]", ErrInvalidConstraint, c.MinConfidence)
		}
	case ConstraintKindConsequenceThresholdMax:
		if c.MaxConsequence.Type == "" {
			return fmt.Errorf("%w: consequence_threshold_max missing type", ErrInvalidConstraint)
		}
		switch c.MaxConsequence.Type {
		case value.ConsequenceTypeMonetary:
			if c.MaxConsequence.Currency == "" {
				return fmt.Errorf("%w: consequence_threshold_max monetary missing currency", ErrInvalidConstraint)
			}
		case value.ConsequenceTypeRiskRating:
			if c.MaxConsequence.RiskRating == "" {
				return fmt.Errorf("%w: consequence_threshold_max risk_rating missing risk_rating", ErrInvalidConstraint)
			}
		}
	case ConstraintKindHumanOnly, ConstraintKindAIOnly:
		// No payload — Kind alone carries the semantics.
	case ConstraintKindTimeWindow:
		if c.StartTime.IsZero() || c.EndTime.IsZero() {
			return fmt.Errorf("%w: time_window requires both start_time and end_time", ErrInvalidConstraint)
		}
		if !c.EndTime.After(c.StartTime) {
			return fmt.Errorf("%w: time_window end_time must be after start_time", ErrInvalidConstraint)
		}
	}
	return nil
}

// ValidateConstraints returns nil when cs is a well-formed grant
// constraint slice:
//
//   - every entry's Kind is canonical and its shape passes Validate;
//   - no two entries share the same Kind;
//   - the set is not internally contradictory (currently only
//     {human_only, ai_only} is rejected).
//
// Empty slices are valid.
func ValidateConstraints(cs []Constraint) error {
	seen := make(map[ConstraintKind]struct{}, len(cs))
	for _, c := range cs {
		if err := c.Validate(); err != nil {
			return err
		}
		if _, dup := seen[c.Kind]; dup {
			return fmt.Errorf("%w: %q", ErrDuplicateConstraintKind, string(c.Kind))
		}
		seen[c.Kind] = struct{}{}
	}
	if _, h := seen[ConstraintKindHumanOnly]; h {
		if _, a := seen[ConstraintKindAIOnly]; a {
			return fmt.Errorf("%w: human_only and ai_only cannot coexist", ErrConflictingConstraints)
		}
	}
	return nil
}

// HasConstraintKind reports whether any Constraint in cs has the
// requested Kind. O(n); n <= 5 by construction.
func HasConstraintKind(cs []Constraint, k ConstraintKind) bool {
	for _, c := range cs {
		if c.Kind == k {
			return true
		}
	}
	return false
}

// ConstraintInput is the runtime data the orchestrator passes to
// EvaluateConstraints. It deliberately avoids importing
// internal/decision types — the constraint package stays a pure
// domain helper.
type ConstraintInput struct {
	Confidence  float64
	Consequence *eval.Consequence
	AgentType   agent.AgentType
}

// ConstraintViolation describes the first constraint that failed.
// Kind identifies which constraint kind violated; Reason is a short
// human-readable message intended for the AUTHORITY_CONSTRAINT_VIOLATED
// audit event payload (not the operator's primary signal — the kind
// is). Returned by EvaluateConstraints.
type ConstraintViolation struct {
	Kind   ConstraintKind
	Reason string
}

// EvaluateConstraints walks cs in slice order and returns the first
// violation it encounters, or nil when every constraint passes. The
// orchestrator is responsible for emitting AUTHORITY_CONSTRAINT_VIOLATED
// and returning Reject; this function only computes the verdict.
//
// Slice order is the on-disk order — callers that need a stable
// per-kind evaluation order should sort cs before calling. The MVP
// orchestrator does not sort because the runtime decision must reject
// on ANY violation, so the first-violation-wins semantics matches.
func EvaluateConstraints(cs []Constraint, in ConstraintInput, now time.Time) *ConstraintViolation {
	for _, c := range cs {
		switch c.Kind {
		case ConstraintKindConfidenceThresholdMin:
			if in.Confidence < c.MinConfidence {
				return &ConstraintViolation{
					Kind:   c.Kind,
					Reason: fmt.Sprintf("confidence %.4f below grant constraint minimum %.4f", in.Confidence, c.MinConfidence),
				}
			}
		case ConstraintKindConsequenceThresholdMax:
			if ExceedsConsequenceThreshold(in.Consequence, c.MaxConsequence) {
				return &ConstraintViolation{
					Kind:   c.Kind,
					Reason: "request consequence exceeds grant constraint maximum",
				}
			}
		case ConstraintKindHumanOnly:
			if in.AgentType != agent.AgentTypeOperator {
				return &ConstraintViolation{
					Kind:   c.Kind,
					Reason: fmt.Sprintf("grant requires operator agent; got %q", string(in.AgentType)),
				}
			}
		case ConstraintKindAIOnly:
			if in.AgentType != agent.AgentTypeAI {
				return &ConstraintViolation{
					Kind:   c.Kind,
					Reason: fmt.Sprintf("grant requires ai agent; got %q", string(in.AgentType)),
				}
			}
		case ConstraintKindTimeWindow:
			if now.Before(c.StartTime) || !now.Before(c.EndTime) {
				return &ConstraintViolation{
					Kind:   c.Kind,
					Reason: fmt.Sprintf("current time %s outside grant constraint window [%s, %s)", now.UTC().Format(time.RFC3339), c.StartTime.UTC().Format(time.RFC3339), c.EndTime.UTC().Format(time.RFC3339)),
				}
			}
		}
	}
	return nil
}
