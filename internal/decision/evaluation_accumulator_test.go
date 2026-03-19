package decision

// Unit tests for evaluationAccumulator.
//
// This file is in package decision (not decision_test) because evaluationAccumulator
// is unexported — the tests access it directly to verify internal state.
//
// The fakes defined here (accFakeEnvRepo, accFakeAuditRepo, accCallLog) are
// intentionally scoped to this file and do not duplicate the fakes in
// orchestrator_lifecycle_test.go (which are in package decision_test).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/store"
)

var accTestNow = time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

// accMakeEnv returns a fresh RECEIVED envelope for accumulator tests.
func accMakeEnv(t *testing.T) *envelope.Envelope {
	t.Helper()
	env, err := envelope.New(
		"env-acc-001", "test-src", "req-acc-001",
		json.RawMessage(`{"action":"test"}`),
		accTestNow,
	)
	if err != nil {
		t.Fatalf("envelope.New: %v", err)
	}
	return env
}

// accMakeAcc creates an accumulator from env, fataling on error.
func accMakeAcc(t *testing.T, env *envelope.Envelope) *evaluationAccumulator {
	t.Helper()
	acc, err := newEvaluationAccumulator(env)
	if err != nil {
		t.Fatalf("newEvaluationAccumulator: %v", err)
	}
	return acc
}

// mustRecord fatals the test if err is non-nil. Used to collapse record* calls
// whose errors are not the subject of the current test.
func mustRecord(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("record: %v", err)
	}
}

// accDriveToAccept advances the accumulator through the full Accept path:
// RECEIVED → EVALUATING → OUTCOME_RECORDED → CLOSED, queuing 4 lifecycle events.
// It sets Explanation, Outcome, and ReasonCode on the envelope as required.
// Used by tests whose subject is persist() behaviour, not lifecycle transitions.
func accDriveToAccept(t *testing.T, acc *evaluationAccumulator) {
	t.Helper()
	env := acc.env

	mustRecord(t, acc.recordLifecycle(env.RequestSource(), env.RequestID(),
		audit.AuditEventEnvelopeCreated, nil))

	if err := acc.transition(envelope.EnvelopeStateEvaluating, accTestNow); err != nil {
		t.Fatalf("transition EVALUATING: %v", err)
	}
	mustRecord(t, acc.recordLifecycle(env.RequestSource(), env.RequestID(),
		audit.AuditEventEvaluationStarted, nil))

	env.Evaluation.Explanation = &envelope.DecisionExplanation{Result: "accept"}
	if err := acc.transition(envelope.EnvelopeStateOutcomeRecorded, accTestNow); err != nil {
		t.Fatalf("transition OUTCOME_RECORDED: %v", err)
	}
	mustRecord(t, acc.recordLifecycle(env.RequestSource(), env.RequestID(),
		audit.AuditEventOutcomeRecorded, nil))

	env.Evaluation.Outcome = eval.OutcomeAccept
	env.Evaluation.ReasonCode = eval.ReasonWithinAuthority
	if err := acc.transition(envelope.EnvelopeStateClosed, accTestNow); err != nil {
		t.Fatalf("transition CLOSED: %v", err)
	}
	mustRecord(t, acc.recordLifecycle(env.RequestSource(), env.RequestID(),
		audit.AuditEventEnvelopeClosed, nil))
}

// =============================================================================
// Fake repositories
//
// accCallLog records every Create, Update, and Append call in order, so tests
// can assert the exact operation sequence without checking DB state.
// =============================================================================

type accCallLog struct {
	ops []string
}

func (l *accCallLog) record(op string) {
	l.ops = append(l.ops, op)
}

// accFakeEnvRepo records Create/Update calls and stores rows in a map.
// createErr and updateErr inject failures for specific steps.
type accFakeEnvRepo struct {
	log       *accCallLog
	rows      map[string]*envelope.Envelope
	createErr error
	updateErr error
}

func newAccFakeEnvRepo(log *accCallLog) *accFakeEnvRepo {
	return &accFakeEnvRepo{log: log, rows: map[string]*envelope.Envelope{}}
}

func (r *accFakeEnvRepo) Create(_ context.Context, env *envelope.Envelope) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.log.record("create")
	r.rows[env.ID()] = env
	return nil
}

func (r *accFakeEnvRepo) Update(_ context.Context, env *envelope.Envelope) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.log.record("update")
	r.rows[env.ID()] = env
	return nil
}

func (r *accFakeEnvRepo) GetByID(_ context.Context, _ string) (*envelope.Envelope, error) {
	return nil, nil
}
func (r *accFakeEnvRepo) GetByRequestID(_ context.Context, _ string) (*envelope.Envelope, error) {
	return nil, nil
}
func (r *accFakeEnvRepo) GetByRequestScope(_ context.Context, _, _ string) (*envelope.Envelope, error) {
	return nil, nil
}
func (r *accFakeEnvRepo) List(_ context.Context) ([]*envelope.Envelope, error) { return nil, nil }

// accFakeAuditRepo records Append calls, assigns a deterministic hash chain,
// and injects failures after a configurable number of successful appends.
//
// If appendErr is non-nil, Append fails once appended >= failAfter.
// Default failAfter=0 means the first Append fails when appendErr is set.
type accFakeAuditRepo struct {
	log       *accCallLog
	events    []*audit.AuditEvent
	appended  int
	appendErr error
	failAfter int
}

func newAccFakeAuditRepo(log *accCallLog) *accFakeAuditRepo {
	return &accFakeAuditRepo{log: log}
}

func (r *accFakeAuditRepo) Append(_ context.Context, ev *audit.AuditEvent) error {
	if r.appendErr != nil && r.appended >= r.failAfter {
		return r.appendErr
	}
	// Assign hash chain like the real repository.
	ev.SequenceNo = len(r.events) + 1
	if len(r.events) > 0 {
		ev.PrevHash = r.events[len(r.events)-1].Hash
	}
	h := fmt.Sprintf("hash_%d_%s_%s", ev.SequenceNo, ev.EnvelopeID, ev.EventType)
	ev.Hash = h
	ev.EventHash = h

	r.log.record(fmt.Sprintf("append:%s", ev.EventType))
	r.events = append(r.events, ev)
	r.appended++
	return nil
}

