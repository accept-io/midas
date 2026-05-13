package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// schema_bootstrap_order_test.go — D33a-impl-1 source-level pins on
// the Postgres bootstrap ordering. The Azure Container Apps
// activation failure on revision midas-api--0000034 surfaced as
//
//   lookup business service bs-consumer-lending: pq: column
//     "fail_mode_policy_id" does not exist at position 5:19 (42703)
//
// during the bootstrap demo seed guard. The ordering invariant that
// these tests enforce is:
//
//   1. Postgres connection / pool tuning
//   2. EnsureSchema (idempotent schema reconciliation)
//   3. Repository construction
//   4. bootstrap.SeedDemo (seed idempotency guard performs repository
//      reads against the freshly-reconciled schema)
//
// Any tranche that re-orders these steps (e.g. moves SeedDemo before
// EnsureSchema, or constructs repositories before reconciling the
// schema) re-introduces the Azure failure class. The pins below run
// at unit-test speed (no Postgres required) so the regression class
// is caught at compile-checked source level.

// mainSource loads cmd/midas/main.go from the test working directory.
// The file path is the same one go test executes from, so the read
// is deterministic across local + docker-based runs.
func mainSource(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(".", "main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	return string(data)
}

// TestStartup_PostgresSchemaReconciledBeforeSeedDemo pins the
// runtime ordering invariant that broke in Azure: schema
// reconciliation must complete BEFORE the demo seed guard performs
// its first repository read. The structural pin is:
//
//   • buildRepositories(...) — which contains postgres.EnsureSchema —
//     is the call that builds and returns `repos`.
//   • main()'s call to buildRepositories(...) must appear BEFORE its
//     call to bootstrap.SeedDemo(...) so the seed guard sees the
//     reconciled schema.
//   • EnsureSchema must live inside the buildRepositories `case
//     "postgres":` block so it runs as part of repository
//     construction, not as a later parallel step.
//
// Together these pins reproduce the exact ordering Azure needs to
// activate cleanly.
func TestStartup_PostgresSchemaReconciledBeforeSeedDemo(t *testing.T) {
	src := mainSource(t)

	// 1. main() must call buildRepositories BEFORE bootstrap.SeedDemo.
	mainIdx := strings.Index(src, "\nfunc main()")
	if mainIdx < 0 {
		t.Fatal("D33a-impl-1: main() function missing from main.go")
	}
	// Bound by the next top-level `\nfunc ` declaration so the slice
	// captures only main()'s body.
	mainEnd := strings.Index(src[mainIdx+1:], "\nfunc ")
	if mainEnd <= 0 {
		mainEnd = len(src) - mainIdx - 1
	}
	mainBody := src[mainIdx : mainIdx+1+mainEnd]
	buildReposIdx := strings.Index(mainBody, "buildRepositories(")
	seedDemoIdx := strings.Index(mainBody, "bootstrap.SeedDemo(")
	if buildReposIdx < 0 {
		t.Fatal("D33a-impl-1: main() must invoke buildRepositories(...) during startup")
	}
	if seedDemoIdx < 0 {
		t.Fatal("D33a-impl-1: main() must invoke bootstrap.SeedDemo(...) during startup")
	}
	if !(buildReposIdx < seedDemoIdx) {
		t.Errorf("D33a-impl-1: main() must call buildRepositories (offset %d) BEFORE bootstrap.SeedDemo (offset %d) — re-ordering re-introduces the Azure activation failure (revision midas-api--0000034)",
			buildReposIdx, seedDemoIdx)
	}

	// 2. buildRepositories' `case "postgres":` block must call
	//    EnsureSchema. We bound the postgres-case slice by the next
	//    `case "memory":` opener.
	postgresCaseIdx := strings.Index(src, "case \"postgres\":")
	memoryCaseIdx := strings.Index(src, "case \"memory\":")
	if postgresCaseIdx < 0 || memoryCaseIdx < 0 || !(postgresCaseIdx < memoryCaseIdx) {
		t.Fatal("D33a-impl-1: buildRepositories must declare both `case \"postgres\":` and `case \"memory\":` (postgres before memory)")
	}
	postgresCase := src[postgresCaseIdx:memoryCaseIdx]
	if !strings.Contains(postgresCase, "postgres.EnsureSchema(db)") {
		t.Error("D33a-impl-1: buildRepositories' `case \"postgres\":` block must call postgres.EnsureSchema(db) — schema reconciliation is part of repository construction")
	}
}

// TestStartup_PostgresSchemaReconciledBeforeRepositoryConstruction
// pins that EnsureSchema runs before postgres.NewStore inside the
// buildRepositories `case "postgres":` block. NewStore triggers
// repository prepared-statement wiring; a SELECT prepared against a
// stale schema would fail with the same column-does-not-exist error
// class as the Azure activation failure.
func TestStartup_PostgresSchemaReconciledBeforeRepositoryConstruction(t *testing.T) {
	src := mainSource(t)
	postgresCaseIdx := strings.Index(src, "case \"postgres\":")
	memoryCaseIdx := strings.Index(src, "case \"memory\":")
	if postgresCaseIdx < 0 || memoryCaseIdx < 0 {
		t.Fatal("D33a-impl-1: buildRepositories must declare both case \"postgres\" and case \"memory\"")
	}
	postgresCase := src[postgresCaseIdx:memoryCaseIdx]
	ensureSchemaIdx := strings.Index(postgresCase, "postgres.EnsureSchema(db)")
	newStoreIdx := strings.Index(postgresCase, "postgres.NewStore(db,")
	if ensureSchemaIdx < 0 {
		t.Fatal("D33a-impl-1: postgres case must call postgres.EnsureSchema(db)")
	}
	if newStoreIdx < 0 {
		t.Fatal("D33a-impl-1: postgres case must call postgres.NewStore(db, ...)")
	}
	if !(ensureSchemaIdx < newStoreIdx) {
		t.Errorf("D33a-impl-1: EnsureSchema (offset %d) must precede NewStore (offset %d) inside the postgres case — repository construction must see the reconciled schema",
			ensureSchemaIdx, newStoreIdx)
	}
}

// TestStartup_PostgresSchemaBootstrapEmitsStructuredLogs pins the
// log-event chain operators rely on to triage an Azure / Cloud Run
// activation failure. The events are:
//
//   postgres_schema_bootstrap_start
//   postgres_schema_bootstrap_complete
//   postgres_schema_bootstrap_failed
//   postgres_schema_bootstrap_skipped
//
// The first three live on the Postgres path; the skipped event is on
// the memory-store path so the absence of bootstrap events on a
// memory deployment is explicit (absence-by-design vs absence-by-bug).
func TestStartup_PostgresSchemaBootstrapEmitsStructuredLogs(t *testing.T) {
	src := mainSource(t)
	for _, event := range []string{
		`"postgres_schema_bootstrap_start"`,
		`"postgres_schema_bootstrap_complete"`,
		`"postgres_schema_bootstrap_failed"`,
		`"postgres_schema_bootstrap_skipped"`,
	} {
		if !strings.Contains(src, event) {
			t.Errorf("D33a-impl-1: main.go must emit the %s structured log event", event)
		}
	}
	// The complete + failed events must carry a duration_ms field so
	// Azure log-analytics can chart bootstrap latency / failure
	// signatures. The start event documents the schema source.
	for _, want := range []string{
		`"duration_ms"`,
		`"schema_source"`,
		`"store_backend"`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("D33a-impl-1: main.go bootstrap log payload must include %s", want)
		}
	}
}

