package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/capability"
)

func TestCapabilityRepo_CreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	repo := NewCapabilityRepo()

	now := time.Now().UTC()
	c := &capability.Capability{
		ID:        "cap-create-001",
		Name:      "Identity Verification",
		Status:    "active",
		Origin:    "manual",
		Managed:   true,
		Owner:     "team-platform",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected capability, got nil")
	}

	checks := []struct{ field, want, got string }{
		{"ID", c.ID, got.ID},
		{"Name", c.Name, got.Name},
		{"Status", c.Status, got.Status},
		{"Origin", c.Origin, got.Origin},
		{"Owner", c.Owner, got.Owner},
	}
	for _, ck := range checks {
		if ck.want != ck.got {
			t.Errorf("%s: want %q, got %q", ck.field, ck.want, ck.got)
		}
	}
	if got.Managed != c.Managed {
		t.Errorf("Managed: want %v, got %v", c.Managed, got.Managed)
	}
}

func TestCapabilityRepo_GetByID_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := NewCapabilityRepo()

	got, err := repo.GetByID(ctx, "cap-nonexistent")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestCapabilityRepo_Exists(t *testing.T) {
	ctx := context.Background()
	repo := NewCapabilityRepo()

	ok, err := repo.Exists(ctx, "cap-nonexistent")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if ok {
		t.Error("expected false for non-existent capability")
	}

	now := time.Now().UTC()
	c := &capability.Capability{
		ID:        "cap-exists-001",
		Name:      "Credit Scoring",
		Status:    "active",
		Origin:    "manual",
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}

	ok, err = repo.Exists(ctx, c.ID)
	if err != nil {
		t.Fatalf("Exists after create: %v", err)
	}
	if !ok {
		t.Error("expected true after create")
	}
}

