package inference

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/accept-io/midas/internal/store/postgres"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping postgres integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("db.PingContext: %v", err)
	}
	if err := postgres.EnsureSchema(db); err != nil {
		_ = db.Close()
		t.Fatalf("EnsureSchema: %v", err)
	}
	return db
}

func newTestService(t *testing.T, db *sql.DB) *Service {
	t.Helper()
	caps, err := postgres.NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}
	procs, err := postgres.NewProcessRepo(db)
	if err != nil {
		t.Fatalf("NewProcessRepo: %v", err)
	}
	surfs, err := postgres.NewSurfaceRepo(db)
	if err != nil {
		t.Fatalf("NewSurfaceRepo: %v", err)
	}
	return NewService(db, caps, procs, surfs)
}

// cleanupInferredChain removes a full inferred chain in FK-safe order.
func cleanupInferredChain(t *testing.T, db *sql.DB, surfaceID, capID, procID string) {
	t.Helper()
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, `DELETE FROM decision_surfaces WHERE id = $1`, surfaceID)
	_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = $1`, procID)
	_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
}

// ---------------------------------------------------------------------------
// 1. First creation
// ---------------------------------------------------------------------------

func TestEnsureInferredStructure_FirstCreation(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	svc := newTestService(t, db)

	t.Cleanup(func() {
		cleanupInferredChain(t, db, "loan.approve", "auto:loan", "auto:loan.approve")
	})

	result, err := svc.EnsureInferredStructure(ctx, "loan.approve")
	if err != nil {
		t.Fatalf("EnsureInferredStructure: %v", err)
	}

	if result.CapabilityID != "auto:loan" {
		t.Errorf("CapabilityID: want %q, got %q", "auto:loan", result.CapabilityID)
	}
	if result.ProcessID != "auto:loan.approve" {
		t.Errorf("ProcessID: want %q, got %q", "auto:loan.approve", result.ProcessID)
	}
	if result.SurfaceID != "loan.approve" {
		t.Errorf("SurfaceID: want %q, got %q", "loan.approve", result.SurfaceID)
	}
	if !result.CapabilityCreated {
		t.Error("CapabilityCreated: want true, got false")
	}
	if !result.ProcessCreated {
		t.Error("ProcessCreated: want true, got false")
	}
	if !result.SurfaceCreated {
		t.Error("SurfaceCreated: want true, got false")
	}

	// Verify rows exist in the DB.
	var count int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM capabilities WHERE capability_id = 'auto:loan' AND origin = 'inferred' AND managed = false`).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 inferred capability, got %d", count)
	}
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processes WHERE process_id = 'auto:loan.approve' AND origin = 'inferred' AND managed = false AND capability_id = 'auto:loan'`).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 inferred process, got %d", count)
	}
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM decision_surfaces WHERE id = 'loan.approve' AND version = 1 AND origin = 'inferred' AND managed = false AND process_id = 'auto:loan.approve'`).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 inferred surface, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// 2. Idempotent second call
// ---------------------------------------------------------------------------

