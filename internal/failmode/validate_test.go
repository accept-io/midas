package failmode

import (
	"strings"
	"testing"
	"time"
)

// closedOnlyRulesNoAxes returns a fresh slice of the five exhaustive rules
// in the pre-D29b shape: only CorrectnessClass + PermittedMode (+ optional
// Reason). Used to pin backward-compat behaviour — these rules must still
// validate after D29b's three-axis admittance lands, because the axis-aware
// defaults fill EnforcementState=evidence_only and Outcome=escalate
// non-mutatingly during Validate.
//
// Renamed from validClosedOnlyRules (pre-D29b) to make the intent
// unambiguous: this fixture exercises the closed-only-rules-no-explicit-
// axes case, not "the only valid shape".
func closedOnlyRulesNoAxes() []FailModePolicyRule {
	return []FailModePolicyRule{
		{CorrectnessClass: CorrectnessClassGovernanceIntegrity, PermittedMode: PermittedModeClosed},
		{CorrectnessClass: CorrectnessClassPersistence, PermittedMode: PermittedModeClosed},
		{CorrectnessClass: CorrectnessClassInput, PermittedMode: PermittedModeNotApplicable},
		{CorrectnessClass: CorrectnessClassResource, PermittedMode: PermittedModeClosed, Reason: "policy evaluator unreachable"},
		{CorrectnessClass: CorrectnessClassConsistency, PermittedMode: PermittedModeClosed},
	}
}

// mixedAxisRules returns a fresh slice of five exhaustive rules exercising
// the three-axis matrix: a mix of closed/soft/open postures at various
// enforcement states with axis-coherent outcomes.
func mixedAxisRules() []FailModePolicyRule {
	return []FailModePolicyRule{
		{CorrectnessClass: CorrectnessClassGovernanceIntegrity, PermittedMode: PermittedModeClosed,
			EnforcementState: EnforcementStateEnforced, Outcome: OutcomeDeny,
			Reason: "governance integrity cannot be relaxed"},
		{CorrectnessClass: CorrectnessClassPersistence, PermittedMode: PermittedModeClosed,
			EnforcementState: EnforcementStateDryRun, Outcome: OutcomeEscalate},
		{CorrectnessClass: CorrectnessClassInput, PermittedMode: PermittedModeNotApplicable,
			EnforcementState: EnforcementStateEvidenceOnly, Outcome: OutcomeEscalate},
		{CorrectnessClass: CorrectnessClassResource, PermittedMode: PermittedModeSoft,
			EnforcementState: EnforcementStateEnforced, Outcome: OutcomePermitWithEvidence,
			Reason: "policy evaluator can fail soft on resource exhaustion"},
		{CorrectnessClass: CorrectnessClassConsistency, PermittedMode: PermittedModeOpen,
			EnforcementState: EnforcementStateDryRun, Outcome: OutcomePermitWithEvidence},
	}
}

// validPolicy returns a freshly constructed FailModePolicy whose Rules
// are closed-only-no-axes. Tests mutate one field at a time and re-call
// Validate to assert each rejection case. Closed-only is the D29b
// backward-compat baseline: the same fixture validates pre- and
// post-D29b because the axis-aware defaults fill EnforcementState and
// Outcome non-mutatingly.
func validPolicy() *FailModePolicy {
	now := time.Now().UTC()
	return &FailModePolicy{
		ID:             "default-fail-mode",
		Version:        1,
		Name:           "Default Fail-Mode Policy",
		Description:    "Closed-only baseline.",
		Status:         FailModePolicyStatusReview,
		EffectiveDate:  now,
		BusinessOwner:  "owner@example.com",
		TechnicalOwner: "platform-team",
		Rules:          closedOnlyRulesNoAxes(),
		Origin:         "manual",
		Managed:        true,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "user-1",
	}
}

