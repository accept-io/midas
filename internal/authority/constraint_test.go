package authority

import (
	"errors"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/value"
)

func TestConstraintKind_IsValid(t *testing.T) {
	for _, k := range []ConstraintKind{
		ConstraintKindConfidenceThresholdMin,
		ConstraintKindConsequenceThresholdMax,
		ConstraintKindHumanOnly,
		ConstraintKindAIOnly,
		ConstraintKindTimeWindow,
	} {
		if !k.IsValid() {
			t.Errorf("canonical kind %q reported invalid", string(k))
		}
	}
	for _, k := range []ConstraintKind{"", "delegation_chain", "Confidence_Threshold_Min"} {
		if k.IsValid() {
			t.Errorf("non-canonical kind %q reported valid", string(k))
		}
	}
}

func TestConstraint_Validate_ConfidenceThresholdMin(t *testing.T) {
	good := Constraint{Kind: ConstraintKindConfidenceThresholdMin, MinConfidence: 0.85}
	if err := good.Validate(); err != nil {
		t.Errorf("valid confidence_threshold_min rejected: %v", err)
	}
	for _, v := range []float64{-0.1, 1.1, 2.0} {
		c := Constraint{Kind: ConstraintKindConfidenceThresholdMin, MinConfidence: v}
		if err := c.Validate(); err == nil {
			t.Errorf("confidence %.2f should be invalid", v)
		} else if !errors.Is(err, ErrInvalidConstraint) {
			t.Errorf("expected ErrInvalidConstraint chain; got %v", err)
		}
	}
}

func TestConstraint_Validate_ConsequenceThresholdMax(t *testing.T) {
	monetaryGood := Constraint{
		Kind: ConstraintKindConsequenceThresholdMax,
		MaxConsequence: Consequence{
			Type: value.ConsequenceTypeMonetary, Amount: 100, Currency: "USD",
		},
	}
	if err := monetaryGood.Validate(); err != nil {
		t.Errorf("monetary consequence rejected: %v", err)
	}
	riskGood := Constraint{
		Kind: ConstraintKindConsequenceThresholdMax,
		MaxConsequence: Consequence{
			Type: value.ConsequenceTypeRiskRating, RiskRating: value.RiskRatingMedium,
		},
	}
	if err := riskGood.Validate(); err != nil {
		t.Errorf("risk_rating consequence rejected: %v", err)
	}
	missingType := Constraint{Kind: ConstraintKindConsequenceThresholdMax}
	if err := missingType.Validate(); err == nil {
		t.Error("missing consequence type should fail")
	}
	monetaryNoCurrency := Constraint{
		Kind: ConstraintKindConsequenceThresholdMax,
		MaxConsequence: Consequence{
			Type: value.ConsequenceTypeMonetary, Amount: 100,
		},
	}
	if err := monetaryNoCurrency.Validate(); err == nil {
		t.Error("monetary without currency should fail")
	}
	riskNoRating := Constraint{
		Kind: ConstraintKindConsequenceThresholdMax,
		MaxConsequence: Consequence{
			Type: value.ConsequenceTypeRiskRating,
		},
	}
	if err := riskNoRating.Validate(); err == nil {
		t.Error("risk_rating without value should fail")
	}
}

func TestConstraint_Validate_HumanAndAIOnly(t *testing.T) {
	for _, k := range []ConstraintKind{ConstraintKindHumanOnly, ConstraintKindAIOnly} {
		c := Constraint{Kind: k}
		if err := c.Validate(); err != nil {
			t.Errorf("%q with no payload rejected: %v", string(k), err)
		}
	}
}

func TestConstraint_Validate_TimeWindow(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	good := Constraint{Kind: ConstraintKindTimeWindow, StartTime: t0, EndTime: t1}
	if err := good.Validate(); err != nil {
		t.Errorf("valid time_window rejected: %v", err)
	}
	for _, c := range []Constraint{
		{Kind: ConstraintKindTimeWindow, EndTime: t1},
		{Kind: ConstraintKindTimeWindow, StartTime: t0},
		{Kind: ConstraintKindTimeWindow, StartTime: t1, EndTime: t0},
		{Kind: ConstraintKindTimeWindow, StartTime: t0, EndTime: t0},
	} {
		if err := c.Validate(); err == nil {
			t.Errorf("malformed time_window accepted: %+v", c)
		}
	}
}

func TestConstraint_Validate_UnknownKind(t *testing.T) {
	c := Constraint{Kind: "delegation_chain"}
	err := c.Validate()
	if err == nil {
		t.Fatal("unknown kind should fail")
	}
	if !errors.Is(err, ErrInvalidConstraintKind) {
		t.Errorf("expected ErrInvalidConstraintKind chain; got %v", err)
	}
}

func TestValidateConstraints_RejectsDuplicateKind(t *testing.T) {
	cs := []Constraint{
		{Kind: ConstraintKindConfidenceThresholdMin, MinConfidence: 0.8},
		{Kind: ConstraintKindConfidenceThresholdMin, MinConfidence: 0.9},
	}
	err := ValidateConstraints(cs)
	if err == nil {
		t.Fatal("expected duplicate-kind error")
	}
	if !errors.Is(err, ErrDuplicateConstraintKind) {
		t.Errorf("expected ErrDuplicateConstraintKind chain; got %v", err)
	}
}

func TestValidateConstraints_RejectsHumanAndAIOnlyTogether(t *testing.T) {
	cs := []Constraint{{Kind: ConstraintKindHumanOnly}, {Kind: ConstraintKindAIOnly}}
	err := ValidateConstraints(cs)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !errors.Is(err, ErrConflictingConstraints) {
		t.Errorf("expected ErrConflictingConstraints chain; got %v", err)
	}
}

