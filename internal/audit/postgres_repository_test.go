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
