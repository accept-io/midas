package audit

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/accept-io/midas/internal/runtimeattr"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("MIDAS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MIDAS_TEST_DATABASE_URL not set")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := db.Ping(); err != nil {
		t.Fatalf("failed to ping test database: %v", err)
	}

	return db
}

func resetAuditEventsTable(t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.Exec(`DELETE FROM audit_events`)
	if err != nil {
		t.Fatalf("failed to clear audit_events: %v", err)
	}
}

// insertTestEnvelope inserts a minimal valid operational_envelopes row so that
// audit event appends satisfy the fk_audit_events_envelope FK constraint.
// A cleanup is registered to remove the envelope's audit events then the
// envelope itself after the test completes, respecting the FK direction.
func insertTestEnvelope(t *testing.T, db *sql.DB, id, requestSource, requestID string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := db.Exec(`
		INSERT INTO operational_envelopes
			(id, request_source, request_id, schema_version, state,
			 resolved_json, integrity_json, created_at, updated_at)
		VALUES ($1, $2, $3, 1, 'received', '{}', '{}', $4, $4)
		ON CONFLICT (id) DO NOTHING`,
		id, requestSource, requestID, now,
	)
	if err != nil {
		t.Fatalf("insertTestEnvelope %q: %v", id, err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM audit_events WHERE envelope_id = $1`, id)
		_, _ = db.Exec(`DELETE FROM operational_envelopes WHERE id = $1`, id)
	})
}

func TestPostgresRepository_Append_AssignsSequenceAndHashChain(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	resetAuditEventsTable(t, db)
	insertTestEnvelope(t, db, "env-1", "actor-1", "req-1")

	repo := NewPostgresRepository(db)
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
	ev1.ID = "audit-1"
	ev1.OccurredAt = time.Now().UTC()

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
	ev2.ID = "audit-2"
	ev2.OccurredAt = time.Now().UTC()

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

	if ev1.PrevHash != "" {
		t.Fatalf("expected first PrevHash empty, got %q", ev1.PrevHash)
	}

	if ev1.EventHash == "" {
		t.Fatal("expected first EventHash to be set")
	}

	if ev2.PrevHash != ev1.EventHash {
		t.Fatalf("expected second PrevHash %q, got %q", ev1.EventHash, ev2.PrevHash)
	}

	if ev2.EventHash == "" {
		t.Fatal("expected second EventHash to be set")
	}
}

func TestPostgresRepository_AppendBatch_AssignsSequenceAndHashChain(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	resetAuditEventsTable(t, db)
	insertTestEnvelope(t, db, "env-batch-1", "actor-batch", "req-batch-1")

	attr := runtimeattr.NewCollector()
	repo := NewPostgresRepository(db).WithAttribution(attr)
	ctx := context.Background()
	baseTime := time.Now().UTC()

	events := make([]*AuditEvent, 0, 10)
	for i := 0; i < 10; i++ {
		ev := NewEvent(
			"env-batch-1",
			"actor-batch",
			"req-batch-1",
			AuditEventEnvelopeCreated,
			EventPerformerSystem,
			"midas-orchestrator",
			map[string]any{"index": i},
		)
		ev.ID = "audit-batch-1-" + string(rune('a'+i))
		ev.OccurredAt = baseTime.Add(time.Duration(i) * time.Millisecond)
		events = append(events, ev)
	}

	if err := repo.AppendBatch(ctx, events); err != nil {
		t.Fatalf("AppendBatch: %v", err)
	}

	persisted, err := repo.ListByEnvelopeID(ctx, "env-batch-1")
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	if len(persisted) != 10 {
		t.Fatalf("persisted events: got %d, want 10", len(persisted))
	}
	for i, ev := range persisted {
		if ev.ID != events[i].ID {
			t.Fatalf("event %d ID: got %q, want %q", i, ev.ID, events[i].ID)
		}
		if ev.SequenceNo != i+1 {
			t.Fatalf("event %d sequence: got %d, want %d", i, ev.SequenceNo, i+1)
		}
		if i == 0 && ev.PrevHash != "" {
			t.Fatalf("first PrevHash: got %q, want empty", ev.PrevHash)
		}
		if i > 0 && ev.PrevHash != persisted[i-1].EventHash {
			t.Fatalf("event %d PrevHash: got %q, want %q", i, ev.PrevHash, persisted[i-1].EventHash)
		}
		expectedHash, err := ComputeEventHash(ev)
		if err != nil {
			t.Fatalf("ComputeEventHash event %d: %v", i, err)
		}
		if ev.EventHash != expectedHash {
			t.Fatalf("event %d hash: got %q, want %q", i, ev.EventHash, expectedHash)
		}
		if events[i].SequenceNo != ev.SequenceNo || events[i].PrevHash != ev.PrevHash || events[i].EventHash != ev.EventHash {
			t.Fatalf("event %d was not populated with persisted chain fields", i)
		}
	}

	snap := attr.Snapshot()
	if got := snap.Counts[runtimeattr.CountAuditSelect]; got != 1 {
		t.Fatalf("audit select attribution: got %d, want 1", got)
	}
	if got := snap.Counts[runtimeattr.CountAuditInsert]; got != 1 {
		t.Fatalf("audit insert attribution: got %d, want 1", got)
	}
	if got := snap.Values[runtimeattr.ValueAuditPayloadBytes].Count; got != 10 {
		t.Fatalf("payload size count: got %d, want 10", got)
	}
}

func TestPostgresRepository_AppendBatch_ContinuesAfterExistingTail(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	resetAuditEventsTable(t, db)
	insertTestEnvelope(t, db, "env-batch-tail", "actor-tail", "req-batch-tail")

	repo := NewPostgresRepository(db)
	ctx := context.Background()
	first := NewEvent("env-batch-tail", "actor-tail", "req-batch-tail", AuditEventEnvelopeCreated, EventPerformerSystem, "midas", nil)
	first.ID = "audit-batch-tail-1"
	if err := repo.Append(ctx, first); err != nil {
		t.Fatalf("Append first: %v", err)
	}

	second := NewEvent("env-batch-tail", "actor-tail", "req-batch-tail", AuditEventEvaluationStarted, EventPerformerSystem, "midas", nil)
	second.ID = "audit-batch-tail-2"
	third := NewEvent("env-batch-tail", "actor-tail", "req-batch-tail", AuditEventOutcomeRecorded, EventPerformerSystem, "midas", nil)
	third.ID = "audit-batch-tail-3"
	if err := repo.AppendBatch(ctx, []*AuditEvent{second, third}); err != nil {
		t.Fatalf("AppendBatch tail: %v", err)
	}

	if second.SequenceNo != 2 || third.SequenceNo != 3 {
		t.Fatalf("sequence after tail: got %d,%d want 2,3", second.SequenceNo, third.SequenceNo)
	}
	if second.PrevHash != first.EventHash {
		t.Fatalf("second PrevHash: got %q, want %q", second.PrevHash, first.EventHash)
	}
	if third.PrevHash != second.EventHash {
		t.Fatalf("third PrevHash: got %q, want %q", third.PrevHash, second.EventHash)
	}
}

func TestPostgresRepository_AppendBatch_Validation(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	repo := NewPostgresRepository(db)
	ctx := context.Background()

	if err := repo.AppendBatch(ctx, nil); err != nil {
		t.Fatalf("nil batch should no-op: %v", err)
	}
	if err := repo.AppendBatch(ctx, []*AuditEvent{}); err != nil {
		t.Fatalf("empty batch should no-op: %v", err)
	}
	if err := repo.AppendBatch(ctx, []*AuditEvent{nil}); err == nil {
		t.Fatal("expected nil event error")
	}
	ev1 := NewEvent("env-a", "src", "req", AuditEventEnvelopeCreated, EventPerformerSystem, "midas", nil)
	ev2 := NewEvent("env-b", "src", "req", AuditEventEnvelopeCreated, EventPerformerSystem, "midas", nil)
	if err := repo.AppendBatch(ctx, []*AuditEvent{ev1, ev2}); err == nil {
		t.Fatal("expected mixed-envelope error")
	}
}

func TestPostgresRepository_AppendBatch_RollbackRemovesRows(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	resetAuditEventsTable(t, db)
	insertTestEnvelope(t, db, "env-batch-rollback", "actor-rollback", "req-batch-rollback")

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	txRepo := NewPostgresRepository(tx)
	ev1 := NewEvent("env-batch-rollback", "actor-rollback", "req-batch-rollback", AuditEventEnvelopeCreated, EventPerformerSystem, "midas", nil)
	ev1.ID = "audit-batch-rollback-1"
	ev2 := NewEvent("env-batch-rollback", "actor-rollback", "req-batch-rollback", AuditEventEvaluationStarted, EventPerformerSystem, "midas", nil)
	ev2.ID = "audit-batch-rollback-2"
	if err := txRepo.AppendBatch(ctx, []*AuditEvent{ev1, ev2}); err != nil {
		_ = tx.Rollback()
		t.Fatalf("AppendBatch in tx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	events, err := NewPostgresRepository(db).ListByEnvelopeID(ctx, "env-batch-rollback")
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected rollback to remove audit rows, got %d", len(events))
	}
}

func TestPostgresRepository_ListByEnvelopeID_ReturnsOrderedEvents(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	resetAuditEventsTable(t, db)
	insertTestEnvelope(t, db, "env-1", "actor-1", "req-1")

	repo := NewPostgresRepository(db)
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
	ev1.ID = "audit-3"

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
	ev2.ID = "audit-4"

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

	if events[0].SequenceNo != 1 || events[1].SequenceNo != 2 {
		t.Fatalf("expected ordered sequence numbers 1,2 got %d,%d", events[0].SequenceNo, events[1].SequenceNo)
	}
}

func TestPostgresRepository_ListByRequestID_ReturnsEvents(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	resetAuditEventsTable(t, db)
	insertTestEnvelope(t, db, "env-2", "actor-2", "req-xyz")

	repo := NewPostgresRepository(db)
	ctx := context.Background()

	ev1 := NewEvent(
		"env-2",
		"actor-2",
		"req-xyz",
		AuditEventEnvelopeCreated,
		EventPerformerSystem,
		"midas-orchestrator",
		nil,
	)
	ev1.ID = "audit-5"

	ev2 := NewEvent(
		"env-2",
		"actor-2",
		"req-xyz",
		AuditEventAgentResolved,
		EventPerformerSystem,
		"midas-orchestrator",
		map[string]any{
			"agent_id": "agent-credit-1",
		},
	)
	ev2.ID = "audit-6"

	if err := repo.Append(ctx, ev1); err != nil {
		t.Fatal(err)
	}

	if err := repo.Append(ctx, ev2); err != nil {
		t.Fatal(err)
	}

	events, err := repo.ListByRequestID(ctx, "req-xyz")
	if err != nil {
		t.Fatal(err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	for _, ev := range events {
		if ev.RequestID != "req-xyz" {
			t.Fatalf("expected request id req-xyz, got %s", ev.RequestID)
		}
	}
}

// ===========================================================================
// List() — Postgres parity tests for the new generic query primitive
// (#56). Each test exercises one filter dimension against the live
// audit_events table and indexes.
// ===========================================================================

// seedListEvent appends an event via the production Append path then
// rewrites occurred_at on the row so time-range queries are pinned to a
// deterministic value rather than wall-clock.
func seedListEvent(
	t *testing.T,
	db *sql.DB,
	repo *PostgresRepository,
	eventType AuditEventType,
	envelopeID, requestSource, requestID string,
	occurredAt time.Time,
	payload map[string]any,
) *AuditEvent {
	t.Helper()
	insertTestEnvelope(t, db, envelopeID, requestSource, requestID)

	ev := NewEvent(envelopeID, requestSource, requestID, eventType,
		EventPerformerSystem, "midas-orchestrator", payload)
	if err := repo.Append(context.Background(), ev); err != nil {
		t.Fatalf("seed Append: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE audit_events SET occurred_at = $1 WHERE id = $2`,
		occurredAt.UTC(), ev.ID,
	); err != nil {
		t.Fatalf("seed UPDATE occurred_at: %v", err)
	}
	ev.OccurredAt = occurredAt.UTC()
	return ev
}

func TestPostgresRepository_List_NoFilter_ReturnsAllUpToDefaultLimit(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	resetAuditEventsTable(t, db)
	repo := NewPostgresRepository(db)

	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	const src = "src-no-filter"
	seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
		"env-list-1", src, "req-1", now, map[string]any{"expectation_id": "ge-1"})
	seedListEvent(t, db, repo, AuditEventGovernanceCoverageGap,
		"env-list-1", src, "req-1", now.Add(time.Second), map[string]any{"expectation_id": "ge-1"})

	got, err := repo.List(context.Background(), ListFilter{OrderDesc: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 events, got %d", len(got))
	}
}

func TestPostgresRepository_List_EventTypes_FiltersToUnion(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	resetAuditEventsTable(t, db)
	repo := NewPostgresRepository(db)

	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	const src = "src-evt-types"
	seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
		"env-list-evt-1", src, "req-1", now, nil)
	seedListEvent(t, db, repo, AuditEventGovernanceCoverageGap,
		"env-list-evt-1", src, "req-1", now.Add(time.Second), nil)
	seedListEvent(t, db, repo, AuditEventEvaluationStarted,
		"env-list-evt-1", src, "req-1", now.Add(2*time.Second), nil)

	got, err := repo.List(context.Background(), ListFilter{
		EventTypes: []AuditEventType{
			AuditEventGovernanceConditionDetected,
			AuditEventGovernanceCoverageGap,
		},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 events (DETECTED + GAP), got %d", len(got))
	}
}

func TestPostgresRepository_List_PayloadContains_TopLevelOnly(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	resetAuditEventsTable(t, db)
	repo := NewPostgresRepository(db)

	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	const src = "src-payload"
	seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
		"env-list-pl-1", src, "req-1", now,
		map[string]any{"expectation_id": "ge-A", "process_id": "proc-1"})
	seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
		"env-list-pl-2", src, "req-2", now.Add(time.Second),
		map[string]any{"expectation_id": "ge-B", "process_id": "proc-1"})

	got, err := repo.List(context.Background(), ListFilter{
		PayloadContains: map[string]any{"expectation_id": "ge-A"},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].EnvelopeID != "env-list-pl-1" {
		t.Errorf("PayloadContains expectation_id=ge-A: want 1 (env-list-pl-1), got %d", len(got))
	}
}