// TestValidate_AcceptsClosedOnlyPolicy_BackwardCompat pins the D29b
// backward-compat guarantee: pre-D29b closed-only-no-axes policies
// continue to validate without operator intervention. The defaulting
// path in failmode.Validate fills the omitted axis fields against a
// non-mutating copy.
func TestValidate_AcceptsClosedOnlyPolicy_BackwardCompat(t *testing.T) {
	p := validPolicy()
	if errs := Validate(p); len(errs) != 0 {
		t.Fatalf("closed-only-no-axes policy must remain valid under D29b; got: %v", errs)
	}
	// Validate must NOT mutate the input rules.
	for i, r := range p.Rules {
		if r.EnforcementState != "" {
			t.Errorf("Validate must not mutate Rules[%d].EnforcementState; got %q", i, r.EnforcementState)
		}
		if r.Outcome != "" {
			t.Errorf("Validate must not mutate Rules[%d].Outcome; got %q", i, r.Outcome)
		}
	}
}

func TestValidate_RejectsNilPolicy(t *testing.T) {
	errs := Validate(nil)
	if len(errs) != 1 {
		t.Fatalf("nil policy should produce exactly one error; got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "must not be nil") {
		t.Errorf("error should mention nil; got %q", errs[0].Error())
	}
}

// validatorRejectionCase describes one rejection scenario. The mutator is
// applied to a fresh validPolicy() and Validate is then expected to return
// at least one error containing wantSubstring.
type validatorRejectionCase struct {
	name          string
	mutate        func(p *FailModePolicy)
	wantSubstring string
}

func runRejectionCases(t *testing.T, cases []validatorRejectionCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := validPolicy()
			tc.mutate(p)
			errs := Validate(p)
			if len(errs) == 0 {
				t.Fatalf("expected rejection for %q; got no errors", tc.name)
			}
			found := false
			for _, e := range errs {
				if strings.Contains(e.Error(), tc.wantSubstring) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected an error containing %q; got: %v", tc.wantSubstring, errs)
			}
		})
	}
}

