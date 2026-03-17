package audit

import (
	"context"
	"testing"
)

func TestMemoryRepository_Append_AssignsSequenceNumbersPerEnvelope(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	ev1 := NewEvent(
		"env-1",
		"actor-1",
		"req-1",
		AuditEventEnvelopeCreated,
		EventPerformerSystem,
		"midas-orchestrator",
		nil,
	)

	ev2 := NewEvent(
		"env-1",
		"actor-1",
		"req-1",
		AuditEventStateTransitioned,
		EventPerformerSystem,
		"midas-orchestrator",
		map[string]any{
			"from_state": "RECEIVED",
			"to_state":   "EVALUATING",
		},
	)

	if err := repo.Append(ctx, ev1); err != nil {
		t.Fatal(err)
	}

	if err := repo.Append(ctx, ev2); err != nil {
		t.Fatal(err)
	}

	if ev1.SequenceNo != 1 {
		t.Fatalf("expected first event sequence 1, got %d", ev1.SequenceNo)
	}

	if ev2.SequenceNo != 2 {
		t.Fatalf("expected second event sequence 2, got %d", ev2.SequenceNo)
	}
}

func TestMemoryRepository_Append_SetsHashChain(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	ev1 := NewEvent(
		"env-1",
		"actor-1",
		"req-1",
		AuditEventEnvelopeCreated,
		EventPerformerSystem,
		"midas-orchestrator",
		nil,
	)

	ev2 := NewEvent(
		"env-1",
		"actor-1",
		"req-1",
		AuditEventStateTransitioned,
		EventPerformerSystem,
		"midas-orchestrator",
		map[string]any{
			"from_state": "RECEIVED",
			"to_state":   "EVALUATING",
		},
	)

	if err := repo.Append(ctx, ev1); err != nil {
		t.Fatal(err)
	}

	if err := repo.Append(ctx, ev2); err != nil {
		t.Fatal(err)
	}

	if ev1.PrevHash != "" {
		t.Fatalf("expected first event PrevHash to be empty, got %q", ev1.PrevHash)
	}

	if ev1.EventHash == "" {
		t.Fatal("expected first event EventHash to be set")
	}

	if ev2.PrevHash != ev1.EventHash {
		t.Fatalf("expected second event PrevHash %q, got %q", ev1.EventHash, ev2.PrevHash)
	}

	if ev2.EventHash == "" {
		t.Fatal("expected second event EventHash to be set")
	}
}

func TestMemoryRepository_ListByEnvelopeID_ReturnsOrderedEvents(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	ev1 := NewEvent(
		"env-1",
		"actor-1",
		"req-1",
		AuditEventEnvelopeCreated,
		EventPerformerSystem,
		"midas-orchestrator",
		nil,
	)

	ev2 := NewEvent(
		"env-1",
		"actor-1",
		"req-1",
		AuditEventSurfaceResolved,
		EventPerformerSystem,
		"midas-orchestrator",
		map[string]any{
			"surface_id":      "loan_auto_approval",
			"surface_version": 1,
		},
	)

	if err := repo.Append(ctx, ev1); err != nil {
		t.Fatal(err)
	}

	if err := repo.Append(ctx, ev2); err != nil {
		t.Fatal(err)
	}

	events, err := repo.ListByEnvelopeID(ctx, "env-1")
	if err != nil {
		t.Fatal(err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	if events[0].SequenceNo != 1 {
		t.Fatalf("expected first returned event sequence 1, got %d", events[0].SequenceNo)
	}

	if events[1].SequenceNo != 2 {
		t.Fatalf("expected second returned event sequence 2, got %d", events[1].SequenceNo)
	}

	if events[0].EventType != AuditEventEnvelopeCreated {
		t.Fatalf("unexpected first event type: %s", events[0].EventType)
	}

	if events[1].EventType != AuditEventSurfaceResolved {
		t.Fatalf("unexpected second event type: %s", events[1].EventType)
	}
}

func TestMemoryRepository_ListByRequestID_ReturnsEvents(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	ev1 := NewEvent(
		"env-1",
		"actor-1",
		"req-123",
		AuditEventEnvelopeCreated,
		EventPerformerSystem,
		"midas-orchestrator",
		nil,
	)

	ev2 := NewEvent(
		"env-1",
		"actor-1",
		"req-123",
		AuditEventAgentResolved,
		EventPerformerSystem,
		"midas-orchestrator",
		map[string]any{
			"agent_id": "agent-credit-1",
		},
	)

	if err := repo.Append(ctx, ev1); err != nil {
		t.Fatal(err)
	}

	if err := repo.Append(ctx, ev2); err != nil {
		t.Fatal(err)
	}

	events, err := repo.ListByRequestID(ctx, "req-123")
	if err != nil {
		t.Fatal(err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	if events[0].RequestID != "req-123" || events[1].RequestID != "req-123" {
		t.Fatalf("expected all events to have request id req-123, got %+v", events)
	}
}

func TestMemoryRepository_SequenceNumbersAreIndependentPerEnvelope(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	ev1 := NewEvent(
		"env-1",
		"actor-1",
		"req-1",
		AuditEventEnvelopeCreated,
		EventPerformerSystem,
		"midas-orchestrator",
		nil,
	)

	ev2 := NewEvent(
		"env-2",
		"actor-2",
		"req-2",
		AuditEventEnvelopeCreated,
		EventPerformerSystem,
		"midas-orchestrator",
		nil,
	)

	ev3 := NewEvent(
		"env-1",
		"actor-1",
		"req-1",
		AuditEventStateTransitioned,
		EventPerformerSystem,
		"midas-orchestrator",
		map[string]any{
			"from_state": "RECEIVED",
			"to_state":   "EVALUATING",
		},
	)

	if err := repo.Append(ctx, ev1); err != nil {
		t.Fatal(err)
	}

	if err := repo.Append(ctx, ev2); err != nil {
		t.Fatal(err)
	}

	if err := repo.Append(ctx, ev3); err != nil {
		t.Fatal(err)
	}

	if ev1.SequenceNo != 1 {
		t.Fatalf("expected env-1 first event sequence 1, got %d", ev1.SequenceNo)
	}

	if ev2.SequenceNo != 1 {
		t.Fatalf("expected env-2 first event sequence 1, got %d", ev2.SequenceNo)
	}

	if ev3.SequenceNo != 2 {
		t.Fatalf("expected env-1 second event sequence 2, got %d", ev3.SequenceNo)
	}
}

func TestMemoryRepository_ListMethods_ReturnCopiesOfSlices(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	ev1 := NewEvent(
		"env-1",
		"actor-1",
		"req-1",
		AuditEventEnvelopeCreated,
		EventPerformerSystem,
		"midas-orchestrator",
		nil,
	)

	ev2 := NewEvent(
		"env-1",
		"actor-1",
		"req-1",
		AuditEventStateTransitioned,
		EventPerformerSystem,
		"midas-orchestrator",
		map[string]any{
			"from_state": "RECEIVED",
			"to_state":   "EVALUATING",
		},
	)

	if err := repo.Append(ctx, ev1); err != nil {
		t.Fatal(err)
	}

	if err := repo.Append(ctx, ev2); err != nil {
		t.Fatal(err)
	}

	events, err := repo.ListByEnvelopeID(ctx, "env-1")
	if err != nil {
		t.Fatal(err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	events[0] = nil

	eventsAgain, err := repo.ListByEnvelopeID(ctx, "env-1")
	if err != nil {
		t.Fatal(err)
	}

	if eventsAgain[0] == nil {
		t.Fatal("expected repository state to be protected from caller slice mutation")
	}
}
