package governancecoverage

import (
	"bytes"
	"encoding/json"

	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/value"
)

// riskConditionPayload is the typed shape of the JSON payload stored on
// a GovernanceExpectation whose ConditionType is "risk_condition". The
// shape is intentionally flat — every populated field is an implicit
// conjunct in the match predicate; an empty payload {} matches every
// request in scope.
//
// Pointer types on the numeric fields let the decoder distinguish "not
// supplied" (nil) from "supplied as 0". Without that distinction, a
// payload that legitimately said "min_confidence: 0" would be
// indistinguishable from one that omitted it.
//
// Strict-decode contract. The decoder rejects unknown fields via
// json.Decoder.DisallowUnknownFields; the matcher treats the rejection
// as "this expectation is non-matching" rather than as a global error.
// This keeps a single mis-shaped expectation from poisoning matching
// for the rest of the bundle.
type riskConditionPayload struct {
	ConsequenceType          string   `json:"consequence_type,omitempty"`
	ConsequenceAmountAtLeast *float64 `json:"consequence_amount_at_least,omitempty"`
	ConsequenceAmountAtMost  *float64 `json:"consequence_amount_at_most,omitempty"`
	ConsequenceCurrency      string   `json:"consequence_currency,omitempty"`
	ConsequenceRiskRating    string   `json:"consequence_risk_rating,omitempty"`
	MinConfidence            *float64 `json:"min_confidence,omitempty"`
}

// decodeRiskCondition strict-decodes payload into a riskConditionPayload.
// Returns ok=false when the bytes are malformed JSON, are not an
// object, or contain unknown keys. Empty input ({} or zero bytes) is
// allowed and produces a zero-value payload that matches every request
// in scope.
func decodeRiskCondition(payload json.RawMessage) (riskConditionPayload, bool) {
	// Empty/zero-length payload is treated as the empty-object identity.
	// The Postgres repo normalises nil inputs to {} on insert; this
	// branch is a defensive fast path for callers that stored nil
	// directly through some other route.
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return riskConditionPayload{}, true
	}

	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()

	var rc riskConditionPayload
	if err := dec.Decode(&rc); err != nil {
		return riskConditionPayload{}, false
	}
	// Reject trailing tokens (e.g. "{}{}") so we do not silently accept
	// degenerate payloads. dec.More() returns true if any data remains.
	if dec.More() {
		return riskConditionPayload{}, false
	}
	return rc, true
}

// matches reports whether the typed risk-shaped facts on in satisfy
// every populated field of rc. Implicit AND across populated fields.
//
// Field/comparator pairs:
//
//	consequence_type             equals    in.Consequence.Type
//	consequence_currency         equals    in.Consequence.Currency
//	consequence_risk_rating      equals    in.Consequence.RiskRating
//	consequence_amount_at_least  >=        in.Consequence.Amount
//	consequence_amount_at_most   <=        in.Consequence.Amount
//	min_confidence               >=        in.Confidence
//
// Consequence-shaped constraints fail when in.Consequence is nil — there
// is no fact to compare against, so the expectation does not match.
func (rc riskConditionPayload) matches(in Input) bool {
	if rc.requiresConsequence() && in.Consequence == nil {
		return false
	}

	if rc.MinConfidence != nil && in.Confidence < *rc.MinConfidence {
		return false
	}

	// All consequence-shaped checks are skipped if the constraint isn't
	// populated; otherwise they require Consequence to be non-nil
	// (already filtered by requiresConsequence above).
	c := in.Consequence

	if rc.ConsequenceType != "" {
		if c == nil || string(c.Type) != rc.ConsequenceType {
			return false
		}
	}
	if rc.ConsequenceCurrency != "" {
		if c == nil || c.Currency != rc.ConsequenceCurrency {
			return false
		}
	}
	if rc.ConsequenceRiskRating != "" {
		if c == nil || string(c.RiskRating) != rc.ConsequenceRiskRating {
			return false
		}
	}
	if rc.ConsequenceAmountAtLeast != nil {
		if c == nil || c.Amount < *rc.ConsequenceAmountAtLeast {
			return false
		}
	}
	if rc.ConsequenceAmountAtMost != nil {
		if c == nil || c.Amount > *rc.ConsequenceAmountAtMost {
			return false
		}
	}

	return true
}

// requiresConsequence reports whether any populated field in rc reads
// from Input.Consequence. Used to short-circuit on nil consequence.
func (rc riskConditionPayload) requiresConsequence() bool {
	return rc.ConsequenceType != "" ||
		rc.ConsequenceCurrency != "" ||
		rc.ConsequenceRiskRating != "" ||
		rc.ConsequenceAmountAtLeast != nil ||
		rc.ConsequenceAmountAtMost != nil
}

// Compile-time alignment with the value-package risk vocabulary.
// Documented here so reviewers can see at a glance that the grammar's
// string keys correspond to value-typed enums in the eval request.
//
//nolint:unused // self-documenting — referenced only at compile time.
var (
	_ = value.ConsequenceTypeMonetary
	_ = value.RiskRatingLow
	_ eval.Consequence
)
