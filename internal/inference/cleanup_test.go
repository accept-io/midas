package inference

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/store/postgres"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// ---------------------------------------------------------------------------
// Stub helpers for CleanupService unit/rollback tests
// ---------------------------------------------------------------------------

// stubErrCapCleanup is a capabilityCleanup that always returns an error from Find.
type stubErrCapCleanup struct{ err error }

func (s *stubErrCapCleanup) FindEligibleForCleanup(_ context.Context, _ sqltx.DBTX, _ time.Time) ([]string, error) {
	return nil, s.err
}

func (s *stubErrCapCleanup) DeleteByIDs(_ context.Context, _ sqltx.DBTX, _ []string) error {
	return nil
}

// ---------------------------------------------------------------------------
// Test helper
// ---------------------------------------------------------------------------

func newTestCleanupServiceFull(t *testing.T) (*CleanupService, func() error) {
	t.Helper()
	db := openTestDB(t)
	caps, err := postgres.NewCapabilityRepo(db)
	if err != nil {
		db.Close()
		t.Fatalf("NewCapabilityRepo: %v", err)
	}
	procs, err := postgres.NewProcessRepo(db)
	if err != nil {
		db.Close()
		t.Fatalf("NewProcessRepo: %v", err)
	}
	svc := NewCleanupService(db, procs, caps)
	return svc, db.Close
}

// ---------------------------------------------------------------------------
// 1. Rollback: capability find error rolls back process deletions
// ---------------------------------------------------------------------------

