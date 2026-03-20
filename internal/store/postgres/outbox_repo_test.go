package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/accept-io/midas/internal/outbox"
)

// openOutboxTestDB re-uses the store_test.go openTestDB helper.
// Since we are in the same package, openTestDB is accessible directly.

// cleanupOutboxEventByID deletes a single outbox event by ID. Used in
// t.Cleanup to ensure each test leaves no residue without performing
// a global DELETE that would interfere with concurrently-running tests.
func cleanupOutboxEventByID(t *testing.T, id string) {
	t.Helper()
	db := openTestDB(t)
	defer db.Close()
	if _, err := db.Exec(`DELETE FROM outbox_events WHERE id = $1`, id); err != nil {
		t.Logf("cleanup outbox event %q: %v", id, err)
	}
}

// mustNewOutboxEvent constructs an OutboxEvent or fails the test.
func mustNewOutboxEvent(t *testing.T, eventType outbox.EventType, aggregateType, aggregateID, topic, eventKey string, payload json.RawMessage) *outbox.OutboxEvent {
	t.Helper()
	ev, err := outbox.New(eventType, aggregateType, aggregateID, topic, eventKey, payload)
	if err != nil {
		t.Fatalf("outbox.New: %v", err)
	}
	return ev
}

func TestOutboxRepo_AppendAndListUnpublished(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	repo, err := NewOutboxRepo(db)
	if err != nil {
		t.Fatalf("NewOutboxRepo: %v", err)
	}

	payload := json.RawMessage(`{"envelope_id":"env-1","outcome":"accept"}`)
	ev := mustNewOutboxEvent(t,
		outbox.EventDecisionCompleted,
		"envelope",
		"outbox-test-env-append-1",
		"midas.decisions",
		"src:req-append-1",
		payload,
	)
	t.Cleanup(func() { cleanupOutboxEventByID(t, ev.ID) })

	if err := repo.Append(ctx, ev); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// List unpublished and find our specific event by ID.
	unpublished, err := repo.ListUnpublished(ctx)
	if err != nil {
		t.Fatalf("ListUnpublished: %v", err)
	}

	var got *outbox.OutboxEvent
	for _, u := range unpublished {
		if u.ID == ev.ID {
			got = u
			break
		}
	}
	if got == nil {
		t.Fatalf("expected to find event %q in unpublished list, not found", ev.ID)
	}
	if got.EventType != outbox.EventDecisionCompleted {
		t.Errorf("expected EventType %q, got %q", outbox.EventDecisionCompleted, got.EventType)
	}
	if got.AggregateType != "envelope" {
		t.Errorf("expected AggregateType %q, got %q", "envelope", got.AggregateType)
	}
	if got.AggregateID != "outbox-test-env-append-1" {
		t.Errorf("expected AggregateID %q, got %q", "outbox-test-env-append-1", got.AggregateID)
	}
	if got.Topic != "midas.decisions" {
		t.Errorf("expected Topic %q, got %q", "midas.decisions", got.Topic)
	}
	if got.EventKey != "src:req-append-1" {
		t.Errorf("expected EventKey %q, got %q", "src:req-append-1", got.EventKey)
	}
	if got.PublishedAt != nil {
		t.Error("expected PublishedAt to be nil")
	}
}

func TestOutboxRepo_MarkPublished(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewOutboxRepo(db)
	if err != nil {
		t.Fatalf("NewOutboxRepo: %v", err)
	}

	ev := mustNewOutboxEvent(t, outbox.EventDecisionEscalated, "envelope", "outbox-test-env-mark-1", "midas.decisions", "k-mark-1", json.RawMessage(`{}`))
	t.Cleanup(func() { cleanupOutboxEventByID(t, ev.ID) })

	if err := repo.Append(ctx, ev); err != nil {
		t.Fatalf("Append: %v", err)
	}

	if err := repo.MarkPublished(ctx, ev.ID); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}

	// Verify this specific event is no longer unpublished.
	unpublished, err := repo.ListUnpublished(ctx)
	if err != nil {
		t.Fatalf("ListUnpublished: %v", err)
	}
	for _, u := range unpublished {
		if u.ID == ev.ID {
			t.Errorf("event %q should be marked published but still appears in unpublished list", ev.ID)
		}
	}

	// Verify published_at is set in the database.
	var publishedAt *time.Time
	row := db.QueryRow(`SELECT published_at FROM outbox_events WHERE id = $1`, ev.ID)
	if err := row.Scan(&publishedAt); err != nil {
		t.Fatalf("scan published_at: %v", err)
	}
	if publishedAt == nil {
		t.Error("expected published_at to be set in database after MarkPublished")
	}
}

