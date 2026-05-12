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

// validEnforcementStates is the closed enumeration accepted by Validate
// for Axis B. The default for omitted / zero values is evidence_only and
// is applied non-mutatingly inside Validate (against a copy).
var validEnforcementStates = map[EnforcementState]struct{}{
	EnforcementStateEvidenceOnly: {},
	EnforcementStateDryRun:       {},
	EnforcementStateEnforced:     {},
}

// validOutcomes is the closed enumeration accepted by Validate for Axis C.
// The default for omitted / zero values is posture-implied and is applied
// non-mutatingly inside Validate.
var validOutcomes = map[Outcome]struct{}{
	OutcomeDeny:               {},
	OutcomeEscalate:           {},
	OutcomePermitWithEvidence: {},
	OutcomeManualReview:       {},
}

// posturePermitsOutcome enumerates the per-posture Outcome subsets that
// are coherent at dry_run or enforced enforcement states. Outcome is
// ignored while EnforcementState is evidence_only (the validator accepts
// any valid Outcome value there).
//
// Closed posture forbids permit_with_evidence: closed means "do not
// relax", so any outcome that admits proceeding is contradictory under
// active enforcement.
//
// Soft and Open postures admit only permit_with_evidence under
// dry_run/enforced: declaring soft or open and then producing a stricter
// outcome (deny/escalate/manual_review) is contradictory — the operator
// either changes posture or drops the relaxation.
//
// not_applicable does not appear here: not_applicable is structurally
// not permitted at dry_run/enforced (only evidence_only is admissible).
var posturePermitsOutcome = map[PermittedMode]map[Outcome]struct{}{
	PermittedModeClosed: {
		OutcomeDeny:         {},
		OutcomeEscalate:     {},
		OutcomeManualReview: {},
	},
	PermittedModeSoft: {
		OutcomePermitWithEvidence: {},
	},
	PermittedModeOpen: {
		OutcomePermitWithEvidence: {},
	},
}

// defaultOutcomeForPosture returns the axis-aware Outcome default for a
// given PermittedMode. The default is chosen so a freshly-declared rule
// remains coherent when the operator later moves the same rule from
// evidence_only to dry_run or enforced — that is, the default Outcome
// is always a valid combination with the posture under future
// enforcement states.
//
// For not_applicable the default is escalate: not_applicable is valid
// only with evidence_only, so the value is ignored at runtime, but a
// consistent default avoids surprising operator reads of "empty" outcome.
func defaultOutcomeForPosture(mode PermittedMode) Outcome {
	switch mode {
	case PermittedModeClosed:
		return OutcomeEscalate
	case PermittedModeSoft:
		return OutcomePermitWithEvidence
	case PermittedModeOpen:
		return OutcomePermitWithEvidence
	case PermittedModeNotApplicable:
		return OutcomeEscalate
	}
	return OutcomeEscalate
}

// ApplyRuleAxisDefaults returns a copy of r with omitted axis fields
// filled in. The function is non-mutating: callers can pass a rule
// loaded from any layer (document, JSONB, in-memory) and receive an
// effective rule for validation or comparison without changing the
// stored value.
//
// EnforcementState defaults to evidence_only when empty.
// Outcome defaults to defaultOutcomeForPosture(r.PermittedMode) when empty.
//
// This helper is exported so the Postgres repository can apply the same
// defaults during JSONB deserialisation; callers in the validator do
// the same via applyDefaultsToRules below.
func ApplyRuleAxisDefaults(r FailModePolicyRule) FailModePolicyRule {
	if r.EnforcementState == "" {
		r.EnforcementState = EnforcementStateEvidenceOnly
	}
	if r.Outcome == "" {
		r.Outcome = defaultOutcomeForPosture(r.PermittedMode)
	}
	return r
}

