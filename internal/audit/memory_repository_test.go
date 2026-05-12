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

	// EffectiveLimit semantics (D30j):
	//   - Limit == 0 → DefaultListLimit.
	//   - 1 ≤ Limit ≤ MaxListLimit+1 → Limit (the +1 slot is the
	//     cursor-pagination has-next probe headroom; public HTTP
	//     callers cannot reach it because the handler validates user
	//     Limit ≤ MaxListLimit before constructing the filter).
	//   - Limit > MaxListLimit+1 → MaxListLimit+1 (silent clamp).
	if (ListFilter{Limit: MaxListLimit + 1}).EffectiveLimit() != MaxListLimit+1 {
		t.Errorf("EffectiveLimit must permit MaxListLimit+1 (cursor probe headroom)")
	}
	if (ListFilter{Limit: MaxListLimit + 2}).EffectiveLimit() != MaxListLimit+1 {
		t.Errorf("EffectiveLimit must clamp Limit > MaxListLimit+1 to MaxListLimit+1")
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

// ===========================================================================
// D30j — cursor pagination (memory backend).
//
// The repository-level contract: when ListFilter.Cursor is non-nil,
// results are restricted to rows strictly past the cursor tuple per
// the requested OrderDesc semantics. Pages are stitched by
// constructing a cursor from the last row of the prior response and
// re-issuing the same filter.
// ===========================================================================

func TestMemoryRepository_List_Cursor_Desc_PaginatesAcrossPages(t *testing.T) {
	repo := NewMemoryRepository()
	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	// Five events spaced by one second so occurred_at is distinct per row.
	for i := 0; i < 5; i++ {
		makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
			"env-cursor-desc", "src", "req-1",
			t0.Add(time.Duration(i)*time.Second), nil)
	}

	// Page 1: newest two.
	p1, err := repo.List(context.Background(), ListFilter{
		OrderDesc: true, Limit: 2,
	})
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if len(p1) != 2 {
		t.Fatalf("page 1: want 2, got %d", len(p1))
	}
	if !p1[0].OccurredAt.Equal(t0.Add(4*time.Second)) ||
		!p1[1].OccurredAt.Equal(t0.Add(3*time.Second)) {
		t.Errorf("page 1: want [t0+4s, t0+3s], got [%s, %s]",
			p1[0].OccurredAt, p1[1].OccurredAt)
	}

	// Page 2: continue from the last row of page 1.
	cur := &ListCursor{
		OccurredAt: p1[1].OccurredAt,
		SequenceNo: p1[1].SequenceNo,
		ID:         p1[1].ID,
	}
	p2, err := repo.List(context.Background(), ListFilter{
		OrderDesc: true, Limit: 2, Cursor: cur,
	})
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if len(p2) != 2 {
		t.Fatalf("page 2: want 2, got %d", len(p2))
	}
	if !p2[0].OccurredAt.Equal(t0.Add(2*time.Second)) ||
		!p2[1].OccurredAt.Equal(t0.Add(1*time.Second)) {
		t.Errorf("page 2: want [t0+2s, t0+1s], got [%s, %s]",
			p2[0].OccurredAt, p2[1].OccurredAt)
	}

	// Page 3: only the oldest event remains.
	cur = &ListCursor{
		OccurredAt: p2[1].OccurredAt,
		SequenceNo: p2[1].SequenceNo,
		ID:         p2[1].ID,
	}
	p3, err := repo.List(context.Background(), ListFilter{
		OrderDesc: true, Limit: 2, Cursor: cur,
	})
	if err != nil {
		t.Fatalf("List page 3: %v", err)
	}
	if len(p3) != 1 {
		t.Fatalf("page 3: want 1, got %d", len(p3))
	}
	if !p3[0].OccurredAt.Equal(t0) {
		t.Errorf("page 3: want t0, got %s", p3[0].OccurredAt)
	}

	// Page 4: cursor past the last row → empty.
	cur = &ListCursor{
		OccurredAt: p3[0].OccurredAt,
		SequenceNo: p3[0].SequenceNo,
		ID:         p3[0].ID,
	}
	p4, err := repo.List(context.Background(), ListFilter{
		OrderDesc: true, Limit: 2, Cursor: cur,
	})
	if err != nil {
		t.Fatalf("List page 4: %v", err)
	}
	if len(p4) != 0 {
		t.Errorf("page 4: want 0, got %d", len(p4))
	}
}

func TestMemoryRepository_List_Cursor_Asc_PaginatesAcrossPages(t *testing.T) {
	repo := NewMemoryRepository()
	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
			"env-cursor-asc", "src", "req-1",
			t0.Add(time.Duration(i)*time.Second), nil)
	}

	p1, err := repo.List(context.Background(), ListFilter{
		OrderDesc: false, Limit: 2,
	})
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if len(p1) != 2 || !p1[0].OccurredAt.Equal(t0) ||
		!p1[1].OccurredAt.Equal(t0.Add(time.Second)) {
		t.Fatalf("page 1: want [t0, t0+1s], got %+v", p1)
	}

	cur := &ListCursor{
		OccurredAt: p1[1].OccurredAt,
		SequenceNo: p1[1].SequenceNo,
		ID:         p1[1].ID,
	}
	p2, err := repo.List(context.Background(), ListFilter{
		OrderDesc: false, Limit: 2, Cursor: cur,
	})
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if len(p2) != 2 || !p2[0].OccurredAt.Equal(t0.Add(2*time.Second)) ||
		!p2[1].OccurredAt.Equal(t0.Add(3*time.Second)) {
		t.Errorf("page 2: want [t0+2s, t0+3s], got %+v", p2)
	}
}

