package decision

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/envelope"
)

// Sentinel errors returned by evaluationAccumulator methods.
var (
	// errAccumulatorAlreadyPersisted is returned when any mutating method is
	// called after persist() has successfully completed.
	errAccumulatorAlreadyPersisted = errors.New("accumulator has already been persisted")

	// errAccumulatorNilEnvelope is returned by newEvaluationAccumulator when
	// the provided envelope pointer is nil.
	errAccumulatorNilEnvelope = errors.New("envelope must not be nil")

	// errAccumulatorWrongState is returned by newEvaluationAccumulator when
	// the envelope is not in the RECEIVED state.
	errAccumulatorWrongState = errors.New("envelope must be in RECEIVED state")

	// errAccumulatorNilEvent is returned by recordEvent when ev is nil.
	errAccumulatorNilEvent = errors.New("audit event must not be nil")

	// errAccumulatorWrongEnvelope is returned by recordEvent when the event's
	// EnvelopeID does not match the accumulator's envelope ID.
	errAccumulatorWrongEnvelope = errors.New("audit event envelope_id does not match accumulator envelope")

	// errAccumulatorPreHashedEvent is returned by recordEvent when the event
	// has pre-populated hash fields. The repository assigns SequenceNo, PrevHash,
	// and EventHash at Append time; a pre-set event indicates a caller error.
	errAccumulatorPreHashedEvent = errors.New("audit event must not have pre-populated hash fields (Hash, PrevHash, SequenceNo)")

	// errAccumulatorNonTerminalState is returned by persist when the envelope
	// is not yet in a terminal state (CLOSED or AWAITING_REVIEW).
	errAccumulatorNonTerminalState = errors.New("envelope must be in a terminal state (CLOSED or AWAITING_REVIEW) before persisting")

	// errAccumulatorMissingOutcome is returned by persist when the envelope
	// has no Outcome set. An outcome must be recorded before persisting.
	errAccumulatorMissingOutcome = errors.New("envelope must have Evaluation.Outcome set before persisting")
)

// evaluationAccumulator builds evaluation state and audit events in memory,
// then persists the complete result in one atomic sequence:
// Envelopes.Create → N×Audit.Append → Envelopes.Update.
//
// This eliminates the N intermediate Envelopes.Update calls produced by the
// current applyStep-based flow (9 writes for a happy-path evaluation).
// Since the entire evaluation runs inside a single transaction, intermediate
// DB state is never externally observable — only the committed terminal state
// matters to callers.
//
// All integrity guarantees are preserved:
//   - FK constraint satisfied: Create runs before any Append
//   - Hash chain intact: Appends run in declaration order; repo assigns SequenceNo,
//     PrevHash, and EventHash
//   - Integrity anchors complete: absorbPersistedEvent updates FirstEventHash,
//     FinalEventHash, and AuditEventIDs for every event (lifecycle and observational)
//
// CONCURRENCY: This type is not safe for concurrent access. Each call to
// Orchestrator.Evaluate creates its own accumulator instance; no sharing occurs.
//
// Usage pattern:
//
//	acc, err := newEvaluationAccumulator(env)
//	if err != nil { return err }
//	if err := acc.recordLifecycle(src, rid, audit.AuditEventEnvelopeCreated, nil); err != nil { ... }
//	if err := acc.transition(envelope.EnvelopeStateEvaluating, now); err != nil { ... }
//	if err := acc.recordLifecycle(src, rid, audit.AuditEventEvaluationStarted, nil); err != nil { ... }
//	// ... resolve authority, check thresholds, record observations ...
//	if err := acc.transition(envelope.EnvelopeStateClosed, now); err != nil { ... }
//	if err := acc.recordLifecycle(src, rid, audit.AuditEventEnvelopeClosed, nil); err != nil { ... }
//	return acc.persist(ctx, envRepo, auditRepo)
type evaluationAccumulator struct {
	env           *envelope.Envelope
	pendingEvents []*audit.AuditEvent
	persisted     bool // true after a successful persist(); guards against reuse
}

// newEvaluationAccumulator creates an accumulator for the given envelope.
// Returns an error if env is nil or not in the RECEIVED state.
// pendingEvents is pre-allocated with capacity 16, which covers the longest
// evaluation path without reallocation.
func newEvaluationAccumulator(env *envelope.Envelope) (*evaluationAccumulator, error) {
	if env == nil {
		return nil, errAccumulatorNilEnvelope
	}
	if env.State != envelope.EnvelopeStateReceived {
		return nil, fmt.Errorf("%w: got %s", errAccumulatorWrongState, env.State)
	}
	return &evaluationAccumulator{
		env:           env,
		pendingEvents: make([]*audit.AuditEvent, 0, 16),
		persisted:     false,
	}, nil
}

