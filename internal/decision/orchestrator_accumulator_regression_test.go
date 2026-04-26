package decision_test

// Accumulator Regression Tests
//
// Locks in orchestrator persistence and audit behavior:
// all evaluation writes are deferred to a single acc.persist() call at the end
// of each evaluation path (Envelopes.Create → N×Audit.Append → Envelopes.Update).
//
// Run: go test ./internal/decision/... -run TestAccumulator
//      go test ./internal/decision/... -run TestAuditEventOrdering
//      go test ./internal/decision/... -run TestIntegrityAnchors
//      go test ./internal/decision/... -run TestTransitionInvariants

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/surface"
	"github.com/accept-io/midas/internal/value"
)

// =============================================================================
// Spy infrastructure
//
// countingEnvelopeRepo and spyStore track the exact number of Envelopes.Create
// and Envelopes.Update calls across an evaluation to enforce the single-flush
// guarantee: Create=1, Update=1 per evaluation path.
// =============================================================================

// countingEnvelopeRepo wraps fakeEnvelopeRepo and increments Creates/Updates
// on every call. Snapshot and restore delegate to the inner repo so that
// spyStore.WithTx can roll back cleanly.
type countingEnvelopeRepo struct {
	inner   *fakeEnvelopeRepo
	creates int
	updates int
}

func (c *countingEnvelopeRepo) Create(ctx context.Context, env *envelope.Envelope) error {
	c.creates++
	return c.inner.Create(ctx, env)
}

func (c *countingEnvelopeRepo) Update(ctx context.Context, env *envelope.Envelope) error {
	c.updates++
	return c.inner.Update(ctx, env)
}

func (c *countingEnvelopeRepo) GetByID(ctx context.Context, id string) (*envelope.Envelope, error) {
	return c.inner.GetByID(ctx, id)
}

func (c *countingEnvelopeRepo) GetByRequestID(ctx context.Context, requestID string) (*envelope.Envelope, error) {
	return c.inner.GetByRequestID(ctx, requestID)
}

func (c *countingEnvelopeRepo) GetByRequestScope(ctx context.Context, requestSource, requestID string) (*envelope.Envelope, error) {
	return c.inner.GetByRequestScope(ctx, requestSource, requestID)
}

func (c *countingEnvelopeRepo) List(ctx context.Context) ([]*envelope.Envelope, error) {
	return c.inner.List(ctx)
}

func (c *countingEnvelopeRepo) ListByState(ctx context.Context, state envelope.EnvelopeState) ([]*envelope.Envelope, error) {
	return c.inner.ListByState(ctx, state)
}

// spyStore is a RepositoryStore that exposes a countingEnvelopeRepo.
// WithTx provides snapshot-based rollback semantics matching fakeStore.
type spyStore struct {
	envelopes        *countingEnvelopeRepo
	audit            *fakeAuditRepo
	surfaces         *fakeSurfaceRepo
	agents           *fakeAgentRepo
	grants           *fakeGrantRepo
	profiles         *fakeProfileRepo
	processes        *fakeProcessRepo
	businessServices *fakeBusinessServiceRepo
	bscLinks         *fakeBSCRepo
	capabilities     *fakeCapabilityRepo
}

func newSpyStore() *spyStore {
	return &spyStore{
		envelopes: &countingEnvelopeRepo{
			inner: &fakeEnvelopeRepo{data: map[string]*envelope.Envelope{}},
		},
		audit:            &fakeAuditRepo{},
		surfaces:         &fakeSurfaceRepo{},
		agents:           &fakeAgentRepo{},
		grants:           &fakeGrantRepo{},
		profiles:         &fakeProfileRepo{},
		processes:        &fakeProcessRepo{},
		businessServices: &fakeBusinessServiceRepo{},
		bscLinks:         &fakeBSCRepo{},
		capabilities:     &fakeCapabilityRepo{},
	}
}

func (s *spyStore) Repositories() (*store.Repositories, error) {
	return s.repos(), nil
}

func (s *spyStore) repos() *store.Repositories {
	return &store.Repositories{
		Envelopes:                   s.envelopes,
		Audit:                       s.audit,
		Surfaces:                    s.surfaces,
		Agents:                      s.agents,
		Grants:                      s.grants,
		Profiles:                    s.profiles,
		Processes:                   s.processes,
		BusinessServices:            s.businessServices,
		BusinessServiceCapabilities: s.bscLinks,
		Capabilities:                s.capabilities,
	}
}

