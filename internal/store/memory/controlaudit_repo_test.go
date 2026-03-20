package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/controlaudit"
)

func makeRecord(actor, kind, id string, occurredAt time.Time) *controlaudit.ControlAuditRecord {
	return controlaudit.NewSurfaceCreatedRecord(actor, id, 1)
}

func TestControlAuditRepo_AppendAndList(t *testing.T) {
	repo := NewControlAuditRepo()
	ctx := context.Background()

	rec := controlaudit.NewSurfaceCreatedRecord("alice", "surf-1", 1)
	if err := repo.Append(ctx, rec); err != nil {
		t.Fatalf("Append: %v", err)
	}

	results, err := repo.List(ctx, controlaudit.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != rec.ID {
		t.Errorf("expected ID %q, got %q", rec.ID, results[0].ID)
	}
}

func TestControlAuditRepo_AppendNilIsNoOp(t *testing.T) {
	repo := NewControlAuditRepo()
	ctx := context.Background()

	if err := repo.Append(ctx, nil); err != nil {
		t.Fatalf("expected nil error for nil record, got %v", err)
	}
	results, _ := repo.List(ctx, controlaudit.ListFilter{})
	if len(results) != 0 {
		t.Errorf("expected 0 results after nil append, got %d", len(results))
	}
}

func TestControlAuditRepo_ListNewestFirst(t *testing.T) {
	repo := NewControlAuditRepo()
	ctx := context.Background()

	r1 := controlaudit.NewSurfaceCreatedRecord("alice", "surf-1", 1)
	r1.OccurredAt = time.Now().Add(-2 * time.Second)

	r2 := controlaudit.NewSurfaceCreatedRecord("alice", "surf-2", 1)
	r2.OccurredAt = time.Now().Add(-1 * time.Second)

	r3 := controlaudit.NewSurfaceCreatedRecord("alice", "surf-3", 1)
	r3.OccurredAt = time.Now()

	_ = repo.Append(ctx, r1)
	_ = repo.Append(ctx, r2)
	_ = repo.Append(ctx, r3)

	results, err := repo.List(ctx, controlaudit.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3, got %d", len(results))
	}
	// Newest first: r3 > r2 > r1
	if results[0].ResourceID != "surf-3" {
		t.Errorf("expected surf-3 first, got %q", results[0].ResourceID)
	}
	if results[1].ResourceID != "surf-2" {
		t.Errorf("expected surf-2 second, got %q", results[1].ResourceID)
	}
	if results[2].ResourceID != "surf-1" {
		t.Errorf("expected surf-1 third, got %q", results[2].ResourceID)
	}
}

func TestControlAuditRepo_FilterByResourceKind(t *testing.T) {
	repo := NewControlAuditRepo()
	ctx := context.Background()

	_ = repo.Append(ctx, controlaudit.NewSurfaceCreatedRecord("alice", "surf-1", 1))
	_ = repo.Append(ctx, controlaudit.NewAgentCreatedRecord("system", "agent-1"))
	_ = repo.Append(ctx, controlaudit.NewGrantCreatedRecord("system", "grant-1"))

	results, err := repo.List(ctx, controlaudit.ListFilter{ResourceKind: controlaudit.ResourceKindSurface})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 surface record, got %d", len(results))
	}
	if results[0].ResourceKind != controlaudit.ResourceKindSurface {
		t.Errorf("expected kind %q, got %q", controlaudit.ResourceKindSurface, results[0].ResourceKind)
	}
}

func TestControlAuditRepo_FilterByResourceID(t *testing.T) {
	repo := NewControlAuditRepo()
	ctx := context.Background()

	_ = repo.Append(ctx, controlaudit.NewSurfaceCreatedRecord("alice", "surf-1", 1))
	_ = repo.Append(ctx, controlaudit.NewSurfaceCreatedRecord("alice", "surf-2", 1))

	results, err := repo.List(ctx, controlaudit.ListFilter{ResourceID: "surf-1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1, got %d", len(results))
	}
	if results[0].ResourceID != "surf-1" {
		t.Errorf("expected surf-1, got %q", results[0].ResourceID)
	}
}