func TestValidate_FieldRejections(t *testing.T) {
	cases := []validatorRejectionCase{
		{
			name:          "missing ID",
			mutate:        func(p *FailModePolicy) { p.ID = "" },
			wantSubstring: "ID must not be empty",
		},
		{
			name:          "non-kebab-case ID",
			mutate:        func(p *FailModePolicy) { p.ID = "Default_Fail_Mode" },
			wantSubstring: "must be kebab-case",
		},
		{
			name:          "Version zero",
			mutate:        func(p *FailModePolicy) { p.Version = 0 },
			wantSubstring: "Version must be > 0",
		},
		{
			name:          "Version negative",
			mutate:        func(p *FailModePolicy) { p.Version = -1 },
			wantSubstring: "Version must be > 0",
		},
		{
			name:          "missing Name",
			mutate:        func(p *FailModePolicy) { p.Name = "" },
			wantSubstring: "Name must not be empty",
		},
		{
			name:          "missing BusinessOwner",
			mutate:        func(p *FailModePolicy) { p.BusinessOwner = "" },
			wantSubstring: "BusinessOwner must not be empty",
		},
		{
			name:          "missing TechnicalOwner",
			mutate:        func(p *FailModePolicy) { p.TechnicalOwner = "" },
			wantSubstring: "TechnicalOwner must not be empty",
		},
		{
			name:          "unknown Status",
			mutate:        func(p *FailModePolicy) { p.Status = FailModePolicyStatus("garbage") },
			wantSubstring: "Status",
		},
		{
			name:          "empty Status",
			mutate:        func(p *FailModePolicy) { p.Status = FailModePolicyStatus("") },
			wantSubstring: "Status",
		},
		{
			name:          "zero EffectiveDate",
			mutate:        func(p *FailModePolicy) { p.EffectiveDate = time.Time{} },
			wantSubstring: "EffectiveDate must not be zero",
		},
		{
			name: "EffectiveUntil at EffectiveDate",
			mutate: func(p *FailModePolicy) {
				until := p.EffectiveDate
				p.EffectiveUntil = &until
			},
			wantSubstring: "EffectiveUntil must be after EffectiveDate",
		},
		{
			name: "EffectiveUntil before EffectiveDate",
			mutate: func(p *FailModePolicy) {
				until := p.EffectiveDate.Add(-time.Hour)
				p.EffectiveUntil = &until
			},
			wantSubstring: "EffectiveUntil must be after EffectiveDate",
		},
		{
			name: "RetiredAt before EffectiveDate",
			mutate: func(p *FailModePolicy) {
				retired := p.EffectiveDate.Add(-time.Hour)
				p.RetiredAt = &retired
			},
			wantSubstring: "RetiredAt must not be before EffectiveDate",
		},
		{
			name:          "missing rules",
			mutate:        func(p *FailModePolicy) { p.Rules = nil },
			wantSubstring: "rules must be exhaustive",
		},
		{
			name: "non-exhaustive rules (4 of 5)",
			mutate: func(p *FailModePolicy) {
				p.Rules = p.Rules[:4]
			},
			wantSubstring: "missing CorrectnessClass",
		},
		{
			name: "duplicate correctness class",
			mutate: func(p *FailModePolicy) {
				// Replace input with a second resource entry — leaves count at 5
				// but introduces a duplicate and drops input.
				p.Rules[2] = FailModePolicyRule{CorrectnessClass: CorrectnessClassResource, PermittedMode: PermittedModeClosed}
			},
			wantSubstring: "duplicated",
		},
		{
			name: "unknown correctness class",
			mutate: func(p *FailModePolicy) {
				p.Rules[0] = FailModePolicyRule{CorrectnessClass: CorrectnessClass("invalid"), PermittedMode: PermittedModeClosed}
			},
			wantSubstring: "unknown CorrectnessClass",
		},
		{
			name: "input with closed",
			mutate: func(p *FailModePolicy) {
				// Find the input rule and set it to closed.
				for i := range p.Rules {
					if p.Rules[i].CorrectnessClass == CorrectnessClassInput {
						p.Rules[i].PermittedMode = PermittedModeClosed
						break
					}
				}
			},
			wantSubstring: "must be \"not_applicable\"",
		},
		{
			name: "non-input class with not_applicable",
			mutate: func(p *FailModePolicy) {
				for i := range p.Rules {
					if p.Rules[i].CorrectnessClass == CorrectnessClassConsistency {
						p.Rules[i].PermittedMode = PermittedModeNotApplicable
						break
					}
				}
			},
			// D29b — wording changed from "must be \"closed\"" to the
			// three-option list because soft/open are now admitted for
			// non-input classes.
			wantSubstring: "must be one of closed|soft|open",
		},
		// D29b — replaced the three closed-only "not admitted" cases.
		// Soft and Open are now admitted as postures; rejection of input
		// + soft/open survives because the input class still requires
		// PermittedMode=not_applicable. The error wording changed from
		// "not admitted by D27j-impl-1a" to "must be \"not_applicable\"".
		{
			name: "input class with soft (rejected: input must be not_applicable)",
			mutate: func(p *FailModePolicy) {
				for i := range p.Rules {
					if p.Rules[i].CorrectnessClass == CorrectnessClassInput {
						p.Rules[i].PermittedMode = PermittedModeSoft
						break
					}
				}
			},
			wantSubstring: "must be \"not_applicable\"",
		},
		{
			name: "input class with open (rejected: input must be not_applicable)",
			mutate: func(p *FailModePolicy) {
				for i := range p.Rules {
					if p.Rules[i].CorrectnessClass == CorrectnessClassInput {
						p.Rules[i].PermittedMode = PermittedModeOpen
						break
					}
				}
			},
			wantSubstring: "must be \"not_applicable\"",
		},
		{
			name:          "unknown Origin",
			mutate:        func(p *FailModePolicy) { p.Origin = "auto" },
			wantSubstring: "Origin",
		},
		{
			name:          "empty Origin",
			mutate:        func(p *FailModePolicy) { p.Origin = "" },
			wantSubstring: "Origin",
		},
		{
			name:          "Replaces equals ID",
			mutate:        func(p *FailModePolicy) { p.Replaces = p.ID },
			wantSubstring: "no self-reference",
		},
		{
			name:          "zero CreatedAt",
			mutate:        func(p *FailModePolicy) { p.CreatedAt = time.Time{} },
			wantSubstring: "CreatedAt must not be zero",
		},
		{
			name:          "zero UpdatedAt",
			mutate:        func(p *FailModePolicy) { p.UpdatedAt = time.Time{} },
			wantSubstring: "UpdatedAt must not be zero",
		},
		{
			name:          "missing CreatedBy",
			mutate:        func(p *FailModePolicy) { p.CreatedBy = "" },
			wantSubstring: "CreatedBy must not be empty",
		},
	}

	runRejectionCases(t, cases)
}

