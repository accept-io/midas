package eval

// Outcome is the top-level result of an authority evaluation.
type Outcome string

const (
	OutcomeAccept               Outcome = "accept"
	OutcomeEscalate             Outcome = "escalate"
	OutcomeReject               Outcome = "reject"
	OutcomeRequestClarification Outcome = "request_clarification"
)

// ReasonCode explains why a particular outcome was reached.
// These values form part of the evaluation contract.
type ReasonCode string

const (
	// Accept reasons
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

	// FailModePolicy dry-run reason codes (D29e).
	//
	// These constants are recorded ONLY inside the
	// FAIL_MODE_POLICY_DRY_RUN_DECISION audit event's dry_run_reason_code
	// payload field. They MUST NOT be set on OUTCOME_RECORDED's
	// reason_code field, on EvaluationResult.ReasonCode, or anywhere
	// else that drives the runtime decision path. D29e is evidence-only:
	// the dry-run computation reads the matched rule's configured
	// Outcome, maps it to one of these reason codes for the event
	// payload, and immediately discards the result for any decision
	// effect. D29f will introduce a controlled path for actually
	// applying these reasons.
	ReasonFailModePolicyDenied             ReasonCode = "FAIL_MODE_POLICY_DENIED"
	ReasonFailModePolicyEscalated          ReasonCode = "FAIL_MODE_POLICY_ESCALATED"
	ReasonFailModePolicyPermitWithEvidence ReasonCode = "FAIL_MODE_POLICY_PERMIT_WITH_EVIDENCE"
	ReasonFailModePolicyManualReview       ReasonCode = "FAIL_MODE_POLICY_MANUAL_REVIEW"
)
