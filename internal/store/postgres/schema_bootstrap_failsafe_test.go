package postgres

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

// schema_bootstrap_failsafe_test.go — D33a-impl-1 regression coverage for
// the Azure Container Apps activation failure on revision
// midas-api--0000034 (image bbc4b65b…). The container exited with:
//
//   lookup business service bs-consumer-lending: pq: column
//     "fail_mode_policy_id" does not exist at position 5:19 (42703)
//
// because schema.sql declared the column inside CREATE TABLE IF NOT
// EXISTS business_services (…) only — and the long-lived Azure
// Postgres database already had the table (from an earlier MIDAS
// deploy), so the CREATE TABLE re-run was a no-op and the new
// column was never added. D33a-impl-1 backs the CREATE-TABLE
// declarations with idempotent ALTER TABLE ADD COLUMN IF NOT EXISTS
// statements; the tests below reproduce the stale-schema scenario,
// run EnsureSchema, and verify the column is added in place plus
// that the seed-guard repository read no longer errors.

// TestPostgresSchemaBootstrap_AddsBusinessServiceFailModePolicyID
// reproduces the Azure activation failure: business_services exists
// from an earlier deploy but lacks fail_mode_policy_id. EnsureSchema
// must add the column idempotently so subsequent repository SELECTs
// referencing it succeed.
func TestPostgresSchemaBootstrap_AddsBusinessServiceFailModePolicyID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Simulate a stale schema: ensure the table exists (from openTestDB's
	// initial EnsureSchema), then drop the column so subsequent SELECTs
	// would fail with the exact Azure error.
	if _, err := db.ExecContext(ctx,
		`ALTER TABLE business_services DROP COLUMN IF EXISTS fail_mode_policy_id`); err != nil {
		t.Fatalf("simulate stale schema: %v", err)
	}
	// Sanity-check: the column really is gone before EnsureSchema runs.
	if columnExists(t, db, "business_services", "fail_mode_policy_id") {
		t.Fatal("stale-schema setup did not drop the column")
	}

	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema (failsafe) returned error: %v", err)
	}

	if !columnExists(t, db, "business_services", "fail_mode_policy_id") {
		t.Fatal("D33a-impl-1: EnsureSchema must add business_services.fail_mode_policy_id when missing — Azure activation failure regression")
	}
}

// TestPostgresSchemaBootstrap_AddsDecisionSurfaceFailModePolicyID
// covers the sibling regression on decision_surfaces — the same
// D27j-impl-2 column gap that would surface the next time the
// startup path performs a SELECT against decision_surfaces.
//
// Test setup uses DROP COLUMN ... CASCADE because the schema
// declares views that reference decision_surfaces.fail_mode_policy_id.
// Production stale schemas predate those views entirely (a database
// that never had the column also never had a view that referenced
// it), so the CASCADE-drop is a test-only construction. EnsureSchema
// re-creates the views via CREATE OR REPLACE VIEW so the database
// returns to a fully-reconciled state.
func TestPostgresSchemaBootstrap_AddsDecisionSurfaceFailModePolicyID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := db.ExecContext(ctx,
		`ALTER TABLE decision_surfaces DROP COLUMN IF EXISTS fail_mode_policy_id CASCADE`); err != nil {
		t.Fatalf("simulate stale schema: %v", err)
	}
	if columnExists(t, db, "decision_surfaces", "fail_mode_policy_id") {
		t.Fatal("stale-schema setup did not drop the column")
	}

	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}

	if !columnExists(t, db, "decision_surfaces", "fail_mode_policy_id") {
		t.Fatal("D33a-impl-1: EnsureSchema must add decision_surfaces.fail_mode_policy_id when missing (same regression class as the Azure failure)")
	}
}

// TestPostgresSchemaBootstrap_PrecedesDemoSeedGuard reproduces the
// exact Azure error path: a stale schema is repaired by EnsureSchema,
// then ensureBusinessService (the seed idempotency guard) performs a
// GetByID("bs-consumer-lending") that includes COALESCE(fail_mode_
// policy_id, '') at position 5:19 of the SELECT. Before D33a-impl-1
// the lookup failed with `pq: column "fail_mode_policy_id" does not
// exist at position 5:19 (42703)`; after D33a-impl-1 the failsafe
// adds the column and the lookup succeeds.
//
// This is the regression that would prevent Azure Container Apps
// revision midas-api--0000034 from failing activation.
func TestPostgresSchemaBootstrap_PrecedesDemoSeedGuard(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Reproduce the stale-schema condition.
	if _, err := db.ExecContext(ctx,
		`ALTER TABLE business_services DROP COLUMN IF EXISTS fail_mode_policy_id`); err != nil {
		t.Fatalf("simulate stale schema: %v", err)
	}

	// Run the failsafe — schema reconciliation MUST repair the column
	// before any repository read.
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}

	// Now perform the exact lookup that failed in Azure. This is the
	// repository SELECT executed by bootstrap.ensureBusinessService
	// against bs-consumer-lending.
	repo, err := NewBusinessServiceRepo(db)
	if err != nil {
		t.Fatalf("NewBusinessServiceRepo: %v", err)
	}
	got, err := repo.GetByID(ctx, "bs-consumer-lending")
	if err != nil {
		t.Fatalf("D33a-impl-1: GetByID(bs-consumer-lending) must succeed after EnsureSchema (Azure regression); got error %v", err)
	}
	// The row may or may not exist depending on whether SeedDemo has
	// run; the regression only requires the SELECT to succeed without
	// the 42703 column-does-not-exist error. A nil result is fine.
	_ = got
}

