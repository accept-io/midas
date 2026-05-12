package audit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/envelope"
)

type stubEnvelopeRepo struct {
	items []*envelope.Envelope
}

func (r stubEnvelopeRepo) List(ctx context.Context) ([]*envelope.Envelope, error) {
	return r.items, nil
}

type stubAuditRepo struct {
	events map[string][]*AuditEvent
}

func (r stubAuditRepo) ListByEnvelopeID(ctx context.Context, envelopeID string) ([]*AuditEvent, error) {
	return r.events[envelopeID], nil
}

// Fixed base time for deterministic tests
var baseTime = time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC)

func TestVerifyAuditIntegrity_ValidChain(t *testing.T) {
	ctx := context.Background()
	env := &envelope.Envelope{
		Identity: envelope.Identity{ID: "env-1"},
		State:    envelope.EnvelopeStateClosed,
	}

	t1 := baseTime
	t2 := baseTime.Add(time.Second)

	ev1 := &AuditEvent{
		ID:              "ev-1",
		EnvelopeID:      env.ID(),
		RequestID:       "req-1",
		SequenceNo:      1,
		EventType:       AuditEventEnvelopeCreated,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "test",
		Payload:         map[string]any{},
		OccurredAt:      t1,
		PrevHash:        "",
	}
	hash1, err := ComputeEventHash(ev1)
	if err != nil {
		t.Fatalf("compute hash1: %v", err)
	}
	ev1.EventHash = hash1

	ev2 := &AuditEvent{
		ID:              "ev-2",
		EnvelopeID:      env.ID(),
		RequestID:       "req-1",
		SequenceNo:      2,
		EventType:       AuditEventEnvelopeClosed,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "test",
		Payload: map[string]any{
			"from_state": string(envelope.EnvelopeStateOutcomeRecorded),
			"to_state":   string(envelope.EnvelopeStateClosed),
		},
		OccurredAt: t2,
		PrevHash:   hash1,
	}
	hash2, err := ComputeEventHash(ev2)
	if err != nil {
		t.Fatalf("compute hash2: %v", err)
	}
	ev2.EventHash = hash2

	envelopeRepo := stubEnvelopeRepo{
		items: []*envelope.Envelope{env},
	}
	auditRepo := stubAuditRepo{
		events: map[string][]*AuditEvent{
			env.ID(): {ev2, ev1}, // deliberately unsorted
		},
	}

	if err := VerifyAuditIntegrity(ctx, envelopeRepo, auditRepo); err != nil {
		t.Fatalf("expected valid chain, got error: %v", err)
	}
}

