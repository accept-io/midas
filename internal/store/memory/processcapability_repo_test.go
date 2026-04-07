package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/processcapability"
)

func TestProcessCapabilityRepo_CreateAndListByProcessID(t *testing.T) {
	ctx := context.Background()
	repo := NewProcessCapabilityRepo()

	now := time.Now().UTC()
	pc := &processcapability.ProcessCapability{
		ProcessID:    "proc-001",
		CapabilityID: "cap-001",
		CreatedAt:    now,
	}

	if err := repo.Create(ctx, pc); err != nil {
		t.Fatalf("Create: %v", err)
	}

	rows, err := repo.ListByProcessID(ctx, "proc-001")
	if err != nil {
		t.Fatalf("ListByProcessID: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListByProcessID: want 1 row, got %d", len(rows))
	}
	if rows[0].ProcessID != "proc-001" {
		t.Errorf("ProcessID: want %q, got %q", "proc-001", rows[0].ProcessID)
	}
	if rows[0].CapabilityID != "cap-001" {
		t.Errorf("CapabilityID: want %q, got %q", "cap-001", rows[0].CapabilityID)
	}
	if !rows[0].CreatedAt.Equal(now) {
		t.Errorf("CreatedAt: want %v, got %v", now, rows[0].CreatedAt)
	}
}

func TestProcessCapabilityRepo_ListByCapabilityID(t *testing.T) {
	ctx := context.Background()
	repo := NewProcessCapabilityRepo()

	now := time.Now().UTC()
	links := []*processcapability.ProcessCapability{
		{ProcessID: "proc-a", CapabilityID: "cap-shared", CreatedAt: now},
		{ProcessID: "proc-b", CapabilityID: "cap-shared", CreatedAt: now},
		{ProcessID: "proc-a", CapabilityID: "cap-other", CreatedAt: now},
	}
	for _, pc := range links {
		if err := repo.Create(ctx, pc); err != nil {
			t.Fatalf("Create %s↔%s: %v", pc.ProcessID, pc.CapabilityID, err)
		}
	}

	rows, err := repo.ListByCapabilityID(ctx, "cap-shared")
	if err != nil {
		t.Fatalf("ListByCapabilityID: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("ListByCapabilityID: want 2 rows for shared cap, got %d", len(rows))
	}

	rows, err = repo.ListByCapabilityID(ctx, "cap-other")
	if err != nil {
		t.Fatalf("ListByCapabilityID cap-other: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListByCapabilityID cap-other: want 1 row, got %d", len(rows))
	}
}

func TestProcessCapabilityRepo_ListEmpty(t *testing.T) {
	ctx := context.Background()
	repo := NewProcessCapabilityRepo()

	rows, err := repo.ListByProcessID(ctx, "proc-nonexistent")
	if err != nil {
		t.Fatalf("ListByProcessID: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rows))
	}

	rows, err = repo.ListByCapabilityID(ctx, "cap-nonexistent")
	if err != nil {
		t.Fatalf("ListByCapabilityID: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rows))
	}
}

func TestProcessCapabilityRepo_Delete(t *testing.T) {
	ctx := context.Background()
	repo := NewProcessCapabilityRepo()

	now := time.Now().UTC()
	pc := &processcapability.ProcessCapability{
		ProcessID:    "proc-del",
		CapabilityID: "cap-del",
		CreatedAt:    now,
	}
	if err := repo.Create(ctx, pc); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, "proc-del", "cap-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	rows, err := repo.ListByProcessID(ctx, "proc-del")
	if err != nil {
		t.Fatalf("ListByProcessID after delete: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows after delete, got %d", len(rows))
	}
}

func TestProcessCapabilityRepo_Delete_OnlyRemovesMatchingLink(t *testing.T) {
	ctx := context.Background()
	repo := NewProcessCapabilityRepo()

	now := time.Now().UTC()
	links := []*processcapability.ProcessCapability{
		{ProcessID: "proc-x", CapabilityID: "cap-1", CreatedAt: now},
		{ProcessID: "proc-x", CapabilityID: "cap-2", CreatedAt: now},
	}
	for _, pc := range links {
		if err := repo.Create(ctx, pc); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	if err := repo.Delete(ctx, "proc-x", "cap-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	rows, err := repo.ListByProcessID(ctx, "proc-x")
	if err != nil {
		t.Fatalf("ListByProcessID: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 remaining link, got %d", len(rows))
	}
	if rows[0].CapabilityID != "cap-2" {
		t.Errorf("remaining link: want cap-2, got %s", rows[0].CapabilityID)
	}
}

func TestProcessCapabilityRepo_DuplicateCreate(t *testing.T) {
	ctx := context.Background()
	repo := NewProcessCapabilityRepo()

	now := time.Now().UTC()
	pc := &processcapability.ProcessCapability{
		ProcessID:    "proc-dup",
		CapabilityID: "cap-dup",
		CreatedAt:    now,
	}

	if err := repo.Create(ctx, pc); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if err := repo.Create(ctx, pc); err == nil {
		t.Error("second Create with duplicate composite key: want error, got nil")
	}
}