func TestEnsureInferredStructure_Idempotent(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	svc := newTestService(t, db)

	t.Cleanup(func() {
		cleanupInferredChain(t, db, "idempotent.check", "auto:idempotent", "auto:idempotent.check")
	})

	// First call — creates the chain.
	if _, err := svc.EnsureInferredStructure(ctx, "idempotent.check"); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Second call — must succeed without creating duplicates.
	result, err := svc.EnsureInferredStructure(ctx, "idempotent.check")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if result.CapabilityID != "auto:idempotent" {
		t.Errorf("CapabilityID: want %q, got %q", "auto:idempotent", result.CapabilityID)
	}
	if result.ProcessID != "auto:idempotent.check" {
		t.Errorf("ProcessID: want %q, got %q", "auto:idempotent.check", result.ProcessID)
	}
	if result.CapabilityCreated {
		t.Error("CapabilityCreated: want false on second call, got true")
	}
	if result.ProcessCreated {
		t.Error("ProcessCreated: want false on second call, got true")
	}
	if result.SurfaceCreated {
		t.Error("SurfaceCreated: want false on second call, got true")
	}

	// Verify no duplicates.
	var count int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM capabilities WHERE capability_id = 'auto:idempotent'`).Scan(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 capability row, got %d", count)
	}
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processes WHERE process_id = 'auto:idempotent.check'`).Scan(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 process row, got %d", count)
	}
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM decision_surfaces WHERE id = 'idempotent.check'`).Scan(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 surface row, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// 3. No-dot fallback (capability = auto:general)
// ---------------------------------------------------------------------------

func TestEnsureInferredStructure_NoDotFallback(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	svc := newTestService(t, db)

	t.Cleanup(func() {
		cleanupInferredChain(t, db, "nodot-svc-test", "auto:general", "auto:nodot-svc-test")
	})

	result, err := svc.EnsureInferredStructure(ctx, "nodot-svc-test")
	if err != nil {
		t.Fatalf("EnsureInferredStructure: %v", err)
	}
	if result.CapabilityID != "auto:general" {
		t.Errorf("CapabilityID: want %q, got %q", "auto:general", result.CapabilityID)
	}
	if result.ProcessID != "auto:nodot-svc-test" {
		t.Errorf("ProcessID: want %q, got %q", "auto:nodot-svc-test", result.ProcessID)
	}
	if result.SurfaceID != "nodot-svc-test" {
		t.Errorf("SurfaceID: want %q, got %q", "nodot-svc-test", result.SurfaceID)
	}
}

// ---------------------------------------------------------------------------
// 4. Invalid surface ID — no DB writes
// ---------------------------------------------------------------------------

func TestEnsureInferredStructure_InvalidSurfaceID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	svc := newTestService(t, db)

	cases := []string{
		"Loan.Approve",  // uppercase
		"loan approve",  // space
		".leadingdot",   // leading dot
		"",              // empty
	}

	for _, id := range cases {
		t.Run(id, func(t *testing.T) {
			_, err := svc.EnsureInferredStructure(ctx, id)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", id)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 5a. Partial existing chain — capability already exists
// ---------------------------------------------------------------------------

func TestEnsureInferredStructure_PartialExisting_CapabilityOnly(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	svc := newTestService(t, db)

	// Pre-create the inferred capability.
	_, err := db.ExecContext(ctx, `
		INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		VALUES ('auto:partial-a', 'auto:partial-a', 'active', 'inferred', false, $1, $1)
		ON CONFLICT (capability_id) DO NOTHING
	`, time.Now().UTC())
	if err != nil {
		t.Fatalf("pre-insert capability: %v", err)
	}

	t.Cleanup(func() {
		cleanupInferredChain(t, db, "partial-a.test", "auto:partial-a", "auto:partial-a.test")
	})

	result, err := svc.EnsureInferredStructure(ctx, "partial-a.test")
	if err != nil {
		t.Fatalf("EnsureInferredStructure: %v", err)
	}
	if result.CapabilityCreated {
		t.Error("CapabilityCreated: want false (pre-existing), got true")
	}
	if !result.ProcessCreated {
		t.Error("ProcessCreated: want true (new), got false")
	}
	if !result.SurfaceCreated {
		t.Error("SurfaceCreated: want true (new), got false")
	}
}

// ---------------------------------------------------------------------------
// 5b. Partial existing chain — capability + process already exist
// ---------------------------------------------------------------------------

func TestEnsureInferredStructure_PartialExisting_CapabilityAndProcess(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	svc := newTestService(t, db)

	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		VALUES ('auto:partial-b', 'auto:partial-b', 'active', 'inferred', false, $1, $1)
		ON CONFLICT (capability_id) DO NOTHING
	`, now)
	if err != nil {
		t.Fatalf("pre-insert capability: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO processes (process_id, capability_id, name, status, origin, managed, created_at, updated_at, level)
		VALUES ('auto:partial-b.test', 'auto:partial-b', 'auto:partial-b.test', 'active', 'inferred', false, $1, $1, NULL)
		ON CONFLICT (process_id) DO NOTHING
	`, now)
	if err != nil {
		t.Fatalf("pre-insert process: %v", err)
	}

	t.Cleanup(func() {
		cleanupInferredChain(t, db, "partial-b.test", "auto:partial-b", "auto:partial-b.test")
	})

	result, err := svc.EnsureInferredStructure(ctx, "partial-b.test")
	if err != nil {
		t.Fatalf("EnsureInferredStructure: %v", err)
	}
	if result.CapabilityCreated {
		t.Error("CapabilityCreated: want false (pre-existing), got true")
	}
	if result.ProcessCreated {
		t.Error("ProcessCreated: want false (pre-existing), got true")
	}
	if !result.SurfaceCreated {
		t.Error("SurfaceCreated: want true (new), got false")
	}
}

// ---------------------------------------------------------------------------
// 6. Reserved namespace collision — incompatible existing row
// ---------------------------------------------------------------------------

func TestEnsureInferredStructure_Collision_ManualCapability(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	svc := newTestService(t, db)

	// Pre-create a manual capability in the auto: namespace — forbidden semantics.
	_, err := db.ExecContext(ctx, `
		INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		VALUES ('auto:collision-a', 'auto:collision-a', 'active', 'manual', true, $1, $1)
		ON CONFLICT (capability_id) DO NOTHING
	`, time.Now().UTC())
	if err != nil {
		t.Fatalf("pre-insert conflicting capability: %v", err)
	}

	t.Cleanup(func() {
		ctx2 := context.Background()
		_, _ = db.ExecContext(ctx2, `DELETE FROM decision_surfaces WHERE id = 'collision-a.test'`)
		_, _ = db.ExecContext(ctx2, `DELETE FROM processes WHERE process_id = 'auto:collision-a.test'`)
		_, _ = db.ExecContext(ctx2, `DELETE FROM capabilities WHERE capability_id = 'auto:collision-a'`)
	})

	_, err = svc.EnsureInferredStructure(ctx, "collision-a.test")
	if err == nil {
		t.Fatal("expected error due to conflicting capability, got nil")
	}

	// No process or surface must have been committed.
	var count int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processes WHERE process_id = 'auto:collision-a.test'`).Scan(&count)
	if count != 0 {
		t.Errorf("expected no process row to be committed, got %d", count)
	}
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM decision_surfaces WHERE id = 'collision-a.test'`).Scan(&count)
	if count != 0 {
		t.Errorf("expected no surface row to be committed, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// 7. Transaction rollback on inner failure
// ---------------------------------------------------------------------------

func TestEnsureInferredStructure_RollbackOnSurfaceConflict(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	svc := newTestService(t, db)

	// Pre-create prerequisite rows for the conflicting surface.
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		VALUES ('prereq-cap-rbtest', 'prereq-cap-rbtest', 'active', 'manual', true, $1, $1)
		ON CONFLICT (capability_id) DO NOTHING
	`, now)
	if err != nil {
		t.Fatalf("pre-insert prereq capability: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO processes (process_id, capability_id, name, status, origin, managed, created_at, updated_at, level)
		VALUES ('prereq-proc-rbtest', 'prereq-cap-rbtest', 'prereq-proc-rbtest', 'active', 'manual', true, $1, $1, NULL)
		ON CONFLICT (process_id) DO NOTHING
	`, now)
	if err != nil {
		t.Fatalf("pre-insert prereq process: %v", err)
	}
	// Pre-create surface (rollback-rb.test, 1) with wrong process linkage.
	_, err = db.ExecContext(ctx, `
		INSERT INTO decision_surfaces
			(id, version, name, domain, business_owner, technical_owner, status,
			 effective_date, created_at, updated_at, origin, managed, process_id)
		VALUES ('rollback-rb.test', 1, 'rollback-rb.test', 'inferred', '', '', 'active',
		        $1, $1, $1, 'inferred', false, 'prereq-proc-rbtest')
		ON CONFLICT (id, version) DO NOTHING
	`, now)
	if err != nil {
		t.Fatalf("pre-insert conflicting surface: %v", err)
	}

	t.Cleanup(func() {
		ctx2 := context.Background()
		_, _ = db.ExecContext(ctx2, `DELETE FROM decision_surfaces WHERE id = 'rollback-rb.test'`)
		_, _ = db.ExecContext(ctx2, `DELETE FROM processes WHERE process_id IN ('prereq-proc-rbtest', 'auto:rollback-rb.test')`)
		_, _ = db.ExecContext(ctx2, `DELETE FROM capabilities WHERE capability_id IN ('prereq-cap-rbtest', 'auto:rollback-rb')`)
	})

	_, err = svc.EnsureInferredStructure(ctx, "rollback-rb.test")
	if err == nil {
		t.Fatal("expected error due to conflicting surface, got nil")
	}

	// The capability and process created within the failed transaction must be gone.
	var count int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM capabilities WHERE capability_id = 'auto:rollback-rb'`).Scan(&count)
	if count != 0 {
		t.Errorf("rollback: expected auto:rollback-rb capability to be rolled back, got %d row(s)", count)
	}
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processes WHERE process_id = 'auto:rollback-rb.test'`).Scan(&count)
	if count != 0 {
		t.Errorf("rollback: expected auto:rollback-rb.test process to be rolled back, got %d row(s)", count)
	}
	// The pre-existing conflicting surface must still be there.
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM decision_surfaces WHERE id = 'rollback-rb.test' AND version = 1`).Scan(&count)
	if count != 1 {
		t.Errorf("rollback: expected pre-existing surface to still exist, got %d row(s)", count)
	}
}