// TestCleanup_RollsbackOnCapabilityError verifies that when the capability
// cleanup step fails, the entire transaction is rolled back and no processes
// are deleted.
func TestCleanup_RollsbackOnCapabilityError(t *testing.T) {
	db := openTestDB(t)
	// Register close first so it runs last (t.Cleanup is LIFO). The data
	// deletion cleanup registered below must run before the connection is closed.
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	const (
		capID  = "cleanup-rb:cap"
		procID = "cleanup-rb.proc"
	)

	// Seed a deprecated inferred capability and process.
	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'deprecated', 'inferred', false, $2, $2)
		 ON CONFLICT DO NOTHING`,
		capID, now,
	); err != nil {
		t.Fatalf("seed capability: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO processes (process_id, capability_id, name, status, origin, managed, created_at, updated_at, level)
		 VALUES ($1, $2, $1, 'deprecated', 'inferred', false, $3, $3, NULL)
		 ON CONFLICT DO NOTHING`,
		procID, capID, now,
	); err != nil {
		t.Fatalf("seed process: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = $1`, procID)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	// Build cleanup service with real proc repo but stub cap that errors.
	procs, err := postgres.NewProcessRepo(db)
	if err != nil {
		t.Fatalf("NewProcessRepo: %v", err)
	}
	stubCaps := &stubErrCapCleanup{err: fmt.Errorf("simulated capability lookup failure")}
	svc := NewCleanupService(db, procs, stubCaps)

	_, err = svc.CleanupInferredEntities(ctx, time.Time{})
	if err == nil {
		t.Fatal("want error, got nil")
	}

	// Verify the process was NOT deleted (rollback succeeded).
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processes WHERE process_id = $1`, procID).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("want process to still exist after rollback, got count=%d", count)
	}
}

// ---------------------------------------------------------------------------
// 2. Nothing eligible → empty result
// ---------------------------------------------------------------------------

// TestCleanup_Integration_NothingEligible verifies that cleanup returns empty
// slices when there are no deprecated inferred entities.
func TestCleanup_Integration_NothingEligible(t *testing.T) {
	svc, closeDB := newTestCleanupServiceFull(t)
	defer closeDB()
	ctx := context.Background()

	result, err := svc.CleanupInferredEntities(ctx, time.Time{})
	if err != nil {
		t.Fatalf("CleanupInferredEntities: %v", err)
	}

	if len(result.ProcessesDeleted) != 0 {
		t.Errorf("want 0 processes deleted, got %d: %v", len(result.ProcessesDeleted), result.ProcessesDeleted)
	}
	if len(result.CapabilitiesDeleted) != 0 {
		t.Errorf("want 0 capabilities deleted, got %d: %v", len(result.CapabilitiesDeleted), result.CapabilitiesDeleted)
	}
}

// ---------------------------------------------------------------------------
// 3. Eligible deprecated process (no surfaces) is deleted
// ---------------------------------------------------------------------------

// TestCleanup_Integration_DeletesEligibleProcess verifies that a deprecated
// inferred process with no surface references is deleted.
func TestCleanup_Integration_DeletesEligibleProcess(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const (
		capID  = "cleanup-ep:cap"
		procID = "cleanup-ep.proc"
	)

	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'active', 'inferred', false, $2, $2) ON CONFLICT DO NOTHING`,
		capID, now,
	); err != nil {
		t.Fatalf("seed cap: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO processes (process_id, capability_id, name, status, origin, managed, created_at, updated_at, level)
		 VALUES ($1, $2, $1, 'deprecated', 'inferred', false, $3, $3, NULL) ON CONFLICT DO NOTHING`,
		procID, capID, now,
	); err != nil {
		t.Fatalf("seed proc: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = $1`, procID)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	caps, _ := postgres.NewCapabilityRepo(db)
	procs, _ := postgres.NewProcessRepo(db)
	svc := NewCleanupService(db, procs, caps)

	result, err := svc.CleanupInferredEntities(ctx, time.Time{})
	if err != nil {
		t.Fatalf("CleanupInferredEntities: %v", err)
	}

	if len(result.ProcessesDeleted) != 1 || result.ProcessesDeleted[0] != procID {
		t.Errorf("want [%s] processes deleted, got %v", procID, result.ProcessesDeleted)
	}
	if len(result.CapabilitiesDeleted) != 0 {
		t.Errorf("want 0 capabilities deleted, got %v", result.CapabilitiesDeleted)
	}

	// Verify row is gone.
	var count int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processes WHERE process_id = $1`, procID).Scan(&count)
	if count != 0 {
		t.Error("want process deleted, but row still exists")
	}
}

// ---------------------------------------------------------------------------
// 4. Eligible deprecated capability (no process refs) is deleted
// ---------------------------------------------------------------------------

// TestCleanup_Integration_DeletesEligibleCapability verifies that a deprecated
// inferred capability with no process references is deleted.
func TestCleanup_Integration_DeletesEligibleCapability(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const capID = "cleanup-ec:cap"

	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'deprecated', 'inferred', false, $2, $2) ON CONFLICT DO NOTHING`,
		capID, now,
	); err != nil {
		t.Fatalf("seed cap: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	caps, _ := postgres.NewCapabilityRepo(db)
	procs, _ := postgres.NewProcessRepo(db)
	svc := NewCleanupService(db, procs, caps)

	result, err := svc.CleanupInferredEntities(ctx, time.Time{})
	if err != nil {
		t.Fatalf("CleanupInferredEntities: %v", err)
	}

	if len(result.CapabilitiesDeleted) != 1 || result.CapabilitiesDeleted[0] != capID {
		t.Errorf("want [%s] capabilities deleted, got %v", capID, result.CapabilitiesDeleted)
	}

	var count int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM capabilities WHERE capability_id = $1`, capID).Scan(&count)
	if count != 0 {
		t.Error("want capability deleted, but row still exists")
	}
}

// ---------------------------------------------------------------------------
// 5. Process blocked by surface reference
// ---------------------------------------------------------------------------

// TestCleanup_Integration_ProcessBlockedBySurface verifies that a deprecated
// inferred process that is referenced by a decision surface is NOT deleted.
func TestCleanup_Integration_ProcessBlockedBySurface(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const surfaceID = "cleanup-bs.proc"

	// EnsureInferredStructure creates the full inferred chain with a surface.
	ensureSvc := newTestService(t, db)
	if _, err := ensureSvc.EnsureInferredStructure(ctx, surfaceID); err != nil {
		t.Fatalf("EnsureInferredStructure: %v", err)
	}

	const (
		capID  = "auto:cleanup-bs"
		procID = "auto:cleanup-bs.proc"
	)

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM decision_surfaces WHERE id = $1`, surfaceID)
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = $1`, procID)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	// Deprecate the process; the surface still references it.
	if _, err := db.ExecContext(ctx,
		`UPDATE processes SET status = 'deprecated' WHERE process_id = $1`, procID,
	); err != nil {
		t.Fatalf("deprecate process: %v", err)
	}

	caps, _ := postgres.NewCapabilityRepo(db)
	procs, _ := postgres.NewProcessRepo(db)
	svc := NewCleanupService(db, procs, caps)

	result, err := svc.CleanupInferredEntities(ctx, time.Time{})
	if err != nil {
		t.Fatalf("CleanupInferredEntities: %v", err)
	}

	for _, id := range result.ProcessesDeleted {
		if id == procID {
			t.Errorf("process %q should not be deleted while surface references it", procID)
		}
	}
}

// ---------------------------------------------------------------------------
// 6. Process blocked by replaces reference from another process
// ---------------------------------------------------------------------------

// TestCleanup_Integration_ProcessBlockedByReplaces verifies that a deprecated
// inferred process that is referenced by another process's replaces column is
// NOT deleted.
func TestCleanup_Integration_ProcessBlockedByReplaces(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const (
		capID      = "cleanup-br:cap"
		oldProcID  = "cleanup-br.old"
		newProcID  = "cleanup-br.new"
	)

	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'active', 'inferred', false, $2, $2) ON CONFLICT DO NOTHING`,
		capID, now,
	); err != nil {
		t.Fatalf("seed cap: %v", err)
	}
	// Old process: deprecated inferred.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO processes (process_id, capability_id, name, status, origin, managed, created_at, updated_at, level)
		 VALUES ($1, $2, $1, 'deprecated', 'inferred', false, $3, $3, NULL) ON CONFLICT DO NOTHING`,
		oldProcID, capID, now,
	); err != nil {
		t.Fatalf("seed old proc: %v", err)
	}
	// New process: manual, replaces old.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO processes (process_id, capability_id, name, status, origin, managed, replaces, created_at, updated_at, level)
		 VALUES ($1, $2, $1, 'active', 'manual', true, $3, $4, $4, NULL) ON CONFLICT DO NOTHING`,
		newProcID, capID, oldProcID, now,
	); err != nil {
		t.Fatalf("seed new proc: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id IN ($1, $2)`, oldProcID, newProcID)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	caps, _ := postgres.NewCapabilityRepo(db)
	procs, _ := postgres.NewProcessRepo(db)
	svc := NewCleanupService(db, procs, caps)

	result, err := svc.CleanupInferredEntities(ctx, time.Time{})
	if err != nil {
		t.Fatalf("CleanupInferredEntities: %v", err)
	}

	for _, id := range result.ProcessesDeleted {
		if id == oldProcID {
			t.Errorf("process %q should not be deleted while another process has replaces=%q", oldProcID, oldProcID)
		}
	}
}

// ---------------------------------------------------------------------------
// 7. Capability blocked by remaining active process
// ---------------------------------------------------------------------------

// TestCleanup_Integration_CapabilityBlockedByProcess verifies that a deprecated
// inferred capability that still has an active process referencing it is NOT
// deleted.
func TestCleanup_Integration_CapabilityBlockedByProcess(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const (
		capID  = "cleanup-cbp:cap"
		procID = "cleanup-cbp.proc"
	)

	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'deprecated', 'inferred', false, $2, $2) ON CONFLICT DO NOTHING`,
		capID, now,
	); err != nil {
		t.Fatalf("seed cap: %v", err)
	}
	// Active process still references this capability.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO processes (process_id, capability_id, name, status, origin, managed, created_at, updated_at, level)
		 VALUES ($1, $2, $1, 'active', 'inferred', false, $3, $3, NULL) ON CONFLICT DO NOTHING`,
		procID, capID, now,
	); err != nil {
		t.Fatalf("seed proc: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = $1`, procID)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	caps, _ := postgres.NewCapabilityRepo(db)
	procs, _ := postgres.NewProcessRepo(db)
	svc := NewCleanupService(db, procs, caps)

	result, err := svc.CleanupInferredEntities(ctx, time.Time{})
	if err != nil {
		t.Fatalf("CleanupInferredEntities: %v", err)
	}

	for _, id := range result.CapabilitiesDeleted {
		if id == capID {
			t.Errorf("capability %q should not be deleted while process %q references it", capID, procID)
		}
	}
}

// ---------------------------------------------------------------------------
// 8. Capability becomes eligible after its process is deleted in same tx
// ---------------------------------------------------------------------------

// TestCleanup_Integration_CapabilityEligibleAfterProcessDeletion verifies that
// a deprecated inferred capability whose only process reference is also a
// deprecated inferred process becomes eligible within the same transaction once
// the process is deleted first.
func TestCleanup_Integration_CapabilityEligibleAfterProcessDeletion(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const (
		capID  = "cleanup-cd:cap"
		procID = "cleanup-cd.proc"
	)

	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'deprecated', 'inferred', false, $2, $2) ON CONFLICT DO NOTHING`,
		capID, now,
	); err != nil {
		t.Fatalf("seed cap: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO processes (process_id, capability_id, name, status, origin, managed, created_at, updated_at, level)
		 VALUES ($1, $2, $1, 'deprecated', 'inferred', false, $3, $3, NULL) ON CONFLICT DO NOTHING`,
		procID, capID, now,
	); err != nil {
		t.Fatalf("seed proc: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = $1`, procID)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	caps, _ := postgres.NewCapabilityRepo(db)
	procs, _ := postgres.NewProcessRepo(db)
	svc := NewCleanupService(db, procs, caps)

	result, err := svc.CleanupInferredEntities(ctx, time.Time{})
	if err != nil {
		t.Fatalf("CleanupInferredEntities: %v", err)
	}

	if len(result.ProcessesDeleted) != 1 || result.ProcessesDeleted[0] != procID {
		t.Errorf("want process %q deleted, got %v", procID, result.ProcessesDeleted)
	}
	if len(result.CapabilitiesDeleted) != 1 || result.CapabilitiesDeleted[0] != capID {
		t.Errorf("want capability %q deleted, got %v", capID, result.CapabilitiesDeleted)
	}

	// Verify both rows are gone.
	var procCount, capCount int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processes WHERE process_id = $1`, procID).Scan(&procCount)
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM capabilities WHERE capability_id = $1`, capID).Scan(&capCount)
	if procCount != 0 {
		t.Error("want process deleted, but row still exists")
	}
	if capCount != 0 {
		t.Error("want capability deleted, but row still exists")
	}
}

