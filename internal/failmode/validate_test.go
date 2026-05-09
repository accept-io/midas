package failmode

import (
	"strings"
	"testing"
	"time"
)

// validClosedOnlyRules returns a fresh slice of the five exhaustive rules
// that satisfy the D27j-impl-1a closed-only invariant: input is
// not_applicable, the four other classes are closed.
func validClosedOnlyRules() []FailModePolicyRule {
	return []FailModePolicyRule{
		{CorrectnessClass: CorrectnessClassGovernanceIntegrity, PermittedMode: PermittedModeClosed},
		{CorrectnessClass: CorrectnessClassPersistence, PermittedMode: PermittedModeClosed},
		{CorrectnessClass: CorrectnessClassInput, PermittedMode: PermittedModeNotApplicable},
		{CorrectnessClass: CorrectnessClassResource, PermittedMode: PermittedModeClosed, Reason: "policy evaluator unreachable"},
		{CorrectnessClass: CorrectnessClassConsistency, PermittedMode: PermittedModeClosed},
	}
}

// validPolicy returns a freshly constructed FailModePolicy that satisfies
// every closed-only invariant. Tests mutate one field at a time and re-call
// Validate to assert each rejection case.
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
		Rules:          validClosedOnlyRules(),
		Origin:         "manual",
		Managed:        true,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "user-1",
	}
}

func TestValidate_AcceptsValidClosedOnlyPolicy(t *testing.T) {
	p := validPolicy()
	if errs := Validate(p); len(errs) != 0 {
		t.Fatalf("valid policy should produce no errors; got: %v", errs)
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
			wantSubstring: "must be \"closed\"",
		},
		{
			name: "non-input class with soft (rejected by closed-only)",
			mutate: func(p *FailModePolicy) {
				for i := range p.Rules {
					if p.Rules[i].CorrectnessClass == CorrectnessClassResource {
						p.Rules[i].PermittedMode = PermittedModeSoft
						break
					}
				}
			},
			wantSubstring: "not admitted by D27j-impl-1a",
		},
		{
			name: "non-input class with open (rejected by closed-only)",
			mutate: func(p *FailModePolicy) {
				for i := range p.Rules {
					if p.Rules[i].CorrectnessClass == CorrectnessClassResource {
						p.Rules[i].PermittedMode = PermittedModeOpen
						break
					}
				}
			},
			wantSubstring: "not admitted by D27j-impl-1a",
		},
		{
			name: "input class with soft (rejected by closed-only)",
			mutate: func(p *FailModePolicy) {
				for i := range p.Rules {
					if p.Rules[i].CorrectnessClass == CorrectnessClassInput {
						p.Rules[i].PermittedMode = PermittedModeSoft
						break
					}
				}
			},
			wantSubstring: "not admitted by D27j-impl-1a",
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