// ---------------------------------------------------------------------------
// 8. Concurrency — no duplicates under concurrent calls
// ---------------------------------------------------------------------------

func TestEnsureInferredStructure_Concurrent(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	svc := newTestService(t, db)

	t.Cleanup(func() {
		cleanupInferredChain(t, db, "concurrent.approve", "auto:concurrent", "auto:concurrent.approve")
	})

	const numGoroutines = 10
	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines)
	resCh := make(chan InferenceResult, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := svc.EnsureInferredStructure(ctx, "concurrent.approve")
			if err != nil {
				errCh <- err
				return
			}
			resCh <- r
		}()
	}
	wg.Wait()
	close(errCh)
	close(resCh)

	for err := range errCh {
		t.Errorf("concurrent call failed: %v", err)
	}

	var results []InferenceResult
	for r := range resCh {
		results = append(results, r)
	}
	if len(results) == 0 {
		t.Fatal("no successful results")
	}

	// All results must carry the same IDs.
	for _, r := range results {
		if r.CapabilityID != "auto:concurrent" {
			t.Errorf("CapabilityID: want %q, got %q", "auto:concurrent", r.CapabilityID)
		}
		if r.ProcessID != "auto:concurrent.approve" {
			t.Errorf("ProcessID: want %q, got %q", "auto:concurrent.approve", r.ProcessID)
		}
		if r.SurfaceID != "concurrent.approve" {
			t.Errorf("SurfaceID: want %q, got %q", "concurrent.approve", r.SurfaceID)
		}
	}

	// Exactly one row must exist for each entity.
	var count int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM capabilities WHERE capability_id = 'auto:concurrent'`).Scan(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 capability row, got %d", count)
	}
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processes WHERE process_id = 'auto:concurrent.approve'`).Scan(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 process row, got %d", count)
	}
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM decision_surfaces WHERE id = 'concurrent.approve'`).Scan(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 surface row, got %d", count)
	}

	// At least one call must have reported creation.
	var anyCreated bool
	for _, r := range results {
		if r.CapabilityCreated || r.ProcessCreated || r.SurfaceCreated {
			anyCreated = true
			break
		}
	}
	if !anyCreated {
		t.Error("expected at least one call to report creation of some entity")
	}
}
