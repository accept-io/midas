package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/capability"
)

func TestCapabilityRepo_CreateAndGetByID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	c := &capability.Capability{
		ID:        "tst-cap-001",
		Name:      "Identity Verification",
		Status:    "active",
		Origin:    "manual",
		Managed:   true,
		Replaces:  "",
		Owner:     "team-platform",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, c.ID)
	})

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
		{"Replaces", c.Replaces, got.Replaces},
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
	if !got.CreatedAt.Equal(c.CreatedAt) {
		t.Errorf("CreatedAt: want %v, got %v", c.CreatedAt, got.CreatedAt)
	}
	if !got.UpdatedAt.Equal(c.UpdatedAt) {
		t.Errorf("UpdatedAt: want %v, got %v", c.UpdatedAt, got.UpdatedAt)
	}
}

func TestCapabilityRepo_GetByID_NotFound(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	got, err := repo.GetByID(ctx, "tst-cap-nonexistent")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestCapabilityRepo_Update_DoesNotMutateLifecycleFields(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	c := &capability.Capability{
		ID:        "tst-cap-upd-001",
		Name:      "Original Name",
		Status:    "active",
		Origin:    "manual",
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, c.ID)
	})

	updated := now.Add(time.Second)
	c.Name = "Updated Name"
	c.Status = "deprecated"
	c.Description = "now has a description"
	c.Owner = "team-new"
	c.UpdatedAt = updated

	if err := repo.Update(ctx, c); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Name != "Updated Name" {
		t.Errorf("Name: want %q, got %q", "Updated Name", got.Name)
	}
	if got.Status != "deprecated" {
		t.Errorf("Status: want deprecated, got %s", got.Status)
	}
	// origin and managed are immutable via Update
	if got.Origin != "manual" {
		t.Errorf("Origin: want manual, got %s", got.Origin)
	}
	if !got.Managed {
		t.Error("Managed: want true, got false")
	}
}

func TestCapabilityRepo_List(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	ids := []string{"tst-cap-list-001", "tst-cap-list-002"}
	for i, id := range ids {
		c := &capability.Capability{
			ID:        id,
			Name:      fmt.Sprintf("Capability %d", i+1),
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
	t.Cleanup(func() {
		for _, id := range ids {
			_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, id)
		}
	})

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
		t.Errorf("List: want %d test capabilities in result, got %d", len(ids), found)
	}
}