// TestValidate_ReasonAllowedButNotRequired pins that reason text on a closed
// rule is accepted (validClosedOnlyRules sets one) and that absence of
// reason on a closed rule is also accepted.
func TestValidate_ReasonAllowedButNotRequired(t *testing.T) {
	p := validPolicy()
	// Strip reason from every rule.
	for i := range p.Rules {
		p.Rules[i].Reason = ""
	}
	if errs := Validate(p); len(errs) != 0 {
		t.Errorf("rules without Reason should still validate; got: %v", errs)
	}
}

// TestValidate_AllowsActivePolicyWithApprovalFields pins that an active
// policy with its approval fields populated validates successfully — the
// status enum is accepted independently of the chunk-1a-mandatory-field set.
func TestValidate_AllowsActivePolicyWithApprovalFields(t *testing.T) {
	p := validPolicy()
	p.Status = FailModePolicyStatusActive
	approvedAt := p.CreatedAt.Add(time.Hour)
	p.ApprovedBy = "approver-1"
	p.ApprovedAt = &approvedAt
	if errs := Validate(p); len(errs) != 0 {
		t.Errorf("active policy should validate; got: %v", errs)
	}
}

// ---------------------------------------------------------------------------
// D29b — three-axis admittance + matrix tests
//
// These tests replace the closed-only "soft/open not admitted" rejection
// cases removed above. The matrix table enumerates every (posture,
// enforcement_state, outcome) combination called out in the brief's
// approved implementation direction and asserts the validator outcome
// for each on a single rule swapped into the resource class. The five
// scenario tests directly above the matrix exercise the most common
// shapes a documentation reader would expect to find.
// ---------------------------------------------------------------------------

// TestValidate_AcceptsMixedAxisPolicy pins that a policy with explicit,
// coherent three-axis rules across closed/soft/open postures and
// evidence_only/dry_run/enforced enforcement states validates without
// error.
func TestValidate_AcceptsMixedAxisPolicy(t *testing.T) {
	p := validPolicy()
	p.Rules = mixedAxisRules()
	if errs := Validate(p); len(errs) != 0 {
		t.Fatalf("mixed-axis policy must validate; got: %v", errs)
	}
}

// TestValidate_AppliesAxisAwareDefaultsNonMutating pins that omitted
// enforcement_state and outcome fields are treated as evidence_only +
// posture-implied outcome by Validate, and that Validate does not
// mutate the caller's rule slice.
func TestValidate_AppliesAxisAwareDefaultsNonMutating(t *testing.T) {
	cases := []struct {
		name        string
		mode        PermittedMode
		wantState   EnforcementState
		wantOutcome Outcome
	}{
		{"closed posture defaults", PermittedModeClosed, EnforcementStateEvidenceOnly, OutcomeEscalate},
		{"soft posture defaults", PermittedModeSoft, EnforcementStateEvidenceOnly, OutcomePermitWithEvidence},
		{"open posture defaults", PermittedModeOpen, EnforcementStateEvidenceOnly, OutcomePermitWithEvidence},
		{"not_applicable posture defaults (input only)", PermittedModeNotApplicable, EnforcementStateEvidenceOnly, OutcomeEscalate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := FailModePolicyRule{
				CorrectnessClass: CorrectnessClassResource,
				PermittedMode:    tc.mode,
			}
			if tc.mode == PermittedModeNotApplicable {
				r.CorrectnessClass = CorrectnessClassInput
			}
			defaulted := ApplyRuleAxisDefaults(r)
			if defaulted.EnforcementState != tc.wantState {
				t.Errorf("EnforcementState default: want %q, got %q", tc.wantState, defaulted.EnforcementState)
			}
			if defaulted.Outcome != tc.wantOutcome {
				t.Errorf("Outcome default: want %q, got %q", tc.wantOutcome, defaulted.Outcome)
			}
			// Non-mutating contract: the input rule must remain at its
			// original (zero) axis values.
			if r.EnforcementState != "" {
				t.Errorf("ApplyRuleAxisDefaults must not mutate input EnforcementState; got %q", r.EnforcementState)
			}
			if r.Outcome != "" {
				t.Errorf("ApplyRuleAxisDefaults must not mutate input Outcome; got %q", r.Outcome)
			}
		})
	}
}

