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

	// AuditEventGovernanceConditionDetected records that an active
	// GovernanceExpectation matched the runtime input during evaluation.
	// It is the runtime-evidence event for Governance Coverage Assurance
	// (#54). Emitted via acc.recordObservation; payload carries the
	// expectation identity (id, version), the structural anchor
	// (process_id, required_surface_id), the condition discriminator
	// (condition_type), and a typed risk-shape summary of the matched
	// input. Multiple matches in the same evaluation produce multiple
	// events in matcher-sorted order. Idempotent replay does not
	// re-emit (orchestrator short-circuits before queueing events).
	AuditEventGovernanceConditionDetected AuditEventType = "GOVERNANCE_CONDITION_DETECTED"

	// AuditEventGovernanceCoverageGap records that an active
	// GovernanceExpectation matched but the evaluation was for a
	// different Surface than the one the expectation required (#55).
	// Emitted immediately after its sibling GOVERNANCE_CONDITION_DETECTED
	// event in the same matcher loop, so each per-match group of events
	// is adjacent in the audit chain. Payload includes the missing and
	// actual surface IDs, a `correlation_basis` discriminator naming the
	// MVP same-evaluation correlation model, and the same context
	// summary shape as the detected event (built by a shared helper to
	// guarantee byte-identical summary fields between the two events).
	//
	// Limitation: this event detects gaps within evaluations that do
	// occur. It does NOT detect bypass — the case where a condition
	// appears in a code path that never invokes /v1/evaluate at all.
	// Bypass detection requires external condition-evidence ingestion
	// and is deferred to a future issue.
	AuditEventGovernanceCoverageGap AuditEventType = "GOVERNANCE_COVERAGE_GAP"

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
