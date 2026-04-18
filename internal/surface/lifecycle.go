package surface

import (
	"context"
	"errors"
	"fmt"
)

// ErrInvalidLifecycleTransition is returned when a requested status change
// does not follow the defined surface lifecycle progression.
var ErrInvalidLifecycleTransition = errors.New("invalid lifecycle transition")

// allowedTransitions defines the complete set of permitted status changes
// for a DecisionSurface. Transitions not listed here are forbidden.
//
// Lifecycle graph:
//
//	draft → review        (submit for governance review)
//	review → active       (approved; becomes usable by agents)
//	review → draft        (returned for revision)
//	active → deprecated   (superseded; still operational)
//	draft → retired       (cancelled before activation)
//	deprecated → retired  (fully retired)
var allowedTransitions = map[SurfaceStatus][]SurfaceStatus{
	SurfaceStatusDraft: {
		SurfaceStatusReview,
		SurfaceStatusRetired,
	},
	SurfaceStatusReview: {
		SurfaceStatusActive,
		SurfaceStatusDraft,
	},
	SurfaceStatusActive: {
		SurfaceStatusDeprecated,
	},
	SurfaceStatusDeprecated: {
		SurfaceStatusRetired,
	},
	SurfaceStatusRetired: {}, // terminal state
}

// ValidateLifecycleTransition checks whether the requested status change is
// permitted by the surface lifecycle model. It returns ErrInvalidLifecycleTransition
// if the transition is not allowed.
//
// This is a package-level function so it can be called without constructing
// a validator. It performs pure state logic with no repository access.
func ValidateLifecycleTransition(from, to SurfaceStatus) error {
	permitted, ok := allowedTransitions[from]
	if !ok {
		return fmt.Errorf("%w: unknown status %q", ErrInvalidLifecycleTransition, from)
	}
	for _, allowed := range permitted {
		if allowed == to {
			return nil
		}
	}
	return fmt.Errorf("%w: %s → %s is not a valid progression", ErrInvalidLifecycleTransition, from, to)
}

// ---------------------------------------------------------------------------
// DefaultSurfaceValidator
// ---------------------------------------------------------------------------

// DefaultSurfaceValidator is the standard implementation of SurfaceValidator.
// ValidateSurface enforces the Surface → Process invariant (I-1) and the
// structural field rules. ValidateContext and ValidateConsequence are left
// for a future implementation pass; ValidateTransition is fully enforced.
type DefaultSurfaceValidator struct{}

// NewDefaultSurfaceValidator returns a ready-to-use DefaultSurfaceValidator.
func NewDefaultSurfaceValidator() *DefaultSurfaceValidator {
	return &DefaultSurfaceValidator{}
}

// ValidateSurface performs structural validation on a DecisionSurface.
func (v *DefaultSurfaceValidator) ValidateSurface(_ context.Context, s *DecisionSurface) error {
	if s == nil {
		return errors.New("surface must not be nil")
	}
	if s.ID == "" {
		return errors.New("surface id must not be empty")
	}
	if s.Name == "" {
		return errors.New("surface name must not be empty")
	}
	if s.Domain == "" {
		return errors.New("surface domain must not be empty")
	}
	if s.ProcessID == "" {
		return errors.New("surface process_id must not be empty")
	}
	if s.MinimumConfidence < 0.0 || s.MinimumConfidence > 1.0 {
		return fmt.Errorf("minimum_confidence must be in [0.0, 1.0], got %f", s.MinimumConfidence)
	}
	if s.AuditRetentionHours < 0 {
		return errors.New("audit_retention_hours must not be negative")
	}
	if s.AuditRetentionHours > 0 && s.AuditRetentionHours < 24 {
		return errors.New("audit_retention_hours must be 0 (system default) or >= 24")
	}
	return nil
}

// ValidateContext checks that a context map satisfies all required fields
// declared in the surface schema. Type and validation-rule checks are not
// yet implemented; this enforces required-key presence only.
func (v *DefaultSurfaceValidator) ValidateContext(_ context.Context, s *DecisionSurface, ctx map[string]any) error {
	if s == nil {
		return errors.New("surface must not be nil")
	}
	for _, field := range s.RequiredContext.Fields {
		if !field.Required {
			continue
		}
		if _, ok := ctx[field.Name]; !ok {
			return fmt.Errorf("required context field %q is missing", field.Name)
		}
	}
	return nil
}

// ValidateConsequence checks that a consequence conforms to the surface's
// declared consequence types. Full measure-type and bounds validation is
// not yet implemented; this enforces TypeID referential integrity only.
func (v *DefaultSurfaceValidator) ValidateConsequence(_ context.Context, s *DecisionSurface, c Consequence) error {
	if s == nil {
		return errors.New("surface must not be nil")
	}
	for _, ct := range s.ConsequenceTypes {
		if ct.ID == c.TypeID {
			return nil
		}
	}
	return fmt.Errorf("consequence type_id %q is not declared on surface %q", c.TypeID, s.ID)
}

// ValidateTransition checks whether a lifecycle status change is permitted.
// It delegates to ValidateLifecycleTransition for the authoritative rule set.
func (v *DefaultSurfaceValidator) ValidateTransition(_ context.Context, from, to SurfaceStatus) error {
	return ValidateLifecycleTransition(from, to)
}