// ---------------------------------------------------------------------------
// 9. Cutoff age filter: old entity deleted, young entity not
// ---------------------------------------------------------------------------

// TestCleanup_Integration_CutoffFiltersYoungEntities verifies that the cutoff
// age filter removes older eligible entities while leaving recently-deprecated
// ones intact.
func TestCleanup_Integration_CutoffFiltersYoungEntities(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const (
		capID     = "cleanup-cf:cap"
		oldProcID = "cleanup-cf.old"
		newProcID = "cleanup-cf.new"
	)

	oldTime := time.Now().UTC().Add(-10 * 24 * time.Hour) // 10 days ago
	newTime := time.Now().UTC()

	if _, err := db.ExecContext(ctx,
		`INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'active', 'inferred', false, $2, $2) ON CONFLICT DO NOTHING`,
		capID, newTime,
	); err != nil {
		t.Fatalf("seed cap: %v", err)
	}
	// Old deprecated process (10 days old).
	if _, err := db.ExecContext(ctx,
		`INSERT INTO processes (process_id, capability_id, name, status, origin, managed, created_at, updated_at, level)
		 VALUES ($1, $2, $1, 'deprecated', 'inferred', false, $3, $3, NULL) ON CONFLICT DO NOTHING`,
		oldProcID, capID, oldTime,
	); err != nil {
		t.Fatalf("seed old proc: %v", err)
	}
	// New deprecated process (just now).
	if _, err := db.ExecContext(ctx,
		`INSERT INTO processes (process_id, capability_id, name, status, origin, managed, created_at, updated_at, level)
		 VALUES ($1, $2, $1, 'deprecated', 'inferred', false, $3, $3, NULL) ON CONFLICT DO NOTHING`,
		newProcID, capID, newTime,
	); err != nil {
		t.Fatalf("seed new proc: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id IN ($1, $2)`, oldProcID, newProcID)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	caps, _ := postgres.NewCapabilityRepo(db)
	procs, _ := postgres.NewProcessRepo(db)
	svc := NewCleanupService(db, procs, caps)

	// cutoff = 7 days ago: old process (10d) is eligible, new process (0d) is not.
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
	result, err := svc.CleanupInferredEntities(ctx, cutoff)
	if err != nil {
		t.Fatalf("CleanupInferredEntities: %v", err)
	}

	deleted := make(map[string]bool, len(result.ProcessesDeleted))
	for _, id := range result.ProcessesDeleted {
		deleted[id] = true
	}

	if !deleted[oldProcID] {
		t.Errorf("want old process %q deleted, but it was not", oldProcID)
	}
	if deleted[newProcID] {
		t.Errorf("want new process %q retained (too young), but it was deleted", newProcID)
	}
}

// ---------------------------------------------------------------------------
// 10. Process blocked by parent_process_id reference
// ---------------------------------------------------------------------------

// TestCleanup_Integration_ProcessBlockedByParentProcessID verifies that a
// deprecated inferred process referenced by another process's parent_process_id
// is NOT deleted.
func TestCleanup_Integration_ProcessBlockedByParentProcessID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const (
		capID       = "cleanup-pp:cap"
		parentProcID = "cleanup-pp.parent"
		childProcID  = "cleanup-pp.child"
	)

	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'active', 'inferred', false, $2, $2) ON CONFLICT DO NOTHING`,
		capID, now,
	); err != nil {
		t.Fatalf("seed cap: %v", err)
	}
	// Parent process: deprecated inferred.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO processes (process_id, capability_id, name, status, origin, managed, created_at, updated_at, level)
		 VALUES ($1, $2, $1, 'deprecated', 'inferred', false, $3, $3, NULL) ON CONFLICT DO NOTHING`,
		parentProcID, capID, now,
	); err != nil {
		t.Fatalf("seed parent proc: %v", err)
	}
	// Child process references parent via parent_process_id.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO processes (process_id, capability_id, name, status, origin, managed, parent_process_id, created_at, updated_at, level)
		 VALUES ($1, $2, $1, 'active', 'inferred', false, $3, $4, $4, NULL) ON CONFLICT DO NOTHING`,
		childProcID, capID, parentProcID, now,
	); err != nil {
		t.Fatalf("seed child proc: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id IN ($1, $2)`, parentProcID, childProcID)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	caps, _ := postgres.NewCapabilityRepo(db)
	procs, _ := postgres.NewProcessRepo(db)
	svc := NewCleanupService(db, procs, caps)

	result, err := svc.CleanupInferredEntities(ctx, time.Time{})
	if err != nil {
		t.Fatalf("CleanupInferredEntities: %v", err)
	}

	for _, id := range result.ProcessesDeleted {
		if id == parentProcID {
			t.Errorf("parent process %q should not be deleted while child has parent_process_id=%q", parentProcID, parentProcID)
		}
	}
}

// ---------------------------------------------------------------------------
// 11. Result slices are never nil
// ---------------------------------------------------------------------------

// TestCleanup_Integration_ResultSlicesNeverNil verifies that CleanupResult
// always returns non-nil slices even when nothing is deleted.
func TestCleanup_Integration_ResultSlicesNeverNil(t *testing.T) {
	svc, closeDB := newTestCleanupServiceFull(t)
	defer closeDB()

	result, err := svc.CleanupInferredEntities(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("CleanupInferredEntities: %v", err)
	}

	if result.ProcessesDeleted == nil {
		t.Error("ProcessesDeleted must not be nil")
	}
	if result.CapabilitiesDeleted == nil {
		t.Error("CapabilitiesDeleted must not be nil")
	}
}