func TestOutboxRepo_MarkPublished_NotFound(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewOutboxRepo(db)
	if err != nil {
		t.Fatalf("NewOutboxRepo: %v", err)
	}

	err = repo.MarkPublished(ctx, "nonexistent-id-outbox-test")
	if err == nil {
		t.Error("expected error for non-existent event ID, got nil")
	}
}

func TestOutboxRepo_TransactionRollbackLeavesNoRow(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Construct the event first to get its ID for cleanup guard.
	ev := mustNewOutboxEvent(t, outbox.EventDecisionCompleted, "envelope", "outbox-test-env-rollback-tx", "midas.decisions", "k-rollback", json.RawMessage(`{}`))
	// Cleanup guard: the row should NOT exist after rollback, but register anyway in case test fails.
	t.Cleanup(func() { cleanupOutboxEventByID(t, ev.ID) })

	// Begin a transaction, append an outbox row, then roll back.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	txRepo, err := NewOutboxRepo(tx)
	if err != nil {
		tx.Rollback()
		t.Fatalf("NewOutboxRepo(tx): %v", err)
	}

	if err := txRepo.Append(ctx, ev); err != nil {
		tx.Rollback()
		t.Fatalf("Append inside tx: %v", err)
	}

	// Roll back — the outbox row must vanish.
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	// Read from the non-transactional repo to confirm the row is gone.
	repo, err := NewOutboxRepo(db)
	if err != nil {
		t.Fatalf("NewOutboxRepo: %v", err)
	}
	unpublished, err := repo.ListUnpublished(ctx)
	if err != nil {
		t.Fatalf("ListUnpublished after rollback: %v", err)
	}
	for _, u := range unpublished {
		if u.ID == ev.ID {
			t.Fatal("outbox row survived transaction rollback — atomicity violated")
		}
	}
}

// ---------------------------------------------------------------------------
// ClaimUnpublished tests
// ---------------------------------------------------------------------------

func TestOutboxRepo_ClaimUnpublished_ReturnsOnlyUnpublished(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewOutboxRepo(db)
	if err != nil {
		t.Fatalf("NewOutboxRepo: %v", err)
	}

	// Insert one published and one unpublished event.
	published := mustNewOutboxEvent(t, outbox.EventDecisionCompleted, "envelope", "claim-test-pub-1", "midas.decisions", "k-pub", json.RawMessage(`{}`))
	unpublishedEv := mustNewOutboxEvent(t, outbox.EventDecisionEscalated, "envelope", "claim-test-unpub-1", "midas.decisions", "k-unpub", json.RawMessage(`{}`))

	t.Cleanup(func() {
		cleanupOutboxEventByID(t, published.ID)
		cleanupOutboxEventByID(t, unpublishedEv.ID)
	})

	if err := repo.Append(ctx, published); err != nil {
		t.Fatalf("Append published: %v", err)
	}
	if err := repo.Append(ctx, unpublishedEv); err != nil {
		t.Fatalf("Append unpublished: %v", err)
	}

	// Mark the first one published directly in the DB.
	if err := repo.MarkPublished(ctx, published.ID); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}

	claimed, err := repo.ClaimUnpublished(ctx, 100)
	if err != nil {
		t.Fatalf("ClaimUnpublished: %v", err)
	}

	// The claimed set may contain rows from other concurrent tests.
	// Verify our unpublished event is present and published is absent.
	foundUnpublished := false
	for _, ev := range claimed {
		if ev.ID == published.ID {
			t.Errorf("published event %q should not appear in ClaimUnpublished", published.ID)
		}
		if ev.ID == unpublishedEv.ID {
			foundUnpublished = true
		}
	}
	if !foundUnpublished {
		t.Errorf("expected unpublished event %q in ClaimUnpublished results", unpublishedEv.ID)
	}
}

