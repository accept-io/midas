package audit

type AuditEventType string

const (
	// ---------------------------------------------------------------------------
	// Lifecycle transition events — emitted via applyStep.
	// Each represents a state change in the envelope state machine.
	// These are the events anchored in Integrity.AuditEventIDs.
	// ---------------------------------------------------------------------------

	AuditEventEnvelopeCreated   AuditEventType = "ENVELOPE_CREATED"
	AuditEventEvaluationStarted AuditEventType = "EVALUATION_STARTED" // RECEIVED → EVALUATING
	AuditEventOutcomeRecorded   AuditEventType = "OUTCOME_RECORDED"   // EVALUATING → OUTCOME_RECORDED or ESCALATED
	AuditEventEscalationPending AuditEventType = "ESCALATION_PENDING" // ESCALATED → AWAITING_REVIEW
	AuditEventEnvelopeClosed    AuditEventType = "ENVELOPE_CLOSED"    // any → CLOSED (normal and escalated paths)

	// ---------------------------------------------------------------------------
	// Semantic events — emitted directly (not via applyStep).
	// Not state changes; record significant domain facts.
	// ---------------------------------------------------------------------------

	AuditEventEscalationReviewed AuditEventType = "ESCALATION_REVIEWED" // review decision recorded before close

	// ---------------------------------------------------------------------------
	// Observational events — emitted via appendObservationEvent.
	// Record facts discovered during evaluation; not anchored in integrity chain.
	// ---------------------------------------------------------------------------

	AuditEventSurfaceResolved        AuditEventType = "SURFACE_RESOLVED"
	AuditEventAgentResolved          AuditEventType = "AGENT_RESOLVED"
	AuditEventAuthorityChainResolved AuditEventType = "AUTHORITY_CHAIN_RESOLVED"
	AuditEventContextValidated       AuditEventType = "CONTEXT_VALIDATED"
	AuditEventConfidenceChecked      AuditEventType = "CONFIDENCE_CHECKED"
	AuditEventConsequenceChecked     AuditEventType = "CONSEQUENCE_CHECKED"
	AuditEventPolicyEvaluated        AuditEventType = "POLICY_EVALUATED"

	// ---------------------------------------------------------------------------
	// Deprecated — retained for backward compatibility with existing audit rows
	// and integrity_test.go stubs. New code must not emit this event type.
	// Use the specific lifecycle constants above instead.
	// ---------------------------------------------------------------------------

	// Deprecated: use AuditEventEvaluationStarted, AuditEventOutcomeRecorded,
	// AuditEventEscalationPending, or AuditEventEnvelopeClosed.
	AuditEventStateTransitioned AuditEventType = "STATE_TRANSITIONED"
)

// EventPerformerType identifies who emitted or executed an audit event.
type EventPerformerType string

const (
	// Event emitted by the MIDAS system itself
	EventPerformerSystem EventPerformerType = "system"
	// Event emitted by an autonomous agent
	EventPerformerAgent EventPerformerType = "agent"
	// Event emitted by a human reviewer
	EventPerformerReviewer EventPerformerType = "reviewer"
)