func TestVerifyAuditIntegrity_HashMismatch(t *testing.T) {
	ctx := context.Background()
	env := &envelope.Envelope{
		Identity: envelope.Identity{ID: "env-1"},
		State:    envelope.EnvelopeStateClosed,
	}

	t1 := baseTime
	t2 := baseTime.Add(time.Second)

	ev1 := &AuditEvent{
		ID:              "ev-1",
		EnvelopeID:      env.ID(),
		RequestID:       "req-1",
		SequenceNo:      1,
		EventType:       AuditEventEnvelopeCreated,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "test",
		Payload:         map[string]any{},
		OccurredAt:      t1,
		PrevHash:        "",
	}
	hash1, err := ComputeEventHash(ev1)
	if err != nil {
		t.Fatalf("compute hash1: %v", err)
	}
	ev1.EventHash = hash1

	ev2 := &AuditEvent{
		ID:              "ev-2",
		EnvelopeID:      env.ID(),
		RequestID:       "req-1",
		SequenceNo:      2,
		EventType:       AuditEventEnvelopeClosed,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "test",
		Payload: map[string]any{
			"from_state": string(envelope.EnvelopeStateOutcomeRecorded),
			"to_state":   string(envelope.EnvelopeStateClosed),
		},
		OccurredAt: t2,
		PrevHash:   hash1,
		EventHash:  "corrupted-hash",
	}

	envelopeRepo := stubEnvelopeRepo{
		items: []*envelope.Envelope{env},
	}
	auditRepo := stubAuditRepo{
		events: map[string][]*AuditEvent{
			env.ID(): {ev1, ev2},
		},
	}

	err = VerifyAuditIntegrity(ctx, envelopeRepo, auditRepo)
	if err == nil {
		t.Fatal("expected hash mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("expected hash mismatch error, got: %v", err)
	}
}

func TestVerifyAuditIntegrity_StateMismatch(t *testing.T) {
	ctx := context.Background()
	env := &envelope.Envelope{
		Identity: envelope.Identity{ID: "env-1"},
		State:    envelope.EnvelopeStateEscalated,
	}

	t1 := baseTime
	t2 := baseTime.Add(time.Second)

	ev1 := &AuditEvent{
		ID:              "ev-1",
		EnvelopeID:      env.ID(),
		RequestID:       "req-1",
		SequenceNo:      1,
		EventType:       AuditEventEnvelopeCreated,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "test",
		Payload:         map[string]any{},
		OccurredAt:      t1,
		PrevHash:        "",
	}
	hash1, err := ComputeEventHash(ev1)
	if err != nil {
		t.Fatalf("compute hash1: %v", err)
	}
	ev1.EventHash = hash1

	ev2 := &AuditEvent{
		ID:              "ev-2",
		EnvelopeID:      env.ID(),
		RequestID:       "req-1",
		SequenceNo:      2,
		EventType:       AuditEventEnvelopeClosed,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "test",
		Payload: map[string]any{
			"from_state": string(envelope.EnvelopeStateOutcomeRecorded),
			"to_state":   string(envelope.EnvelopeStateClosed),
		},
		OccurredAt: t2,
		PrevHash:   hash1,
	}
	hash2, err := ComputeEventHash(ev2)
	if err != nil {
		t.Fatalf("compute hash2: %v", err)
	}
	ev2.EventHash = hash2

	envelopeRepo := stubEnvelopeRepo{
		items: []*envelope.Envelope{env},
	}
	auditRepo := stubAuditRepo{
		events: map[string][]*AuditEvent{
			env.ID(): {ev1, ev2},
		},
	}

	err = VerifyAuditIntegrity(ctx, envelopeRepo, auditRepo)
	if err == nil {
		t.Fatal("expected state mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "state mismatch") {
		t.Fatalf("expected state mismatch error, got: %v", err)
	}
}

func TestVerifyAuditIntegrity_FirstEventWrongSequence(t *testing.T) {
	ctx := context.Background()
	env := &envelope.Envelope{
		Identity: envelope.Identity{ID: "env-1"},
		State:    envelope.EnvelopeStateClosed,
	}

	t1 := baseTime

	ev1 := &AuditEvent{
		ID:              "ev-1",
		EnvelopeID:      env.ID(),
		RequestID:       "req-1",
		SequenceNo:      2, // Should be 1
		EventType:       AuditEventEnvelopeCreated,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "test",
		Payload:         map[string]any{},
		OccurredAt:      t1,
		PrevHash:        "",
	}
	hash1, err := ComputeEventHash(ev1)
	if err != nil {
		t.Fatalf("compute hash1: %v", err)
	}
	ev1.EventHash = hash1

	envelopeRepo := stubEnvelopeRepo{items: []*envelope.Envelope{env}}
	auditRepo := stubAuditRepo{
		events: map[string][]*AuditEvent{env.ID(): {ev1}},
	}

	err = VerifyAuditIntegrity(ctx, envelopeRepo, auditRepo)
	if err == nil {
		t.Fatal("expected first event sequence error, got nil")
	}
	if !strings.Contains(err.Error(), "first event sequence_no=2, expected 1") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestVerifyAuditIntegrity_FirstEventNonEmptyPrevHash(t *testing.T) {
	ctx := context.Background()
	env := &envelope.Envelope{
		Identity: envelope.Identity{ID: "env-1"},
		State:    envelope.EnvelopeStateClosed,
	}

	t1 := baseTime

	ev1 := &AuditEvent{
		ID:              "ev-1",
		EnvelopeID:      env.ID(),
		RequestID:       "req-1",
		SequenceNo:      1,
		EventType:       AuditEventEnvelopeCreated,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "test",
		Payload:         map[string]any{},
		OccurredAt:      t1,
		PrevHash:        "should-be-empty", // Should be ""
	}
	hash1, err := ComputeEventHash(ev1)
	if err != nil {
		t.Fatalf("compute hash1: %v", err)
	}
	ev1.EventHash = hash1

	envelopeRepo := stubEnvelopeRepo{items: []*envelope.Envelope{env}}
	auditRepo := stubAuditRepo{
		events: map[string][]*AuditEvent{env.ID(): {ev1}},
	}

	err = VerifyAuditIntegrity(ctx, envelopeRepo, auditRepo)
	if err == nil {
		t.Fatal("expected non-empty prev_hash error, got nil")
	}
	if !strings.Contains(err.Error(), "non-empty prev_hash") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestVerifyAuditIntegrity_SequenceGap(t *testing.T) {
	ctx := context.Background()
	env := &envelope.Envelope{
		Identity: envelope.Identity{ID: "env-1"},
		State:    envelope.EnvelopeStateClosed,
	}

	t1 := baseTime
	t2 := baseTime.Add(time.Second)

	ev1 := &AuditEvent{
		ID:              "ev-1",
		EnvelopeID:      env.ID(),
		RequestID:       "req-1",
		SequenceNo:      1,
		EventType:       AuditEventEnvelopeCreated,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "test",
		Payload:         map[string]any{},
		OccurredAt:      t1,
		PrevHash:        "",
	}
	hash1, err := ComputeEventHash(ev1)
	if err != nil {
		t.Fatalf("compute hash1: %v", err)
	}
	ev1.EventHash = hash1

	// Skip sequence 2, jump to 3
	ev3 := &AuditEvent{
		ID:              "ev-3",
		EnvelopeID:      env.ID(),
		RequestID:       "req-1",
		SequenceNo:      3, // Gap: should be 2
		EventType:       AuditEventEnvelopeClosed,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "test",
		Payload: map[string]any{
			"from_state": string(envelope.EnvelopeStateOutcomeRecorded),
			"to_state":   string(envelope.EnvelopeStateClosed),
		},
		OccurredAt: t2,
		PrevHash:   hash1,
	}
	hash3, err := ComputeEventHash(ev3)
	if err != nil {
		t.Fatalf("compute hash3: %v", err)
	}
	ev3.EventHash = hash3

	envelopeRepo := stubEnvelopeRepo{items: []*envelope.Envelope{env}}
	auditRepo := stubAuditRepo{
		events: map[string][]*AuditEvent{env.ID(): {ev1, ev3}},
	}

	err = VerifyAuditIntegrity(ctx, envelopeRepo, auditRepo)
	if err == nil {
		t.Fatal("expected sequence gap error, got nil")
	}
	if !strings.Contains(err.Error(), "sequence gap") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestVerifyAuditIntegrity_ChainBreak(t *testing.T) {
	ctx := context.Background()
	env := &envelope.Envelope{
		Identity: envelope.Identity{ID: "env-1"},
		State:    envelope.EnvelopeStateClosed,
	}

	t1 := baseTime
	t2 := baseTime.Add(time.Second)

	ev1 := &AuditEvent{
		ID:              "ev-1",
		EnvelopeID:      env.ID(),
		RequestID:       "req-1",
		SequenceNo:      1,
		EventType:       AuditEventEnvelopeCreated,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "test",
		Payload:         map[string]any{},
		OccurredAt:      t1,
		PrevHash:        "",
	}
	hash1, err := ComputeEventHash(ev1)
	if err != nil {
		t.Fatalf("compute hash1: %v", err)
	}
	ev1.EventHash = hash1

	ev2 := &AuditEvent{
		ID:              "ev-2",
		EnvelopeID:      env.ID(),
		RequestID:       "req-1",
		SequenceNo:      2,
		EventType:       AuditEventEnvelopeClosed,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "test",
		Payload: map[string]any{
			"from_state": string(envelope.EnvelopeStateOutcomeRecorded),
			"to_state":   string(envelope.EnvelopeStateClosed),
		},
		OccurredAt: t2,
		PrevHash:   "wrong-hash", // Should be hash1
	}
	hash2, err := ComputeEventHash(ev2)
	if err != nil {
		t.Fatalf("compute hash2: %v", err)
	}
	ev2.EventHash = hash2

	envelopeRepo := stubEnvelopeRepo{items: []*envelope.Envelope{env}}
	auditRepo := stubAuditRepo{
		events: map[string][]*AuditEvent{env.ID(): {ev1, ev2}},
	}

	err = VerifyAuditIntegrity(ctx, envelopeRepo, auditRepo)
	if err == nil {
		t.Fatal("expected chain break error, got nil")
	}
	if !strings.Contains(err.Error(), "chain break") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestVerifyAuditIntegrity_FinalEventNotStateTransition(t *testing.T) {
	ctx := context.Background()
	env := &envelope.Envelope{
		Identity: envelope.Identity{ID: "env-1"},
		State:    envelope.EnvelopeStateClosed,
	}

	t1 := baseTime
	t2 := baseTime.Add(time.Second)

	ev1 := &AuditEvent{
		ID:              "ev-1",
		EnvelopeID:      env.ID(),
		RequestID:       "req-1",
		SequenceNo:      1,
		EventType:       AuditEventEnvelopeCreated,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "test",
		Payload:         map[string]any{},
		OccurredAt:      t1,
		PrevHash:        "",
	}
	hash1, err := ComputeEventHash(ev1)
	if err != nil {
		t.Fatalf("compute hash1: %v", err)
	}
	ev1.EventHash = hash1

	// Final event is NOT AuditEventEnvelopeClosed
	ev2 := &AuditEvent{
		ID:              "ev-2",
		EnvelopeID:      env.ID(),
		RequestID:       "req-1",
		SequenceNo:      2,
		EventType:       AuditEventOutcomeRecorded, // Should be AuditEventEnvelopeClosed
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "test",
		Payload: map[string]any{
			"outcome":     "EXECUTE",
			"reason_code": "WITHIN_AUTHORITY",
		},
		OccurredAt: t2,
		PrevHash:   hash1,
	}
	hash2, err := ComputeEventHash(ev2)
	if err != nil {
		t.Fatalf("compute hash2: %v", err)
	}
	ev2.EventHash = hash2

	envelopeRepo := stubEnvelopeRepo{items: []*envelope.Envelope{env}}
	auditRepo := stubAuditRepo{
		events: map[string][]*AuditEvent{env.ID(): {ev1, ev2}},
	}

	err = VerifyAuditIntegrity(ctx, envelopeRepo, auditRepo)
	if err == nil {
		t.Fatal("expected final event type error, got nil")
	}
	if !strings.Contains(err.Error(), "final event is") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestVerifyAuditIntegrity_NoAuditTrail(t *testing.T) {
	ctx := context.Background()
	env := &envelope.Envelope{
		Identity: envelope.Identity{ID: "env-1"},
		State:    envelope.EnvelopeStateClosed,
	}

	envelopeRepo := stubEnvelopeRepo{items: []*envelope.Envelope{env}}
	auditRepo := stubAuditRepo{
		events: map[string][]*AuditEvent{
			env.ID(): {}, // Empty audit trail
		},
	}

	err := VerifyAuditIntegrity(ctx, envelopeRepo, auditRepo)
	if err == nil {
		t.Fatal("expected no audit trail error, got nil")
	}
	if !strings.Contains(err.Error(), "no audit trail") {
		t.Fatalf("wrong error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// D30d — VerifyEnvelopeIntegrity exported helper
// ---------------------------------------------------------------------------

// d30dValidPair returns a deterministic two-event closed envelope
// chain (created → closed) with valid sequence numbers and a correct
// hash chain. Tests mutate the returned events to fabricate each
// integrity-finding scenario.
func d30dValidPair(t *testing.T, envID string) (*envelope.Envelope, *AuditEvent, *AuditEvent) {
	t.Helper()
	env := &envelope.Envelope{
		Identity: envelope.Identity{ID: envID},
		State:    envelope.EnvelopeStateClosed,
	}
	ev1 := &AuditEvent{
		ID:              "ev-1",
		EnvelopeID:      envID,
		RequestID:       "req-1",
		SequenceNo:      1,
		EventType:       AuditEventEnvelopeCreated,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "test",
		Payload:         map[string]any{},
		OccurredAt:      baseTime,
		PrevHash:        "",
	}
	h1, err := ComputeEventHash(ev1)
	if err != nil {
		t.Fatalf("compute hash1: %v", err)
	}
	ev1.EventHash = h1

	ev2 := &AuditEvent{
		ID:              "ev-2",
		EnvelopeID:      envID,
		RequestID:       "req-1",
		SequenceNo:      2,
		EventType:       AuditEventEnvelopeClosed,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "test",
		Payload: map[string]any{
			"from_state": string(envelope.EnvelopeStateOutcomeRecorded),
			"to_state":   string(envelope.EnvelopeStateClosed),
		},
		OccurredAt: baseTime.Add(time.Second),
		PrevHash:   h1,
	}
	h2, err := ComputeEventHash(ev2)
	if err != nil {
		t.Fatalf("compute hash2: %v", err)
	}
	ev2.EventHash = h2
	return env, ev1, ev2
}

func TestVerifyEnvelopeIntegrity_Helper_ValidChain(t *testing.T) {
	env, ev1, ev2 := d30dValidPair(t, "env-d30d-valid")
	repo := stubAuditRepo{events: map[string][]*AuditEvent{env.ID(): {ev1, ev2}}}

	res, err := VerifyEnvelopeIntegrity(context.Background(), repo, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Valid {
		t.Errorf("Valid: want true, got false (%+v)", res)
	}
	if res.EnvelopeID != env.ID() {
		t.Errorf("EnvelopeID: want %q, got %q", env.ID(), res.EnvelopeID)
	}
	if res.ChainLength != 2 {
		t.Errorf("ChainLength: want 2, got %d", res.ChainLength)
	}
	if res.FirstEventHash != ev1.EventHash {
		t.Errorf("FirstEventHash: want %q, got %q", ev1.EventHash, res.FirstEventHash)
	}
	if res.FinalEventHash != ev2.EventHash {
		t.Errorf("FinalEventHash: want %q, got %q", ev2.EventHash, res.FinalEventHash)
	}
	if res.ErrorKind != IntegrityErrorKindNone {
		t.Errorf("ErrorKind: want empty, got %q", res.ErrorKind)
	}
	if res.ErrorMessage != "" {
		t.Errorf("ErrorMessage: want empty, got %q", res.ErrorMessage)
	}
}

// errStubAuditRepo returns a fixed error from ListByEnvelopeID, used
// to drive the repository-error code path in the helper.
type errStubAuditRepo struct{ err error }

func (r errStubAuditRepo) ListByEnvelopeID(_ context.Context, _ string) ([]*AuditEvent, error) {
	return nil, r.err
}

func TestVerifyEnvelopeIntegrity_Helper_RepositoryError_PropagatesError(t *testing.T) {
	env := &envelope.Envelope{Identity: envelope.Identity{ID: "env-d30d-err"}}
	sentinel := errors.New("simulated postgres failure")
	res, err := VerifyEnvelopeIntegrity(context.Background(), errStubAuditRepo{err: sentinel}, env)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("returned error must wrap sentinel; got %v", err)
	}
	if res != (IntegrityVerificationResult{}) {
		t.Errorf("repository error must return zero result; got %+v", res)
	}
}

func TestVerifyEnvelopeIntegrity_Helper_NilArguments(t *testing.T) {
	if _, err := VerifyEnvelopeIntegrity(context.Background(), nil, &envelope.Envelope{}); err == nil {
		t.Error("nil audit repository must return error")
	}
	if _, err := VerifyEnvelopeIntegrity(context.Background(), errStubAuditRepo{}, nil); err == nil {
		t.Error("nil envelope must return error")
	}
}

func TestVerifyEnvelopeIntegrity_Helper_ErrorKinds(t *testing.T) {
	type setupFn func(t *testing.T) (*envelope.Envelope, []*AuditEvent)

	cases := []struct {
		name              string
		setup             setupFn
		wantKind          IntegrityErrorKind
		wantMessageSubstr string
	}{
		{
			name: "missing_events",
			setup: func(t *testing.T) (*envelope.Envelope, []*AuditEvent) {
				return &envelope.Envelope{
					Identity: envelope.Identity{ID: "env-d30d-missing"},
					State:    envelope.EnvelopeStateClosed,
				}, nil
			},
			wantKind:          IntegrityErrorKindMissingEvents,
			wantMessageSubstr: "no audit trail",
		},
		{
			name: "sequence_gap_first_event",
			setup: func(t *testing.T) (*envelope.Envelope, []*AuditEvent) {
				env, ev1, _ := d30dValidPair(t, "env-d30d-firstseq")
				ev1.SequenceNo = 2
				h, _ := ComputeEventHash(ev1)
				ev1.EventHash = h
				return env, []*AuditEvent{ev1}
			},
			wantKind:          IntegrityErrorKindSequenceGap,
			wantMessageSubstr: "first event sequence_no=2, expected 1",
		},
		{
			name: "sequence_gap_midchain",
			setup: func(t *testing.T) (*envelope.Envelope, []*AuditEvent) {
				env, ev1, ev2 := d30dValidPair(t, "env-d30d-midgap")
				ev2.SequenceNo = 3 // gap from 1 → 3
				h, _ := ComputeEventHash(ev2)
				ev2.EventHash = h
				return env, []*AuditEvent{ev1, ev2}
			},
			wantKind:          IntegrityErrorKindSequenceGap,
			wantMessageSubstr: "sequence gap",
		},
		{
			name: "prev_hash_mismatch_first_event",
			setup: func(t *testing.T) (*envelope.Envelope, []*AuditEvent) {
				env, ev1, _ := d30dValidPair(t, "env-d30d-firstprev")
				ev1.PrevHash = "should-be-empty"
				h, _ := ComputeEventHash(ev1)
				ev1.EventHash = h
				return env, []*AuditEvent{ev1}
			},
			wantKind:          IntegrityErrorKindPrevHashMismatch,
			wantMessageSubstr: "non-empty prev_hash",
		},
		{
			name: "prev_hash_mismatch_midchain",
			setup: func(t *testing.T) (*envelope.Envelope, []*AuditEvent) {
				env, ev1, ev2 := d30dValidPair(t, "env-d30d-chainbreak")
				ev2.PrevHash = "wrong-hash"
				h, _ := ComputeEventHash(ev2)
				ev2.EventHash = h
				return env, []*AuditEvent{ev1, ev2}
			},
			wantKind:          IntegrityErrorKindPrevHashMismatch,
			wantMessageSubstr: "chain break",
		},
		{
			name: "event_hash_mismatch",
			setup: func(t *testing.T) (*envelope.Envelope, []*AuditEvent) {
				env, ev1, ev2 := d30dValidPair(t, "env-d30d-hashbad")
				ev2.EventHash = "corrupted-hash"
				return env, []*AuditEvent{ev1, ev2}
			},
			wantKind:          IntegrityErrorKindEventHashMismatch,
			wantMessageSubstr: "hash mismatch",
		},
		{
			name: "terminal_state_mismatch_final_event_type",
			setup: func(t *testing.T) (*envelope.Envelope, []*AuditEvent) {
				env, ev1, ev2 := d30dValidPair(t, "env-d30d-finaltype")
				ev2.EventType = AuditEventOutcomeRecorded
				ev2.Payload = map[string]any{
					"outcome":     "EXECUTE",
					"reason_code": "WITHIN_AUTHORITY",
				}
				h, _ := ComputeEventHash(ev2)
				ev2.EventHash = h
				return env, []*AuditEvent{ev1, ev2}
			},
			wantKind:          IntegrityErrorKindTerminalStateMismatch,
			wantMessageSubstr: "final event is",
		},
		{
			name: "terminal_state_mismatch_to_state_diverged",
			setup: func(t *testing.T) (*envelope.Envelope, []*AuditEvent) {
				env, ev1, ev2 := d30dValidPair(t, "env-d30d-tostate")
				env.State = envelope.EnvelopeStateEscalated // diverges from to_state=closed
				return env, []*AuditEvent{ev1, ev2}
			},
			wantKind:          IntegrityErrorKindTerminalStateMismatch,
			wantMessageSubstr: "state mismatch",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env, events := c.setup(t)
			repo := stubAuditRepo{events: map[string][]*AuditEvent{env.ID(): events}}
			res, err := VerifyEnvelopeIntegrity(context.Background(), repo, env)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Valid {
				t.Errorf("Valid: want false, got true (%+v)", res)
			}
			if res.ErrorKind != c.wantKind {
				t.Errorf("ErrorKind: want %q, got %q", c.wantKind, res.ErrorKind)
			}
			if !strings.Contains(res.ErrorMessage, c.wantMessageSubstr) {
				t.Errorf("ErrorMessage missing %q; got %q", c.wantMessageSubstr, res.ErrorMessage)
			}
			if res.EnvelopeID != env.ID() {
				t.Errorf("EnvelopeID: want %q, got %q", env.ID(), res.EnvelopeID)
			}
		})
	}
}

// TestVerifyEnvelopeIntegrity_Helper_ReportsObservedHashes pins that
// FirstEventHash and FinalEventHash on the result come from the
// observed audit chain (events[0].EventHash / events[len-1].EventHash),
// not from Envelope.Integrity. The helper deliberately does not
// cross-check Envelope.Integrity in D30d.
func TestVerifyEnvelopeIntegrity_Helper_ReportsObservedHashes(t *testing.T) {
	env, ev1, ev2 := d30dValidPair(t, "env-d30d-hashreport")
	// Populate envelope integrity fields with sentinel values that
	// MUST NOT appear on the result — the helper reads the chain,
	// not the envelope.
	env.Integrity = envelope.IntegrityRecord{
		FirstEventHash: "envelope-integrity-first-DO-NOT-USE",
		FinalEventHash: "envelope-integrity-final-DO-NOT-USE",
	}
	repo := stubAuditRepo{events: map[string][]*AuditEvent{env.ID(): {ev1, ev2}}}

	res, err := VerifyEnvelopeIntegrity(context.Background(), repo, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.FirstEventHash != ev1.EventHash {
		t.Errorf("FirstEventHash must come from observed chain; want %q, got %q",
			ev1.EventHash, res.FirstEventHash)
	}
	if res.FinalEventHash != ev2.EventHash {
		t.Errorf("FinalEventHash must come from observed chain; want %q, got %q",
			ev2.EventHash, res.FinalEventHash)
	}
}