// TestPostgresSchemaBootstrap_RepositoryListWorksAfterReconciliation
// extends the seed-guard regression to the List code path that also
// SELECTs fail_mode_policy_id. The Explorer Services catalogue and
// several apply-path validators exercise this query, so a stale
// schema would have surfaced here too if startup got past seeding.
func TestPostgresSchemaBootstrap_RepositoryListWorksAfterReconciliation(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := db.ExecContext(ctx,
		`ALTER TABLE business_services DROP COLUMN IF EXISTS fail_mode_policy_id`); err != nil {
		t.Fatalf("simulate stale schema: %v", err)
	}
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}

	repo, err := NewBusinessServiceRepo(db)
	if err != nil {
		t.Fatalf("NewBusinessServiceRepo: %v", err)
	}
	if _, err := repo.List(ctx); err != nil {
		t.Fatalf("D33a-impl-1: List() must succeed after EnsureSchema; got %v", err)
	}
}

// TestPostgresSchemaBootstrap_Idempotent runs EnsureSchema twice and
// verifies no error. schema.sql is written with idempotent DDL
// (CREATE TABLE IF NOT EXISTS, ALTER TABLE ADD COLUMN IF NOT EXISTS,
// CREATE INDEX IF NOT EXISTS, CREATE OR REPLACE VIEW); repeated
// application must be a no-op.
func TestPostgresSchemaBootstrap_Idempotent(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// First application happened inside openTestDB; the second one is
	// the regression target.
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema (second pass) returned error: %v", err)
	}
	// Third pass — defensive: confirms the failsafe ALTER statements
	// remain no-ops when columns already exist.
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema (third pass) returned error: %v", err)
	}
}

// TestPostgresSchemaBootstrap_AddsBusinessServiceFailModePolicyIDIsRepeatable
// is the explicit idempotency pin for the new ALTER statements added
// in D33a-impl-1. After the first repair the column exists; the
// second repair must NOT error (ALTER TABLE ADD COLUMN IF NOT EXISTS
// is the SQL contract).
func TestPostgresSchemaBootstrap_AddsBusinessServiceFailModePolicyIDIsRepeatable(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := db.ExecContext(ctx,
		`ALTER TABLE business_services DROP COLUMN IF EXISTS fail_mode_policy_id`); err != nil {
		t.Fatalf("simulate stale schema: %v", err)
	}

	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema (first pass after drop) returned error: %v", err)
	}
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema (second pass, column already restored) returned error: %v", err)
	}

	// And the repository SELECT still works.
	repo, err := NewBusinessServiceRepo(db)
	if err != nil {
		t.Fatalf("NewBusinessServiceRepo: %v", err)
	}
	if _, err := repo.List(ctx); err != nil {
		t.Fatalf("BusinessServices.List() after repeated EnsureSchema: %v", err)
	}
}

// TestPostgresSchemaBootstrap_DDLDeclaresFailModePolicyIDAlters is a
// static pin on schema.sql so a future tranche that "consolidates"
// the additive ALTER section into the CREATE TABLE block alone
// (re-introducing the Azure regression class) fails loudly. The
// CREATE TABLE block must remain authoritative for fresh databases;
// the ALTER TABLE block must remain the failsafe for stale ones.
func TestPostgresSchemaBootstrap_DDLDeclaresFailModePolicyIDAlters(t *testing.T) {
	if !strings.Contains(schemaSQL, `ALTER TABLE business_services  ADD COLUMN IF NOT EXISTS fail_mode_policy_id TEXT;`) {
		t.Error("D33a-impl-1: schema.sql must declare an additive ALTER TABLE for business_services.fail_mode_policy_id (Azure regression failsafe)")
	}
	if !strings.Contains(schemaSQL, `ALTER TABLE decision_surfaces  ADD COLUMN IF NOT EXISTS fail_mode_policy_id TEXT;`) {
		t.Error("D33a-impl-1: schema.sql must declare an additive ALTER TABLE for decision_surfaces.fail_mode_policy_id")
	}
}

// columnExists queries information_schema.columns for the named
// column on the named table. Used by the failsafe regression tests
// to assert the post-EnsureSchema state.
func columnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const q = `
		SELECT 1
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = $1
		  AND column_name = $2`
	var one int
	err := db.QueryRowContext(ctx, q, table, column).Scan(&one)
	if err != nil {
		return false
	}
	return one == 1
}