func TestValidateConstraints_EmptyIsValid(t *testing.T) {
	if err := ValidateConstraints(nil); err != nil {
		t.Errorf("nil constraints should validate: %v", err)
	}
	if err := ValidateConstraints([]Constraint{}); err != nil {
		t.Errorf("empty constraints should validate: %v", err)
	}
}

func TestHasConstraintKind(t *testing.T) {
	cs := []Constraint{
		{Kind: ConstraintKindConfidenceThresholdMin, MinConfidence: 0.7},
		{Kind: ConstraintKindHumanOnly},
	}
	if !HasConstraintKind(cs, ConstraintKindConfidenceThresholdMin) {
		t.Error("expected confidence_threshold_min present")
	}
	if !HasConstraintKind(cs, ConstraintKindHumanOnly) {
		t.Error("expected human_only present")
	}
	if HasConstraintKind(cs, ConstraintKindTimeWindow) {
		t.Error("time_window must not be reported present")
	}
}

func TestEvaluateConstraints_AllPass(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	cs := []Constraint{
		{Kind: ConstraintKindConfidenceThresholdMin, MinConfidence: 0.7},
		{Kind: ConstraintKindTimeWindow, StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour)},
	}
	in := ConstraintInput{Confidence: 0.9, AgentType: agent.AgentTypeAI}
	if v := EvaluateConstraints(cs, in, now); v != nil {
		t.Errorf("expected no violation; got %+v", v)
	}
}

func TestEvaluateConstraints_ConfidenceBelowMin(t *testing.T) {
	cs := []Constraint{{Kind: ConstraintKindConfidenceThresholdMin, MinConfidence: 0.9}}
	v := EvaluateConstraints(cs, ConstraintInput{Confidence: 0.5}, time.Now())
	if v == nil {
		t.Fatal("expected violation")
	}
	if v.Kind != ConstraintKindConfidenceThresholdMin {
		t.Errorf("Kind: want %q, got %q", ConstraintKindConfidenceThresholdMin, v.Kind)
	}
}

func TestEvaluateConstraints_ConsequenceExceedsMax(t *testing.T) {
	cs := []Constraint{{
		Kind: ConstraintKindConsequenceThresholdMax,
		MaxConsequence: Consequence{
			Type: value.ConsequenceTypeMonetary, Amount: 100, Currency: "USD",
		},
	}}
	in := ConstraintInput{
		Consequence: &eval.Consequence{
			Type: value.ConsequenceTypeMonetary, Amount: 500, Currency: "USD",
		},
	}
	v := EvaluateConstraints(cs, in, time.Now())
	if v == nil || v.Kind != ConstraintKindConsequenceThresholdMax {
		t.Fatalf("expected consequence_threshold_max violation; got %+v", v)
	}
}

func TestEvaluateConstraints_HumanOnly_RejectsAI(t *testing.T) {
	cs := []Constraint{{Kind: ConstraintKindHumanOnly}}
	v := EvaluateConstraints(cs, ConstraintInput{AgentType: agent.AgentTypeAI}, time.Now())
	if v == nil || v.Kind != ConstraintKindHumanOnly {
		t.Fatalf("expected human_only violation for AI agent; got %+v", v)
	}
	v = EvaluateConstraints(cs, ConstraintInput{AgentType: agent.AgentTypeOperator}, time.Now())
	if v != nil {
		t.Errorf("operator agent should satisfy human_only; got %+v", v)
	}
}

func TestEvaluateConstraints_AIOnly_RejectsOperator(t *testing.T) {
	cs := []Constraint{{Kind: ConstraintKindAIOnly}}
	v := EvaluateConstraints(cs, ConstraintInput{AgentType: agent.AgentTypeOperator}, time.Now())
	if v == nil || v.Kind != ConstraintKindAIOnly {
		t.Fatalf("expected ai_only violation for operator agent; got %+v", v)
	}
	v = EvaluateConstraints(cs, ConstraintInput{AgentType: agent.AgentTypeAI}, time.Now())
	if v != nil {
		t.Errorf("ai agent should satisfy ai_only; got %+v", v)
	}
}

func TestEvaluateConstraints_TimeWindow(t *testing.T) {
	start := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 15, 17, 0, 0, 0, time.UTC)
	cs := []Constraint{{Kind: ConstraintKindTimeWindow, StartTime: start, EndTime: end}}

	for _, tc := range []struct {
		name   string
		now    time.Time
		expect bool // expect violation
	}{
		{"before start", start.Add(-time.Minute), true},
		{"at start", start, false},
		{"middle", start.Add(4 * time.Hour), false},
		{"at end", end, true}, // half-open: [start, end)
		{"after end", end.Add(time.Minute), true},
	} {
		v := EvaluateConstraints(cs, ConstraintInput{}, tc.now)
		got := v != nil
		if got != tc.expect {
			t.Errorf("%s: violation=%v want=%v", tc.name, got, tc.expect)
		}
	}
}

func TestEvaluateConstraints_FirstViolationWins(t *testing.T) {
	// Confidence violation first, time-window second — only the
	// first should be reported.
	now := time.Now()
	cs := []Constraint{
		{Kind: ConstraintKindConfidenceThresholdMin, MinConfidence: 0.9},
		{Kind: ConstraintKindTimeWindow, StartTime: now.Add(time.Hour), EndTime: now.Add(2 * time.Hour)},
	}
	v := EvaluateConstraints(cs, ConstraintInput{Confidence: 0.5}, now)
	if v == nil || v.Kind != ConstraintKindConfidenceThresholdMin {
		t.Errorf("expected confidence-violation first; got %+v", v)
	}
}