func TestOutboxRepo_ClaimUnpublished_OrderedByCreatedAtThenID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewOutboxRepo(db)
	if err != nil {
		t.Fatalf("NewOutboxRepo: %v", err)
	}

	ev1 := mustNewOutboxEvent(t, outbox.EventDecisionCompleted, "envelope", "claim-order-1", "midas.decisions", "k1", json.RawMessage(`{}`))
	ev2 := mustNewOutboxEvent(t, outbox.EventDecisionEscalated, "envelope", "claim-order-2", "midas.decisions", "k2", json.RawMessage(`{}`))

	t.Cleanup(func() {
		cleanupOutboxEventByID(t, ev1.ID)
		cleanupOutboxEventByID(t, ev2.ID)
	})

	// Insert in order; created_at will be slightly different due to time.Now().
	if err := repo.Append(ctx, ev1); err != nil {
		t.Fatalf("Append ev1: %v", err)
	}
	if err := repo.Append(ctx, ev2); err != nil {
		t.Fatalf("Append ev2: %v", err)
	}

	claimed, err := repo.ClaimUnpublished(ctx, 100)
	if err != nil {
		t.Fatalf("ClaimUnpublished: %v", err)
	}

	// Find our two events in claimed and check relative ordering.
	idx := func(id string) int {
		for i, ev := range claimed {
			if ev.ID == id {
				return i
			}
		}
		return -1
	}

	i1 := idx(ev1.ID)
	i2 := idx(ev2.ID)
	if i1 < 0 {
		t.Fatalf("ev1 not found in claimed results")
	}
	if i2 < 0 {
		t.Fatalf("ev2 not found in claimed results")
	}
	if i1 >= i2 {
		t.Errorf("expected ev1 (inserted first) before ev2 in ordered results, got positions %d and %d", i1, i2)
	}
}

func TestOutboxRepo_ClaimUnpublished_AfterMarkPublished_NotReturned(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewOutboxRepo(db)
	if err != nil {
		t.Fatalf("NewOutboxRepo: %v", err)
	}

	ev := mustNewOutboxEvent(t, outbox.EventSurfaceApproved, "surface", "claim-mark-1", "midas.surfaces", "k-surf", json.RawMessage(`{}`))
	t.Cleanup(func() { cleanupOutboxEventByID(t, ev.ID) })

	if err := repo.Append(ctx, ev); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Claim, then mark published.
	claimed, err := repo.ClaimUnpublished(ctx, 100)
	if err != nil {
		t.Fatalf("ClaimUnpublished (first): %v", err)
	}
	found := false
	for _, c := range claimed {
		if c.ID == ev.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected to find event %q in initial claim, not found", ev.ID)
	}

	if err := repo.MarkPublished(ctx, ev.ID); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}

	// Second claim should not return the now-published event.
	claimed2, err := repo.ClaimUnpublished(ctx, 100)
	if err != nil {
		t.Fatalf("ClaimUnpublished (second): %v", err)
	}
	for _, c := range claimed2 {
		if c.ID == ev.ID {
			t.Errorf("event %q should not be in ClaimUnpublished after MarkPublished", ev.ID)
		}
	}
}

func TestOutboxRepo_ClaimUnpublished_UsesTransaction(t *testing.T) {
	// Verify that ClaimUnpublished commits its internal transaction so that
	// subsequent write operations on the same db succeed. Specifically, calling
	// MarkPublished after a claim must not fail due to an uncommitted lock.
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewOutboxRepo(db)
	if err != nil {
		t.Fatalf("NewOutboxRepo: %v", err)
	}

	ev := mustNewOutboxEvent(t, outbox.EventDecisionCompleted, "envelope", "claim-tx-verify-1", "midas.decisions", "k-tx", json.RawMessage(`{}`))
	t.Cleanup(func() { cleanupOutboxEventByID(t, ev.ID) })

	if err := repo.Append(ctx, ev); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Claim the event — this opens and commits an internal transaction.
	if _, err := repo.ClaimUnpublished(ctx, 10); err != nil {
		t.Fatalf("ClaimUnpublished: %v", err)
	}

	// MarkPublished must succeed; if the claim left a lock open this would deadlock.
	if err := repo.MarkPublished(ctx, ev.ID); err != nil {
		t.Fatalf("MarkPublished after claim: %v", err)
	}
}

func TestOutboxRepo_TransactionCommitPersistsRow(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	ev := mustNewOutboxEvent(t, outbox.EventSurfaceApproved, "surface", "outbox-test-surf-commit", "midas.surfaces", "surf-commit", json.RawMessage(`{}`))
	t.Cleanup(func() { cleanupOutboxEventByID(t, ev.ID) })

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	txRepo, err := NewOutboxRepo(tx)
	if err != nil {
		tx.Rollback()
		t.Fatalf("NewOutboxRepo(tx): %v", err)
	}

	if err := txRepo.Append(ctx, ev); err != nil {
		tx.Rollback()
		t.Fatalf("Append inside tx: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Confirm the row is visible outside the transaction.
	repo, err := NewOutboxRepo(db)
	if err != nil {
		t.Fatalf("NewOutboxRepo: %v", err)
	}
	unpublished, err := repo.ListUnpublished(ctx)
	if err != nil {
		t.Fatalf("ListUnpublished after commit: %v", err)
	}
	found := false
	for _, u := range unpublished {
		if u.ID == ev.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected outbox row to be present after commit, got none")
	}
}
