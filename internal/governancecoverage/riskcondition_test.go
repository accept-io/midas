package governancecoverage

import (
	"encoding/json"
	"testing"

	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/value"
)

// floatPtr is a small helper for the optional pointer fields on
// riskConditionPayload (and for assertions that need to disambiguate
// "not supplied" from "supplied as 0").
func floatPtr(v float64) *float64 { return &v }

func TestDecodeRiskCondition_EmptyPayload_ReturnsZero(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty_object", `{}`},
		{"only_whitespace", "   "},
		{"empty_string", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rc, ok := decodeRiskCondition(json.RawMessage(tc.raw))
			if !ok {
				t.Fatalf("want ok, got !ok for %q", tc.raw)
			}
			if rc.MinConfidence != nil ||
				rc.ConsequenceAmountAtLeast != nil ||
				rc.ConsequenceAmountAtMost != nil ||
				rc.ConsequenceType != "" ||
				rc.ConsequenceCurrency != "" ||
				rc.ConsequenceRiskRating != "" {
				t.Errorf("zero payload must produce a zero struct; got %+v", rc)
			}
		})
	}
}

func TestDecodeRiskCondition_StrictRejection(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"unknown_field", `{"unknown_field": 42}`},
		{"malformed_json", `{not even json`},
		{"trailing_tokens", `{}{"unexpected": true}`},
		{"top_level_array", `["a", "b"]`},
		{"top_level_string", `"a string"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := decodeRiskCondition(json.RawMessage(tc.raw))
			if ok {
				t.Errorf("decoder must reject %q", tc.raw)
			}
		})
	}
}

func TestDecodeRiskCondition_SuppliedZeroIsDistinguishable(t *testing.T) {
	// `min_confidence: 0` is a meaningful constraint (every confidence
	// satisfies >=0). The pointer field encoding must preserve this.
	rc, ok := decodeRiskCondition(json.RawMessage(`{"min_confidence": 0}`))
	if !ok {
		t.Fatal("decode failed")
	}
	if rc.MinConfidence == nil {
		t.Fatal("MinConfidence must be non-nil for explicit 0")
	}
	if *rc.MinConfidence != 0 {
		t.Errorf("MinConfidence: want 0, got %v", *rc.MinConfidence)
	}
}

// emptyInput is the canonical no-consequence input. ObservedAt is left
// zero — these grammar tests don't depend on time.
func emptyInput() Input { return Input{} }

func inputWithConsequence(c *eval.Consequence, confidence float64) Input {
	return Input{
		Confidence:  confidence,
		Consequence: c,
	}
}

// ---------------------------------------------------------------------------
// Empty payload matches everything in scope.
// ---------------------------------------------------------------------------

func TestRiskCondition_EmptyPayload_Matches(t *testing.T) {
	rc := riskConditionPayload{}
	if !rc.matches(emptyInput()) {
		t.Error("empty payload must match every input")
	}
	if !rc.matches(inputWithConsequence(&eval.Consequence{Type: value.ConsequenceTypeMonetary, Amount: 1}, 0.1)) {
		t.Error("empty payload must match input with consequence")
	}
}

// ---------------------------------------------------------------------------
// min_confidence (>=)
// ---------------------------------------------------------------------------

func TestRiskCondition_MinConfidence(t *testing.T) {
	cases := []struct {
		name       string
		threshold  float64
		confidence float64
		match      bool
	}{
		{"satisfied_above", 0.5, 0.9, true},
		{"satisfied_equal_boundary", 0.5, 0.5, true}, // inclusive
		{"unsatisfied_below", 0.5, 0.49, false},
		{"unsatisfied_zero", 0.5, 0.0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rc := riskConditionPayload{MinConfidence: floatPtr(tc.threshold)}
			got := rc.matches(inputWithConsequence(nil, tc.confidence))
			if got != tc.match {
				t.Errorf("min_confidence %v vs confidence %v: want match=%v, got %v",
					tc.threshold, tc.confidence, tc.match, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// consequence_amount_at_least / at_most
// ---------------------------------------------------------------------------

func TestRiskCondition_AmountAtLeast(t *testing.T) {
	cases := []struct {
		name      string
		threshold float64
		amount    float64
		match     bool
	}{
		{"satisfied_above", 5000, 7500, true},
		{"satisfied_equal_boundary", 5000, 5000, true}, // inclusive
		{"unsatisfied_below", 5000, 4999, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rc := riskConditionPayload{ConsequenceAmountAtLeast: floatPtr(tc.threshold)}
			c := &eval.Consequence{Type: value.ConsequenceTypeMonetary, Amount: tc.amount}
			got := rc.matches(inputWithConsequence(c, 0))
			if got != tc.match {
				t.Errorf("at_least %v vs amount %v: want match=%v, got %v",
					tc.threshold, tc.amount, tc.match, got)
			}
		})
	}
}

func TestRiskCondition_AmountAtMost(t *testing.T) {
	cases := []struct {
		name      string
		threshold float64
		amount    float64
		match     bool
	}{
		{"satisfied_below", 10000, 7500, true},
		{"satisfied_equal_boundary", 10000, 10000, true}, // inclusive
		{"unsatisfied_above", 10000, 10001, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rc := riskConditionPayload{ConsequenceAmountAtMost: floatPtr(tc.threshold)}
			c := &eval.Consequence{Type: value.ConsequenceTypeMonetary, Amount: tc.amount}
			got := rc.matches(inputWithConsequence(c, 0))
			if got != tc.match {
				t.Errorf("at_most %v vs amount %v: want match=%v, got %v",
					tc.threshold, tc.amount, tc.match, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// consequence_currency
// ---------------------------------------------------------------------------

func TestRiskCondition_Currency_Equals(t *testing.T) {
	rc := riskConditionPayload{ConsequenceCurrency: "GBP"}
	c := &eval.Consequence{Type: value.ConsequenceTypeMonetary, Currency: "GBP"}
	if !rc.matches(inputWithConsequence(c, 0)) {
		t.Error("currency match must succeed")
	}
}

func TestRiskCondition_Currency_Mismatch(t *testing.T) {
	rc := riskConditionPayload{ConsequenceCurrency: "GBP"}
	c := &eval.Consequence{Type: value.ConsequenceTypeMonetary, Currency: "USD"}
	if rc.matches(inputWithConsequence(c, 0)) {
		t.Error("currency mismatch must fail")
	}
}

// ---------------------------------------------------------------------------
// consequence_risk_rating
// ---------------------------------------------------------------------------

func TestRiskCondition_RiskRating_Equals(t *testing.T) {
	rc := riskConditionPayload{ConsequenceRiskRating: "high"}
	c := &eval.Consequence{Type: value.ConsequenceTypeRiskRating, RiskRating: value.RiskRatingHigh}
	if !rc.matches(inputWithConsequence(c, 0)) {
		t.Error("risk_rating match must succeed")
	}
}

func TestRiskCondition_RiskRating_Mismatch(t *testing.T) {
	rc := riskConditionPayload{ConsequenceRiskRating: "high"}
	c := &eval.Consequence{Type: value.ConsequenceTypeRiskRating, RiskRating: value.RiskRatingLow}
	if rc.matches(inputWithConsequence(c, 0)) {
		t.Error("risk_rating mismatch must fail")
	}
}

// ---------------------------------------------------------------------------
// consequence_type
// ---------------------------------------------------------------------------

func TestRiskCondition_ConsequenceType_Equals(t *testing.T) {
	rc := riskConditionPayload{ConsequenceType: "monetary"}
	c := &eval.Consequence{Type: value.ConsequenceTypeMonetary}
	if !rc.matches(inputWithConsequence(c, 0)) {
		t.Error("consequence_type match must succeed")
	}
}

func TestRiskCondition_ConsequenceType_Mismatch(t *testing.T) {
	rc := riskConditionPayload{ConsequenceType: "monetary"}
	c := &eval.Consequence{Type: value.ConsequenceTypeRiskRating}
	if rc.matches(inputWithConsequence(c, 0)) {
		t.Error("consequence_type mismatch must fail")
	}
}

// ---------------------------------------------------------------------------
// nil consequence
// ---------------------------------------------------------------------------

func TestRiskCondition_NilConsequence_FailsConsequenceConstraints(t *testing.T) {
	cases := []struct {
		name string
		rc   riskConditionPayload
	}{
		{"consequence_type", riskConditionPayload{ConsequenceType: "monetary"}},
		{"consequence_currency", riskConditionPayload{ConsequenceCurrency: "GBP"}},
		{"consequence_risk_rating", riskConditionPayload{ConsequenceRiskRating: "high"}},
		{"consequence_amount_at_least", riskConditionPayload{ConsequenceAmountAtLeast: floatPtr(1)}},
		{"consequence_amount_at_most", riskConditionPayload{ConsequenceAmountAtMost: floatPtr(1)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.rc.matches(inputWithConsequence(nil, 0)) {
				t.Errorf("%s with nil consequence must not match", tc.name)
			}
		})
	}
}

func TestRiskCondition_NilConsequence_NonConsequenceConstraintsStillEvaluate(t *testing.T) {
	// min_confidence is the only constraint that doesn't read from
	// Consequence — it must still evaluate when Consequence is nil.
	rc := riskConditionPayload{MinConfidence: floatPtr(0.5)}
	if !rc.matches(inputWithConsequence(nil, 0.9)) {
		t.Error("min_confidence must evaluate when consequence is nil")
	}
	if rc.matches(inputWithConsequence(nil, 0.1)) {
		t.Error("min_confidence below threshold must fail even with nil consequence")
	}
}

// ---------------------------------------------------------------------------
// Implicit AND across populated fields.
// ---------------------------------------------------------------------------

func TestRiskCondition_AllFieldsConjunctive(t *testing.T) {
	rc := riskConditionPayload{
		ConsequenceType:          "monetary",
		ConsequenceCurrency:      "GBP",
		ConsequenceAmountAtLeast: floatPtr(5000),
		ConsequenceAmountAtMost:  floatPtr(10000),
		MinConfidence:            floatPtr(0.85),
	}
	all := &eval.Consequence{
		Type:     value.ConsequenceTypeMonetary,
		Currency: "GBP",
		Amount:   7500,
	}
	if !rc.matches(inputWithConsequence(all, 0.9)) {
		t.Error("all conditions satisfied must match")
	}

	// Drop one field at a time; each drop must produce a non-match.
	cases := []struct {
		name string
		mut  func(*eval.Consequence, *float64) (*eval.Consequence, float64)
	}{
		{"wrong_currency", func(c *eval.Consequence, _ *float64) (*eval.Consequence, float64) {
			cp := *c
			cp.Currency = "USD"
			return &cp, 0.9
		}},
		{"amount_below_min", func(c *eval.Consequence, _ *float64) (*eval.Consequence, float64) {
			cp := *c
			cp.Amount = 100
			return &cp, 0.9
		}},
		{"amount_above_max", func(c *eval.Consequence, _ *float64) (*eval.Consequence, float64) {
			cp := *c
			cp.Amount = 99_999
			return &cp, 0.9
		}},
		{"confidence_below_min", func(c *eval.Consequence, _ *float64) (*eval.Consequence, float64) {
			return c, 0.1
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, conf := tc.mut(all, nil)
			if rc.matches(inputWithConsequence(c, conf)) {
				t.Errorf("%s must fail the conjunction", tc.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Currency case-sensitivity (documented behaviour).
// ---------------------------------------------------------------------------

func TestRiskCondition_Currency_CaseSensitive(t *testing.T) {
	// Currency comparison is case-sensitive. ISO 4217 codes are
	// conventionally upper-case; lower-case "gbp" must not match
	// upper-case "GBP". Documented in the package, pinned here.
	rc := riskConditionPayload{ConsequenceCurrency: "GBP"}
	c := &eval.Consequence{Type: value.ConsequenceTypeMonetary, Currency: "gbp"}
	if rc.matches(inputWithConsequence(c, 0)) {
		t.Error("currency comparison must be case-sensitive")
	}
}