func (s *spyStore) WithTx(_ context.Context, _ string, fn func(*store.Repositories) error) error {
	envSnap := s.envelopes.inner.snapshot()
	auditEvts, auditCount := s.audit.snapshot()

	err := fn(s.repos())
	if err != nil {
		s.envelopes.inner.restore(envSnap)
		s.audit.restore(auditEvts, auditCount)
	}
	return err
}

// testProcessID and testBusinessServiceID anchor the v1 service-led
// structural chain that orchestrator evaluations now resolve as part of
// envelope structural denormalisation (ADR-0001).
const (
	testProcessID         = "proc-test-spy"
	testBusinessServiceID = "bs-test-spy"
)

// seedSpyStore populates a spyStore with a valid authority chain plus the
// minimum service-led structural chain (Process + BusinessService, empty
// capability set) so that the full resolution path completes. The empty
// capability set is a valid v1 state per ADR-0001 PR-3 and keeps the seed
// minimal for tests that don't otherwise care about capabilities.
func seedSpyStore(s *spyStore) {
	s.surfaces.surfaces = map[string]*surface.DecisionSurface{
		testSurfaceID: {
			ID:        testSurfaceID,
			Version:   1,
			Status:    surface.SurfaceStatusActive,
			ProcessID: testProcessID,
		},
	}
	seedStructuralChain(s.processes, s.businessServices, s.bscLinks, s.capabilities,
		testProcessID, testBusinessServiceID, nil)
	s.agents.agents = map[string]*agent.Agent{
		testAgentID: {ID: testAgentID},
	}
	s.grants.grants = map[string][]*authority.AuthorityGrant{
		testAgentID: {
			{
				ID:        testGrantID,
				AgentID:   testAgentID,
				ProfileID: testProfileID,
				Status:    authority.GrantStatusActive,
			},
		},
	}
	s.profiles.profiles = map[string]*authority.AuthorityProfile{
		testProfileID: {
			ID:                  testProfileID,
			Version:             1,
			Status:              authority.ProfileStatusActive,
			SurfaceID:           testSurfaceID,
			ConfidenceThreshold: 0.80,
			ConsequenceThreshold: authority.Consequence{
				Type:       value.ConsequenceTypeRiskRating,
				RiskRating: value.RiskRatingHigh,
			},
		},
	}
}

func buildSpyOrchestrator(t *testing.T, s *spyStore, pol policy.PolicyEvaluator) *decision.Orchestrator {
	t.Helper()
	o, err := decision.NewOrchestratorWithClock(
		s,
		pol,
		decision.NoOpEvaluationRecorder{},
		func() time.Time { return testNow },
	)
	if err != nil {
		t.Fatalf("NewOrchestratorWithClock: %v", err)
	}
	return o
}

func marshalReq(t *testing.T, req eval.DecisionRequest) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return b
}

// =============================================================================
// Test 1: Persistence write counts — happy path
//
// Verifies the exact number of Envelopes.Create, Envelopes.Update, and
// Audit.Append calls during a successful Accept evaluation.
//
// Create=1, Update=1, Audit.Append=10
//
// All writes are deferred to a single acc.persistNew() call at the end of the
// evaluation path: Envelopes.Create → N×Audit.Append → Envelopes.Update.
// Audit.Append count is 10 because the sequential hash chain requires each
// event to be appended individually in order.
// =============================================================================

func TestAccumulatorRegression_PersistenceCount_HappyPath(t *testing.T) {
	s := newSpyStore()
	seedSpyStore(s)

	req := lifecycleBaseRequest()
	result, err := buildSpyOrchestrator(t, s, &allowAllPolicies{}).Evaluate(context.Background(), req, marshalReq(t, req))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Outcome != eval.OutcomeAccept {
		t.Fatalf("unexpected outcome %q; test requires Accept path", result.Outcome)
	}
	if result.State != envelope.EnvelopeStateClosed {
		t.Fatalf("unexpected state %q; Accept path must reach CLOSED", result.State)
	}

	// --- Envelope write counts ---

	// 1 Create: FK constraint requires the envelope row before any Audit.Append.
	if s.envelopes.creates != 1 {
		t.Errorf("Envelopes.Create: got %d calls, want 1", s.envelopes.creates)
	}

	// 1 Update: single final flush after all events are appended.
	const wantUpdates = 1
	if s.envelopes.updates != wantUpdates {
		t.Errorf("Envelopes.Update: got %d calls, want %d", s.envelopes.updates, wantUpdates)
	}

	// --- Audit write counts ---

	events, err := s.audit.ListByEnvelopeID(context.Background(), result.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}

	// 10 Audit.Append calls for Accept with no policy:
	//   envelope_created, evaluation_started, surface_resolved, agent_resolved,
	//   authority_chain_resolved, context_validated, confidence_checked,
	//   consequence_checked, outcome_recorded, envelope_closed.
	// Sequential hash chain (PrevHash dependency) requires each event in order.
	const wantAuditEvents = 10
	if len(events) != wantAuditEvents {
		t.Errorf("Audit.Append: got %d events, want %d", len(events), wantAuditEvents)
	}
}