func (r *accFakeAuditRepo) ListByEnvelopeID(_ context.Context, id string) ([]*audit.AuditEvent, error) {
	var out []*audit.AuditEvent
	for _, ev := range r.events {
		if ev.EnvelopeID == id {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (r *accFakeAuditRepo) ListByRequestID(_ context.Context, _ string) ([]*audit.AuditEvent, error) {
	return nil, nil
}

// makeAccTestRepos wires the fake repos into a *store.Repositories so tests
// can call the narrowed persist(ctx, repos) signature.
func makeAccTestRepos(envRepo *accFakeEnvRepo, auditRepo *accFakeAuditRepo) *store.Repositories {
	return &store.Repositories{
		Envelopes: envRepo,
		Audit:     auditRepo,
	}
}

// =============================================================================
// Test group 1: Constructor validation
// =============================================================================

func TestAccumulatorNew_Validation(t *testing.T) {
	t.Run("nil envelope returns errAccumulatorNilEnvelope", func(t *testing.T) {
		_, err := newEvaluationAccumulator(nil)
		if !errors.Is(err, errAccumulatorNilEnvelope) {
			t.Errorf("expected errAccumulatorNilEnvelope, got: %v", err)
		}
	})

	t.Run("non-RECEIVED state returns errAccumulatorWrongState", func(t *testing.T) {
		env := accMakeEnv(t)
		// Advance to EVALUATING so the state is no longer RECEIVED.
		if err := env.Transition(envelope.EnvelopeStateEvaluating, accTestNow); err != nil {
			t.Fatalf("Transition: %v", err)
		}
		_, err := newEvaluationAccumulator(env)
		if !errors.Is(err, errAccumulatorWrongState) {
			t.Errorf("expected errAccumulatorWrongState, got: %v", err)
		}
		// Error message must include the actual state for debuggability.
		if !strings.Contains(err.Error(), string(envelope.EnvelopeStateEvaluating)) {
			t.Errorf("error message missing actual state %q: %s", envelope.EnvelopeStateEvaluating, err)
		}
	})

	t.Run("valid RECEIVED envelope succeeds", func(t *testing.T) {
		env := accMakeEnv(t)
		acc, err := newEvaluationAccumulator(env)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if acc == nil {
			t.Fatal("returned nil accumulator")
		}
		if acc.persisted {
			t.Error("new accumulator must not be persisted")
		}
		if len(acc.pendingEvents) != 0 {
			t.Errorf("new accumulator has %d pending events, want 0", len(acc.pendingEvents))
		}
	})
}

// =============================================================================
// Test group 2: transition() validates the envelope state machine
// =============================================================================

func TestAccumulatorTransition_ValidatesStateMachine(t *testing.T) {
	t.Run("valid transition advances in-memory state", func(t *testing.T) {
		acc := accMakeAcc(t, accMakeEnv(t))
		if err := acc.transition(envelope.EnvelopeStateEvaluating, accTestNow); err != nil {
			t.Errorf("valid transition failed: %v", err)
		}
		if acc.env.State != envelope.EnvelopeStateEvaluating {
			t.Errorf("state not updated: got %q, want evaluating", acc.env.State)
		}
	})

	t.Run("invalid edge returns wrapped ErrInvalidTransition", func(t *testing.T) {
		acc := accMakeAcc(t, accMakeEnv(t))
		// RECEIVED → CLOSED is not a valid edge in the state machine.
		err := acc.transition(envelope.EnvelopeStateClosed, accTestNow)
		if !errors.Is(err, envelope.ErrInvalidTransition) {
			t.Errorf("expected ErrInvalidTransition, got: %v", err)
		}
		// State must not have changed on failure.
		if acc.env.State != envelope.EnvelopeStateReceived {
			t.Errorf("state mutated on failed transition: got %q", acc.env.State)
		}
	})

	t.Run("error message includes from-state and to-state", func(t *testing.T) {
		acc := accMakeAcc(t, accMakeEnv(t))
		err := acc.transition(envelope.EnvelopeStateClosed, accTestNow)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		msg := err.Error()
		if !strings.Contains(msg, string(envelope.EnvelopeStateReceived)) {
			t.Errorf("error missing from-state %q: %s", envelope.EnvelopeStateReceived, msg)
		}
		if !strings.Contains(msg, string(envelope.EnvelopeStateClosed)) {
			t.Errorf("error missing to-state %q: %s", envelope.EnvelopeStateClosed, msg)
		}
	})

	t.Run("missing Explanation blocks OUTCOME_RECORDED", func(t *testing.T) {
		acc := accMakeAcc(t, accMakeEnv(t))
		_ = acc.transition(envelope.EnvelopeStateEvaluating, accTestNow)
		err := acc.transition(envelope.EnvelopeStateOutcomeRecorded, accTestNow)
		if !errors.Is(err, envelope.ErrMissingExplanation) {
			t.Errorf("expected ErrMissingExplanation, got: %v", err)
		}
	})

	t.Run("missing Outcome or ReasonCode blocks CLOSED", func(t *testing.T) {
		acc := accMakeAcc(t, accMakeEnv(t))
		_ = acc.transition(envelope.EnvelopeStateEvaluating, accTestNow)
		acc.env.Evaluation.Explanation = &envelope.DecisionExplanation{Result: "test"}
		_ = acc.transition(envelope.EnvelopeStateOutcomeRecorded, accTestNow)

		// No Outcome or ReasonCode — must be blocked.
		err := acc.transition(envelope.EnvelopeStateClosed, accTestNow)
		if !errors.Is(err, envelope.ErrMissingOutcome) {
			t.Errorf("expected ErrMissingOutcome, got: %v", err)
		}

		// Outcome set but no ReasonCode — still blocked.
		acc.env.Evaluation.Outcome = eval.OutcomeAccept
		err = acc.transition(envelope.EnvelopeStateClosed, accTestNow)
		if !errors.Is(err, envelope.ErrMissingOutcome) {
			t.Errorf("expected ErrMissingOutcome with Outcome-only, got: %v", err)
		}

		// Both set — must succeed.
		acc.env.Evaluation.ReasonCode = eval.ReasonWithinAuthority
		if err := acc.transition(envelope.EnvelopeStateClosed, accTestNow); err != nil {
			t.Errorf("transition to CLOSED with Outcome+ReasonCode: unexpected error %v", err)
		}
	})

	t.Run("returns errAccumulatorAlreadyPersisted after persist", func(t *testing.T) {
		log := &accCallLog{}
		env := accMakeEnv(t)
		acc := accMakeAcc(t, env)
		accDriveToAccept(t, acc)
		if err := acc.persistNew(context.Background(), makeAccTestRepos(newAccFakeEnvRepo(log), newAccFakeAuditRepo(log))); err != nil {
			t.Fatalf("persist: %v", err)
		}
		// Further transition must fail.
		err := acc.transition(envelope.EnvelopeStateEvaluating, accTestNow)
		if !errors.Is(err, errAccumulatorAlreadyPersisted) {
			t.Errorf("expected errAccumulatorAlreadyPersisted, got: %v", err)
		}
	})
}

// =============================================================================
// Test group 3: recordEvent validation
// =============================================================================

func TestAccumulatorRecordEvent_Validation(t *testing.T) {
	t.Run("nil event returns errAccumulatorNilEvent", func(t *testing.T) {
		acc := accMakeAcc(t, accMakeEnv(t))
		if err := acc.recordEvent(nil); !errors.Is(err, errAccumulatorNilEvent) {
			t.Errorf("expected errAccumulatorNilEvent, got: %v", err)
		}
	})

	t.Run("wrong EnvelopeID returns errAccumulatorWrongEnvelope", func(t *testing.T) {
		acc := accMakeAcc(t, accMakeEnv(t))
		ev := audit.NewEvent(
			"different-envelope-id", "src", "rid",
			audit.AuditEventEnvelopeCreated, audit.EventPerformerSystem, "midas-orchestrator", nil,
		)
		err := acc.recordEvent(ev)
		if !errors.Is(err, errAccumulatorWrongEnvelope) {
			t.Errorf("expected errAccumulatorWrongEnvelope, got: %v", err)
		}
		if !strings.Contains(err.Error(), "different-envelope-id") {
			t.Errorf("error message missing wrong ID: %s", err)
		}
	})

	t.Run("pre-populated Hash returns errAccumulatorPreHashedEvent", func(t *testing.T) {
		env := accMakeEnv(t)
		acc := accMakeAcc(t, env)
		ev := audit.NewEvent(env.ID(), "src", "rid",
			audit.AuditEventEnvelopeCreated, audit.EventPerformerSystem, "midas-orchestrator", nil)
		ev.Hash = "already-computed"
		err := acc.recordEvent(ev)
		if !errors.Is(err, errAccumulatorPreHashedEvent) {
			t.Errorf("expected errAccumulatorPreHashedEvent, got: %v", err)
		}
	})

	t.Run("pre-populated PrevHash returns errAccumulatorPreHashedEvent", func(t *testing.T) {
		env := accMakeEnv(t)
		acc := accMakeAcc(t, env)
		ev := audit.NewEvent(env.ID(), "src", "rid",
			audit.AuditEventEnvelopeCreated, audit.EventPerformerSystem, "midas-orchestrator", nil)
		ev.PrevHash = "some-prev"
		err := acc.recordEvent(ev)
		if !errors.Is(err, errAccumulatorPreHashedEvent) {
			t.Errorf("expected errAccumulatorPreHashedEvent, got: %v", err)
		}
	})

	t.Run("pre-populated SequenceNo returns errAccumulatorPreHashedEvent", func(t *testing.T) {
		env := accMakeEnv(t)
		acc := accMakeAcc(t, env)
		ev := audit.NewEvent(env.ID(), "src", "rid",
			audit.AuditEventEnvelopeCreated, audit.EventPerformerSystem, "midas-orchestrator", nil)
		ev.SequenceNo = 7
		err := acc.recordEvent(ev)
		if !errors.Is(err, errAccumulatorPreHashedEvent) {
			t.Errorf("expected errAccumulatorPreHashedEvent, got: %v", err)
		}
	})

	t.Run("returns errAccumulatorAlreadyPersisted after persist", func(t *testing.T) {
		log := &accCallLog{}
		env := accMakeEnv(t)
		acc := accMakeAcc(t, env)
		accDriveToAccept(t, acc)
		if err := acc.persistNew(context.Background(), makeAccTestRepos(newAccFakeEnvRepo(log), newAccFakeAuditRepo(log))); err != nil {
			t.Fatalf("persist: %v", err)
		}
		ev := audit.NewEvent(env.ID(), "src", "rid",
			audit.AuditEventEnvelopeCreated, audit.EventPerformerSystem, "midas-orchestrator", nil)
		if err := acc.recordEvent(ev); !errors.Is(err, errAccumulatorAlreadyPersisted) {
			t.Errorf("expected errAccumulatorAlreadyPersisted, got: %v", err)
		}
	})
}

// =============================================================================
// Test group 4: recordEvent and record* queue without persisting
// =============================================================================

func TestAccumulatorRecord_QueuesWithoutPersisting(t *testing.T) {
	env := accMakeEnv(t)
	acc := accMakeAcc(t, env)

	// New accumulator must have an empty queue.
	if len(acc.pendingEvents) != 0 {
		t.Fatalf("new accumulator has %d pending events, want 0", len(acc.pendingEvents))
	}

	// recordEvent queues a pre-built event.
	ev1 := audit.NewEvent(
		env.ID(), env.RequestSource(), env.RequestID(),
		audit.AuditEventEnvelopeCreated, audit.EventPerformerSystem, "midas-orchestrator", nil,
	)
	mustRecord(t, acc.recordEvent(ev1))
	if len(acc.pendingEvents) != 1 {
		t.Fatalf("after recordEvent: got %d pending events, want 1", len(acc.pendingEvents))
	}
	if acc.pendingEvents[0] != ev1 {
		t.Error("pendingEvents[0] is not the queued event pointer")
	}

	// recordObservation creates and queues.
	mustRecord(t, acc.recordObservation(env.RequestSource(), env.RequestID(),
		audit.AuditEventSurfaceResolved, map[string]any{"surface_id": "s1"}))
	if len(acc.pendingEvents) != 2 {
		t.Fatalf("after recordObservation: got %d pending events, want 2", len(acc.pendingEvents))
	}
	if acc.pendingEvents[1].EventType != audit.AuditEventSurfaceResolved {
		t.Errorf("pendingEvents[1].EventType = %q, want SURFACE_RESOLVED", acc.pendingEvents[1].EventType)
	}
	if acc.pendingEvents[1].EnvelopeID != env.ID() {
		t.Errorf("pendingEvents[1].EnvelopeID = %q, want %q", acc.pendingEvents[1].EnvelopeID, env.ID())
	}

	// recordLifecycle creates and queues.
	mustRecord(t, acc.recordLifecycle(env.RequestSource(), env.RequestID(),
		audit.AuditEventEvaluationStarted, nil))
	if len(acc.pendingEvents) != 3 {
		t.Fatalf("after recordLifecycle: got %d pending events, want 3", len(acc.pendingEvents))
	}
	if acc.pendingEvents[2].EventType != audit.AuditEventEvaluationStarted {
		t.Errorf("pendingEvents[2].EventType = %q, want EVALUATION_STARTED", acc.pendingEvents[2].EventType)
	}

	// Integrity must be untouched — no absorbPersistedEvent has been called.
	if acc.env.Integrity.FirstEventHash != "" {
		t.Error("FirstEventHash set before persist — should be empty")
	}
	if acc.env.Integrity.FinalEventHash != "" {
		t.Error("FinalEventHash set before persist — should be empty")
	}
	if len(acc.env.Integrity.AuditEventIDs) != 0 {
		t.Errorf("AuditEventIDs has %d entries before persist — should be empty",
			len(acc.env.Integrity.AuditEventIDs))
	}
}

// =============================================================================
// Test group 5: absorbPersistedEvent updates Integrity fields correctly
// =============================================================================

func TestAccumulatorAbsorbPersistedEvent_UpdatesIntegrity(t *testing.T) {
	acc := accMakeAcc(t, accMakeEnv(t))

	ev1 := &audit.AuditEvent{ID: "id-001", EventType: audit.AuditEventEnvelopeCreated, Hash: "hash-aaa", EventHash: "hash-aaa"}
	ev2 := &audit.AuditEvent{ID: "id-002", EventType: audit.AuditEventEvaluationStarted, Hash: "hash-bbb", EventHash: "hash-bbb"}
	ev3 := &audit.AuditEvent{ID: "id-003", EventType: audit.AuditEventEnvelopeClosed, Hash: "hash-ccc", EventHash: "hash-ccc"}

	// First absorb: anchors FirstEventHash; FinalEventHash = same value.
	acc.absorbPersistedEvent(ev1)
	if acc.env.Integrity.FirstEventHash != "hash-aaa" {
		t.Errorf("FirstEventHash after ev1: got %q, want hash-aaa", acc.env.Integrity.FirstEventHash)
	}
	if acc.env.Integrity.FinalEventHash != "hash-aaa" {
		t.Errorf("FinalEventHash after ev1: got %q, want hash-aaa", acc.env.Integrity.FinalEventHash)
	}
	if len(acc.env.Integrity.AuditEventIDs) != 1 || acc.env.Integrity.AuditEventIDs[0] != "id-001" {
		t.Errorf("AuditEventIDs after ev1: got %v, want [id-001]", acc.env.Integrity.AuditEventIDs)
	}

	// Second absorb: FirstEventHash must NOT change; FinalEventHash advances.
	acc.absorbPersistedEvent(ev2)
	if acc.env.Integrity.FirstEventHash != "hash-aaa" {
		t.Errorf("FirstEventHash changed on ev2: got %q, want hash-aaa (immutable)", acc.env.Integrity.FirstEventHash)
	}
	if acc.env.Integrity.FinalEventHash != "hash-bbb" {
		t.Errorf("FinalEventHash after ev2: got %q, want hash-bbb", acc.env.Integrity.FinalEventHash)
	}
	if len(acc.env.Integrity.AuditEventIDs) != 2 {
		t.Errorf("AuditEventIDs len after ev2: got %d, want 2", len(acc.env.Integrity.AuditEventIDs))
	}

	// Third absorb: FinalEventHash advances again; FirstEventHash unchanged.
	acc.absorbPersistedEvent(ev3)
	if acc.env.Integrity.FirstEventHash != "hash-aaa" {
		t.Errorf("FirstEventHash changed on ev3: got %q, want hash-aaa (immutable)", acc.env.Integrity.FirstEventHash)
	}
	if acc.env.Integrity.FinalEventHash != "hash-ccc" {
		t.Errorf("FinalEventHash after ev3: got %q, want hash-ccc", acc.env.Integrity.FinalEventHash)
	}
	if len(acc.env.Integrity.AuditEventIDs) != 3 || acc.env.Integrity.AuditEventIDs[2] != "id-003" {
		t.Errorf("AuditEventIDs after ev3: got %v, want [id-001 id-002 id-003]",
			acc.env.Integrity.AuditEventIDs)
	}
}

// =============================================================================
// Test group 6: persistNew() pre-flight checks
// =============================================================================

func TestAccumulatorPersist_PreflightChecks(t *testing.T) {
	t.Run("non-terminal state returns errAccumulatorNonTerminalState", func(t *testing.T) {
		// persistNew() must reject an envelope not in a terminal state.
		// EVALUATING is not terminal; the pre-flight must fire before any DB write.
		log := &accCallLog{}
		env := accMakeEnv(t)
		acc := accMakeAcc(t, env)
		if err := acc.transition(envelope.EnvelopeStateEvaluating, accTestNow); err != nil {
			t.Fatalf("transition EVALUATING: %v", err)
		}
		repos := makeAccTestRepos(newAccFakeEnvRepo(log), newAccFakeAuditRepo(log))
		err := acc.persistNew(context.Background(), repos)
		if !errors.Is(err, errAccumulatorNonTerminalState) {
			t.Errorf("expected errAccumulatorNonTerminalState, got: %v", err)
		}
		if len(log.ops) != 0 {
			t.Errorf("DB operations attempted before pre-flight check: %v", log.ops)
		}
	})

	t.Run("missing outcome returns errAccumulatorMissingOutcome", func(t *testing.T) {
		// persistNew() must reject an envelope that reached CLOSED but has no Outcome.
		// Drive to CLOSED via accDriveToAccept then clear Outcome to simulate.
		log := &accCallLog{}
		env := accMakeEnv(t)
		acc := accMakeAcc(t, env)
		accDriveToAccept(t, acc)
		env.Evaluation.Outcome = "" // clear after transition to simulate missing outcome

		repos := makeAccTestRepos(newAccFakeEnvRepo(log), newAccFakeAuditRepo(log))
		err := acc.persistNew(context.Background(), repos)
		if !errors.Is(err, errAccumulatorMissingOutcome) {
			t.Errorf("expected errAccumulatorMissingOutcome, got: %v", err)
		}
		if len(log.ops) != 0 {
			t.Errorf("DB operations attempted before pre-flight check: %v", log.ops)
		}
	})

	t.Run("empty queue returns error (evaluation incomplete)", func(t *testing.T) {
		// Even in a terminal state with Outcome set, an empty event queue is
		// rejected — at minimum envelope_created must be queued.
		// Bypass acc.transition to advance state without queuing events.
		log := &accCallLog{}
		env := accMakeEnv(t)
		acc := accMakeAcc(t, env)

		env.Evaluation.Explanation = &envelope.DecisionExplanation{Result: "accept"}
		env.Evaluation.Outcome = eval.OutcomeAccept
		env.Evaluation.ReasonCode = eval.ReasonWithinAuthority
		// Advance state directly on the envelope, not through the accumulator.
		_ = env.Transition(envelope.EnvelopeStateEvaluating, accTestNow)
		_ = env.Transition(envelope.EnvelopeStateOutcomeRecorded, accTestNow)
		_ = env.Transition(envelope.EnvelopeStateClosed, accTestNow)

		repos := makeAccTestRepos(newAccFakeEnvRepo(log), newAccFakeAuditRepo(log))
		err := acc.persistNew(context.Background(), repos)
		if err == nil {
			t.Error("expected error for empty event queue, got nil")
		}
		if len(log.ops) != 0 {
			t.Errorf("DB operations attempted before pre-flight check: %v", log.ops)
		}
	})

	t.Run("returns errAccumulatorAlreadyPersisted on double persistNew", func(t *testing.T) {
		log := &accCallLog{}
		env := accMakeEnv(t)
		acc := accMakeAcc(t, env)
		repos := makeAccTestRepos(newAccFakeEnvRepo(log), newAccFakeAuditRepo(log))
		accDriveToAccept(t, acc)

		if err := acc.persistNew(context.Background(), repos); err != nil {
			t.Fatalf("first persistNew: %v", err)
		}
		err := acc.persistNew(context.Background(), repos)
		if !errors.Is(err, errAccumulatorAlreadyPersisted) {
			t.Errorf("expected errAccumulatorAlreadyPersisted on second persistNew, got: %v", err)
		}
	})
}

// =============================================================================
// Test group 7: persistNew() calls repos in the correct order: Create → N×Append → Update
// =============================================================================

func TestAccumulatorPersist_CallOrder(t *testing.T) {
	log := &accCallLog{}
	envRepo := newAccFakeEnvRepo(log)
	auditRepo := newAccFakeAuditRepo(log)
	repos := makeAccTestRepos(envRepo, auditRepo)

	env := accMakeEnv(t)
	acc := accMakeAcc(t, env)
	accDriveToAccept(t, acc) // queues 4 events; drives to CLOSED

	if err := acc.persistNew(context.Background(), repos); err != nil {
		t.Fatalf("persist: %v", err)
	}

	// Verify exact call sequence: 1 Create → N×Append → 1 Update.
	want := []string{
		"create",
		fmt.Sprintf("append:%s", audit.AuditEventEnvelopeCreated),
		fmt.Sprintf("append:%s", audit.AuditEventEvaluationStarted),
		fmt.Sprintf("append:%s", audit.AuditEventOutcomeRecorded),
		fmt.Sprintf("append:%s", audit.AuditEventEnvelopeClosed),
		"update",
	}
	if len(log.ops) != len(want) {
		t.Fatalf("call log: got %v, want %v", log.ops, want)
	}
	for i, op := range want {
		if log.ops[i] != op {
			t.Errorf("call[%d]: got %q, want %q", i, log.ops[i], op)
		}
	}

	// Exactly 1 Create and 1 Update.
	creates, updates := 0, 0
	for _, op := range log.ops {
		switch op {
		case "create":
			creates++
		case "update":
			updates++
		}
	}
	if creates != 1 {
		t.Errorf("Create called %d times, want 1", creates)
	}
	if updates != 1 {
		t.Errorf("Update called %d times, want 1", updates)
	}

	// Integrity anchors fully populated after persist.
	if env.Integrity.FirstEventHash == "" {
		t.Error("Integrity.FirstEventHash is empty after persist")
	}
	if env.Integrity.FinalEventHash == "" {
		t.Error("Integrity.FinalEventHash is empty after persist")
	}
	// All 4 events tracked.
	if len(env.Integrity.AuditEventIDs) != 4 {
		t.Errorf("Integrity.AuditEventIDs len: got %d, want 4", len(env.Integrity.AuditEventIDs))
	}
	// persisted flag set after success.
	if !acc.persisted {
		t.Error("persisted flag not set after successful persist")
	}
}

// =============================================================================
// Test group 8: persist() error paths
// =============================================================================

func TestAccumulatorPersist_CreateFailure(t *testing.T) {
	// persistNew: Create fails → no Appends attempted, no Update attempted, persisted stays false.
	log := &accCallLog{}
	envRepo := newAccFakeEnvRepo(log)
	auditRepo := newAccFakeAuditRepo(log)

	sentinelErr := errors.New("db: duplicate key")
	envRepo.createErr = sentinelErr

	env := accMakeEnv(t)
	acc := accMakeAcc(t, env)
	accDriveToAccept(t, acc)
	repos := makeAccTestRepos(envRepo, auditRepo)

	err := acc.persistNew(context.Background(), repos)
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected sentinel error, got: %v", err)
	}

	// No Appends or Updates must have been attempted after Create failure.
	for _, op := range log.ops {
		if strings.HasPrefix(op, "append:") {
			t.Errorf("Append called after Create failure: %q", op)
		}
		if op == "update" {
			t.Error("Update called after Create failure")
		}
	}
	if acc.persisted {
		t.Error("persisted flag set despite Create failure")
	}
}

func TestAccumulatorPersist_UpdateFailure(t *testing.T) {
	// persistNew: Create and all Appends succeed; final Update fails → error returned, persisted stays false.
	log := &accCallLog{}
	envRepo := newAccFakeEnvRepo(log)
	auditRepo := newAccFakeAuditRepo(log)

	sentinelErr := errors.New("db: connection refused")
	envRepo.updateErr = sentinelErr

	env := accMakeEnv(t)
	acc := accMakeAcc(t, env)
	accDriveToAccept(t, acc)
	repos := makeAccTestRepos(envRepo, auditRepo)

	err := acc.persistNew(context.Background(), repos)
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected sentinel error, got: %v", err)
	}

	// Create and all 4 Appends must have been attempted before Update was tried.
	createCount, appendCount := 0, 0
	for _, op := range log.ops {
		if op == "create" {
			createCount++
		}
		if strings.HasPrefix(op, "append:") {
			appendCount++
		}
	}
	if createCount != 1 {
		t.Errorf("expected 1 Create before Update failure, got %d", createCount)
	}
	if appendCount != 4 {
		t.Errorf("expected 4 Appends before Update failure, got %d", appendCount)
	}
	// persisted must remain false.
	if acc.persisted {
		t.Error("persisted flag set despite Update failure")
	}
}

func TestAccumulatorPersist_AppendFailure(t *testing.T) {
	// persistNew: Create succeeds; first Append fails → Update not attempted, Integrity unchanged.
	log := &accCallLog{}
	envRepo := newAccFakeEnvRepo(log)
	auditRepo := newAccFakeAuditRepo(log)

	sentinelErr := errors.New("audit: disk quota exceeded")
	auditRepo.appendErr = sentinelErr
	auditRepo.failAfter = 0 // fail on first Append

	env := accMakeEnv(t)
	acc := accMakeAcc(t, env)
	accDriveToAccept(t, acc)
	repos := makeAccTestRepos(envRepo, auditRepo)

	err := acc.persistNew(context.Background(), repos)
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected sentinel error, got: %v", err)
	}

	// Create must have been called (it precedes all Appends).
	createCount := 0
	for _, op := range log.ops {
		if op == "create" {
			createCount++
		}
	}
	if createCount != 1 {
		t.Errorf("expected Create to be called before first Append, got %d Create calls", createCount)
	}

	// Update must NOT have been called.
	for _, op := range log.ops {
		if op == "update" {
			t.Error("Update called after Append failure")
		}
	}
	// absorbPersistedEvent not called for failed Append — Integrity unchanged.
	if env.Integrity.FirstEventHash != "" {
		t.Errorf("FirstEventHash set after Append failure: %q", env.Integrity.FirstEventHash)
	}
	if len(env.Integrity.AuditEventIDs) != 0 {
		t.Errorf("AuditEventIDs has %d entries after first Append failure (want 0)",
			len(env.Integrity.AuditEventIDs))
	}
	// persisted must remain false.
	if acc.persisted {
		t.Error("persisted flag set despite Append failure")
	}
}

