package surface_test

import (
	"github.com/accept-io/midas/internal/surface"
)

// Example Consequence Values - Demonstrating Tagged Union Usage

// NewFinancialConsequence creates a numeric financial consequence example.
func NewFinancialConsequence() surface.Consequence {
	return surface.Consequence{
		TypeID: "financial_exposure",
		Value: surface.ConsequenceValue{
			Numeric: &surface.NumericConsequence{
				Amount: 250000.0,
				Unit:   "USD", // Optional: override surface default currency
			},
		},
	}
}

// NewTemporalConsequence creates a numeric temporal consequence example.
func NewTemporalConsequence() surface.Consequence {
	return surface.Consequence{
		TypeID: "commitment_duration",
		Value: surface.ConsequenceValue{
			Numeric: &surface.NumericConsequence{
				Amount: 360.0, // 360 months = 30 years
				Unit:   "",    // Use surface default DurationUnit (months)
			},
		},
	}
}

// NewRiskRatingConsequence creates a categorical risk rating consequence example.
func NewRiskRatingConsequence() surface.Consequence {
	ordinal := 2 // "moderate" is index 2 in ["minimal", "low", "moderate", "elevated", "high", "critical"]
	return surface.Consequence{
		TypeID: "credit_risk_rating",
		Value: surface.ConsequenceValue{
			Categorical: &surface.CategoricalConsequence{
				Level:   "moderate",
				Ordinal: &ordinal, // Optional: validator can auto-populate
			},
		},
	}
}

// NewImpactScopeConsequence creates a categorical impact scope consequence example.
func NewImpactScopeConsequence() surface.Consequence {
	ordinal := 1 // "household" is index 1 in ["individual", "household", "business", "institutional"]
	return surface.Consequence{
		TypeID: "impact_scope",
		Value: surface.ConsequenceValue{
			Categorical: &surface.CategoricalConsequence{
				Level:   "household",
				Ordinal: &ordinal,
			},
		},
	}
}

// NewMultipleConsequences creates multiple consequences for a single decision.
// A loan approval might declare multiple consequence types.
func NewMultipleConsequences() []surface.Consequence {
	moderateRisk := 2

	return []surface.Consequence{
		// Financial exposure
		{
			TypeID: "financial_exposure",
			Value: surface.ConsequenceValue{
				Numeric: &surface.NumericConsequence{
					Amount: 250000.0,
					Unit:   "USD",
				},
			},
		},
		// Loan term duration
		{
			TypeID: "commitment_duration",
			Value: surface.ConsequenceValue{
				Numeric: &surface.NumericConsequence{
					Amount: 360.0, // months
				},
			},
		},
		// Credit risk assessment
		{
			TypeID: "credit_risk_rating",
			Value: surface.ConsequenceValue{
				Categorical: &surface.CategoricalConsequence{
					Level:   "moderate",
					Ordinal: &moderateRisk,
				},
			},
		},
	}
}

// NewFraudSuspensionConsequences creates fraud suspension consequences with temporal + categorical types.
func NewFraudSuspensionConsequences() []surface.Consequence {
	moderateImpact := 1
	lowFPRisk := 1

	return []surface.Consequence{
		// How long will the suspension last?
		{
			TypeID: "suspension_duration",
			Value: surface.ConsequenceValue{
				Numeric: &surface.NumericConsequence{
					Amount: 72.0, // 72 hours = 3 days
				},
			},
		},
		// Customer service disruption level
		{
			TypeID: "customer_impact",
			Value: surface.ConsequenceValue{
				Categorical: &surface.CategoricalConsequence{
					Level:   "moderate", // ["minimal", "moderate", "significant", "severe"]
					Ordinal: &moderateImpact,
				},
			},
		},
		// Risk this is a false positive
		{
			TypeID: "false_positive_risk",
			Value: surface.ConsequenceValue{
				Categorical: &surface.CategoricalConsequence{
					Level:   "low", // ["very_low", "low", "medium", "high"]
					Ordinal: &lowFPRisk,
				},
			},
		},
	}
}

// NewPaymentConsequences creates payment consequences with multi-type examples.
func NewPaymentConsequences() []surface.Consequence {
	highFraudRisk := 3
	individualScope := 0

	return []surface.Consequence{
		// Payment amount
		{
			TypeID: "financial_amount",
			Value: surface.ConsequenceValue{
				Numeric: &surface.NumericConsequence{
					Amount: 5000.0,
					Unit:   "USD",
				},
			},
		},
		// Fraud risk level
		{
			TypeID: "fraud_risk",
			Value: surface.ConsequenceValue{
				Categorical: &surface.CategoricalConsequence{
					Level:   "high", // ["minimal", "low", "medium", "high", "critical"]
					Ordinal: &highFraudRisk,
				},
			},
		},
		// Scope of impact
		{
			TypeID: "impact_scope",
			Value: surface.ConsequenceValue{
				Categorical: &surface.CategoricalConsequence{
					Level:   "individual", // ["individual", "household", "business", "institutional"]
					Ordinal: &individualScope,
				},
			},
		},
	}
}

// NewInvalidConsequenceBothSet creates an invalid consequence with both fields set (validator will reject).
func NewInvalidConsequenceBothSet() surface.Consequence {
	ordinal := 1
	return surface.Consequence{
		TypeID: "financial_exposure",
		Value: surface.ConsequenceValue{
			// INVALID: Both Numeric AND Categorical are set
			Numeric: &surface.NumericConsequence{
				Amount: 100000.0,
			},
			Categorical: &surface.CategoricalConsequence{
				Level:   "high",
				Ordinal: &ordinal,
			},
		},
	}
}

// NewInvalidConsequenceNeitherSet creates an invalid consequence with neither field set (validator will reject).
func NewInvalidConsequenceNeitherSet() surface.Consequence {
	return surface.Consequence{
		TypeID: "financial_exposure",
		Value: surface.ConsequenceValue{
			// INVALID: Neither Numeric nor Categorical is set
			Numeric:     nil,
			Categorical: nil,
		},
	}
}

// NewInvalidConsequenceWrongType creates an invalid consequence with wrong type for MeasureType (validator will reject).
func NewInvalidConsequenceWrongType() surface.Consequence {
	ordinal := 2
	return surface.Consequence{
		TypeID: "financial_exposure", // MeasureType is Financial
		Value: surface.ConsequenceValue{
			// INVALID: Financial consequences require Numeric, not Categorical
			Categorical: &surface.CategoricalConsequence{
				Level:   "high",
				Ordinal: &ordinal,
			},
		},
	}
}