// =============================================================================
// Test 2: Audit event ordering — Accept, Reject, and Escalate paths
//
// Verifies the exact event type sequence for each outcome path.
// =============================================================================

func TestAuditEventOrdering_AllOutcomePaths(t *testing.T) {
	t.Run("Accept", func(t *testing.T) {
		// Full happy path: surface → agent → authority → context → confidence →
		// consequence → policy(noop) → Accept → OUTCOME_RECORDED → CLOSED.
		// Expected: 10 events, no policy_evaluated event (NoOp skips emission).
		s := newSpyStore()
		seedSpyStore(s)

		req := lifecycleBaseRequest()
		result, err := buildSpyOrchestrator(t, s, &allowAllPolicies{}).Evaluate(
			context.Background(), req, marshalReq(t, req),
		)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}

		events, _ := s.audit.ListByEnvelopeID(context.Background(), result.EnvelopeID)

		wantSeq := []audit.AuditEventType{
			audit.AuditEventEnvelopeCreated,
			audit.AuditEventEvaluationStarted,
			audit.AuditEventSurfaceResolved,
			audit.AuditEventAgentResolved,
			audit.AuditEventAuthorityChainResolved,
			audit.AuditEventContextValidated,
			audit.AuditEventConfidenceChecked,
			audit.AuditEventConsequenceChecked,
			audit.AuditEventOutcomeRecorded,
			audit.AuditEventEnvelopeClosed,
		}
		assertEventSequence(t, events, wantSeq)
	})

	t.Run("EarlyReject_SurfaceNotFound", func(t *testing.T) {
		// Surface resolution fails immediately after the first two lifecycle events.
		// Observational events (surface_resolved etc.) are never emitted.
		// Expected: 4 events — created, started, outcome_recorded, closed.
		s := newSpyStore()
		seedSpyStore(s)

		req := lifecycleBaseRequest()
		req.SurfaceID = "surface-does-not-exist"
		result, err := buildSpyOrchestrator(t, s, &allowAllPolicies{}).Evaluate(
			context.Background(), req, marshalReq(t, req),
		)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if result.Outcome != eval.OutcomeReject || result.ReasonCode != eval.ReasonSurfaceNotFound {
			t.Fatalf("expected Reject/SURFACE_NOT_FOUND, got %s/%s", result.Outcome, result.ReasonCode)
		}

		events, _ := s.audit.ListByEnvelopeID(context.Background(), result.EnvelopeID)

		wantSeq := []audit.AuditEventType{
			audit.AuditEventEnvelopeCreated,
			audit.AuditEventEvaluationStarted,
			audit.AuditEventOutcomeRecorded,
			audit.AuditEventEnvelopeClosed,
		}
		assertEventSequence(t, events, wantSeq)
	})

	t.Run("Escalate_ConfidenceBelowThreshold", func(t *testing.T) {
		// Confidence check emits its event before the check, then fails.
		// Consequence is never checked. Outcome is ESCALATED → AWAITING_REVIEW.
		// Expected: 9 events — no consequence_checked, no envelope_closed.
		s := newSpyStore()
		seedSpyStore(s)

		req := lifecycleBaseRequest()
		req.Confidence = 0.10 // below 0.80 threshold
		result, err := buildSpyOrchestrator(t, s, &allowAllPolicies{}).Evaluate(
			context.Background(), req, marshalReq(t, req),
		)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if result.Outcome != eval.OutcomeEscalate || result.ReasonCode != eval.ReasonConfidenceBelowThreshold {
			t.Fatalf("expected Escalate/CONFIDENCE_BELOW_THRESHOLD, got %s/%s", result.Outcome, result.ReasonCode)
		}
		if result.State != envelope.EnvelopeStateAwaitingReview {
			t.Fatalf("expected AWAITING_REVIEW state, got %s", result.State)
		}

		events, _ := s.audit.ListByEnvelopeID(context.Background(), result.EnvelopeID)

		wantSeq := []audit.AuditEventType{
			audit.AuditEventEnvelopeCreated,
			audit.AuditEventEvaluationStarted,
			audit.AuditEventSurfaceResolved,
			audit.AuditEventAgentResolved,
			audit.AuditEventAuthorityChainResolved,
			audit.AuditEventContextValidated,
			audit.AuditEventConfidenceChecked, // emitted before the check; check then fails
			audit.AuditEventOutcomeRecorded,   // EVALUATING → ESCALATED
			audit.AuditEventEscalationPending, // ESCALATED → AWAITING_REVIEW
		}
		assertEventSequence(t, events, wantSeq)

		// Envelope must be open (AWAITING_REVIEW), not closed.
		assertAuditAbsent(t, events, audit.AuditEventEnvelopeClosed)
		// consequence_checked is never emitted because the confidence check short-circuits.
		assertAuditAbsent(t, events, audit.AuditEventConsequenceChecked)
	})

	t.Run("Accept_WithPolicy", func(t *testing.T) {
		// Same as Accept, but with a PolicyReference on the profile and an
		// allow-all policy evaluator. The policy step is now active, so
		// policy_evaluated is emitted between consequence_checked and outcome_recorded.
		// Expected: 11 events.
		s := newSpyStore()
		seedSpyStore(s)
		s.profiles.profiles[testProfileID].PolicyReference = "payments/allow-all"

		req := lifecycleBaseRequest()
		result, err := buildSpyOrchestrator(t, s, &allowAllPolicies{}).Evaluate(
			context.Background(), req, marshalReq(t, req),
		)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if result.Outcome != eval.OutcomeAccept {
			t.Fatalf("unexpected outcome %q; test requires Accept path", result.Outcome)
		}

		events, _ := s.audit.ListByEnvelopeID(context.Background(), result.EnvelopeID)

		wantSeq := []audit.AuditEventType{
			audit.AuditEventEnvelopeCreated,
			audit.AuditEventEvaluationStarted,
			audit.AuditEventSurfaceResolved,
			audit.AuditEventAgentResolved,
			audit.AuditEventAuthorityChainResolved,
			audit.AuditEventContextValidated,
			audit.AuditEventConfidenceChecked,
			audit.AuditEventConsequenceChecked,
			audit.AuditEventPolicyEvaluated,
			audit.AuditEventOutcomeRecorded,
			audit.AuditEventEnvelopeClosed,
		}
		assertEventSequence(t, events, wantSeq)
	})
}