func TestCapabilityRepo_List(t *testing.T) {
	ctx := context.Background()
	repo := NewCapabilityRepo()

	now := time.Now().UTC()
	ids := []string{"cap-list-001", "cap-list-002"}
	for _, id := range ids {
		c := &capability.Capability{
			ID:        id,
			Name:      "Capability " + id,
			Status:    "active",
			Origin:    "manual",
			Managed:   true,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := repo.Create(ctx, c); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}

	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	found := 0
	for _, c := range all {
		for _, id := range ids {
			if c.ID == id {
				found++
			}
		}
	}
	if found != len(ids) {
		t.Errorf("List: want %d capabilities, got %d total with %d matching", len(ids), len(all), found)
	}
}

// ---------------------------------------------------------------------------
// ListByParentCapabilityID (Phase 2)
//
// Direct-children lookup. Returns only the immediate children of the
// given parent — recursive descendants are intentionally NOT
// traversed; a caller that wants the full subtree walks the
// hierarchy itself. Empty result is a non-nil empty slice; empty
// parentID short-circuits to the same.
// ---------------------------------------------------------------------------

// seedCap is a small constructor for the direct-children tests below.
// Mirrors the existing TestCapabilityRepo_* fixture style.
func seedCap(t *testing.T, repo *CapabilityRepo, id, parentID string) {
	t.Helper()
	now := time.Now().UTC()
	if err := repo.Create(context.Background(), &capability.Capability{
		ID:                 id,
		Name:               id,
		Status:             "active",
		Origin:             "manual",
		Managed:            true,
		ParentCapabilityID: parentID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}); err != nil {
		t.Fatalf("Create %s: %v", id, err)
	}
}

func TestCapabilityRepo_ListByParentCapabilityID_Empty(t *testing.T) {
	ctx := context.Background()
	repo := NewCapabilityRepo()

	// No data, lookup against a non-existent parent.
	out, err := repo.ListByParentCapabilityID(ctx, "cap-no-such-parent")
	if err != nil {
		t.Fatalf("ListByParentCapabilityID: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(out) != 0 {
		t.Errorf("expected 0 children, got %d", len(out))
	}

	// With a populated parent that has no children, same contract.
	seedCap(t, repo, "cap-leaf-only", "")
	out2, err := repo.ListByParentCapabilityID(ctx, "cap-leaf-only")
	if err != nil {
		t.Fatalf("ListByParentCapabilityID(leaf): %v", err)
	}
	if out2 == nil || len(out2) != 0 {
		t.Errorf("expected non-nil empty slice for leaf parent, got %v", out2)
	}

	// Empty parentID short-circuits per documented contract.
	out3, err := repo.ListByParentCapabilityID(ctx, "")
	if err != nil {
		t.Fatalf("ListByParentCapabilityID(\"\"): %v", err)
	}
	if out3 == nil || len(out3) != 0 {
		t.Errorf("empty parentID must short-circuit to non-nil empty slice; got %v", out3)
	}
}

func TestCapabilityRepo_ListByParentCapabilityID_FiltersByParent(t *testing.T) {
	ctx := context.Background()
	repo := NewCapabilityRepo()

	// Two parents A and B with their own children plus an unrelated root.
	seedCap(t, repo, "cap-parent-a", "")
	seedCap(t, repo, "cap-parent-b", "")
	seedCap(t, repo, "cap-other-root", "")
	seedCap(t, repo, "cap-a-child-1", "cap-parent-a")
	seedCap(t, repo, "cap-a-child-2", "cap-parent-a")
	seedCap(t, repo, "cap-b-child-1", "cap-parent-b")

	got, err := repo.ListByParentCapabilityID(ctx, "cap-parent-a")
	if err != nil {
		t.Fatalf("ListByParentCapabilityID: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 children of cap-parent-a, got %d", len(got))
	}
	for _, c := range got {
		if c.ParentCapabilityID != "cap-parent-a" {
			t.Errorf("filter leaked: %s has parent_capability_id=%q", c.ID, c.ParentCapabilityID)
		}
	}

	// Sanity-check the other branch returns only its own children, not
	// A's, and definitely not the other-root.
	gotB, err := repo.ListByParentCapabilityID(ctx, "cap-parent-b")
	if err != nil {
		t.Fatalf("ListByParentCapabilityID(B): %v", err)
	}
	if len(gotB) != 1 || gotB[0].ID != "cap-b-child-1" {
		t.Errorf("ListByParentCapabilityID(B): want [cap-b-child-1], got %+v", gotB)
	}
}

func TestCapabilityRepo_ListByParentCapabilityID_DirectChildrenOnly(t *testing.T) {
	ctx := context.Background()
	repo := NewCapabilityRepo()

	// Three-level hierarchy: parent → child → grandchild.
	seedCap(t, repo, "cap-top", "")
	seedCap(t, repo, "cap-mid", "cap-top")
	seedCap(t, repo, "cap-leaf", "cap-mid")

	got, err := repo.ListByParentCapabilityID(ctx, "cap-top")
	if err != nil {
		t.Fatalf("ListByParentCapabilityID: %v", err)
	}
	if len(got) != 1 || got[0].ID != "cap-mid" {
		t.Fatalf("ListByParentCapabilityID(cap-top): want only direct child cap-mid, got %v", got)
	}
	for _, c := range got {
		if c.ID == "cap-leaf" {
			t.Error("recursive descendant cap-leaf must NOT be returned by ListByParentCapabilityID(cap-top)")
		}
	}
}

func TestCapabilityRepo_ListByParentCapabilityID_OrdersByID(t *testing.T) {
	ctx := context.Background()
	repo := NewCapabilityRepo()

	// Insert children out of lexical order. Memory map iteration is
	// non-deterministic; the ordering contract is enforced by an
	// explicit sort.
	seedCap(t, repo, "cap-order-parent", "")
	seedCap(t, repo, "cap-z-child", "cap-order-parent")
	seedCap(t, repo, "cap-m-child", "cap-order-parent")
	seedCap(t, repo, "cap-a-child", "cap-order-parent")

	got, err := repo.ListByParentCapabilityID(ctx, "cap-order-parent")
	if err != nil {
		t.Fatalf("ListByParentCapabilityID: %v", err)
	}
	want := []string{"cap-a-child", "cap-m-child", "cap-z-child"}
	if len(got) != len(want) {
		t.Fatalf("want %d children, got %d", len(want), len(got))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("got[%d] = %q, want %q (full=%v)", i, got[i].ID, id, got)
		}
	}
}
