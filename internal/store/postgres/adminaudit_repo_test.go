package postgres

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/adminaudit"
)

func TestAdminAuditRepo_Postgres_AppendAndList(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`DELETE FROM platform_admin_audit_events`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM platform_admin_audit_events`) }) //nolint

	repo, err := NewAdminAuditRepo(db)
	if err != nil {
		t.Fatalf("NewAdminAuditRepo: %v", err)
	}

	ctx := context.Background()
	rec := adminaudit.NewRecord(adminaudit.ActionApplyInvoked, adminaudit.OutcomeSuccess, adminaudit.ActorTypeUser)
	rec.ActorID = "alice"
	rec.TargetType = adminaudit.TargetTypeBundle
	rec.RequestID = "req-42"
	rec.ClientIP = "10.0.0.1"
	rec.RequiredPermission = "controlplane:apply"
	rec.Details = &adminaudit.Details{BundleBytes: 512, CreatedCount: 3}

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
	got := results[0]
	if got.ID != rec.ID {
		t.Errorf("ID: want %q, got %q", rec.ID, got.ID)
	}
	if got.ActorID != "alice" {
		t.Errorf("ActorID: want alice, got %q", got.ActorID)
	}
	if got.Action != adminaudit.ActionApplyInvoked {
		t.Errorf("Action: want %q, got %q", adminaudit.ActionApplyInvoked, got.Action)
	}
	if got.Outcome != adminaudit.OutcomeSuccess {
		t.Errorf("Outcome: want success, got %q", got.Outcome)
	}
	if got.RequestID != "req-42" {
		t.Errorf("RequestID: want req-42, got %q", got.RequestID)
	}
	if got.ClientIP != "10.0.0.1" {
		t.Errorf("ClientIP: want 10.0.0.1, got %q", got.ClientIP)
	}
	if got.RequiredPermission != "controlplane:apply" {
		t.Errorf("RequiredPermission: want controlplane:apply, got %q", got.RequiredPermission)
	}
	if got.Details == nil || got.Details.BundleBytes != 512 || got.Details.CreatedCount != 3 {
		t.Errorf("Details round-trip failed: got %+v", got.Details)
	}
}

func TestAdminAuditRepo_Postgres_FilterByAction(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`DELETE FROM platform_admin_audit_events`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM platform_admin_audit_events`) }) //nolint

	repo, err := NewAdminAuditRepo(db)
	if err != nil {
		t.Fatalf("NewAdminAuditRepo: %v", err)
	}

	ctx := context.Background()
	a := adminaudit.NewRecord(adminaudit.ActionApplyInvoked, adminaudit.OutcomeSuccess, adminaudit.ActorTypeUser)
	a.ActorID = "alice"
	b := adminaudit.NewRecord(adminaudit.ActionPasswordChanged, adminaudit.OutcomeSuccess, adminaudit.ActorTypeUser)
	b.ActorID = "bob"
	_ = repo.Append(ctx, a)
	_ = repo.Append(ctx, b)

	results, err := repo.List(ctx, adminaudit.ListFilter{Action: adminaudit.ActionPasswordChanged})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1, got %d", len(results))
	}
	if results[0].Action != adminaudit.ActionPasswordChanged {
		t.Errorf("want password.changed, got %q", results[0].Action)
	}
}

func TestAdminAuditRepo_Postgres_NilSafeAppend(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	repo, err := NewAdminAuditRepo(db)
	if err != nil {
		t.Fatalf("NewAdminAuditRepo: %v", err)
	}
	if err := repo.Append(context.Background(), nil); err != nil {
		t.Errorf("Append(nil) should be no-op, got error: %v", err)
	}
}

func TestNewAdminAuditRepo_NilDB(t *testing.T) {
	if _, err := NewAdminAuditRepo(nil); err == nil {
		t.Error("expected error for nil db")
	}
}