func TestPostgresRepository_List_TimeRange_SinceInclusive_UntilExclusive(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	resetAuditEventsTable(t, db)
	repo := NewPostgresRepository(db)

	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	const src = "src-time-range"
	seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
		"env-list-time-1", src, "req-1", t0, nil)
	seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
		"env-list-time-2", src, "req-2", t0.Add(time.Second), nil)
	seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
		"env-list-time-3", src, "req-3", t0.Add(2*time.Second), nil)

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

func TestPostgresRepository_List_OrderDesc_NewestFirst(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	resetAuditEventsTable(t, db)
	repo := NewPostgresRepository(db)

	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	const src = "src-order"
	seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
		"env-list-ord-1", src, "req-1", t0, nil)
	seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
		"env-list-ord-2", src, "req-2", t0.Add(time.Second), nil)

	got, err := repo.List(context.Background(), ListFilter{OrderDesc: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 || got[0].EnvelopeID != "env-list-ord-2" {
		t.Errorf("OrderDesc=true: want newest first; got %+v", got)
	}
}

// ===========================================================================
// D30j — cursor pagination (Postgres backend).
//
// Each test exercises the cursor predicate against the live
// audit_events table. The cursor SQL fragment is a 3-key
// (occurred_at, sequence_no, id) tuple comparison; these tests pin
// that it actually restricts the scan correctly across pages, with
// ties, and alongside other filters.
// ===========================================================================

// updateEventID overwrites the persisted audit_events.id and the
// in-memory pointer so id-based tie-breaker assertions can use a
// known lexical order. Audit IDs are not FK-referenced from other
// tables (only audit_events.envelope_id has an FK), so this is safe
// within the test transaction scope.
func updateEventID(t *testing.T, db *sql.DB, ev *AuditEvent, newID string) {
	t.Helper()
	if _, err := db.Exec(
		`UPDATE audit_events SET id = $1 WHERE id = $2`,
		newID, ev.ID,
	); err != nil {
		t.Fatalf("UPDATE audit_events.id: %v", err)
	}
	ev.ID = newID
}

func TestPostgresRepository_List_Cursor_Desc_PaginatesAcrossPages(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	resetAuditEventsTable(t, db)
	repo := NewPostgresRepository(db)

	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	const src = "src-cursor-desc"
	for i := 0; i < 5; i++ {
		seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
			"env-cursor-desc-1", src, "req-1",
			t0.Add(time.Duration(i)*time.Second), nil)
	}

	p1, err := repo.List(context.Background(), ListFilter{
		RequestSource: src, OrderDesc: true, Limit: 2,
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

	cur := &ListCursor{
		OccurredAt: p1[1].OccurredAt,
		SequenceNo: p1[1].SequenceNo,
		ID:         p1[1].ID,
	}
	p2, err := repo.List(context.Background(), ListFilter{
		RequestSource: src, OrderDesc: true, Limit: 2, Cursor: cur,
	})
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if len(p2) != 2 {
		t.Fatalf("page 2: want 2, got %d", len(p2))
	}
	if !p2[0].OccurredAt.Equal(t0.Add(2*time.Second)) ||
		!p2[1].OccurredAt.Equal(t0.Add(time.Second)) {
		t.Errorf("page 2: want [t0+2s, t0+1s], got [%s, %s]",
			p2[0].OccurredAt, p2[1].OccurredAt)
	}

	cur = &ListCursor{
		OccurredAt: p2[1].OccurredAt,
		SequenceNo: p2[1].SequenceNo,
		ID:         p2[1].ID,
	}
	p3, err := repo.List(context.Background(), ListFilter{
		RequestSource: src, OrderDesc: true, Limit: 2, Cursor: cur,
	})
	if err != nil {
		t.Fatalf("List page 3: %v", err)
	}
	if len(p3) != 1 || !p3[0].OccurredAt.Equal(t0) {
		t.Errorf("page 3: want [t0], got %+v", p3)
	}

	cur = &ListCursor{
		OccurredAt: p3[0].OccurredAt,
		SequenceNo: p3[0].SequenceNo,
		ID:         p3[0].ID,
	}
	p4, err := repo.List(context.Background(), ListFilter{
		RequestSource: src, OrderDesc: true, Limit: 2, Cursor: cur,
	})
	if err != nil {
		t.Fatalf("List page 4: %v", err)
	}
	if len(p4) != 0 {
		t.Errorf("page 4: want 0, got %d", len(p4))
	}
}

func TestPostgresRepository_List_Cursor_Asc_PaginatesAcrossPages(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	resetAuditEventsTable(t, db)
	repo := NewPostgresRepository(db)

	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	const src = "src-cursor-asc"
	for i := 0; i < 5; i++ {
		seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
			"env-cursor-asc-1", src, "req-1",
			t0.Add(time.Duration(i)*time.Second), nil)
	}

	p1, err := repo.List(context.Background(), ListFilter{
		RequestSource: src, OrderDesc: false, Limit: 2,
	})
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if len(p1) != 2 ||
		!p1[0].OccurredAt.Equal(t0) ||
		!p1[1].OccurredAt.Equal(t0.Add(time.Second)) {
		t.Fatalf("page 1: want [t0, t0+1s], got %+v", p1)
	}

	cur := &ListCursor{
		OccurredAt: p1[1].OccurredAt,
		SequenceNo: p1[1].SequenceNo,
		ID:         p1[1].ID,
	}
	p2, err := repo.List(context.Background(), ListFilter{
		RequestSource: src, OrderDesc: false, Limit: 2, Cursor: cur,
	})
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if len(p2) != 2 ||
		!p2[0].OccurredAt.Equal(t0.Add(2*time.Second)) ||
		!p2[1].OccurredAt.Equal(t0.Add(3*time.Second)) {
		t.Errorf("page 2: want [t0+2s, t0+3s], got %+v", p2)
	}
}

func TestPostgresRepository_List_Cursor_TieBreaker_SameOccurredAtSameSequenceNo(t *testing.T) {
	// Three events across distinct envelopes, identical occurred_at,
	// each with sequence_no=1 in its envelope. The id tertiary
	// tie-breaker must produce a globally deterministic order.
	db := openTestDB(t)
	defer db.Close()
	resetAuditEventsTable(t, db)
	repo := NewPostgresRepository(db)

	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	const src = "src-cursor-tie"
	// Distinct request_ids across the three envelopes — the
	// (request_source, request_id) tuple is unique in
	// operational_envelopes.
	a := seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
		"env-tie-a", src, "req-tie-a", t0, nil)
	b := seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
		"env-tie-b", src, "req-tie-b", t0, nil)
	c := seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
		"env-tie-c", src, "req-tie-c", t0, nil)
	updateEventID(t, db, a, "11111111-1111-1111-1111-111111111111")
	updateEventID(t, db, b, "22222222-2222-2222-2222-222222222222")
	updateEventID(t, db, c, "33333333-3333-3333-3333-333333333333")

	all, err := repo.List(context.Background(), ListFilter{
		RequestSource: src, OrderDesc: false, Limit: 10,
	})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("want 3 events, got %d", len(all))
	}
	if all[0].ID != a.ID || all[1].ID != b.ID || all[2].ID != c.ID {
		t.Fatalf("tertiary tie-breaker order: want [a, b, c], got [%s, %s, %s]",
			all[0].ID, all[1].ID, all[2].ID)
	}

	cur := &ListCursor{
		OccurredAt: all[0].OccurredAt,
		SequenceNo: all[0].SequenceNo,
		ID:         all[0].ID,
	}
	rest, err := repo.List(context.Background(), ListFilter{
		RequestSource: src, OrderDesc: false, Limit: 10, Cursor: cur,
	})
	if err != nil {
		t.Fatalf("List after cursor: %v", err)
	}
	if len(rest) != 2 || rest[0].ID != b.ID || rest[1].ID != c.ID {
		t.Errorf("cursor past id a: want [b, c], got %+v", rest)
	}
}

