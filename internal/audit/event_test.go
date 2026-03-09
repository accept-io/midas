package audit

import (
	"testing"
)

func TestNewEvent_NormalizesNilPayload(t *testing.T) {
	ev := NewEvent(
		"env-1",
		"req-1",
		AuditEventEnvelopeCreated,
		EventPerformerSystem,
		"midas-orchestrator",
		nil,
	)

	if ev.Payload == nil {
		t.Fatal("expected payload to be normalized to empty map")
	}

	if len(ev.Payload) != 0 {
		t.Fatalf("expected empty payload map, got %+v", ev.Payload)
	}
}

func TestNewEvent_SetsBasicFields(t *testing.T) {
	payload := map[string]any{
		"surface_id": "loan_auto_approval",
	}

	ev := NewEvent(
		"env-123",
		"req-456",
		AuditEventSurfaceResolved,
		EventPerformerSystem,
		"midas-orchestrator",
		payload,
	)

	if ev.EnvelopeID != "env-123" {
		t.Fatalf("expected EnvelopeID env-123, got %s", ev.EnvelopeID)
	}

	if ev.RequestID != "req-456" {
		t.Fatalf("expected RequestID req-456, got %s", ev.RequestID)
	}

	if ev.EventType != AuditEventSurfaceResolved {
		t.Fatalf("unexpected event type: %s", ev.EventType)
	}

	if ev.PerformedByType != EventPerformerSystem {
		t.Fatalf("unexpected performer type: %s", ev.PerformedByType)
	}

	if ev.PerformedByID != "midas-orchestrator" {
		t.Fatalf("unexpected performer id: %s", ev.PerformedByID)
	}

	if ev.Payload["surface_id"] != "loan_auto_approval" {
		t.Fatalf("payload not preserved: %+v", ev.Payload)
	}
}

func TestNewEvent_SetsOccurredAt(t *testing.T) {
	ev := NewEvent(
		"env-1",
		"req-1",
		AuditEventEnvelopeCreated,
		EventPerformerSystem,
		"midas-orchestrator",
		nil,
	)

	if ev.OccurredAt.IsZero() {
		t.Fatal("expected OccurredAt to be set")
	}
}

func TestNewEvent_InitializesEventIdentityAndLeavesComputedFieldsUnset(t *testing.T) {
	ev := NewEvent(
		"env-1",
		"req-1",
		AuditEventEnvelopeCreated,
		EventPerformerSystem,
		"midas-orchestrator",
		map[string]any{"key": "value"},
	)

	if ev.ID == "" {
		t.Fatal("expected ID to be set")
	}
	if ev.EnvelopeID != "env-1" {
		t.Fatalf("expected EnvelopeID %q, got %q", "env-1", ev.EnvelopeID)
	}
	if ev.RequestID != "req-1" {
		t.Fatalf("expected RequestID %q, got %q", "req-1", ev.RequestID)
	}
	if ev.EventType != AuditEventEnvelopeCreated {
		t.Fatalf("expected EventType %q, got %q", AuditEventEnvelopeCreated, ev.EventType)
	}
	if ev.PerformedByType != EventPerformerSystem {
		t.Fatalf("expected PerformedByType %q, got %q", EventPerformerSystem, ev.PerformedByType)
	}
	if ev.PerformedByID != "midas-orchestrator" {
		t.Fatalf("expected PerformedByID %q, got %q", "midas-orchestrator", ev.PerformedByID)
	}
	if ev.SequenceNo != 0 {
		t.Fatalf("expected SequenceNo 0, got %d", ev.SequenceNo)
	}
	if ev.PrevHash != "" {
		t.Fatalf("expected PrevHash empty, got %q", ev.PrevHash)
	}
	if ev.EventHash != "" {
		t.Fatalf("expected EventHash empty, got %q", ev.EventHash)
	}
	if ev.OccurredAt.IsZero() {
		t.Fatal("expected OccurredAt to be set")
	}
}