// TestValidate_AcceptsSoftOnResource_EvidenceOnly is the named positive
// replacement for the deleted closed-only "non-input class with soft
// (rejected by closed-only)" case.
func TestValidate_AcceptsSoftOnResource_EvidenceOnly(t *testing.T) {
	p := validPolicy()
	for i := range p.Rules {
		if p.Rules[i].CorrectnessClass == CorrectnessClassResource {
			p.Rules[i].PermittedMode = PermittedModeSoft
			p.Rules[i].EnforcementState = EnforcementStateEvidenceOnly
			break
		}
	}
	if errs := Validate(p); len(errs) != 0 {
		t.Errorf("soft + evidence_only on resource must validate; got: %v", errs)
	}
}

// TestValidate_AcceptsOpenOnResource_EvidenceOnly is the named positive
// replacement for the deleted closed-only "non-input class with open
// (rejected by closed-only)" case.
func TestValidate_AcceptsOpenOnResource_EvidenceOnly(t *testing.T) {
	p := validPolicy()
	for i := range p.Rules {
		if p.Rules[i].CorrectnessClass == CorrectnessClassResource {
			p.Rules[i].PermittedMode = PermittedModeOpen
			p.Rules[i].EnforcementState = EnforcementStateEvidenceOnly
			break
		}
	}
	if errs := Validate(p); len(errs) != 0 {
		t.Errorf("open + evidence_only on resource must validate; got: %v", errs)
	}
}

// TestValidate_ThreeAxisMatrix is a table-driven enumeration of the
// (posture, enforcement_state, outcome) combinations called out in the
// brief. The class is always resource (the non-input slot in the
// fixture) so the matrix table reflects pure axis interaction rather
// than class-shape constraints.
func TestValidate_ThreeAxisMatrix(t *testing.T) {
	cases := []struct {
		name    string
		mode    PermittedMode
		state   EnforcementState
		outcome Outcome
		want    bool // true = expect valid; false = expect rejection
	}{
		// closed posture
		{"closed + evidence_only + escalate", PermittedModeClosed, EnforcementStateEvidenceOnly, OutcomeEscalate, true},
		{"closed + evidence_only + permit_with_evidence (ignored under evidence_only)", PermittedModeClosed, EnforcementStateEvidenceOnly, OutcomePermitWithEvidence, true},
		{"closed + dry_run + deny", PermittedModeClosed, EnforcementStateDryRun, OutcomeDeny, true},
		{"closed + dry_run + escalate", PermittedModeClosed, EnforcementStateDryRun, OutcomeEscalate, true},
		{"closed + dry_run + manual_review", PermittedModeClosed, EnforcementStateDryRun, OutcomeManualReview, true},
		{"closed + dry_run + permit_with_evidence (forbidden)", PermittedModeClosed, EnforcementStateDryRun, OutcomePermitWithEvidence, false},
		{"closed + enforced + escalate", PermittedModeClosed, EnforcementStateEnforced, OutcomeEscalate, true},
		{"closed + enforced + permit_with_evidence (forbidden)", PermittedModeClosed, EnforcementStateEnforced, OutcomePermitWithEvidence, false},
		// soft posture
		{"soft + evidence_only + escalate (ignored)", PermittedModeSoft, EnforcementStateEvidenceOnly, OutcomeEscalate, true},
		{"soft + dry_run + permit_with_evidence", PermittedModeSoft, EnforcementStateDryRun, OutcomePermitWithEvidence, true},
		{"soft + dry_run + deny (forbidden)", PermittedModeSoft, EnforcementStateDryRun, OutcomeDeny, false},
		{"soft + enforced + permit_with_evidence", PermittedModeSoft, EnforcementStateEnforced, OutcomePermitWithEvidence, true},
		{"soft + enforced + deny (forbidden)", PermittedModeSoft, EnforcementStateEnforced, OutcomeDeny, false},
		{"soft + enforced + escalate (forbidden)", PermittedModeSoft, EnforcementStateEnforced, OutcomeEscalate, false},
		// open posture
		{"open + evidence_only + escalate (ignored)", PermittedModeOpen, EnforcementStateEvidenceOnly, OutcomeEscalate, true},
		{"open + dry_run + permit_with_evidence", PermittedModeOpen, EnforcementStateDryRun, OutcomePermitWithEvidence, true},
		{"open + enforced + permit_with_evidence", PermittedModeOpen, EnforcementStateEnforced, OutcomePermitWithEvidence, true},
		{"open + enforced + manual_review (forbidden)", PermittedModeOpen, EnforcementStateEnforced, OutcomeManualReview, false},
		{"open + enforced + deny (forbidden)", PermittedModeOpen, EnforcementStateEnforced, OutcomeDeny, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := validPolicy()
			for i := range p.Rules {
				if p.Rules[i].CorrectnessClass == CorrectnessClassResource {
					p.Rules[i].PermittedMode = tc.mode
					p.Rules[i].EnforcementState = tc.state
					p.Rules[i].Outcome = tc.outcome
					break
				}
			}
			errs := Validate(p)
			if tc.want && len(errs) != 0 {
				t.Errorf("expected valid; got errors: %v", errs)
			}
			if !tc.want && len(errs) == 0 {
				t.Errorf("expected rejection; got no errors")
			}
		})
	}
}