func TestMemoryRepository_List_Cursor_TieBreaker_SameOccurredAtSameSequenceNo(t *testing.T) {
	// Three events across different envelopes, all with the same
	// occurred_at and identical sequence_no (= 1 within their own
	// envelope). The id tertiary tie-breaker must produce a globally
	// deterministic order, and the cursor must walk through it.
	repo := NewMemoryRepository()
	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	a := makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-a", "src", "req-1", t0, nil)
	b := makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-b", "src", "req-1", t0, nil)
	c := makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-c", "src", "req-1", t0, nil)
	// Pin IDs to a known order so the tertiary tie-breaker is
	// exercised deterministically. After this, lexical id order is
	// 1 < 2 < 3 → all sort comparisons hit the id tertiary branch.
	a.ID = "id-1"
	b.ID = "id-2"
	c.ID = "id-3"

	all, err := repo.List(context.Background(), ListFilter{
		OrderDesc: false, Limit: 10,
	})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("want 3 events, got %d", len(all))
	}
	if all[0].ID != "id-1" || all[1].ID != "id-2" || all[2].ID != "id-3" {
		t.Fatalf("tertiary tie-breaker: want [id-1, id-2, id-3], got [%s, %s, %s]",
			all[0].ID, all[1].ID, all[2].ID)
	}

	// Cursor from id-1 → must return id-2 and id-3.
	cur := &ListCursor{
		OccurredAt: all[0].OccurredAt,
		SequenceNo: all[0].SequenceNo,
		ID:         all[0].ID,
	}
	rest, err := repo.List(context.Background(), ListFilter{
		OrderDesc: false, Limit: 10, Cursor: cur,
	})
	if err != nil {
		t.Fatalf("List after cursor: %v", err)
	}
	if len(rest) != 2 || rest[0].ID != "id-2" || rest[1].ID != "id-3" {
		t.Errorf("cursor past id-1: want [id-2, id-3], got %+v", rest)
	}

	// Cursor from id-3 (the last row) → empty.
	cur = &ListCursor{
		OccurredAt: rest[1].OccurredAt,
		SequenceNo: rest[1].SequenceNo,
		ID:         rest[1].ID,
	}
	tail, err := repo.List(context.Background(), ListFilter{
		OrderDesc: false, Limit: 10, Cursor: cur,
	})
	if err != nil {
		t.Fatalf("List after final cursor: %v", err)
	}
	if len(tail) != 0 {
		t.Errorf("cursor past final row: want 0, got %d", len(tail))
	}
}

func TestMemoryRepository_List_Cursor_AppliesAlongsideEventTypeFilter(t *testing.T) {
	// Cursor predicate composes with the other ListFilter dimensions —
	// the cursor narrows the ordered scan but does not bypass any
	// filter. Seed a mixed event-type chain and walk only the
	// DETECTED events with a cursor.
	repo := NewMemoryRepository()
	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-mix", "src", "req-1", t0, nil)
	makeListTestEvent(t, repo, AuditEventEvaluationStarted,
		"env-mix", "src", "req-1", t0.Add(time.Second), nil)
	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-mix", "src", "req-1", t0.Add(2*time.Second), nil)
	makeListTestEvent(t, repo, AuditEventEvaluationStarted,
		"env-mix", "src", "req-1", t0.Add(3*time.Second), nil)
	makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-mix", "src", "req-1", t0.Add(4*time.Second), nil)

	p1, err := repo.List(context.Background(), ListFilter{
		EventType: AuditEventGovernanceConditionDetected,
		OrderDesc: false, Limit: 2,
	})
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if len(p1) != 2 {
		t.Fatalf("page 1: want 2 DETECTED events, got %d", len(p1))
	}
	if !p1[0].OccurredAt.Equal(t0) || !p1[1].OccurredAt.Equal(t0.Add(2*time.Second)) {
		t.Errorf("page 1: want [t0, t0+2s], got [%s, %s]",
			p1[0].OccurredAt, p1[1].OccurredAt)
	}
	for _, ev := range p1 {
		if ev.EventType != AuditEventGovernanceConditionDetected {
			t.Errorf("page 1: leaked non-DETECTED event %s", ev.EventType)
		}
	}

	cur := &ListCursor{
		OccurredAt: p1[1].OccurredAt,
		SequenceNo: p1[1].SequenceNo,
		ID:         p1[1].ID,
	}
	p2, err := repo.List(context.Background(), ListFilter{
		EventType: AuditEventGovernanceConditionDetected,
		OrderDesc: false, Limit: 2, Cursor: cur,
	})
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if len(p2) != 1 {
		t.Fatalf("page 2: want 1 remaining DETECTED event, got %d", len(p2))
	}
	if !p2[0].OccurredAt.Equal(t0.Add(4 * time.Second)) {
		t.Errorf("page 2: want t0+4s, got %s", p2[0].OccurredAt)
	}
	if p2[0].EventType != AuditEventGovernanceConditionDetected {
		t.Errorf("page 2: leaked non-DETECTED event %s", p2[0].EventType)
	}
}

func TestMemoryRepository_List_Cursor_PastEnd_ReturnsEmpty(t *testing.T) {
	// Repository-level "no more pages" signal: a cursor strictly past
	// the last sorted row returns an empty slice, never a partial page.
	repo := NewMemoryRepository()
	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	ev := makeListTestEvent(t, repo, AuditEventGovernanceConditionDetected,
		"env-end", "src", "req-1", t0, nil)

	cur := &ListCursor{
		OccurredAt: ev.OccurredAt,
		SequenceNo: ev.SequenceNo,
		ID:         ev.ID,
	}
	got, err := repo.List(context.Background(), ListFilter{
		OrderDesc: true, Limit: 10, Cursor: cur,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("cursor at last row: want 0, got %d", len(got))
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