func TestAccumulatorPersist_PartialAppendFailure(t *testing.T) {
	// persistNew: Create succeeds; first 2 Appends succeed; 3rd fails. Verify partial absorb and no Update.
	log := &accCallLog{}
	envRepo := newAccFakeEnvRepo(log)
	auditRepo := newAccFakeAuditRepo(log)

	sentinelErr := errors.New("audit: backend unavailable")
	auditRepo.appendErr = sentinelErr
	auditRepo.failAfter = 2 // first 2 succeed; 3rd fails

	env := accMakeEnv(t)
	acc := accMakeAcc(t, env)
	accDriveToAccept(t, acc) // queues 4 events; 3rd will fail
	repos := makeAccTestRepos(envRepo, auditRepo)

	err := acc.persistNew(context.Background(), repos)
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected sentinel error, got: %v", err)
	}

	// The 2 successful Appends were absorbed into Integrity.
	if len(env.Integrity.AuditEventIDs) != 2 {
		t.Errorf("AuditEventIDs after 2 successful appends: got %d, want 2",
			len(env.Integrity.AuditEventIDs))
	}
	if env.Integrity.FirstEventHash == "" {
		t.Error("FirstEventHash should be set after 1st successful Append")
	}

	// Update not attempted.
	for _, op := range log.ops {
		if op == "update" {
			t.Error("Update called despite Append failure")
		}
	}
	if acc.persisted {
		t.Error("persisted flag set despite partial Append failure")
	}
	// Create must have been called before any Append.
	createCount := 0
	for _, op := range log.ops {
		if op == "create" {
			createCount++
		}
	}
	if createCount != 1 {
		t.Errorf("expected 1 Create, got %d", createCount)
	}
}

