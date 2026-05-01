package audit

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
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
