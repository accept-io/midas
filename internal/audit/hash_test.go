package audit

import (
	"testing"
	"time"
)

func TestComputeEventHash_Deterministic(t *testing.T) {
	now := time.Now().UTC()

	e1 := &AuditEvent{
		EnvelopeID:      "env-1",
		RequestID:       "req-1",
		SequenceNo:      1,
		EventType:       AuditEventEnvelopeCreated,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "midas-orchestrator",
		Payload: map[string]any{
			"surface_id": "loan_auto_approval",
		},
		OccurredAt: now,
		PrevHash:   "",
	}

	e2 := &AuditEvent{
		EnvelopeID:      "env-1",
		RequestID:       "req-1",
		SequenceNo:      1,
		EventType:       AuditEventEnvelopeCreated,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "midas-orchestrator",
		Payload: map[string]any{
			"surface_id": "loan_auto_approval",
		},
		OccurredAt: now,
		PrevHash:   "",
	}

	h1, err := ComputeEventHash(e1)
	if err != nil {
		t.Fatal(err)
	}

	h2, err := ComputeEventHash(e2)
	if err != nil {
		t.Fatal(err)
	}

	if h1 != h2 {
		t.Fatalf("expected identical hashes, got %s vs %s", h1, h2)
	}
}

func TestComputeEventHash_PayloadChangeChangesHash(t *testing.T) {
	now := time.Now().UTC()

	e1 := &AuditEvent{
		EnvelopeID:      "env-1",
		RequestID:       "req-1",
		SequenceNo:      1,
		EventType:       AuditEventEnvelopeCreated,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "midas-orchestrator",
		Payload: map[string]any{
			"confidence": 0.87,
		},
		OccurredAt: now,
	}

	e2 := &AuditEvent{
		EnvelopeID:      "env-1",
		RequestID:       "req-1",
		SequenceNo:      1,
		EventType:       AuditEventEnvelopeCreated,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "midas-orchestrator",
		Payload: map[string]any{
			"confidence": 0.70,
		},
		OccurredAt: now,
	}

	h1, err := ComputeEventHash(e1)
	if err != nil {
		t.Fatal(err)
	}

	h2, err := ComputeEventHash(e2)
	if err != nil {
		t.Fatal(err)
	}

	if h1 == h2 {
		t.Fatalf("expected different hashes but got same value %s", h1)
	}
}

func TestComputeEventHash_PrevHashAffectsHash(t *testing.T) {
	now := time.Now().UTC()

	e1 := &AuditEvent{
		EnvelopeID:      "env-1",
		RequestID:       "req-1",
		SequenceNo:      2,
		EventType:       AuditEventStateTransitioned,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "midas-orchestrator",
		Payload: map[string]any{
			"from_state": "RECEIVED",
			"to_state":   "EVALUATING",
		},
		OccurredAt: now,
		PrevHash:   "abc",
	}

	e2 := *e1
	e2.PrevHash = "different"

	h1, err := ComputeEventHash(e1)
	if err != nil {
		t.Fatal(err)
	}

	h2, err := ComputeEventHash(&e2)
	if err != nil {
		t.Fatal(err)
	}

	if h1 == h2 {
		t.Fatalf("expected different hashes when prev_hash changes")
	}
}

func TestComputeEventHash_PayloadKeyOrderDoesNotAffectHash(t *testing.T) {
	now := time.Now().UTC()

	e1 := &AuditEvent{
		EnvelopeID:      "env-1",
		RequestID:       "req-1",
		SequenceNo:      1,
		EventType:       AuditEventAuthorityChainResolved,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "midas-orchestrator",
		Payload: map[string]any{
			"profile_id":      "profile-loan-low",
			"profile_version": 1,
			"agent_id":        "agent-credit-1",
		},
		OccurredAt: now,
	}

	e2 := &AuditEvent{
		EnvelopeID:      "env-1",
		RequestID:       "req-1",
		SequenceNo:      1,
		EventType:       AuditEventAuthorityChainResolved,
		PerformedByType: EventPerformerSystem,
		PerformedByID:   "midas-orchestrator",
		Payload: map[string]any{
			"agent_id":        "agent-credit-1",
			"profile_version": 1,
			"profile_id":      "profile-loan-low",
		},
		OccurredAt: now,
	}

	h1, err := ComputeEventHash(e1)
	if err != nil {
		t.Fatal(err)
	}

	h2, err := ComputeEventHash(e2)
	if err != nil {
		t.Fatal(err)
	}

	if h1 != h2 {
		t.Fatalf("payload key order should not affect hash, got %s vs %s", h1, h2)
	}
}
