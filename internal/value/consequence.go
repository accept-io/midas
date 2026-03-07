package value

// ConsequenceType discriminates the kind of consequence being assessed.
type ConsequenceType string

const (
	ConsequenceTypeMonetary   ConsequenceType = "monetary"
	ConsequenceTypeRiskRating ConsequenceType = "risk_rating"
)

// RiskRating is an ordered severity level used for risk-based consequence evaluation.
type RiskRating string

const (
	RiskRatingLow      RiskRating = "low"
	RiskRatingMedium   RiskRating = "medium"
	RiskRatingHigh     RiskRating = "high"
	RiskRatingCritical RiskRating = "critical"
)