// applyDefaultsToRules returns a defaulted copy of the rule slice. Used
// internally by Validate so callers' input rules are never mutated.
func applyDefaultsToRules(rs []FailModePolicyRule) []FailModePolicyRule {
	out := make([]FailModePolicyRule, len(rs))
	for i, r := range rs {
		out[i] = ApplyRuleAxisDefaults(r)
	}
	return out
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

// validateRules enforces the three-axis matrix on the rule set:
//   - exhaustive over the five CorrectnessClass values
//   - no duplicates
//   - no unknown classes
//   - input  → not_applicable (only valid posture for input)
//   - others → closed | soft | open (not_applicable forbidden)
//   - EnforcementState ∈ {evidence_only, dry_run, enforced}
//   - Outcome ∈ {deny, escalate, permit_with_evidence, manual_review}
//   - posture × enforcement_state × outcome combination is permitted by
//     the matrix in validateAxisMatrix
//
// validateRules operates on a defaulted copy of the input slice; the
// caller's rules are never mutated. Returned errors are appended to the
// parent Validate slice.
func validateRules(rules []FailModePolicyRule) []error {
	var errs []error

	defaulted := applyDefaultsToRules(rules)

	seen := make(map[CorrectnessClass]bool, len(allCorrectnessClasses))
	known := make(map[CorrectnessClass]struct{}, len(allCorrectnessClasses))
	for _, c := range allCorrectnessClasses {
		known[c] = struct{}{}
	}

	for _, r := range defaulted {
		if _, ok := known[r.CorrectnessClass]; !ok {
			errs = append(errs, fmt.Errorf("rule has unknown CorrectnessClass %q", r.CorrectnessClass))
			continue
		}
		if seen[r.CorrectnessClass] {
			errs = append(errs, fmt.Errorf("rule for CorrectnessClass %q is duplicated", r.CorrectnessClass))
			continue
		}
		seen[r.CorrectnessClass] = true

		errs = append(errs, validateAxisMatrix(r)...)
	}

	if len(defaulted) != len(allCorrectnessClasses) {
		errs = append(errs, fmt.Errorf("rules must be exhaustive over %d correctness classes; got %d entries", len(allCorrectnessClasses), len(defaulted)))
	}

	for _, c := range allCorrectnessClasses {
		if !seen[c] {
			errs = append(errs, fmt.Errorf("rules missing CorrectnessClass %q", c))
		}
	}

	return errs
}

// validateAxisMatrix enforces the three-axis combination matrix for a
// single rule. The rule is assumed to be defaulted (callers default
// before invoking).
//
// Per-axis enum membership is checked first. Cross-axis constraints:
//
//   - input correctness class requires PermittedMode = not_applicable.
//   - non-input correctness classes forbid PermittedMode = not_applicable.
//   - not_applicable is only valid with EnforcementState = evidence_only.
//   - closed + dry_run/enforced + permit_with_evidence is forbidden
//     (closed means "do not relax").
//   - soft  + dry_run/enforced + outcome != permit_with_evidence is
//     forbidden (soft cannot produce a stricter outcome under active
//     enforcement).
//   - open  + dry_run/enforced + outcome != permit_with_evidence is
//     forbidden (open cannot produce a stricter outcome under active
//     enforcement).
//
// Outcome is intentionally NOT cross-validated under evidence_only: the
// declared Outcome is preserved so the operator can later flip
// EnforcementState without re-editing every rule. Coherence under
// dry_run/enforced is enforced as soon as the operator selects those
// states.
func validateAxisMatrix(r FailModePolicyRule) []error {
	var errs []error

	if _, ok := validEnforcementStates[r.EnforcementState]; !ok {
		errs = append(errs, fmt.Errorf("EnforcementState for CorrectnessClass %q is not one of evidence_only|dry_run|enforced; got %q",
			r.CorrectnessClass, r.EnforcementState))
		// Continue to other checks so the caller sees the full set.
	}

	if _, ok := validOutcomes[r.Outcome]; !ok {
		errs = append(errs, fmt.Errorf("Outcome for CorrectnessClass %q is not one of deny|escalate|permit_with_evidence|manual_review; got %q",
			r.CorrectnessClass, r.Outcome))
	}

	if r.CorrectnessClass == CorrectnessClassInput {
		if r.PermittedMode != PermittedModeNotApplicable {
			errs = append(errs, fmt.Errorf("PermittedMode for CorrectnessClass %q must be %q, got %q",
				r.CorrectnessClass, PermittedModeNotApplicable, r.PermittedMode))
		}
	} else {
		switch r.PermittedMode {
		case PermittedModeClosed, PermittedModeSoft, PermittedModeOpen:
			// valid for non-input
		case PermittedModeNotApplicable:
			errs = append(errs, fmt.Errorf("PermittedMode for CorrectnessClass %q must be one of closed|soft|open (not_applicable is only valid for input); got %q",
				r.CorrectnessClass, r.PermittedMode))
		default:
			errs = append(errs, fmt.Errorf("PermittedMode for CorrectnessClass %q is not one of closed|soft|open|not_applicable; got %q",
				r.CorrectnessClass, r.PermittedMode))
		}
	}

	// not_applicable is only admissible under evidence_only.
	if r.PermittedMode == PermittedModeNotApplicable &&
		r.EnforcementState != EnforcementStateEvidenceOnly {
		errs = append(errs, fmt.Errorf("PermittedMode %q on CorrectnessClass %q is only valid with EnforcementState=evidence_only; got EnforcementState=%q",
			r.PermittedMode, r.CorrectnessClass, r.EnforcementState))
	}

	// Outcome is ignored while evidence_only — accept any valid value.
	if r.EnforcementState == EnforcementStateEvidenceOnly {
		return errs
	}

	// Under dry_run / enforced, the (posture, outcome) pair must be in
	// the per-posture admissible set. not_applicable has no entry — the
	// not_applicable-only-evidence-only check above already rejected
	// this combination.
	allowed, ok := posturePermitsOutcome[r.PermittedMode]
	if !ok {
		return errs
	}
	if _, permitted := allowed[r.Outcome]; !permitted {
		errs = append(errs, fmt.Errorf("Outcome %q is not permitted for PermittedMode %q under EnforcementState %q on CorrectnessClass %q",
			r.Outcome, r.PermittedMode, r.EnforcementState, r.CorrectnessClass))
	}

	return errs
}