// =============================================================================
// Test group 9: newExistingEnvelopeAccumulator constructor validation
// =============================================================================

func TestExistingEnvelopeAccumulator_Validation(t *testing.T) {
	t.Run("nil envelope returns errAccumulatorNilEnvelope", func(t *testing.T) {
		_, err := newExistingEnvelopeAccumulator(nil)
		if !errors.Is(err, errAccumulatorNilEnvelope) {
			t.Errorf("expected errAccumulatorNilEnvelope, got: %v", err)
		}
	})

	t.Run("non-AWAITING_REVIEW state returns errAccumulatorWrongState", func(t *testing.T) {
		// Only AWAITING_REVIEW is valid; RECEIVED and EVALUATING are rejected.
		for _, state := range []envelope.EnvelopeState{
			envelope.EnvelopeStateReceived,
			envelope.EnvelopeStateEvaluating,
			envelope.EnvelopeStateClosed,
		} {
			env := accMakeEnv(t)
			// Manually poke the state to avoid running through the full state machine.
			// (We just need a non-nil envelope with the wrong state for the constructor test.)
			switch state {
			case envelope.EnvelopeStateEvaluating:
				_ = env.Transition(envelope.EnvelopeStateEvaluating, accTestNow)
			case envelope.EnvelopeStateClosed:
				env.Evaluation.Explanation = &envelope.DecisionExplanation{Result: "accept"}
				env.Evaluation.Outcome = eval.OutcomeAccept
				env.Evaluation.ReasonCode = eval.ReasonWithinAuthority
				_ = env.Transition(envelope.EnvelopeStateEvaluating, accTestNow)
				_ = env.Transition(envelope.EnvelopeStateOutcomeRecorded, accTestNow)
				_ = env.Transition(envelope.EnvelopeStateClosed, accTestNow)
			}
			_, err := newExistingEnvelopeAccumulator(env)
			if !errors.Is(err, errAccumulatorWrongState) {
				t.Errorf("state %s: expected errAccumulatorWrongState, got: %v", state, err)
			}
			if err != nil && !strings.Contains(err.Error(), "AWAITING_REVIEW") {
				t.Errorf("state %s: error should mention expected state AWAITING_REVIEW: %s", state, err)
			}
		}
	})

	t.Run("AWAITING_REVIEW envelope succeeds", func(t *testing.T) {
		env := accMakeAwaitingReviewEnv(t)
		acc, err := newExistingEnvelopeAccumulator(env)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if acc == nil {
			t.Fatal("returned nil accumulator")
		}
		if acc.persisted {
			t.Error("new accumulator must not be persisted")
		}
		if len(acc.pendingEvents) != 0 {
			t.Errorf("new accumulator has %d pending events, want 0", len(acc.pendingEvents))
		}
	})
}

