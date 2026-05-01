package audit

import (
	"context"
	"errors"
	"testing"
	"time"
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

// ===========================================================================
// List() — generic event-query primitive added in #56.
//
// Filter semantics are pinned here for the memory backend; the Postgres
// repo's tests assert parity against the same fixtures.
// ===========================================================================

// makeListTestEvent constructs an event with the supplied event_type +
// envelope/request scope + payload, anchored at the supplied
// occurred_at. Sequence/hash are managed by Append; tests overwrite
// occurred_at after the Append call so time-range queries are
// deterministic regardless of wall-clock.
func makeListTestEvent(
	t *testing.T,
	repo *MemoryRepository,
	eventType AuditEventType,
	envelopeID, requestSource, requestID string,
	occurredAt time.Time,
	payload map[string]any,
) *AuditEvent {
	t.Helper()
	ev := NewEvent(envelopeID, requestSource, requestID, eventType,
		EventPerformerSystem, "midas-orchestrator", payload)
	if err := repo.Append(context.Background(), ev); err != nil {
		t.Fatalf("seed Append: %v", err)
	}
	// Pin the timestamp deterministically so the time-range tests are
	// independent of wall-clock.
	ev.OccurredAt = occurredAt.UTC()
	return ev
}

func TestMemoryRepository_List_NoFilter_ReturnsAllUpToDefaultLimit(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-1", "src", "req-1", now, map[string]any{"expectation_id": "ge-1"})
	makeListTestEvent(t, repo, AuditEventGovernanceCoverageGap,
		"env-1", "src", "req-1", now.Add(time.Second), map[string]any{"expectation_id": "ge-1"})

	got, err := repo.List(context.Background(), ListFilter{OrderDesc: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 events, got %d", len(got))
	}
}

func TestMemoryRepository_List_EventType_FiltersToSingleType(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-1", "src", "req-1", now, map[string]any{"expectation_id": "ge-1"})
	makeListTestEvent(t, repo, AuditEventGovernanceCoverageGap,
		"env-1", "src", "req-1", now.Add(time.Second), map[string]any{"expectation_id": "ge-1"})

	got, err := repo.List(context.Background(), ListFilter{
		EventType: AuditEventGovernanceCoverageGap,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].EventType != AuditEventGovernanceCoverageGap {
		t.Errorf("want 1 gap event, got %+v", got)
	}
}

func TestMemoryRepository_List_EventTypes_Wins_OverEventType(t *testing.T) {
	// Per the brief: when EventTypes is non-empty, it wins over EventType.
	repo := NewMemoryRepository()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-1", "src", "req-1", now, map[string]any{})
	makeListTestEvent(t, repo, AuditEventGovernanceCoverageGap,
		"env-1", "src", "req-1", now.Add(time.Second), map[string]any{})

	got, err := repo.List(context.Background(), ListFilter{
		EventType:  AuditEventGovernanceCoverageGap, // narrower
		EventTypes: []AuditEventType{AuditEventGovernanceConditionDetected, AuditEventGovernanceCoverageGap},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("EventTypes must win over EventType; want 2 events, got %d", len(got))
	}
}

func TestMemoryRepository_List_EnvelopeAndRequestScope_Filters(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-1", "src-A", "req-1", now, nil)
	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-2", "src-A", "req-2", now.Add(time.Second), nil)
	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-3", "src-B", "req-1", now.Add(2*time.Second), nil)

	cases := []struct {
		name       string
		filter     ListFilter
		wantCount  int
		wantFirstE string
	}{
		{"envelope_id", ListFilter{EnvelopeID: "env-2"}, 1, "env-2"},
		{"request_source", ListFilter{RequestSource: "src-A"}, 2, ""},
		{"request_id", ListFilter{RequestID: "req-1"}, 2, ""},
		{"request_source+request_id", ListFilter{RequestSource: "src-A", RequestID: "req-1"}, 1, "env-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.List(context.Background(), tc.filter)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(got) != tc.wantCount {
				t.Errorf("want %d, got %d", tc.wantCount, len(got))
			}
			if tc.wantFirstE != "" && got[0].EnvelopeID != tc.wantFirstE {
				t.Errorf("first envelope_id: want %q, got %q", tc.wantFirstE, got[0].EnvelopeID)
			}
		})
	}
}

func TestMemoryRepository_List_PayloadContains_TopLevelOnly(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-1", "src", "req-1", now,
		map[string]any{"expectation_id": "ge-A", "process_id": "proc-1"})
	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-2", "src", "req-2", now.Add(time.Second),
		map[string]any{"expectation_id": "ge-B", "process_id": "proc-1"})
	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-3", "src", "req-3", now.Add(2*time.Second),
		map[string]any{"expectation_id": "ge-A", "process_id": "proc-2"})

	got, err := repo.List(context.Background(), ListFilter{
		PayloadContains: map[string]any{"expectation_id": "ge-A"},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expectation_id=ge-A: want 2, got %d", len(got))
	}

	got, err = repo.List(context.Background(), ListFilter{
		PayloadContains: map[string]any{"expectation_id": "ge-A", "process_id": "proc-1"},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].EnvelopeID != "env-1" {
		t.Errorf("expectation_id=ge-A AND process_id=proc-1: want 1 (env-1), got %d", len(got))
	}
}

func TestMemoryRepository_List_TimeRange_SinceInclusive_UntilExclusive(t *testing.T) {
	repo := NewMemoryRepository()
	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-1", "src", "req-1", t0, nil)
	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-2", "src", "req-2", t0.Add(time.Second), nil)
	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-3", "src", "req-3", t0.Add(2*time.Second), nil)

	// Since == t0 (inclusive), Until == t0+2s (exclusive) → only env-1
	// and env-2 should appear.
	got, err := repo.List(context.Background(), ListFilter{
		Since: t0,
		Until: t0.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 events in [t0, t0+2s), got %d", len(got))
	}
}

func TestMemoryRepository_List_OrderDesc_NewestFirst(t *testing.T) {
	repo := NewMemoryRepository()
	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-1", "src", "req-1", t0, nil)
	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-2", "src", "req-2", t0.Add(time.Second), nil)
	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-3", "src", "req-3", t0.Add(2*time.Second), nil)

	got, err := repo.List(context.Background(), ListFilter{OrderDesc: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3, got %d", len(got))
	}
	if got[0].EnvelopeID != "env-3" || got[1].EnvelopeID != "env-2" || got[2].EnvelopeID != "env-1" {
		t.Errorf("OrderDesc: want [env-3, env-2, env-1], got [%s, %s, %s]",
			got[0].EnvelopeID, got[1].EnvelopeID, got[2].EnvelopeID)
	}
}

func TestMemoryRepository_List_LimitClamps(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
			"env-1", "src", "req-1", now.Add(time.Duration(i)*time.Second), nil)
	}

	got, err := repo.List(context.Background(), ListFilter{Limit: 3})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("Limit=3: want 3, got %d", len(got))
	}

	// Limit > MaxListLimit clamps silently. We can't easily seed >500
	// in a unit test, so assert via EffectiveLimit.
	if (ListFilter{Limit: MaxListLimit + 1}).EffectiveLimit() != MaxListLimit {
		t.Errorf("EffectiveLimit must clamp to MaxListLimit")
	}
	if (ListFilter{}).EffectiveLimit() != DefaultListLimit {
		t.Errorf("EffectiveLimit must default when zero")
	}
}

func TestMemoryRepository_List_InvalidTimeRange_ReturnsError(t *testing.T) {
	repo := NewMemoryRepository()
	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	_, err := repo.List(context.Background(), ListFilter{
		Since: t0.Add(time.Hour),
		Until: t0,
	})
	if !errors.Is(err, ErrInvalidTimeRange) {
		t.Errorf("want ErrInvalidTimeRange, got %v", err)
	}
}

func TestMemoryRepository_List_ReturnsDefensiveCopies(t *testing.T) {
	repo := NewMemoryRepository()
	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-1", "src", "req-1", time.Now().UTC(),
		map[string]any{"original": "value"})

	got, err := repo.List(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	got[0].EventType = AuditEventType("MUTATED")

	again, _ := repo.List(context.Background(), ListFilter{})
	if again[0].EventType == "MUTATED" {
		t.Error("List must return defensive copies; caller mutation leaked into stored state")
	}
}
