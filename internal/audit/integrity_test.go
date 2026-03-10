package audit

import (
	"context"
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
		ID:    "env-1",
		State: envelope.EnvelopeStateClosed,
	}

	t1 := baseTime
	t2 := baseTime.Add(time.Second)

	ev1 := &AuditEvent{
		ID:              "ev-1",
		EnvelopeID:      env.ID,
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
		EnvelopeID:      env.ID,
		RequestID:       "req-1",
		SequenceNo:      2,
		EventType:       AuditEventStateTransitioned,
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
			env.ID: {ev2, ev1}, // deliberately unsorted
		},
	}

	if err := VerifyAuditIntegrity(ctx, envelopeRepo, auditRepo); err != nil {
		t.Fatalf("expected valid chain, got error: %v", err)
	}
}

func TestVerifyAuditIntegrity_HashMismatch(t *testing.T) {
	ctx := context.Background()
	env := &envelope.Envelope{
		ID:    "env-1",
		State: envelope.EnvelopeStateClosed,
	}

	t1 := baseTime
	t2 := baseTime.Add(time.Second)

	ev1 := &AuditEvent{
		ID:              "ev-1",
		EnvelopeID:      env.ID,
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
		EnvelopeID:      env.ID,
		RequestID:       "req-1",
		SequenceNo:      2,
		EventType:       AuditEventStateTransitioned,
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
			env.ID: {ev1, ev2},
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
		ID:    "env-1",
		State: envelope.EnvelopeStateEscalated,
	}

	t1 := baseTime
	t2 := baseTime.Add(time.Second)

	ev1 := &AuditEvent{
		ID:              "ev-1",
		EnvelopeID:      env.ID,
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
		EnvelopeID:      env.ID,
		RequestID:       "req-1",
		SequenceNo:      2,
		EventType:       AuditEventStateTransitioned,
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
			env.ID: {ev1, ev2},
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
		ID:    "env-1",
		State: envelope.EnvelopeStateClosed,
	}

	t1 := baseTime

	ev1 := &AuditEvent{
		ID:              "ev-1",
		EnvelopeID:      env.ID,
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
		events: map[string][]*AuditEvent{env.ID: {ev1}},
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
		ID:    "env-1",
		State: envelope.EnvelopeStateClosed,
	}

	t1 := baseTime

	ev1 := &AuditEvent{
		ID:              "ev-1",
		EnvelopeID:      env.ID,
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
		events: map[string][]*AuditEvent{env.ID: {ev1}},
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
		ID:    "env-1",
		State: envelope.EnvelopeStateClosed,
	}

	t1 := baseTime
	t2 := baseTime.Add(time.Second)

	ev1 := &AuditEvent{
		ID:              "ev-1",
		EnvelopeID:      env.ID,
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
		EnvelopeID:      env.ID,
		RequestID:       "req-1",
		SequenceNo:      3, // Gap: should be 2
		EventType:       AuditEventStateTransitioned,
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
		events: map[string][]*AuditEvent{env.ID: {ev1, ev3}},
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
		ID:    "env-1",
		State: envelope.EnvelopeStateClosed,
	}

	t1 := baseTime
	t2 := baseTime.Add(time.Second)

	ev1 := &AuditEvent{
		ID:              "ev-1",
		EnvelopeID:      env.ID,
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
		EnvelopeID:      env.ID,
		RequestID:       "req-1",
		SequenceNo:      2,
		EventType:       AuditEventStateTransitioned,
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
		events: map[string][]*AuditEvent{env.ID: {ev1, ev2}},
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
		ID:    "env-1",
		State: envelope.EnvelopeStateClosed,
	}

	t1 := baseTime
	t2 := baseTime.Add(time.Second)

	ev1 := &AuditEvent{
		ID:              "ev-1",
		EnvelopeID:      env.ID,
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

	// Final event is NOT StateTransitioned
	ev2 := &AuditEvent{
		ID:              "ev-2",
		EnvelopeID:      env.ID,
		RequestID:       "req-1",
		SequenceNo:      2,
		EventType:       AuditEventOutcomeRecorded, // Should be StateTransitioned
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
		events: map[string][]*AuditEvent{env.ID: {ev1, ev2}},
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
		ID:    "env-1",
		State: envelope.EnvelopeStateClosed,
	}

	envelopeRepo := stubEnvelopeRepo{items: []*envelope.Envelope{env}}
	auditRepo := stubAuditRepo{
		events: map[string][]*AuditEvent{
			env.ID: {}, // Empty audit trail
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
