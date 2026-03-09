package audit

type AuditEventType string

const (
	AuditEventEnvelopeCreated        AuditEventType = "ENVELOPE_CREATED"
	AuditEventStateTransitioned      AuditEventType = "STATE_TRANSITIONED"
	AuditEventSurfaceResolved        AuditEventType = "SURFACE_RESOLVED"
	AuditEventAgentResolved          AuditEventType = "AGENT_RESOLVED"
	AuditEventAuthorityChainResolved AuditEventType = "AUTHORITY_CHAIN_RESOLVED"
	AuditEventContextValidated       AuditEventType = "CONTEXT_VALIDATED"
	AuditEventConfidenceChecked      AuditEventType = "CONFIDENCE_CHECKED"
	AuditEventConsequenceChecked     AuditEventType = "CONSEQUENCE_CHECKED"
	AuditEventPolicyEvaluated        AuditEventType = "POLICY_EVALUATED"
	AuditEventOutcomeRecorded        AuditEventType = "OUTCOME_RECORDED"
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