// =============================================================================
// Test group 10: persistExisting() call order and behaviour
// =============================================================================

// accMakeAwaitingReviewEnv returns an envelope already in AWAITING_REVIEW state,
// simulating one that was previously persisted during an escalation evaluation.
func accMakeAwaitingReviewEnv(t *testing.T) *envelope.Envelope {
	t.Helper()
	env := accMakeEnv(t)
	if err := env.Transition(envelope.EnvelopeStateEvaluating, accTestNow); err != nil {
		t.Fatalf("Transition EVALUATING: %v", err)
	}
	env.Evaluation.Outcome = eval.OutcomeEscalate
	env.Evaluation.ReasonCode = eval.ReasonConfidenceBelowThreshold
	env.Evaluation.Explanation = &envelope.DecisionExplanation{Result: "escalate"}
	if err := env.Transition(envelope.EnvelopeStateEscalated, accTestNow); err != nil {
		t.Fatalf("Transition ESCALATED: %v", err)
	}
	if err := env.Transition(envelope.EnvelopeStateAwaitingReview, accTestNow); err != nil {
		t.Fatalf("Transition AWAITING_REVIEW: %v", err)
	}
	return env
}

func TestAccumulatorPersistUpdate_CallOrder(t *testing.T) {
	// persistExisting must issue N×Append then 1 Update — no Create.
	log := &accCallLog{}
	envRepo := newAccFakeEnvRepo(log)
	auditRepo := newAccFakeAuditRepo(log)
	repos := makeAccTestRepos(envRepo, auditRepo)

	env := accMakeAwaitingReviewEnv(t)
	acc, err := newExistingEnvelopeAccumulator(env)
	if err != nil {
		t.Fatalf("newExistingEnvelopeAccumulator: %v", err)
	}

	mustRecord(t, acc.recordObservation(env.RequestSource(), env.RequestID(),
		audit.AuditEventEscalationReviewed, map[string]any{"decision": "APPROVED"}))

	env.Review = &envelope.EscalationReview{
		Decision:   envelope.ReviewDecisionApproved,
		ReviewerID: "reviewer-jane",
		ReviewedAt: accTestNow,
	}
	if err := acc.transition(envelope.EnvelopeStateClosed, accTestNow); err != nil {
		t.Fatalf("transition CLOSED: %v", err)
	}
	mustRecord(t, acc.recordLifecycle(env.RequestSource(), env.RequestID(),
		audit.AuditEventEnvelopeClosed, map[string]any{"from_state": "AWAITING_REVIEW", "to_state": "CLOSED"}))

	if err := acc.persistExisting(context.Background(), repos); err != nil {
		t.Fatalf("persistExisting: %v", err)
	}

	// Verify exact call sequence: N×Append then 1 Update — no Create.
	want := []string{
		"append:" + string(audit.AuditEventEscalationReviewed),
		"append:" + string(audit.AuditEventEnvelopeClosed),
		"update",
	}
	if len(log.ops) != len(want) {
		t.Fatalf("call log: got %v, want %v", log.ops, want)
	}
	for i, op := range want {
		if log.ops[i] != op {
			t.Errorf("call[%d]: got %q, want %q", i, log.ops[i], op)
		}
	}

	// persisted flag must be set.
	if !acc.persisted {
		t.Error("persisted flag not set after successful persistExisting")
	}

	// Integrity anchors populated.
	if env.Integrity.FirstEventHash == "" {
		t.Error("Integrity.FirstEventHash empty after persistExisting")
	}
	if env.Integrity.FinalEventHash == "" {
		t.Error("Integrity.FinalEventHash empty after persistExisting")
	}
	if len(env.Integrity.AuditEventIDs) != 2 {
		t.Errorf("Integrity.AuditEventIDs len: got %d, want 2", len(env.Integrity.AuditEventIDs))
	}
}

