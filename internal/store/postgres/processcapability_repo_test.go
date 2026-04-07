package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/processcapability"
)

func TestProcessCapabilityRepo_CreateAndList(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	capID := "tst-cap-pc-001"
	procID := "tst-proc-pc-001"

	if _, err := db.ExecContext(ctx,
		`INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'active', 'manual', true, NOW(), NOW())
		 ON CONFLICT (capability_id) DO NOTHING`,
		capID,
	); err != nil {
		t.Fatalf("insert capability: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO processes (process_id, capability_id, name, status, origin, managed, created_at, updated_at, level)
		 VALUES ($1, $2, $1, 'active', 'manual', true, NOW(), NOW(), NULL)
		 ON CONFLICT (process_id) DO NOTHING`,
		procID, capID,
	); err != nil {
		t.Fatalf("insert process: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM process_capabilities WHERE process_id = $1`, procID)
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = $1`, procID)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	repo, err := NewProcessCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewProcessCapabilityRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	pc := &processcapability.ProcessCapability{
		ProcessID:    procID,
		CapabilityID: capID,
		CreatedAt:    now,
	}

	if err := repo.Create(ctx, pc); err != nil {
		t.Fatalf("Create: %v", err)
	}

	byProc, err := repo.ListByProcessID(ctx, procID)
	if err != nil {
		t.Fatalf("ListByProcessID: %v", err)
	}
	if len(byProc) != 1 {
		t.Fatalf("ListByProcessID: want 1 row, got %d", len(byProc))
	}
	if byProc[0].ProcessID != procID {
		t.Errorf("ProcessID: want %q, got %q", procID, byProc[0].ProcessID)
	}
	if byProc[0].CapabilityID != capID {
		t.Errorf("CapabilityID: want %q, got %q", capID, byProc[0].CapabilityID)
	}
	if !byProc[0].CreatedAt.Equal(now) {
		t.Errorf("CreatedAt: want %v, got %v", now, byProc[0].CreatedAt)
	}

	byCap, err := repo.ListByCapabilityID(ctx, capID)
	if err != nil {
		t.Fatalf("ListByCapabilityID: %v", err)
	}
	if len(byCap) != 1 {
		t.Fatalf("ListByCapabilityID: want 1 row, got %d", len(byCap))
	}
	if byCap[0].ProcessID != procID {
		t.Errorf("ProcessID: want %q, got %q", procID, byCap[0].ProcessID)
	}
}

func TestProcessCapabilityRepo_ListEmpty(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewProcessCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewProcessCapabilityRepo: %v", err)
	}

	rows, err := repo.ListByProcessID(ctx, "tst-proc-nonexistent")
	if err != nil {
		t.Fatalf("ListByProcessID: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rows))
	}

	rows, err = repo.ListByCapabilityID(ctx, "tst-cap-nonexistent")
	if err != nil {
		t.Fatalf("ListByCapabilityID: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rows))
	}
}

func TestProcessCapabilityRepo_Delete(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	capID := "tst-cap-pc-del-001"
	procID := "tst-proc-pc-del-001"

	if _, err := db.ExecContext(ctx,
		`INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'active', 'manual', true, NOW(), NOW())
		 ON CONFLICT (capability_id) DO NOTHING`,
		capID,
	); err != nil {
		t.Fatalf("insert capability: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO processes (process_id, capability_id, name, status, origin, managed, created_at, updated_at, level)
		 VALUES ($1, $2, $1, 'active', 'manual', true, NOW(), NOW(), NULL)
		 ON CONFLICT (process_id) DO NOTHING`,
		procID, capID,
	); err != nil {
		t.Fatalf("insert process: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM process_capabilities WHERE process_id = $1`, procID)
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = $1`, procID)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	repo, err := NewProcessCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewProcessCapabilityRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	pc := &processcapability.ProcessCapability{
		ProcessID:    procID,
		CapabilityID: capID,
		CreatedAt:    now,
	}
	if err := repo.Create(ctx, pc); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, procID, capID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	rows, err := repo.ListByProcessID(ctx, procID)
	if err != nil {
		t.Fatalf("ListByProcessID after delete: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows after delete, got %d", len(rows))
	}
}

func TestProcessCapabilityRepo_DuplicatePK(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	capID := "tst-cap-pc-dup-001"
	procID := "tst-proc-pc-dup-001"

	if _, err := db.ExecContext(ctx,
		`INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'active', 'manual', true, NOW(), NOW())
		 ON CONFLICT (capability_id) DO NOTHING`,
		capID,
	); err != nil {
		t.Fatalf("insert capability: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO processes (process_id, capability_id, name, status, origin, managed, created_at, updated_at, level)
		 VALUES ($1, $2, $1, 'active', 'manual', true, NOW(), NOW(), NULL)
		 ON CONFLICT (process_id) DO NOTHING`,
		procID, capID,
	); err != nil {
		t.Fatalf("insert process: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM process_capabilities WHERE process_id = $1`, procID)
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = $1`, procID)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	repo, err := NewProcessCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewProcessCapabilityRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	pc := &processcapability.ProcessCapability{
		ProcessID:    procID,
		CapabilityID: capID,
		CreatedAt:    now,
	}
	if err := repo.Create(ctx, pc); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if err := repo.Create(ctx, pc); err == nil {
		t.Error("second Create with duplicate PK: want error, got nil")
	}
}

func TestProcessCapabilityRepo_FKViolation(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewProcessCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewProcessCapabilityRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	pc := &processcapability.ProcessCapability{
		ProcessID:    "tst-proc-nonexistent",
		CapabilityID: "tst-cap-nonexistent",
		CreatedAt:    now,
	}
	if err := repo.Create(ctx, pc); err == nil {
		t.Error("Create with invalid FK: want error, got nil")
	}
}
