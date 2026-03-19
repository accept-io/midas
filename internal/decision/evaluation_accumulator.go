package decision

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/store"
)

// Sentinel errors returned by evaluationAccumulator methods.
var (
	// errAccumulatorAlreadyPersisted is returned when any mutating method is
	// called after a successful persist call.
	errAccumulatorAlreadyPersisted = errors.New("accumulator has already been persisted")

	// errAccumulatorNilEnvelope is returned by accumulator constructors when
	// the provided envelope pointer is nil.
	errAccumulatorNilEnvelope = errors.New("envelope must not be nil")

	// errAccumulatorWrongState is returned by accumulator constructors when
	// the envelope is not in the required starting state.
	errAccumulatorWrongState = errors.New("envelope is in unexpected state")

	// errAccumulatorNilEvent is returned by recordEvent when ev is nil.
	errAccumulatorNilEvent = errors.New("audit event must not be nil")

	// errAccumulatorWrongEnvelope is returned by recordEvent when the event's
	// EnvelopeID does not match the accumulator's envelope ID.
	errAccumulatorWrongEnvelope = errors.New("audit event envelope_id does not match accumulator envelope")

	// errAccumulatorPreHashedEvent is returned by recordEvent when the event
	// has pre-populated hash fields. The repository assigns SequenceNo, PrevHash,
	// and EventHash at Append time; a pre-set event indicates a caller error.
	errAccumulatorPreHashedEvent = errors.New("audit event must not have pre-populated hash fields (Hash, PrevHash, SequenceNo)")

	// errAccumulatorNonTerminalState is returned by persistNew when the envelope
	// is not yet in a terminal state (CLOSED or AWAITING_REVIEW).
	errAccumulatorNonTerminalState = errors.New("envelope must be in a terminal state (CLOSED or AWAITING_REVIEW) before persisting")

	// errAccumulatorMissingOutcome is returned by persistNew when the envelope
	// has no Outcome set. An outcome must be recorded before persisting.
	errAccumulatorMissingOutcome = errors.New("envelope must have Evaluation.Outcome set before persisting")
)

// evaluationAccumulator builds envelope state and audit events in memory,
// then flushes the complete result atomically.
//
// Two persistence modes cover the two orchestrator flows:
//
//   - persistNew (Evaluate): Create → N×Append → Update
//     Creates a new envelope row, satisfying the FK constraint before any
//     audit rows are appended.
//
//   - persistExisting (ResolveEscalation): N×Append → Update
//     The envelope row already exists; only audit events and the updated
//     state are written.
//
// Integrity guarantees:
//   - Hash chain intact: Appends run in declaration order; the repository
//     assigns SequenceNo, PrevHash, and EventHash.
//   - Integrity anchors complete: absorbPersistedEvent updates FirstEventHash,
//     FinalEventHash, and AuditEventIDs after every successful Append.
//
// FAILURE SEMANTICS: If any persist call returns an error the accumulator must
// be discarded. The caller's transaction rollback removes any DB writes made
// before the failure, but the in-memory Integrity section may be partially
// updated. Do not reuse after error.
//
// CONCURRENCY: Not safe for concurrent access. Each Evaluate and
// ResolveEscalation call creates its own accumulator instance.
//
// Usage — new evaluation (Evaluate path):
//
//	acc, err := newEvaluationAccumulator(env)
//	if err != nil { return err }
//	acc.recordLifecycle(src, rid, audit.AuditEventEnvelopeCreated, nil)
//	acc.transition(envelope.EnvelopeStateEvaluating, now)
//	// ... resolve authority, check thresholds, record observations ...
//	acc.transition(envelope.EnvelopeStateClosed, now)
//	acc.recordLifecycle(src, rid, audit.AuditEventEnvelopeClosed, nil)
//	return acc.persistNew(ctx, repos)
//
// Usage — escalation resolution (ResolveEscalation path):
//
//	acc, err := newExistingEnvelopeAccumulator(env) // env in AWAITING_REVIEW
//	if err != nil { return err }
//	acc.recordObservation(src, rid, audit.AuditEventEscalationReviewed, payload)
//	acc.transition(envelope.EnvelopeStateClosed, now)
//	acc.recordLifecycle(src, rid, audit.AuditEventEnvelopeClosed, payload)
//	return acc.persistExisting(ctx, repos)
type evaluationAccumulator struct {
	env           *envelope.Envelope
	pendingEvents []*audit.AuditEvent
	persisted     bool // true after a successful persist call; guards against reuse
}

// newEvaluationAccumulator creates an accumulator for a new evaluation.
// Requires env to be non-nil and in RECEIVED state.
// pendingEvents is pre-allocated with capacity 16, which covers the longest
// evaluation path without reallocation.
func newEvaluationAccumulator(env *envelope.Envelope) (*evaluationAccumulator, error) {
	if env == nil {
		return nil, errAccumulatorNilEnvelope
	}
	if env.State != envelope.EnvelopeStateReceived {
		return nil, fmt.Errorf("%w: expected RECEIVED, got %s", errAccumulatorWrongState, env.State)
	}
	return &evaluationAccumulator{
		env:           env,
		pendingEvents: make([]*audit.AuditEvent, 0, 16),
	}, nil
}