func TestAccumulatorPersistUpdate_CreateNotCalled(t *testing.T) {
	// persistExisting must never call Envelopes.Create.
	log := &accCallLog{}
	envRepo := newAccFakeEnvRepo(log)
	auditRepo := newAccFakeAuditRepo(log)
	repos := makeAccTestRepos(envRepo, auditRepo)

	env := accMakeAwaitingReviewEnv(t)
	acc, err := newExistingEnvelopeAccumulator(env)
	if err != nil {
		t.Fatalf("newExistingEnvelopeAccumulator: %v", err)
	}
	mustRecord(t, acc.recordObservation(env.RequestSource(), env.RequestID(),
		audit.AuditEventEscalationReviewed, nil))

	env.Review = &envelope.EscalationReview{
		Decision:   envelope.ReviewDecisionApproved,
		ReviewerID: "r1",
		ReviewedAt: accTestNow,
	}
	if err := acc.transition(envelope.EnvelopeStateClosed, accTestNow); err != nil {
		t.Fatalf("transition CLOSED: %v", err)
	}
	mustRecord(t, acc.recordLifecycle(env.RequestSource(), env.RequestID(),
		audit.AuditEventEnvelopeClosed, nil))

	if err := acc.persistExisting(context.Background(), repos); err != nil {
		t.Fatalf("persistExisting: %v", err)
	}

	for _, op := range log.ops {
		if op == "create" {
			t.Error("Envelopes.Create called by persistExisting — must never happen")
		}
	}
}