// assertEventSequence verifies that events match wantSeq exactly — same length,
// same types, same order. Use assertAuditContains for subset checks.
func assertEventSequence(t *testing.T, events []*audit.AuditEvent, wantSeq []audit.AuditEventType) {
	t.Helper()

	if len(events) != len(wantSeq) {
		got := make([]audit.AuditEventType, len(events))
		for i, ev := range events {
			got[i] = ev.EventType
		}
		t.Fatalf("event count: got %d, want %d\n  got:  %v\n  want: %v",
			len(events), len(wantSeq), got, wantSeq)
	}

	for i, want := range wantSeq {
		if events[i].EventType != want {
			t.Errorf("event[%d]: got %q, want %q", i, events[i].EventType, want)
		}
	}
}

// =============================================================================
// Test 3: Integrity anchor correctness
//
// Verifies that the envelope's Integrity section correctly references the
// audit hash chain after a complete evaluation. All integrity anchors
// (FirstEventHash, FinalEventHash, AuditEventIDs) are written in the single
// final Envelopes.Update issued by acc.persistNew().
// =============================================================================

func TestIntegrityAnchors_HashChainCorrectness(t *testing.T) {
	s := newSpyStore()
	seedSpyStore(s)

	req := lifecycleBaseRequest()
	result, err := buildSpyOrchestrator(t, s, &allowAllPolicies{}).Evaluate(
		context.Background(), req, marshalReq(t, req),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	env := s.envelopes.inner.data[result.EnvelopeID]
	if env == nil {
		t.Fatal("envelope not found in store after Evaluate")
	}

	events, err := s.audit.ListByEnvelopeID(context.Background(), result.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("no audit events found for envelope")
	}

	// 1. FirstEventHash must equal the hash of the first audit event.
	//    Set by absorbPersistedEvent after the first Append; persisted in the final Update.
	if env.Integrity.FirstEventHash == "" {
		t.Error("Integrity.FirstEventHash is empty")
	}
	if events[0].EventType != audit.AuditEventEnvelopeCreated {
		t.Errorf("first event type: got %q, want envelope_created", events[0].EventType)
	}
	if env.Integrity.FirstEventHash != events[0].Hash {
		t.Errorf("Integrity.FirstEventHash mismatch:\n  got:  %q\n  want: %q (hash of events[0])",
			env.Integrity.FirstEventHash, events[0].Hash)
	}

	// 2. FinalEventHash must equal the hash of the last audit event.
	//    Updated by absorbPersistedEvent on each Append; final value reflects
	//    the terminal event (envelope_closed for Accept path).
	last := events[len(events)-1]
	if env.Integrity.FinalEventHash == "" {
		t.Error("Integrity.FinalEventHash is empty")
	}
	if env.Integrity.FinalEventHash != last.Hash {
		t.Errorf("Integrity.FinalEventHash mismatch:\n  got:  %q\n  want: %q (hash of last event %q)",
			env.Integrity.FinalEventHash, last.Hash, last.EventType)
	}

	// 3. Verify the hash chain is unbroken across all events.
	//    Event[n].PrevHash must equal Event[n-1].Hash.
	//    SequenceNo must be monotonically increasing from 1.
	for i, ev := range events {
		wantSeq := i + 1
		if ev.SequenceNo != wantSeq {
			t.Errorf("events[%d].SequenceNo: got %d, want %d", i, ev.SequenceNo, wantSeq)
		}

		if i == 0 {
			if ev.PrevHash != "" {
				t.Errorf("events[0].PrevHash must be empty (chain anchor), got %q", ev.PrevHash)
			}
		} else {
			prevHash := events[i-1].Hash
			if ev.PrevHash != prevHash {
				t.Errorf("events[%d].PrevHash chain broken:\n  got:  %q\n  want: %q (events[%d].Hash)",
					i, ev.PrevHash, prevHash, i-1)
			}
		}

		if ev.Hash == "" {
			t.Errorf("events[%d] (%s) has empty Hash", i, ev.EventType)
		}
	}

	// 4. AuditEventIDs tracks every event the envelope absorbed via absorbPersistedEvent.
	//    The accumulator calls absorbPersistedEvent for every queued event — lifecycle
	//    and observational — so the index is complete and the count matches the total
	//    event count.
	if len(env.Integrity.AuditEventIDs) == 0 {
		t.Error("Integrity.AuditEventIDs is empty")
	}

	// All 10 events for the Accept path must be tracked.
	const wantTrackedIDs = 10
	if len(env.Integrity.AuditEventIDs) != wantTrackedIDs {
		t.Errorf("Integrity.AuditEventIDs length: got %d, want %d",
			len(env.Integrity.AuditEventIDs), wantTrackedIDs)
	}

	// All tracked IDs must correspond to real audit events.
	auditIDSet := make(map[string]bool, len(events))
	for _, ev := range events {
		auditIDSet[ev.ID] = true
	}
	for _, id := range env.Integrity.AuditEventIDs {
		if !auditIDSet[id] {
			t.Errorf("Integrity.AuditEventIDs contains %q which has no matching audit event", id)
		}
	}
}

// =============================================================================
// Test 4: Transaction rollback on Audit.Append failure
//
// Verifies the atomic-or-nothing guarantee: if any Audit.Append fails, the
// entire transaction rolls back — no envelope row, no partial audit events.
//
// This test injects failure on the 3rd Append (surface_resolved). At that
// point the envelope store has received Create + 2 successful Appends; all
// writes must roll back. This complements TestLifecycle_AuditFailureRollback
// (which fails on the 2nd append) by verifying atomicity holds when more
// writes have accumulated before the failure.
// =============================================================================

func TestAccumulatorRegression_Rollback_AuditAppendFailure(t *testing.T) {
	ctx := context.Background()
	st := newFakeStore()
	seedStore(st)

	sentinelErr := errors.New("audit backend unavailable: disk full")

	// Allow the first 2 appends to succeed:
	//   append #1: envelope_created    (succeeds)
	//   append #2: evaluation_started  (succeeds)
	//   append #3: surface_resolved    (fails — this is the target failure point)
	//
	// At failure time, the envelope store has received Create #1 + 2 Appends.
	// All writes must be rolled back.
	st.audit.failErr = sentinelErr
	st.audit.failAfter = 2

	o := buildOrchestrator(t, st, &allowAllPolicies{})
	req := lifecycleBaseRequest()

	_, err := o.Evaluate(ctx, req, rawRequest(t, req))
	if err == nil {
		t.Fatal("expected error from Evaluate, got nil")
	}
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected sentinel error in chain, got: %v", err)
	}

	// Envelope store must be completely empty — Create rolled back.
	if len(st.envelopes.data) != 0 {
		t.Errorf("rollback incomplete: %d envelope(s) remain in store (want 0)", len(st.envelopes.data))
	}

	// Audit log must be completely empty — all 2 successful appends rolled back.
	if len(st.audit.events) != 0 {
		t.Errorf("rollback incomplete: %d audit event(s) remain in log (want 0)", len(st.audit.events))
	}

	// After rollback, a fresh evaluation on the same request must succeed.
	// This verifies that the rollback leaves no orphaned state that would
	// block a retry.
	st.audit.failErr = nil
	st.audit.failAfter = 0

	retryResult, err := o.Evaluate(ctx, req, rawRequest(t, req))
	if err != nil {
		t.Fatalf("retry after rollback failed: %v", err)
	}
	if retryResult.Outcome != eval.OutcomeAccept {
		t.Errorf("retry outcome: got %q, want Accept", retryResult.Outcome)
	}
	if retryResult.EnvelopeID == "" {
		t.Error("retry produced empty EnvelopeID")
	}
}