func TestPostgresRepository_List_Cursor_AppliesAlongsideEventTypeFilter(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	resetAuditEventsTable(t, db)
	repo := NewPostgresRepository(db)

	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	const src = "src-cursor-evt"
	seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
		"env-evt-1", src, "req-1", t0, nil)
	seedListEvent(t, db, repo, AuditEventEvaluationStarted,
		"env-evt-1", src, "req-1", t0.Add(time.Second), nil)
	seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
		"env-evt-1", src, "req-1", t0.Add(2*time.Second), nil)
	seedListEvent(t, db, repo, AuditEventEvaluationStarted,
		"env-evt-1", src, "req-1", t0.Add(3*time.Second), nil)
	seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
		"env-evt-1", src, "req-1", t0.Add(4*time.Second), nil)

	p1, err := repo.List(context.Background(), ListFilter{
		RequestSource: src,
		EventType:     AuditEventGovernanceConditionDetected,
		OrderDesc:     false, Limit: 2,
	})
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if len(p1) != 2 {
		t.Fatalf("page 1: want 2, got %d", len(p1))
	}
	for _, ev := range p1 {
		if ev.EventType != AuditEventGovernanceConditionDetected {
			t.Errorf("page 1 leaked %s", ev.EventType)
		}
	}

	cur := &ListCursor{
		OccurredAt: p1[1].OccurredAt,
		SequenceNo: p1[1].SequenceNo,
		ID:         p1[1].ID,
	}
	p2, err := repo.List(context.Background(), ListFilter{
		RequestSource: src,
		EventType:     AuditEventGovernanceConditionDetected,
		OrderDesc:     false, Limit: 2, Cursor: cur,
	})
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if len(p2) != 1 || !p2[0].OccurredAt.Equal(t0.Add(4*time.Second)) {
		t.Errorf("page 2: want [t0+4s], got %+v", p2)
	}
	if p2[0].EventType != AuditEventGovernanceConditionDetected {
		t.Errorf("page 2 leaked %s", p2[0].EventType)
	}
}

func TestPostgresRepository_List_Cursor_PastEnd_ReturnsEmpty(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	resetAuditEventsTable(t, db)
	repo := NewPostgresRepository(db)

	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	ev := seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
		"env-end-1", "src-cursor-end", "req-1", t0, nil)

	cur := &ListCursor{
		OccurredAt: ev.OccurredAt,
		SequenceNo: ev.SequenceNo,
		ID:         ev.ID,
	}
	got, err := repo.List(context.Background(), ListFilter{
		RequestSource: "src-cursor-end",
		OrderDesc:     true, Limit: 10, Cursor: cur,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("cursor at last row: want 0, got %d", len(got))
	}
}

func TestPostgresRepository_List_LimitCapped(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	resetAuditEventsTable(t, db)
	repo := NewPostgresRepository(db)

	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	const src = "src-limit"
	for i := 0; i < 5; i++ {
		seedListEvent(t, db, repo, AuditEventGovernanceConditionDetected,
			"env-list-lim-1", src, "req-1", now.Add(time.Duration(i)*time.Second), nil)
	}

	got, err := repo.List(context.Background(), ListFilter{Limit: 3})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("Limit=3: want 3, got %d", len(got))
	}
}