func TestAccumulatorPersistUpdate_AppendFailure(t *testing.T) {
	// When Append fails, Update must not be called.
	log := &accCallLog{}
	envRepo := newAccFakeEnvRepo(log)
	auditRepo := newAccFakeAuditRepo(log)

	sentinelErr := errors.New("audit: disk full")
	auditRepo.appendErr = sentinelErr
	auditRepo.failAfter = 0 // fail on first Append

	repos := makeAccTestRepos(envRepo, auditRepo)

	env := accMakeAwaitingReviewEnv(t)
	acc, err := newExistingEnvelopeAccumulator(env)
	if err != nil {
		t.Fatalf("newExistingEnvelopeAccumulator: %v", err)
	}
	mustRecord(t, acc.recordObservation(env.RequestSource(), env.RequestID(),
		audit.AuditEventEscalationReviewed, nil))

	err = acc.persistExisting(context.Background(), repos)
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected sentinel error, got: %v", err)
	}

	// Update must not have been called.
	for _, op := range log.ops {
		if op == "update" {
			t.Error("Update called after Append failure")
		}
	}
	if acc.persisted {
		t.Error("persisted flag set despite Append failure")
	}
}

func TestAccumulatorPersistUpdate_UpdateFailure(t *testing.T) {
	// All appends run then Update fails.
	log := &accCallLog{}
	envRepo := newAccFakeEnvRepo(log)
	auditRepo := newAccFakeAuditRepo(log)

	sentinelErr := errors.New("db: connection refused")
	envRepo.updateErr = sentinelErr

	repos := makeAccTestRepos(envRepo, auditRepo)

	env := accMakeAwaitingReviewEnv(t)
	acc, err := newExistingEnvelopeAccumulator(env)
	if err != nil {
		t.Fatalf("newExistingEnvelopeAccumulator: %v", err)
	}
	mustRecord(t, acc.recordObservation(env.RequestSource(), env.RequestID(),
		audit.AuditEventEscalationReviewed, nil))

	env.Review = &envelope.EscalationReview{
		Decision:   envelope.ReviewDecisionApproved,
		ReviewerID: "r1",
		ReviewedAt: accTestNow,
	}
	if err := acc.transition(envelope.EnvelopeStateClosed, accTestNow); err != nil {
		t.Fatalf("transition CLOSED: %v", err)
	}
	mustRecord(t, acc.recordLifecycle(env.RequestSource(), env.RequestID(),
		audit.AuditEventEnvelopeClosed, nil))

	err = acc.persistExisting(context.Background(), repos)
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected sentinel error, got: %v", err)
	}

	// Both appends must have run before the Update was attempted.
	appendCount := 0
	for _, op := range log.ops {
		if strings.HasPrefix(op, "append:") {
			appendCount++
		}
	}
	if appendCount != 2 {
		t.Errorf("expected 2 Appends before Update failure, got %d", appendCount)
	}
	if acc.persisted {
		t.Error("persisted flag set despite Update failure")
	}
}