// transition advances the in-memory envelope to next, enforcing the state
// machine's structural and content invariants immediately (via Envelope.Transition).
// No DB write occurs — the new state is deferred to persist.
//
// Returns errAccumulatorAlreadyPersisted if called after a successful persist.
// Returns a wrapped error (ErrInvalidTransition, ErrMissingExplanation, etc.)
// if the envelope state machine rejects the transition.
func (a *evaluationAccumulator) transition(next envelope.EnvelopeState, now time.Time) error {
	if a.persisted {
		return fmt.Errorf("transition %s: %w", next, errAccumulatorAlreadyPersisted)
	}
	from := a.env.State
	if err := a.env.Transition(next, now); err != nil {
		return fmt.Errorf("transition %s→%s: %w", from, next, err)
	}
	return nil
}

// recordEvent queues a pre-built audit event for persistence.
// No DB write occurs. SequenceNo, PrevHash, and Hash must be zero/empty —
// the repository assigns these at Append time.
//
// Returns an error if:
//   - the accumulator has already been persisted
//   - ev is nil
//   - ev.EnvelopeID does not match the accumulator's envelope ID
//   - ev.Hash, ev.PrevHash, or ev.SequenceNo are pre-populated
func (a *evaluationAccumulator) recordEvent(ev *audit.AuditEvent) error {
	if a.persisted {
		return errAccumulatorAlreadyPersisted
	}
	if ev == nil {
		return errAccumulatorNilEvent
	}
	if ev.EnvelopeID != a.env.ID() {
		return fmt.Errorf("%w: event has %q, accumulator has %q",
			errAccumulatorWrongEnvelope, ev.EnvelopeID, a.env.ID())
	}
	if ev.Hash != "" || ev.PrevHash != "" || ev.SequenceNo != 0 {
		return fmt.Errorf("%w (Hash=%q PrevHash=%q SequenceNo=%d)",
			errAccumulatorPreHashedEvent, ev.Hash, ev.PrevHash, ev.SequenceNo)
	}
	a.pendingEvents = append(a.pendingEvents, ev)
	return nil
}

// recordObservation queues an observational audit event (no state change).
// Observational events record facts discovered during evaluation:
// surface_resolved, agent_resolved, authority_chain_resolved, confidence_checked, etc.
// The event is constructed via audit.NewEvent using the accumulator's envelope ID,
// then queued via recordEvent.
//
// Returns any error from recordEvent.
func (a *evaluationAccumulator) recordObservation(
	requestSource, requestID string,
	eventType audit.AuditEventType,
	payload map[string]any,
) error {
	ev := audit.NewEvent(
		a.env.ID(), requestSource, requestID,
		eventType,
		audit.EventPerformerSystem,
		"midas-orchestrator",
		payload,
	)
	return a.recordEvent(ev)
}

// recordLifecycle queues a lifecycle audit event (a state transition marker).
// Lifecycle events correspond to envelope state machine transitions:
// envelope_created, evaluation_started, outcome_recorded, envelope_closed, etc.
// The event is constructed via audit.NewEvent using the accumulator's envelope ID,
// then queued via recordEvent.
//
// NOTE: Currently identical to recordObservation in implementation — separated
// for semantic clarity. The two may be merged once the accumulator fully replaces
// applyStep and the distinction between lifecycle and observational events is
// expressed only in the event type constant.
//
// Returns any error from recordEvent.
func (a *evaluationAccumulator) recordLifecycle(
	requestSource, requestID string,
	eventType audit.AuditEventType,
	payload map[string]any,
) error {
	ev := audit.NewEvent(
		a.env.ID(), requestSource, requestID,
		eventType,
		audit.EventPerformerSystem,
		"midas-orchestrator",
		payload,
	)
	return a.recordEvent(ev)
}

// absorbPersistedEvent updates the envelope's Integrity section to reflect an
// audit event that has just been successfully appended to the repository.
// It must be called after each successful Audit.Append, in the same order
// that events were appended.
//
// Unlike the current applyStep flow (which populates AuditEventIDs and
// FinalEventHash only for lifecycle events), the accumulator calls
// absorbPersistedEvent for every event — lifecycle and observational — so the
// integrity index is complete and the AuditEventIDs count matches the total
// event count.
//
// Fields updated on each call:
//   - Integrity.FirstEventHash — set once from the first appended event; immutable thereafter
//   - Integrity.FinalEventHash — always updated to the most-recently-appended event's hash
//   - Integrity.AuditEventIDs — the ID of every appended event, appended in order
func (a *evaluationAccumulator) absorbPersistedEvent(ev *audit.AuditEvent) {
	if a.env.Integrity.FirstEventHash == "" {
		a.env.Integrity.FirstEventHash = ev.Hash
	}
	a.env.Integrity.FinalEventHash = ev.Hash
	a.env.Integrity.AuditEventIDs = append(a.env.Integrity.AuditEventIDs, ev.ID)
}

