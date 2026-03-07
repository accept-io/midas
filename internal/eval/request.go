package eval

import "github.com/accept-io/midas/internal/value"

// Consequence represents the submitted impact of a decision request.
// Only one variant should be populated depending on Type.
type Consequence struct {
	Type value.ConsequenceType

	// Monetary variant
	Amount   float64
	Currency string

	// Risk rating variant
	RiskRating value.RiskRating
}

// DecisionRequest represents the input to the authority evaluation engine.
type DecisionRequest struct {
	SurfaceID   string
	AgentID     string
	Confidence  float64
	Consequence *Consequence
	Context     map[string]any
	RequestID   string
}