// TestValidate_NotApplicableMatrix pins the not_applicable-specific
// constraints: only valid for input correctness class; only valid under
// evidence_only.
func TestValidate_NotApplicableMatrix(t *testing.T) {
	cases := []struct {
		name  string
		class CorrectnessClass
		state EnforcementState
		want  bool
	}{
		{"not_applicable + input + evidence_only", CorrectnessClassInput, EnforcementStateEvidenceOnly, true},
		{"not_applicable + input + dry_run (forbidden)", CorrectnessClassInput, EnforcementStateDryRun, false},
		{"not_applicable + input + enforced (forbidden)", CorrectnessClassInput, EnforcementStateEnforced, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := validPolicy()
			for i := range p.Rules {
				if p.Rules[i].CorrectnessClass == tc.class {
					p.Rules[i].PermittedMode = PermittedModeNotApplicable
					p.Rules[i].EnforcementState = tc.state
					p.Rules[i].Outcome = OutcomeEscalate
					break
				}
			}
			errs := Validate(p)
			if tc.want && len(errs) != 0 {
				t.Errorf("expected valid; got errors: %v", errs)
			}
			if !tc.want && len(errs) == 0 {
				t.Errorf("expected rejection; got no errors")
			}
		})
	}
}

// TestValidate_RejectsNotApplicableOnNonInputClass replaces the prior
// closed-only "non-input class with not_applicable" rejection. The error
// message now references the soft/open option set: "must be one of
// closed|soft|open" rather than "must be closed".
func TestValidate_RejectsNotApplicableOnNonInputClass(t *testing.T) {
	p := validPolicy()
	for i := range p.Rules {
		if p.Rules[i].CorrectnessClass == CorrectnessClassConsistency {
			p.Rules[i].PermittedMode = PermittedModeNotApplicable
			break
		}
	}
	errs := Validate(p)
	if len(errs) == 0 {
		t.Fatal("not_applicable on non-input class must be rejected")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "must be one of closed|soft|open") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error 'must be one of closed|soft|open'; got: %v", errs)
	}
}

// TestValidate_RejectsUnknownEnforcementState pins the enum membership
// check on EnforcementState.
func TestValidate_RejectsUnknownEnforcementState(t *testing.T) {
	p := validPolicy()
	for i := range p.Rules {
		if p.Rules[i].CorrectnessClass == CorrectnessClassResource {
			p.Rules[i].EnforcementState = EnforcementState("garbage")
			break
		}
	}
	errs := Validate(p)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "EnforcementState") &&
			strings.Contains(e.Error(), "evidence_only|dry_run|enforced") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected EnforcementState enum error; got: %v", errs)
	}
}

// TestValidate_RejectsUnknownOutcome pins the enum membership check on
// Outcome.
func TestValidate_RejectsUnknownOutcome(t *testing.T) {
	p := validPolicy()
	for i := range p.Rules {
		if p.Rules[i].CorrectnessClass == CorrectnessClassResource {
			p.Rules[i].Outcome = Outcome("garbage")
			break
		}
	}
	errs := Validate(p)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "Outcome") &&
			strings.Contains(e.Error(), "deny|escalate|permit_with_evidence|manual_review") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Outcome enum error; got: %v", errs)
	}
}