// =============================================================================
// Test 5: Envelope state machine transition invariants
//
// Validates that the content invariants enforced by envelope.Transition()
// are preserved. These invariants live in envelope.go, but the orchestrator
// calls Transition() via acc.transition() and must satisfy all preconditions
// before each call. Tests here confirm those preconditions are always met.
// =============================================================================

func TestTransitionInvariants_ValidationPreserved(t *testing.T) {
	makeEnv := func(t *testing.T) *envelope.Envelope {
		t.Helper()
		env, err := envelope.New("env-test", "test-source", "req-test",
			json.RawMessage(`{"test":true}`), testNow)
		if err != nil {
			t.Fatalf("envelope.New: %v", err)
		}
		return env
	}

	t.Run("OutcomeRecorded_RequiresExplanation", func(t *testing.T) {
		// EVALUATING → OUTCOME_RECORDED must be blocked when Explanation is nil.
		// The orchestrator seeds Explanation before the first outcome transition;
		// the accumulator must maintain the same ordering.
		env := makeEnv(t)

		// Advance to EVALUATING (no content invariant for this transition).
		if err := env.Transition(envelope.EnvelopeStateEvaluating, testNow); err != nil {
			t.Fatalf("Transition to EVALUATING: %v", err)
		}

		// Attempt OUTCOME_RECORDED without setting Explanation.
		// This must fail with ErrMissingExplanation.
		err := env.Transition(envelope.EnvelopeStateOutcomeRecorded, testNow)
		if !errors.Is(err, envelope.ErrMissingExplanation) {
			t.Errorf("Transition to OUTCOME_RECORDED without Explanation: got %v, want ErrMissingExplanation", err)
		}

		// Now set Explanation and retry — must succeed.
		env.Evaluation.Explanation = &envelope.DecisionExplanation{Result: "accept"}
		if err := env.Transition(envelope.EnvelopeStateOutcomeRecorded, testNow); err != nil {
			t.Errorf("Transition to OUTCOME_RECORDED with Explanation: unexpected error %v", err)
		}
	})

	t.Run("Escalated_RequiresExplanation", func(t *testing.T) {
		// EVALUATING → ESCALATED has the same Explanation requirement.
		env := makeEnv(t)

		if err := env.Transition(envelope.EnvelopeStateEvaluating, testNow); err != nil {
			t.Fatalf("Transition to EVALUATING: %v", err)
		}

		err := env.Transition(envelope.EnvelopeStateEscalated, testNow)
		if !errors.Is(err, envelope.ErrMissingExplanation) {
			t.Errorf("Transition to ESCALATED without Explanation: got %v, want ErrMissingExplanation", err)
		}

		env.Evaluation.Explanation = &envelope.DecisionExplanation{Result: "escalate"}
		if err := env.Transition(envelope.EnvelopeStateEscalated, testNow); err != nil {
			t.Errorf("Transition to ESCALATED with Explanation: unexpected error %v", err)
		}
	})

	t.Run("Closed_RequiresOutcomeAndReasonCode", func(t *testing.T) {
		// OUTCOME_RECORDED → CLOSED requires both Outcome and ReasonCode.
		// The orchestrator sets these in finish() before calling acc.transition(CLOSED);
		// the accumulator enforces this ordering via envelope.Transition's content invariants.
		env := makeEnv(t)

		if err := env.Transition(envelope.EnvelopeStateEvaluating, testNow); err != nil {
			t.Fatalf("Transition to EVALUATING: %v", err)
		}
		env.Evaluation.Explanation = &envelope.DecisionExplanation{Result: "accept"}
		if err := env.Transition(envelope.EnvelopeStateOutcomeRecorded, testNow); err != nil {
			t.Fatalf("Transition to OUTCOME_RECORDED: %v", err)
		}

		// Attempt CLOSED without Outcome or ReasonCode.
		err := env.Transition(envelope.EnvelopeStateClosed, testNow)
		if !errors.Is(err, envelope.ErrMissingOutcome) {
			t.Errorf("Transition to CLOSED without Outcome: got %v, want ErrMissingOutcome", err)
		}

		// Set only Outcome — still blocked.
		// ErrMissingOutcome covers both "no Outcome" and "no ReasonCode":
		// the invariant requires both fields populated before CLOSED is reachable.
		env.Evaluation.Outcome = eval.OutcomeAccept
		err = env.Transition(envelope.EnvelopeStateClosed, testNow)
		if !errors.Is(err, envelope.ErrMissingOutcome) {
			t.Errorf("Transition to CLOSED with Outcome but no ReasonCode: got %v, want ErrMissingOutcome", err)
		}

		// Set both — must succeed.
		env.Evaluation.ReasonCode = eval.ReasonWithinAuthority
		if err := env.Transition(envelope.EnvelopeStateClosed, testNow); err != nil {
			t.Errorf("Transition to CLOSED with Outcome+ReasonCode: unexpected error %v", err)
		}

		// Post-condition: ClosedAt is set by the state machine.
		if env.ClosedAt == nil {
			t.Error("ClosedAt is nil after transition to CLOSED")
		}
	})

	t.Run("ClosedFromAwaitingReview_RequiresReview", func(t *testing.T) {
		// AWAITING_REVIEW → CLOSED requires Review to be set.
		// ResolveEscalation sets env.Review before calling acc.transition(CLOSED);
		// the accumulator enforces this via envelope.Transition's content invariants.
		env := makeEnv(t)

		// Drive to AWAITING_REVIEW through the escalation path.
		if err := env.Transition(envelope.EnvelopeStateEvaluating, testNow); err != nil {
			t.Fatalf("Transition to EVALUATING: %v", err)
		}
		env.Evaluation.Outcome = eval.OutcomeEscalate
		env.Evaluation.ReasonCode = eval.ReasonConfidenceBelowThreshold
		env.Evaluation.Explanation = &envelope.DecisionExplanation{Result: "escalate"}
		if err := env.Transition(envelope.EnvelopeStateEscalated, testNow); err != nil {
			t.Fatalf("Transition to ESCALATED: %v", err)
		}
		if err := env.Transition(envelope.EnvelopeStateAwaitingReview, testNow); err != nil {
			t.Fatalf("Transition to AWAITING_REVIEW: %v", err)
		}

		// Attempt CLOSED without Review — must be blocked.
		err := env.Transition(envelope.EnvelopeStateClosed, testNow)
		if !errors.Is(err, envelope.ErrMissingReview) {
			t.Errorf("Transition to CLOSED from AWAITING_REVIEW without Review: got %v, want ErrMissingReview", err)
		}

		// Set Review — must now succeed.
		env.Review = &envelope.EscalationReview{
			Decision:   envelope.ReviewDecisionApproved,
			ReviewerID: "reviewer-jane",
			ReviewedAt: testNow,
		}
		if err := env.Transition(envelope.EnvelopeStateClosed, testNow); err != nil {
			t.Errorf("Transition to CLOSED from AWAITING_REVIEW with Review: unexpected error %v", err)
		}
	})

	t.Run("ClosedEnvelope_ImmutableToFurtherTransitions", func(t *testing.T) {
		// Once CLOSED, any further Transition call must return ErrEnvelopeClosed.
		// This protects the terminal state of the governance record.
		env := makeEnv(t)

		if err := env.Transition(envelope.EnvelopeStateEvaluating, testNow); err != nil {
			t.Fatalf("Transition to EVALUATING: %v", err)
		}
		env.Evaluation.Outcome = eval.OutcomeAccept
		env.Evaluation.ReasonCode = eval.ReasonWithinAuthority
		env.Evaluation.Explanation = &envelope.DecisionExplanation{Result: "accept"}
		if err := env.Transition(envelope.EnvelopeStateOutcomeRecorded, testNow); err != nil {
			t.Fatalf("Transition to OUTCOME_RECORDED: %v", err)
		}
		if err := env.Transition(envelope.EnvelopeStateClosed, testNow); err != nil {
			t.Fatalf("Transition to CLOSED: %v", err)
		}

		// Any further transition must be blocked.
		err := env.Transition(envelope.EnvelopeStateClosed, testNow)
		if !errors.Is(err, envelope.ErrEnvelopeClosed) {
			t.Errorf("re-transition on CLOSED envelope: got %v, want ErrEnvelopeClosed", err)
		}

		err = env.Transition(envelope.EnvelopeStateEvaluating, testNow)
		if !errors.Is(err, envelope.ErrEnvelopeClosed) {
			t.Errorf("invalid re-transition on CLOSED envelope: got %v, want ErrEnvelopeClosed", err)
		}
	})
}