func TestAccumulatorPersistUpdate_AlreadyPersisted(t *testing.T) {
	// Double persistExisting returns errAccumulatorAlreadyPersisted.
	log := &accCallLog{}
	repos := makeAccTestRepos(newAccFakeEnvRepo(log), newAccFakeAuditRepo(log))

	env := accMakeAwaitingReviewEnv(t)
	acc, err := newExistingEnvelopeAccumulator(env)
	if err != nil {
		t.Fatalf("newExistingEnvelopeAccumulator: %v", err)
	}
	mustRecord(t, acc.recordObservation(env.RequestSource(), env.RequestID(),
		audit.AuditEventEscalationReviewed, nil))

	env.Review = &envelope.EscalationReview{
		Decision:   envelope.ReviewDecisionApproved,
		ReviewerID: "r1",
		ReviewedAt: accTestNow,
	}
	if err := acc.transition(envelope.EnvelopeStateClosed, accTestNow); err != nil {
		t.Fatalf("transition CLOSED: %v", err)
	}
	mustRecord(t, acc.recordLifecycle(env.RequestSource(), env.RequestID(),
		audit.AuditEventEnvelopeClosed, nil))

	if err := acc.persistExisting(context.Background(), repos); err != nil {
		t.Fatalf("first persistExisting: %v", err)
	}

	err = acc.persistExisting(context.Background(), repos)
	if !errors.Is(err, errAccumulatorAlreadyPersisted) {
		t.Errorf("expected errAccumulatorAlreadyPersisted on second persistExisting, got: %v", err)
	}
}

func TestAccumulatorPersistUpdate_EmptyQueue(t *testing.T) {
	// Empty pending event queue returns an error without any DB writes.
	log := &accCallLog{}
	repos := makeAccTestRepos(newAccFakeEnvRepo(log), newAccFakeAuditRepo(log))

	env := accMakeAwaitingReviewEnv(t)
	acc, err := newExistingEnvelopeAccumulator(env)
	if err != nil {
		t.Fatalf("newExistingEnvelopeAccumulator: %v", err)
	}
	// Do not queue any events.

	err = acc.persistExisting(context.Background(), repos)
	if err == nil {
		t.Error("expected error for empty event queue, got nil")
	}
	if len(log.ops) != 0 {
		t.Errorf("DB operations attempted with empty queue: %v", log.ops)
	}
	if acc.persisted {
		t.Error("persisted flag set despite empty queue error")
	}
}