// TestStartup_MemoryStoreSkipsPostgresSchemaBootstrap pins that the
// memory-store branch does NOT invoke postgres.EnsureSchema. Test-
// only and ephemeral deployments need to start without a Postgres
// schema; the postgres_schema_bootstrap_skipped log makes the
// absence of bootstrap events explicit.
func TestStartup_MemoryStoreSkipsPostgresSchemaBootstrap(t *testing.T) {
	src := mainSource(t)
	memCaseIdx := strings.Index(src, "case \"memory\":")
	if memCaseIdx < 0 {
		t.Fatal("D33a-impl-1: memory-store branch of buildRepositories must exist")
	}
	// Bound the memory-branch slice by the next `case ` keyword in
	// the switch (the default case).
	end := strings.Index(src[memCaseIdx+1:], "\tcase ")
	if end <= 0 {
		// Fall back to the default branch.
		end = strings.Index(src[memCaseIdx:], "default:")
		if end <= 0 {
			t.Fatal("D33a-impl-1: cannot bound the memory case slice")
		}
	}
	memCase := src[memCaseIdx : memCaseIdx+1+end]

	// The memory case must NOT call EnsureSchema (no schema to apply).
	if strings.Contains(memCase, "postgres.EnsureSchema(") {
		t.Error("D33a-impl-1: memory-store branch must NOT invoke postgres.EnsureSchema (memory store has no schema)")
	}
	// And it MUST emit the postgres_schema_bootstrap_skipped event so
	// absence of bootstrap events is explicit in operator logs.
	if !strings.Contains(memCase, `"postgres_schema_bootstrap_skipped"`) {
		t.Error("D33a-impl-1: memory-store branch must emit postgres_schema_bootstrap_skipped (absence-by-design log signal)")
	}
}

// TestStartup_PostgresSchemaBootstrapErrorWraps pins that an
// EnsureSchema failure produces a clearly-prefixed error so the
// failing-component is unambiguous in Azure / Cloud Run logs. An
// unwrapped pq error would surface as "column does not exist at
// position 5:19" with no hint that schema bootstrap was the
// originating phase.
func TestStartup_PostgresSchemaBootstrapErrorWraps(t *testing.T) {
	src := mainSource(t)
	if !strings.Contains(src, `"postgres schema bootstrap failed: %w"`) {
		t.Error("D33a-impl-1: main.go must wrap EnsureSchema errors with the `postgres schema bootstrap failed:` prefix so the failing phase is unambiguous in startup logs")
	}
}
