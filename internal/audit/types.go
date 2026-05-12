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

	// AuditEventFailModePolicyResolved records that the runtime
	// FailModePolicy resolver successfully attributed an effective policy
	// for this evaluation (D27j-impl-3). The event is evidence-only — it
	// names what policy was resolved and why (Surface override vs.
	// BusinessService default vs. deployment default), but never carries
	// rules, permitted modes, allow/deny markers, or any decision-outcome
	// fields. The event is emitted between SURFACE_RESOLVED and
	// AGENT_RESOLVED, well before POLICY_EVALUATED, and only when a
	// non-empty source resolved without error. No event is emitted when
	// no policy is configured at any level, when the resolver returns an
	// error (logged-only per the D27j-impl-2 posture), or when the
	// FailModePolicies repository is not wired in. The runtime resolver
	// remains observability-only in this tranche; the new event
	// participates in the audit hash chain like every other observation
	// but does not influence outcome computation.
	AuditEventFailModePolicyResolved AuditEventType = "FAIL_MODE_POLICY_RESOLVED"

	// AuditEventFailModePolicyTriggerFired records that a configured
	// FailModePolicy trigger condition fired during an evaluation while
	// a FailModePolicy had resolved. The supported trigger conditions
	// are enumerated by the failmode trigger taxonomy
	// (failmode.SupportedTriggerConditions / .CorrectnessClassForTrigger);
	// at present they are policy_evaluator_error (D29c-2) and
	// authority_resolution_failure (D29j). The event is evidence-only
	// — it records the resolved policy identity, the trigger condition,
	// the relevant correctness class, and the matched rule's posture /
	// enforcement_state / outcome, but never carries raw error text,
	// runtime-effect markers (degraded / allowed / fail_open / fail_soft
	// / decision_changed), or rules. The runtime outcome on the
	// evidence_only path is governed entirely by authority.FailMode; on
	// the enforced path, FAIL_MODE_POLICY_ENFORCED records the override
	// and OUTCOME_RECORDED reflects the FailModePolicy outcome.
	//
	// Emission gates depend on the trigger:
	//   - policy_evaluator_error: the policy evaluator returned an
	//     error AND a FailModePolicy resolved for the evaluation.
	//   - authority_resolution_failure: authority chain resolution
	//     produced a deterministic Reject (NO_ACTIVE_GRANT,
	//     PROFILE_NOT_FOUND, GRANT_PROFILE_SURFACE_MISMATCH) AND a
	//     FailModePolicy resolved for the evaluation.
	// Not emitted when no policy resolved, when resolution errored, or
	// when no supported trigger condition fired.
	//
	// Like FAIL_MODE_POLICY_RESOLVED, this event participates in the
	// audit hash chain via acc.recordObservation and is included in
	// Envelope.Integrity.AuditEventIDs.
	AuditEventFailModePolicyTriggerFired AuditEventType = "FAIL_MODE_POLICY_TRIGGER_FIRED"

	// AuditEventFailModePolicyDryRunDecision records the dry-run
	// outcome that MIDAS would have applied for the matched
	// FailModePolicy rule, without actually applying it (D29e). The
	// event is evidence-only: it captures the resolved policy
	// identity, the trigger condition, the matched rule's three-axis
	// declaration, the would-be outcome / reason code, the actual
	// outcome / reason code the orchestrator did apply, and a boolean
	// divergence flag. The runtime decision continues to be governed
	// by authority.FailMode and the existing orchestrator paths; this
	// event records what enforcement WOULD have produced so operators
	// can observe the effect of a future enforcement_state flip
	// before flipping it.
	//
	// Emitted only when ALL of the following hold:
	//   (a) FAIL_MODE_POLICY_TRIGGER_FIRED would be emitted (per
	//       D29c-2 conditions),
	//   (b) the matching rule for the correctness class is found
	//       (rule_status != not_found), AND
	//   (c) the matching rule's enforcement_state == dry_run.
	//
	// Not emitted when enforcement_state is evidence_only or enforced,
	// when no rule matches the correctness class, or when any of the
	// trigger-fired preconditions fail. Enforced rules in D29e still
	// do NOT enforce — enforcement is D29f's scope. The event must
	// never carry raw error text, applied/enforced markers, or
	// runtime-effect fields.
	//
	// Like FAIL_MODE_POLICY_RESOLVED and FAIL_MODE_POLICY_TRIGGER_FIRED,
	// this event participates in the audit hash chain via
	// acc.recordObservation and is included in
	// Envelope.Integrity.AuditEventIDs.
	AuditEventFailModePolicyDryRunDecision AuditEventType = "FAIL_MODE_POLICY_DRY_RUN_DECISION"

	// AuditEventFailModePolicyEnforced records that an effective
	// FailModePolicy rule with enforcement_state=enforced applied its
	// configured Outcome to the actual runtime decision (D29f). This
	// is the first FailModePolicy event whose emission corresponds to
	// a runtime outcome change: when present, OUTCOME_RECORDED's
	// outcome and reason_code reflect the FailModePolicy-derived
	// values rather than authority.FailMode's fallback.
	//
	// Emitted only when ALL of the following hold:
	//   (a) the policy evaluator returned an error,
	//   (b) a FailModePolicy resolved successfully for this evaluation,
	//   (c) the resolved policy entity is available for rule inspection,
	//   (d) a matching rule for correctness_class=resource exists
	//       (rule_status != not_found), AND
	//   (e) the matching rule's enforcement_state == enforced.
	//
	// When emitted, FAIL_MODE_POLICY_DRY_RUN_DECISION is NOT emitted
	// in the same evaluation — the two events correspond to mutually
	// exclusive values of enforcement_state. authority.FailMode is NOT
	// consulted on the enforced path; the FailModePolicy outcome wins.
	//
	// The event records the would-be outcome that authority.FailMode
	// would have produced (previous_outcome / previous_reason_code)
	// alongside the enforced outcome that was actually applied, so the
	// audit trail captures both the operator intent and the
	// counterfactual behaviour.
	//
	// Like the other FAIL_MODE_POLICY_* observational events, this
	// event participates in the audit hash chain via
	// acc.recordObservation and is included in
	// Envelope.Integrity.AuditEventIDs. The payload never carries
	// raw error text or runtime-effect markers (allowed, fail_open,
	// fail_soft, decision_changed, applied).
	AuditEventFailModePolicyEnforced AuditEventType = "FAIL_MODE_POLICY_ENFORCED"

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

	// AuditEventAuthorityConstraintViolated records that a runtime
	// request failed to satisfy an AuthorityGrant's typed constraint
	// (D31i). Emitted only when authority chain resolution succeeded
	// (we have a real grant in hand) AND the first constraint
	// evaluation against the request returned a violation. The event
	// is the runtime-evidence companion to the deterministic
	// Reject + CONSTRAINT_VIOLATED outcome the orchestrator returns;
	// it carries the violating grant id, the constraint kind that
	// failed, and a short human-readable reason in the payload. The
	// event MUST NOT carry raw secrets, the full constraint
	// configuration, or stack traces — operators read the kind and
	// follow the grant id to the configuration store for full detail.
	//
	// Emission is observational: it participates in the audit hash
	// chain via acc.recordObservation and is included in
	// Envelope.Integrity.AuditEventIDs, just like the FAIL_MODE_*
	// observational events.
	AuditEventAuthorityConstraintViolated AuditEventType = "AUTHORITY_CONSTRAINT_VIOLATED"

	// AuditEventAgentOperationalStateBlockedAuthority records that
	// the runtime blocked authority resolution because the resolved
	// agent was not in operational_state=active (D31j). Emitted only
	// after authority chain resolution succeeded — we have a real
	// agent, grant, profile, and surface — and the gate fired
	// because the resolved agent.OperationalState was suspended,
	// revoked, or any non-canonical/empty value. The orchestrator
	// follows emission with a deterministic Reject using
	// ReasonAgentOperationalStateBlocked.
	//
	// The event's payload carries agent_id, grant_id, profile_id,
	// surface_id, operational_state, and a short reason string —
	// enough for an operator reading the audit chain to understand
	// why the otherwise-valid grant did not authorise the request.
	// Like AUTHORITY_CONSTRAINT_VIOLATED it is observational: emitted
	// via acc.recordObservation, participates in the audit hash
	// chain, and is included in Envelope.Integrity.AuditEventIDs.
	AuditEventAgentOperationalStateBlockedAuthority AuditEventType = "AGENT_OPERATIONAL_STATE_BLOCKED_AUTHORITY"

	// AuditEventEscalationTargetSelected records that the runtime
	// resolved a non-empty EscalationTargetID on the active profile
	// at escalation time AND the EscalationTarget repository
	// returned an active version (D31k-impl-1). Emitted via
	// acc.recordObservation, participates in the audit hash chain,
	// and is included in Envelope.Integrity.AuditEventIDs.
	//
	// Payload (where existing audit style allows):
	//
	//   profile_id, profile_version, grant_id, agent_id, surface_id,
	//   escalation_target_id, escalation_target_version,
	//   escalation_target_kind, escalation_target_handle
	//
	// The event describes the routing decision only — the outcome
	// remains the underlying Escalate verdict produced by the
	// confidence/consequence/policy steps. EscalationMode (auto /
	// manual) is independent and is not changed by this event.
	AuditEventEscalationTargetSelected AuditEventType = "ESCALATION_TARGET_SELECTED"

	// AuditEventEscalationTargetResolutionFailed records that the
	// active profile carried a non-empty EscalationTargetID but the
	// repository returned no active version at evaluation time
	// (D31k-impl-1). The orchestrator preserves the escalation
	// outcome and emits this event so operators can see the dangling
	// reference. Payload mirrors ESCALATION_TARGET_SELECTED minus
	// the resolved-target metadata (only escalation_target_id is
	// populated alongside the authority spine).
	AuditEventEscalationTargetResolutionFailed AuditEventType = "ESCALATION_TARGET_RESOLUTION_FAILED"

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
