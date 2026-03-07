package eval

// Outcome is the top-level result of an authority evaluation.
type Outcome string

const (
	OutcomeExecute              Outcome = "Execute"
	OutcomeEscalate             Outcome = "Escalate"
	OutcomeReject               Outcome = "Reject"
	OutcomeRequestClarification Outcome = "RequestClarification"
)

// ReasonCode explains why a particular outcome was reached.
// These values form part of the evaluation contract.
type ReasonCode string

const (
	// Execute reasons
	ReasonWithinAuthority ReasonCode = "WITHIN_AUTHORITY"

	// Escalation reasons
	ReasonConfidenceBelowThreshold ReasonCode = "CONFIDENCE_BELOW_THRESHOLD"
	ReasonConsequenceExceedsLimit  ReasonCode = "CONSEQUENCE_EXCEEDS_LIMIT"
	ReasonPolicyDeny               ReasonCode = "POLICY_DENY"
	ReasonPolicyError              ReasonCode = "POLICY_ERROR"

	// Reject reasons
	ReasonAgentNotFound               ReasonCode = "AGENT_NOT_FOUND"
	ReasonSurfaceNotFound             ReasonCode = "SURFACE_NOT_FOUND"
	ReasonSurfaceInactive             ReasonCode = "SURFACE_INACTIVE"
	ReasonNoActiveGrant               ReasonCode = "NO_ACTIVE_GRANT"
	ReasonProfileNotFound             ReasonCode = "PROFILE_NOT_FOUND"
	ReasonGrantProfileSurfaceMismatch ReasonCode = "GRANT_PROFILE_SURFACE_MISMATCH"

	// Request clarification reasons
	ReasonInsufficientContext ReasonCode = "INSUFFICIENT_CONTEXT"
)
