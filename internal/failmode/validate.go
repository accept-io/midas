package failmode

import (
	"errors"
	"fmt"
	"regexp"
)

// kebabCase matches a non-empty kebab-case identifier: lowercase letters or
// digits, separated by single hyphens. Same shape used by the project's
// other governed-resource ID conventions.
var kebabCase = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// allCorrectnessClasses lists the five classes the validator requires to be
// exhaustively present on every policy. Order is not significant — duplicate
// detection is performed independently.
var allCorrectnessClasses = []CorrectnessClass{
	CorrectnessClassGovernanceIntegrity,
	CorrectnessClassPersistence,
	CorrectnessClassInput,
	CorrectnessClassResource,
	CorrectnessClassConsistency,
}

// validStatuses is the closed enumeration accepted by Validate.
var validStatuses = map[FailModePolicyStatus]struct{}{
	FailModePolicyStatusDraft:      {},
	FailModePolicyStatusReview:     {},
	FailModePolicyStatusActive:     {},
	FailModePolicyStatusDeprecated: {},
	FailModePolicyStatusRetired:    {},
}

// validOrigins mirrors the schema CHECK constraint on fail_mode_policies.origin.
var validOrigins = map[string]struct{}{
	"manual":   {},
	"inferred": {},
}

// Validate returns the set of validation errors for p. A nil error slice
// (len == 0) indicates the policy is well-formed under the closed-only
// invariant.
//
// Callers (control-plane mappers, tests) invoke Validate explicitly.
// Repositories do not call Validate; they trust the caller to validate
// before persisting and rely on the database CHECK constraints as the
// integrity backstop.
//
// Closed-only invariant (D27j-impl-1a): rules for CorrectnessClassInput must
// carry PermittedModeNotApplicable; rules for the four other classes must
// carry PermittedModeClosed. PermittedModeSoft and PermittedModeOpen are
// rejected outright. Admitting them is a deliberate later change.
func Validate(p *FailModePolicy) []error {
	if p == nil {
		return []error{errors.New("policy must not be nil")}
	}

	var errs []error

	if p.ID == "" {
		errs = append(errs, errors.New("ID must not be empty"))
	} else if !kebabCase.MatchString(p.ID) {
		errs = append(errs, fmt.Errorf("ID %q must be kebab-case", p.ID))
	}

	if p.Version <= 0 {
		errs = append(errs, fmt.Errorf("Version must be > 0, got %d", p.Version))
	}

	if p.Name == "" {
		errs = append(errs, errors.New("Name must not be empty"))
	}

	if p.BusinessOwner == "" {
		errs = append(errs, errors.New("BusinessOwner must not be empty"))
	}

	if p.TechnicalOwner == "" {
		errs = append(errs, errors.New("TechnicalOwner must not be empty"))
	}

	if _, ok := validStatuses[p.Status]; !ok {
		errs = append(errs, fmt.Errorf("Status %q is not one of draft|review|active|deprecated|retired", p.Status))
	}

	if p.EffectiveDate.IsZero() {
		errs = append(errs, errors.New("EffectiveDate must not be zero"))
	}

	if p.EffectiveUntil != nil && !p.EffectiveDate.IsZero() && !p.EffectiveUntil.After(p.EffectiveDate) {
		errs = append(errs, errors.New("EffectiveUntil must be after EffectiveDate"))
	}

	if p.RetiredAt != nil && !p.EffectiveDate.IsZero() && p.RetiredAt.Before(p.EffectiveDate) {
		errs = append(errs, errors.New("RetiredAt must not be before EffectiveDate"))
	}

	errs = append(errs, validateRules(p.Rules)...)

	if _, ok := validOrigins[p.Origin]; !ok {
		errs = append(errs, fmt.Errorf("Origin %q is not one of manual|inferred", p.Origin))
	}

	if p.Replaces != "" && p.Replaces == p.ID {
		errs = append(errs, errors.New("Replaces must not equal ID (no self-reference)"))
	}

	if p.CreatedAt.IsZero() {
		errs = append(errs, errors.New("CreatedAt must not be zero"))
	}

	if p.UpdatedAt.IsZero() {
		errs = append(errs, errors.New("UpdatedAt must not be zero"))
	}

	if p.CreatedBy == "" {
		errs = append(errs, errors.New("CreatedBy must not be empty"))
	}

	return errs
}

// validateRules enforces the closed-only invariant on the rule set:
//   - exhaustive over the five CorrectnessClass values
//   - no duplicates
//   - no unknown classes
//   - input  → not_applicable
//   - others → closed
//
// Returned errors are appended to the parent Validate slice.
func validateRules(rules []FailModePolicyRule) []error {
	var errs []error

	seen := make(map[CorrectnessClass]bool, len(allCorrectnessClasses))
	known := make(map[CorrectnessClass]struct{}, len(allCorrectnessClasses))
	for _, c := range allCorrectnessClasses {
		known[c] = struct{}{}
	}

	for _, r := range rules {
		if _, ok := known[r.CorrectnessClass]; !ok {
			errs = append(errs, fmt.Errorf("rule has unknown CorrectnessClass %q", r.CorrectnessClass))
			continue
		}
		if seen[r.CorrectnessClass] {
			errs = append(errs, fmt.Errorf("rule for CorrectnessClass %q is duplicated", r.CorrectnessClass))
			continue
		}
		seen[r.CorrectnessClass] = true

		errs = append(errs, validatePermittedModeForClass(r.CorrectnessClass, r.PermittedMode)...)
	}

	if len(rules) != len(allCorrectnessClasses) {
		errs = append(errs, fmt.Errorf("rules must be exhaustive over %d correctness classes; got %d entries", len(allCorrectnessClasses), len(rules)))
	}

	for _, c := range allCorrectnessClasses {
		if !seen[c] {
			errs = append(errs, fmt.Errorf("rules missing CorrectnessClass %q", c))
		}
	}

	return errs
}

// validatePermittedModeForClass enforces the closed-only invariant for a
// single (class, mode) pair. Soft and Open are rejected with class-specific
// messages so callers can differentiate "not allowed yet" from "wrong mode
// for this class".
func validatePermittedModeForClass(class CorrectnessClass, mode PermittedMode) []error {
	switch mode {
	case PermittedModeSoft:
		return []error{fmt.Errorf("PermittedMode %q is not admitted by D27j-impl-1a (closed-only); class %q", mode, class)}
	case PermittedModeOpen:
		return []error{fmt.Errorf("PermittedMode %q is not admitted by D27j-impl-1a (closed-only); class %q", mode, class)}
	}

	if class == CorrectnessClassInput {
		if mode != PermittedModeNotApplicable {
			return []error{fmt.Errorf("PermittedMode for CorrectnessClass %q must be %q, got %q", class, PermittedModeNotApplicable, mode)}
		}
		return nil
	}

	if mode != PermittedModeClosed {
		return []error{fmt.Errorf("PermittedMode for CorrectnessClass %q must be %q, got %q", class, PermittedModeClosed, mode)}
	}
	return nil
}