// newExistingEnvelopeAccumulator creates an accumulator for an already-persisted
// envelope in AWAITING_REVIEW state. Used by ResolveEscalation to queue and flush
// the review and close events atomically.
func newExistingEnvelopeAccumulator(env *envelope.Envelope) (*evaluationAccumulator, error) {
	if env == nil {
		return nil, errAccumulatorNilEnvelope
	}
	if env.State != envelope.EnvelopeStateAwaitingReview {
		return nil, fmt.Errorf("%w: expected AWAITING_REVIEW, got %s", errAccumulatorWrongState, env.State)
	}
	return &evaluationAccumulator{
		env:           env,
		pendingEvents: make([]*audit.AuditEvent, 0, 4),
	}, nil
}

// transition advances the in-memory envelope to next, enforcing the state
// machine's structural and content invariants immediately (via Envelope.Transition).
// No DB write occurs — the new state is deferred to the persist call.
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
// Kept separate from recordObservation to distinguish transition markers from
// observational facts at call sites, even though both are queued the same way.
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

// flushEventsAndUpdate appends all queued events in declaration order, absorbing
// each into Integrity, then writes the final envelope state in a single Update.
// Called by persistNew and persistExisting after their respective pre-flight checks.
func (a *evaluationAccumulator) flushEventsAndUpdate(ctx context.Context, repos *store.Repositories) error {
	for _, ev := range a.pendingEvents {
		if err := repos.Audit.Append(ctx, ev); err != nil {
			return fmt.Errorf("audit append %s [envelope %s]: %w", ev.EventType, a.env.ID(), err)
		}
		a.absorbPersistedEvent(ev)
	}
	if err := repos.Envelopes.Update(ctx, a.env); err != nil {
		return fmt.Errorf("persist final envelope state [%s]: %w", a.env.ID(), err)
	}
	return nil
}

// persistNew writes a complete new evaluation to the database:
//
//  1. Envelopes.Create — establishes the envelope row before any audit rows.
//     Required by the FK constraint: audit_events.envelope_id → operational_envelopes(id).
//
//  2. N×Audit.Append + Envelopes.Update — via flushEventsAndUpdate.
//
// Pre-flight checks:
//   - accumulator must not have been persisted already
//   - envelope must be in a terminal state (CLOSED or AWAITING_REVIEW)
//   - envelope must have Evaluation.Outcome set
//   - at least one audit event must be queued
//
// On success, persisted is set to true. Do not reuse after error.
func (a *evaluationAccumulator) persistNew(
	ctx context.Context,
	repos *store.Repositories,
) error {
	if a.persisted {
		return fmt.Errorf("persistNew [%s]: %w", a.env.ID(), errAccumulatorAlreadyPersisted)
	}

	// CLOSED is the normal terminal; AWAITING_REVIEW is the escalation terminal.
	if a.env.State != envelope.EnvelopeStateClosed && a.env.State != envelope.EnvelopeStateAwaitingReview {
		return fmt.Errorf("persistNew [%s]: %w (got %s)", a.env.ID(), errAccumulatorNonTerminalState, a.env.State)
	}

	if a.env.Evaluation.Outcome == "" {
		return fmt.Errorf("persistNew [%s]: %w", a.env.ID(), errAccumulatorMissingOutcome)
	}

	if len(a.pendingEvents) == 0 {
		return fmt.Errorf("persistNew [%s]: no audit events queued — evaluation incomplete", a.env.ID())
	}

	if err := repos.Envelopes.Create(ctx, a.env); err != nil {
		return fmt.Errorf("create envelope [%s]: %w", a.env.ID(), err)
	}

	if err := a.flushEventsAndUpdate(ctx, repos); err != nil {
		return err
	}

	a.persisted = true
	return nil
}

// persistExisting writes queued audit events and a final envelope update for
// an already-persisted envelope. Does NOT call Envelopes.Create.
//
// Steps (via flushEventsAndUpdate):
//  1. N×Audit.Append — in declaration order, absorbPersistedEvent after each
//  2. Envelopes.Update — single final write with the complete updated state
//
// Pre-flight checks:
//   - accumulator must not have been persisted already
//   - at least one audit event must be queued
//
// On success, persisted is set to true. Do not reuse after error.
func (a *evaluationAccumulator) persistExisting(
	ctx context.Context,
	repos *store.Repositories,
) error {
	if a.persisted {
		return fmt.Errorf("persistExisting [%s]: %w", a.env.ID(), errAccumulatorAlreadyPersisted)
	}

	if len(a.pendingEvents) == 0 {
		return fmt.Errorf("persistExisting [%s]: no audit events queued", a.env.ID())
	}

	if err := a.flushEventsAndUpdate(ctx, repos); err != nil {
		return err
	}

	a.persisted = true
	return nil
}
