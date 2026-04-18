package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/adminaudit"
)

func applyInvokedRec(actor string) *adminaudit.AdminAuditRecord {
	r := adminaudit.NewRecord(adminaudit.ActionApplyInvoked, adminaudit.OutcomeSuccess, adminaudit.ActorTypeUser)
	r.ActorID = actor
	r.TargetType = adminaudit.TargetTypeBundle
	return r
}

func TestAdminAuditRepo_AppendAndList(t *testing.T) {
	repo := NewAdminAuditRepo()
	ctx := context.Background()
	rec := applyInvokedRec("alice")
	if err := repo.Append(ctx, rec); err != nil {
		t.Fatalf("Append: %v", err)
	}
	results, err := repo.List(ctx, adminaudit.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != rec.ID {
		t.Errorf("ID mismatch: got %q, want %q", results[0].ID, rec.ID)
	}
}

func TestAdminAuditRepo_AppendNilIsNoOp(t *testing.T) {
	repo := NewAdminAuditRepo()
	ctx := context.Background()
	if err := repo.Append(ctx, nil); err != nil {
		t.Fatalf("Append(nil): %v", err)
	}
	results, _ := repo.List(ctx, adminaudit.ListFilter{})
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestAdminAuditRepo_ListNewestFirst(t *testing.T) {
	repo := NewAdminAuditRepo()
	ctx := context.Background()
	now := time.Now().UTC()

	r1 := applyInvokedRec("a")
	r1.OccurredAt = now.Add(-2 * time.Second)
	r2 := applyInvokedRec("b")
	r2.OccurredAt = now.Add(-1 * time.Second)
	r3 := applyInvokedRec("c")
	r3.OccurredAt = now

	_ = repo.Append(ctx, r1)
	_ = repo.Append(ctx, r2)
	_ = repo.Append(ctx, r3)

	results, _ := repo.List(ctx, adminaudit.ListFilter{})
	if len(results) != 3 {
		t.Fatalf("expected 3, got %d", len(results))
	}
	if results[0].ActorID != "c" || results[1].ActorID != "b" || results[2].ActorID != "a" {
		t.Errorf("want order c,b,a; got %q,%q,%q",
			results[0].ActorID, results[1].ActorID, results[2].ActorID)
	}
}

func TestAdminAuditRepo_FilterByAction(t *testing.T) {
	repo := NewAdminAuditRepo()
	ctx := context.Background()

	_ = repo.Append(ctx, applyInvokedRec("a"))
	pwRec := adminaudit.NewRecord(adminaudit.ActionPasswordChanged, adminaudit.OutcomeSuccess, adminaudit.ActorTypeUser)
	pwRec.ActorID = "b"
	_ = repo.Append(ctx, pwRec)

	results, _ := repo.List(ctx, adminaudit.ListFilter{Action: adminaudit.ActionPasswordChanged})
	if len(results) != 1 {
		t.Fatalf("expected 1 password record, got %d", len(results))
	}
	if results[0].Action != adminaudit.ActionPasswordChanged {
		t.Errorf("want password.changed, got %q", results[0].Action)
	}
}

func TestAdminAuditRepo_FilterByActorID(t *testing.T) {
	repo := NewAdminAuditRepo()
	ctx := context.Background()
	_ = repo.Append(ctx, applyInvokedRec("alice"))
	_ = repo.Append(ctx, applyInvokedRec("bob"))
	_ = repo.Append(ctx, applyInvokedRec("alice"))

	results, _ := repo.List(ctx, adminaudit.ListFilter{ActorID: "alice"})
	if len(results) != 2 {
		t.Fatalf("expected 2 alice records, got %d", len(results))
	}
	for _, r := range results {
		if r.ActorID != "alice" {
			t.Errorf("expected alice, got %q", r.ActorID)
		}
	}
}

func TestAdminAuditRepo_FilterByOutcome(t *testing.T) {
	repo := NewAdminAuditRepo()
	ctx := context.Background()
	ok := applyInvokedRec("a")
	fail := applyInvokedRec("b")
	fail.Outcome = adminaudit.OutcomeFailure
	_ = repo.Append(ctx, ok)
	_ = repo.Append(ctx, fail)

	results, _ := repo.List(ctx, adminaudit.ListFilter{Outcome: adminaudit.OutcomeFailure})
	if len(results) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(results))
	}
	if results[0].Outcome != adminaudit.OutcomeFailure {
		t.Errorf("want failure, got %q", results[0].Outcome)
	}
}

func TestAdminAuditRepo_LimitDefaultAndClamp(t *testing.T) {
	repo := NewAdminAuditRepo()
	ctx := context.Background()
	for i := 0; i < adminaudit.DefaultListLimit+10; i++ {
		_ = repo.Append(ctx, applyInvokedRec("x"))
	}
	results, _ := repo.List(ctx, adminaudit.ListFilter{})
	if len(results) != adminaudit.DefaultListLimit {
		t.Errorf("default limit: got %d want %d", len(results), adminaudit.DefaultListLimit)
	}
	clamped, _ := repo.List(ctx, adminaudit.ListFilter{Limit: adminaudit.MaxListLimit + 1000})
	if len(clamped) > adminaudit.MaxListLimit {
		t.Errorf("max limit clamp failed: got %d", len(clamped))
	}
}

func TestAdminAuditRepo_DefensiveCopy(t *testing.T) {
	repo := NewAdminAuditRepo()
	ctx := context.Background()
	rec := applyInvokedRec("alice")
	rec.Details = &adminaudit.Details{BundleBytes: 123}
	_ = repo.Append(ctx, rec)
	rec.ActorID = "mutated"
	rec.Details.BundleBytes = 999

	results, _ := repo.List(ctx, adminaudit.ListFilter{})
	if len(results) == 0 {
		t.Fatal("expected one record")
	}
	if results[0].ActorID == "mutated" {
		t.Error("top-level defensive copy failed")
	}
	if results[0].Details == nil || results[0].Details.BundleBytes == 999 {
		t.Error("details defensive copy failed")
	}
}
