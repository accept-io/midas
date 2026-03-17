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

	// Reversibility
	Reversible            bool
	ReversalWindowSeconds int
}

// AgentRuntime holds the model and version of the agent making the request.
type AgentRuntime struct {
	Model   string
	Version string
}

// Delegation holds delegation chain claims submitted with the request.
type Delegation struct {
	InitiatedBy     string
	SessionID       string
	Chain           []string
	Scope           []string
	AuthorizedAt    string
	AuthorizedUntil string
}

// Subject identifies the entity the governed action acts upon.
type Subject struct {
	Type         string
	ID           string
	SecondaryIDs map[string]string
}

// DecisionRequest represents the input to the authority evaluation engine.
type DecisionRequest struct {
	// Idempotency scoping (schema v2.1)
	RequestSource string // Source system identifier (e.g., "loan-origination", "payments-api")
	RequestID     string // Idempotent request ID within the source

	SurfaceID   string
	AgentID     string
	Confidence  float64
	Consequence *Consequence
	Context     map[string]any

	// Governance metadata
	ActionType string
	ActionDesc string
	AgentKind  string
	ExpiresAt  string
	Runtime    *AgentRuntime
	Delegation *Delegation
	Subject    *Subject
}
