package authority

import (
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/value"
)

// ExceedsConsequenceThreshold returns true when the submitted consequence
// exceeds the configured threshold.
//
// Conservative rules:
// - nil submitted consequence does not exceed the threshold
// - type mismatch exceeds the threshold
// - unsupported values exceed the threshold
// - currency mismatch exceeds the threshold
func ExceedsConsequenceThreshold(submitted *eval.Consequence, threshold Consequence) bool {
	if submitted == nil {
		return false
	}

	if submitted.Type != threshold.Type {
		return true
	}

	switch threshold.Type {
	case value.ConsequenceTypeMonetary:
		if submitted.Currency == "" || threshold.Currency == "" {
			return true
		}
		if submitted.Currency != threshold.Currency {
			return true
		}
		return submitted.Amount > threshold.Amount

	case value.ConsequenceTypeRiskRating:
		return riskRatingRank(submitted.RiskRating) > riskRatingRank(threshold.RiskRating)

	default:
		return true
	}
}

func riskRatingRank(r value.RiskRating) int {
	switch r {
	case value.RiskRatingLow:
		return 1
	case value.RiskRatingMedium:
		return 2
	case value.RiskRatingHigh:
		return 3
	case value.RiskRatingCritical:
		return 4
	default:
		return 999
	}
}
