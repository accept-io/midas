package governanceexpectation

import (
	"errors"
	"testing"
)

// allStatuses is the canonical 5-element slice used by the matrix tests
// to assert behaviour for every from→to pair. Order is intentional but
// not load-bearing.
var allStatuses = []ExpectationStatus{
	ExpectationStatusDraft,
	ExpectationStatusReview,
	ExpectationStatusActive,
	ExpectationStatusDeprecated,
	ExpectationStatusRetired,
}

// validTransitions enumerates the only two valid lifecycle transitions.
// Any drift in CanTransitionTo / ValidateLifecycleTransition / the
// allowedTransitions map will fail one of the tests below.
var validTransitions = map[ExpectationStatus]ExpectationStatus{
	ExpectationStatusReview: ExpectationStatusActive,
	ExpectationStatusActive: ExpectationStatusDeprecated,
}

// TestValidateLifecycleTransition asserts the package-level transition
// validator returns nil for exactly the two permitted transitions and
// an error wrapping ErrInvalidLifecycleTransition for every other
// from→to pair across the 5×5 matrix.
func TestValidateLifecycleTransition(t *testing.T) {
	for _, from := range allStatuses {
		for _, to := range allStatuses {
			from, to := from, to
			t.Run(string(from)+"→"+string(to), func(t *testing.T) {
				err := ValidateLifecycleTransition(from, to)

				expected, isValid := validTransitions[from]
				wantNil := isValid && expected == to

				if wantNil {
					if err != nil {
						t.Fatalf("ValidateLifecycleTransition(%q, %q): want nil, got %v",
							from, to, err)
					}
					return
				}

				if err == nil {
					t.Fatalf("ValidateLifecycleTransition(%q, %q): want error, got nil",
						from, to)
				}
				if !errors.Is(err, ErrInvalidLifecycleTransition) {
					t.Errorf("ValidateLifecycleTransition(%q, %q): err must wrap ErrInvalidLifecycleTransition; got %v",
						from, to, err)
				}
			})
		}
	}
}

// TestGovernanceExpectation_CanTransitionTo_AgreesWithValidator asserts
// that the receiver method and the package-level function agree on
// every from→to pair. Drift between them would let the lifecycle
// behaviour split across two code paths; this test is the regression
// guard against that split.
func TestGovernanceExpectation_CanTransitionTo_AgreesWithValidator(t *testing.T) {
	for _, from := range allStatuses {
		for _, to := range allStatuses {
			from, to := from, to
			t.Run(string(from)+"→"+string(to), func(t *testing.T) {
				e := &GovernanceExpectation{Status: from}
				gotMethod := e.CanTransitionTo(to)
				gotValidator := ValidateLifecycleTransition(from, to) == nil
				if gotMethod != gotValidator {
					t.Fatalf("disagreement at (%q, %q): CanTransitionTo=%v, ValidateLifecycleTransition==nil=%v",
						from, to, gotMethod, gotValidator)
				}
			})
		}
	}
}

// TestExpectationStatus_AllReachable asserts that the five status
// constants have the literal string values shared with Profile and
// Surface — this alignment is load-bearing for the schema CHECK that a
// later issue will introduce.
func TestExpectationStatus_AllReachable(t *testing.T) {
	cases := []struct {
		got  ExpectationStatus
		want string
	}{
		{ExpectationStatusDraft, "draft"},
		{ExpectationStatusReview, "review"},
		{ExpectationStatusActive, "active"},
		{ExpectationStatusDeprecated, "deprecated"},
		{ExpectationStatusRetired, "retired"},
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("status constant: want %q, got %q", c.want, string(c.got))
		}
	}
}

// TestConditionType_ClosedEnumIsRiskConditionOnly asserts the
// single-value ConditionType enum. Any future widening must update
// this test deliberately, not silently.
func TestConditionType_ClosedEnumIsRiskConditionOnly(t *testing.T) {
	if string(ConditionTypeRiskCondition) != "risk_condition" {
		t.Errorf("ConditionTypeRiskCondition: want %q, got %q",
			"risk_condition", string(ConditionTypeRiskCondition))
	}
}

// TestScopeKind_ClosedEnumValues asserts the three scope-kind constants
// have the literal string values used by the matching engine's index
// shape. Adding a new ScopeKind must update this test deliberately.
func TestScopeKind_ClosedEnumValues(t *testing.T) {
	cases := []struct {
		got  ScopeKind
		want string
	}{
		{ScopeKindProcess, "process"},
		{ScopeKindBusinessService, "business_service"},
		{ScopeKindCapability, "capability"},
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("scope-kind constant: want %q, got %q", c.want, string(c.got))
		}
	}
}

// TestGovernanceExpectation_StructInvariants asserts the zero-value
// behaviour of the domain struct: empty status, nil payload, zero
// version, all *time.Time pointer fields nil. This pins the field set
// against accidental defaults.
func TestGovernanceExpectation_StructInvariants(t *testing.T) {
	var e GovernanceExpectation

	if e.Status != "" {
		t.Errorf("zero-value Status: want empty string, got %q", e.Status)
	}
	if e.ConditionPayload != nil {
		t.Errorf("zero-value ConditionPayload: want nil, got %v", e.ConditionPayload)
	}
	if e.Version != 0 {
		t.Errorf("zero-value Version: want 0, got %d", e.Version)
	}
	if e.EffectiveUntil != nil {
		t.Errorf("zero-value EffectiveUntil: want nil, got %v", e.EffectiveUntil)
	}
	if e.RetiredAt != nil {
		t.Errorf("zero-value RetiredAt: want nil, got %v", e.RetiredAt)
	}
	if e.ApprovedAt != nil {
		t.Errorf("zero-value ApprovedAt: want nil, got %v", e.ApprovedAt)
	}
}