func TestControlAuditRepo_FilterByActor(t *testing.T) {
	repo := NewControlAuditRepo()
	ctx := context.Background()

	_ = repo.Append(ctx, controlaudit.NewSurfaceCreatedRecord("alice", "surf-1", 1))
	_ = repo.Append(ctx, controlaudit.NewSurfaceCreatedRecord("bob", "surf-2", 1))
	_ = repo.Append(ctx, controlaudit.NewSurfaceCreatedRecord("alice", "surf-3", 1))

	results, err := repo.List(ctx, controlaudit.ListFilter{Actor: "alice"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 alice records, got %d", len(results))
	}
	for _, r := range results {
		if r.Actor != "alice" {
			t.Errorf("expected actor alice, got %q", r.Actor)
		}
	}
}

func TestControlAuditRepo_FilterByAction(t *testing.T) {
	repo := NewControlAuditRepo()
	ctx := context.Background()

	_ = repo.Append(ctx, controlaudit.NewSurfaceCreatedRecord("alice", "surf-1", 1))
	_ = repo.Append(ctx, controlaudit.NewSurfaceApprovedRecord("bob", "surf-1", 1))
	_ = repo.Append(ctx, controlaudit.NewSurfaceDeprecatedRecord("ops", "surf-1", 2, "outdated", ""))

	results, err := repo.List(ctx, controlaudit.ListFilter{Action: controlaudit.ActionSurfaceApproved})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 approved record, got %d", len(results))
	}
	if results[0].Action != controlaudit.ActionSurfaceApproved {
		t.Errorf("expected %q, got %q", controlaudit.ActionSurfaceApproved, results[0].Action)
	}
}

func TestControlAuditRepo_LimitDefault(t *testing.T) {
	repo := NewControlAuditRepo()
	ctx := context.Background()

	// Insert more than the default limit.
	for i := 0; i < controlaudit.DefaultListLimit+10; i++ {
		_ = repo.Append(ctx, controlaudit.NewAgentCreatedRecord("system", "agent"))
	}

	results, err := repo.List(ctx, controlaudit.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != controlaudit.DefaultListLimit {
		t.Errorf("expected %d results (default limit), got %d", controlaudit.DefaultListLimit, len(results))
	}
}

func TestControlAuditRepo_LimitExplicit(t *testing.T) {
	repo := NewControlAuditRepo()
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		_ = repo.Append(ctx, controlaudit.NewAgentCreatedRecord("system", "agent"))
	}

	results, err := repo.List(ctx, controlaudit.ListFilter{Limit: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("expected 5, got %d", len(results))
	}
}

func TestControlAuditRepo_LimitClampedToMax(t *testing.T) {
	repo := NewControlAuditRepo()
	ctx := context.Background()

	for i := 0; i < controlaudit.MaxListLimit+10; i++ {
		_ = repo.Append(ctx, controlaudit.NewAgentCreatedRecord("system", "agent"))
	}

	results, err := repo.List(ctx, controlaudit.ListFilter{Limit: controlaudit.MaxListLimit + 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != controlaudit.MaxListLimit {
		t.Errorf("expected %d (max limit), got %d", controlaudit.MaxListLimit, len(results))
	}
}

func TestControlAuditRepo_EmptyFilterReturnsAll(t *testing.T) {
	repo := NewControlAuditRepo()
	ctx := context.Background()

	_ = repo.Append(ctx, controlaudit.NewSurfaceCreatedRecord("a", "surf-1", 1))
	_ = repo.Append(ctx, controlaudit.NewAgentCreatedRecord("b", "agent-1"))
	_ = repo.Append(ctx, controlaudit.NewGrantCreatedRecord("c", "grant-1"))

	results, err := repo.List(ctx, controlaudit.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3, got %d", len(results))
	}
}

func TestControlAuditRepo_DefensiveCopy(t *testing.T) {
	repo := NewControlAuditRepo()
	ctx := context.Background()

	rec := controlaudit.NewSurfaceCreatedRecord("alice", "surf-1", 1)
	_ = repo.Append(ctx, rec)

	// Mutate the original after appending.
	rec.Actor = "mutated"

	results, _ := repo.List(ctx, controlaudit.ListFilter{})
	if len(results) == 0 {
		t.Fatal("expected 1 result")
	}
	if results[0].Actor == "mutated" {
		t.Error("defensive copy failed: stored record was mutated via original pointer")
	}
}
