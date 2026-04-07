package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/processbusinessservice"
)

func TestProcessBusinessServiceRepo_CreateAndListByProcessID(t *testing.T) {
	ctx := context.Background()
	repo := NewProcessBusinessServiceRepo()

	now := time.Now().UTC()
	pbs := &processbusinessservice.ProcessBusinessService{
		ProcessID:         "proc-001",
		BusinessServiceID: "bs-001",
		CreatedAt:         now,
	}

	if err := repo.Create(ctx, pbs); err != nil {
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
	if rows[0].BusinessServiceID != "bs-001" {
		t.Errorf("BusinessServiceID: want %q, got %q", "bs-001", rows[0].BusinessServiceID)
	}
	if !rows[0].CreatedAt.Equal(now) {
		t.Errorf("CreatedAt: want %v, got %v", now, rows[0].CreatedAt)
	}
}

func TestProcessBusinessServiceRepo_ListByBusinessServiceID(t *testing.T) {
	ctx := context.Background()
	repo := NewProcessBusinessServiceRepo()

	now := time.Now().UTC()
	links := []*processbusinessservice.ProcessBusinessService{
		{ProcessID: "proc-a", BusinessServiceID: "bs-shared", CreatedAt: now},
		{ProcessID: "proc-b", BusinessServiceID: "bs-shared", CreatedAt: now},
		{ProcessID: "proc-a", BusinessServiceID: "bs-other", CreatedAt: now},
	}
	for _, pbs := range links {
		if err := repo.Create(ctx, pbs); err != nil {
			t.Fatalf("Create %s↔%s: %v", pbs.ProcessID, pbs.BusinessServiceID, err)
		}
	}

	rows, err := repo.ListByBusinessServiceID(ctx, "bs-shared")
	if err != nil {
		t.Fatalf("ListByBusinessServiceID: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("ListByBusinessServiceID: want 2 rows for shared bs, got %d", len(rows))
	}

	rows, err = repo.ListByBusinessServiceID(ctx, "bs-other")
	if err != nil {
		t.Fatalf("ListByBusinessServiceID bs-other: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListByBusinessServiceID bs-other: want 1 row, got %d", len(rows))
	}
}

func TestProcessBusinessServiceRepo_ListEmpty(t *testing.T) {
	ctx := context.Background()
	repo := NewProcessBusinessServiceRepo()

	rows, err := repo.ListByProcessID(ctx, "proc-nonexistent")
	if err != nil {
		t.Fatalf("ListByProcessID: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rows))
	}

	rows, err = repo.ListByBusinessServiceID(ctx, "bs-nonexistent")
	if err != nil {
		t.Fatalf("ListByBusinessServiceID: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rows))
	}
}

func TestProcessBusinessServiceRepo_Delete(t *testing.T) {
	ctx := context.Background()
	repo := NewProcessBusinessServiceRepo()

	now := time.Now().UTC()
	pbs := &processbusinessservice.ProcessBusinessService{
		ProcessID:         "proc-del",
		BusinessServiceID: "bs-del",
		CreatedAt:         now,
	}
	if err := repo.Create(ctx, pbs); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, "proc-del", "bs-del"); err != nil {
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

func TestProcessBusinessServiceRepo_Delete_OnlyRemovesMatchingLink(t *testing.T) {
	ctx := context.Background()
	repo := NewProcessBusinessServiceRepo()

	now := time.Now().UTC()
	links := []*processbusinessservice.ProcessBusinessService{
		{ProcessID: "proc-x", BusinessServiceID: "bs-1", CreatedAt: now},
		{ProcessID: "proc-x", BusinessServiceID: "bs-2", CreatedAt: now},
	}
	for _, pbs := range links {
		if err := repo.Create(ctx, pbs); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	if err := repo.Delete(ctx, "proc-x", "bs-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	rows, err := repo.ListByProcessID(ctx, "proc-x")
	if err != nil {
		t.Fatalf("ListByProcessID: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 remaining link, got %d", len(rows))
	}
	if rows[0].BusinessServiceID != "bs-2" {
		t.Errorf("remaining link: want bs-2, got %s", rows[0].BusinessServiceID)
	}
}

func TestProcessBusinessServiceRepo_DuplicateCreate(t *testing.T) {
	ctx := context.Background()
	repo := NewProcessBusinessServiceRepo()

	now := time.Now().UTC()
	pbs := &processbusinessservice.ProcessBusinessService{
		ProcessID:         "proc-dup",
		BusinessServiceID: "bs-dup",
		CreatedAt:         now,
	}

	if err := repo.Create(ctx, pbs); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if err := repo.Create(ctx, pbs); err == nil {
		t.Error("second Create with duplicate composite key: want error, got nil")
	}
}