// =============================================================================
// Test 6: ResolveEscalation persistence write counts
//
// Verifies that ResolveEscalation uses acc.persistExisting() — no Create, one
// Update, exactly two audit events — and that the event order is correct:
//   - Envelopes.Create = 0 (envelope already exists)
//   - Envelopes.Update = 1 (single final flush)
//   - Audit events     = 2 (escalation_reviewed, then envelope_closed)
// =============================================================================

func TestResolveEscalation_PersistenceCount(t *testing.T) {
	ctx := context.Background()
	s := newSpyStore()
	seedSpyStore(s)

	o := buildSpyOrchestrator(t, s, &allowAllPolicies{})

	// First, create an escalated envelope by evaluating below confidence threshold.
	req := eval.DecisionRequest{
		RequestID:     "req-resolve-count-001",
		RequestSource: "test-source",
		SurfaceID:     testSurfaceID,
		AgentID:       testAgentID,
		Confidence:    0.10, // below 0.80 threshold → escalate
	}
	raw, _ := json.Marshal(req)
	evalResult, err := o.Evaluate(ctx, req, raw)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if evalResult.State != envelope.EnvelopeStateAwaitingReview {
		t.Fatalf("expected AWAITING_REVIEW after escalation, got %q", evalResult.State)
	}

	// Capture write counts after the initial evaluation (baseline).
	createsAfterEval := s.envelopes.creates
	updatesAfterEval := s.envelopes.updates
	auditCountAfterEval := len(s.audit.events)

	// Now resolve the escalation.
	resolved, err := o.ResolveEscalation(ctx, decision.EscalationResolution{
		EnvelopeID:   evalResult.EnvelopeID,
		Decision:     envelope.ReviewDecisionApproved,
		ReviewerID:   "reviewer-jane",
		ReviewerKind: "human",
		Notes:        "approved after manual review",
	})
	if err != nil {
		t.Fatalf("ResolveEscalation: %v", err)
	}
	if resolved.State != envelope.EnvelopeStateClosed {
		t.Fatalf("expected CLOSED after resolution, got %q", resolved.State)
	}

	// --- Envelope write counts during ResolveEscalation only ---

	// Create must not be called — the envelope already exists.
	createsDelta := s.envelopes.creates - createsAfterEval
	if createsDelta != 0 {
		t.Errorf("Envelopes.Create called %d time(s) during ResolveEscalation, want 0", createsDelta)
	}

	// Exactly 1 Update — the single final persistExisting flush.
	updatesDelta := s.envelopes.updates - updatesAfterEval
	if updatesDelta != 1 {
		t.Errorf("Envelopes.Update called %d time(s) during ResolveEscalation, want 1", updatesDelta)
	}

	// Exactly 2 audit events: escalation_reviewed + envelope_closed.
	auditDelta := len(s.audit.events) - auditCountAfterEval
	if auditDelta != 2 {
		t.Errorf("Audit.Append called %d time(s) during ResolveEscalation, want 2", auditDelta)
	}

	// Verify the two new events are in the correct order.
	newEvents := s.audit.events[auditCountAfterEval:]
	if len(newEvents) == 2 {
		if newEvents[0].EventType != audit.AuditEventEscalationReviewed {
			t.Errorf("first ResolveEscalation event: got %q, want %q",
				newEvents[0].EventType, audit.AuditEventEscalationReviewed)
		}
		if newEvents[1].EventType != audit.AuditEventEnvelopeClosed {
			t.Errorf("second ResolveEscalation event: got %q, want %q",
				newEvents[1].EventType, audit.AuditEventEnvelopeClosed)
		}
	}
}
