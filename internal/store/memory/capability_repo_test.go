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