// persist writes the complete evaluation to the database in three steps:
//
//  1. Envelopes.Create — establishes the envelope row before any audit rows.
//     Required by the FK constraint: audit_events.envelope_id → operational_envelopes(id).
//
//  2. Audit.Append × N — appends each queued event in declaration order.
//     The repository computes SequenceNo, PrevHash, and EventHash internally.
//     absorbPersistedEvent is called after each successful Append to track
//     the integrity anchors in the in-memory envelope.
//
//  3. Envelopes.Update — single final write that persists the terminal envelope
//     state: all resolved sections, evaluation outcome, state, ClosedAt, and
//     the complete integrity record (FirstEventHash, FinalEventHash, AuditEventIDs).
//
// Pre-flight checks (run before any DB writes):
//   - accumulator must not have been persisted already
//   - envelope must be in a terminal state (CLOSED or AWAITING_REVIEW)
//   - envelope must have Evaluation.Outcome set
//
// FAILURE SEMANTICS: If persist returns an error, the accumulator must be
// discarded. The caller's transaction rollback removes any DB writes, but
// the in-memory envelope's Integrity section may be partially updated (if
// some Appends succeeded before the failure). Do not reuse after error.
//
// On success, persisted is set to true; any subsequent call to any mutating
// method returns errAccumulatorAlreadyPersisted.
func (a *evaluationAccumulator) persist(
	ctx context.Context,
	envRepo envelope.EnvelopeRepository,
	auditRepo audit.AuditEventRepository,
) error {
	if a.persisted {
		return fmt.Errorf("persist [%s]: %w", a.env.ID(), errAccumulatorAlreadyPersisted)
	}

	// Pre-flight: envelope must be in a terminal state before persisting.
	// CLOSED is the normal terminal state; AWAITING_REVIEW is the terminal state
	// for escalated envelopes that have not yet been reviewed.
	if a.env.State != envelope.EnvelopeStateClosed && a.env.State != envelope.EnvelopeStateAwaitingReview {
		return fmt.Errorf("persist [%s]: %w (got %s)", a.env.ID(), errAccumulatorNonTerminalState, a.env.State)
	}

	// Pre-flight: outcome must be set. Every evaluation path sets an Outcome
	// before reaching a terminal state; a missing Outcome indicates a caller bug.
	if a.env.Evaluation.Outcome == "" {
		return fmt.Errorf("persist [%s]: %w", a.env.ID(), errAccumulatorMissingOutcome)
	}

	// Pre-flight: at least one audit event must be queued. An empty event list
	// means the evaluation flow did not record the mandatory envelope_created event.
	if len(a.pendingEvents) == 0 {
		return fmt.Errorf("persist [%s]: no audit events queued — evaluation incomplete", a.env.ID())
	}

	// Guard against nil repositories.
	if envRepo == nil {
		return fmt.Errorf("persist [%s]: envelope repository is nil", a.env.ID())
	}
	if auditRepo == nil {
		return fmt.Errorf("persist [%s]: audit repository is nil", a.env.ID())
	}

	// Step 1: Create the envelope row. This must come before any Audit.Append
	// to satisfy the FK constraint (audit_events.envelope_id → operational_envelopes(id)).
	if err := envRepo.Create(ctx, a.env); err != nil {
		return fmt.Errorf("create envelope [%s]: %w", a.env.ID(), err)
	}

	// Step 2: Append audit events in declaration order. Each Append assigns
	// SequenceNo and computes the hash chain; we absorb results into Integrity.
	for _, ev := range a.pendingEvents {
		if err := auditRepo.Append(ctx, ev); err != nil {
			return fmt.Errorf("audit append %s [envelope %s]: %w", ev.EventType, a.env.ID(), err)
		}
		a.absorbPersistedEvent(ev)
	}

	// Step 3: Flush the complete terminal envelope state in a single write.
	// This is the only Envelopes.Update call — it carries the final state,
	// all resolved sections, evaluation outcome, and complete integrity anchors.
	if err := envRepo.Update(ctx, a.env); err != nil {
		return fmt.Errorf("persist final envelope state [%s]: %w", a.env.ID(), err)
	}

	// Mark as persisted — guard against accidental reuse.
	a.persisted = true
	return nil
}
