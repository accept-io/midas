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

	// D31i Reject reasons — recorded in EvaluationResult.ReasonCode and
	// the OUTCOME_RECORDED audit event when the orchestrator rejects a
	// request because the grant lacks the requested capability or one
	// of the grant's constraints failed for this specific request.
	//
	// ReasonCapabilityNotGranted: the request named a RequestedCapability
	// that is not in the grant's Capabilities set. The granular
	// capability value flows through AUTHORITY_CHAIN_RESOLVED's payload
	// and the future capability-violation event; OUTCOME_RECORDED
	// carries only the generic reason code.
	//
	// ReasonInvalidCapability: the request named a RequestedCapability
	// value that is not one of the canonical five Capability constants.
	// A caller bug; rejecting deterministically keeps invalid input
	// from silently sneaking through under a more permissive code
	// path.
	//
	// ReasonConstraintViolated: at least one of the grant's typed
	// constraints failed against the request. The granular constraint
	// kind and reason flow through AUTHORITY_CONSTRAINT_VIOLATED;
	// OUTCOME_RECORDED carries the generic reason code.
	ReasonCapabilityNotGranted ReasonCode = "CAPABILITY_NOT_GRANTED"
	ReasonInvalidCapability    ReasonCode = "INVALID_CAPABILITY"
	ReasonConstraintViolated   ReasonCode = "CONSTRAINT_VIOLATED"

	// D31j Reject reason — the resolved agent was not in
	// operational_state=active (suspended, revoked, or any
	// non-canonical/empty value). The orchestrator emits
	// AGENT_OPERATIONAL_STATE_BLOCKED_AUTHORITY and rejects with
	// this reason BEFORE evaluating grant constraints, capabilities,
	// profile thresholds, or policy. A non-active agent must not be
	// able to exercise authority even when the grant otherwise
	// satisfies every other check.
	ReasonAgentOperationalStateBlocked ReasonCode = "AGENT_OPERATIONAL_STATE_BLOCKED"

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
