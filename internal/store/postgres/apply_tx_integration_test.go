package postgres

// Real-postgres verification of the atomic-apply contract (see Issue:
// Wire postgres WithTx into apply TxRunner). Skipped automatically when
// DATABASE_URL is not set.
//
// The contract under test: when a mid-bundle persistence error occurs
// inside a transactional control-plane apply, no earlier writes remain
// persisted in real Postgres. This complements the in-memory atomicity
// contract test — that one uses a fake TxRunner to prove the executor
// aborts and calls rollback; this one proves the rollback actually
// rolls Postgres rows back through NewApplyTxRunner → *Store.WithTx.
//
// Failure-injection design: the test wraps only the ProcessRepository
// in the scoped RepositorySet with a fail-on-id decorator. Every other
// repository is the real transaction-scoped Postgres repo returned by
// the adapter. This means the Capability.Create that runs before the
// failure lands in the real transaction, and the rollback that follows
// is real Postgres rollback. If real atomicity is broken, the
// Capability row will be visible via a post-apply SELECT against the
// base (non-transactional) connection.

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/store"
)

// failOnIDProcessRepo decorates the transaction-scoped ProcessRepository
// and returns an injected error on a specific Create(id). Every other
// method delegates through so plan-time lookups see the real tx state.
type failOnIDProcessRepo struct {
	inner   process.ProcessRepository
	failID  string
	failErr error
}

func (r *failOnIDProcessRepo) Exists(ctx context.Context, id string) (bool, error) {
	return r.inner.Exists(ctx, id)
}
func (r *failOnIDProcessRepo) GetByID(ctx context.Context, id string) (*process.Process, error) {
	return r.inner.GetByID(ctx, id)
}
func (r *failOnIDProcessRepo) Create(ctx context.Context, p *process.Process) error {
	if p.ID == r.failID {
		return r.failErr
	}
	return r.inner.Create(ctx, p)
}

// atomicityIntegrationTxRunner wraps *Store.WithTx the same way
// NewApplyTxRunner does, but injects the failing Process repo into the
// scoped RepositorySet. This replaces production's ApplyTxRunner ONLY
// for the purpose of driving the failure; the transaction path it takes
// is the same real Postgres transaction path.
type atomicityIntegrationTxRunner struct {
	store          *Store
	failProcessID  string
	failProcessErr error
}

func (r *atomicityIntegrationTxRunner) WithTx(
	ctx context.Context,
	operation string,
	fn func(*apply.RepositorySet) error,
) error {
	return r.store.WithTx(ctx, operation, func(repos *store.Repositories) error {
		return fn(&apply.RepositorySet{
			Surfaces:         repos.Surfaces,
			Agents:           repos.Agents,
			Profiles:         repos.Profiles,
			Grants:           repos.Grants,
			Processes:        &failOnIDProcessRepo{inner: repos.Processes, failID: r.failProcessID, failErr: r.failProcessErr},
			Capabilities:     repos.Capabilities,
			BusinessServices: repos.BusinessServices,
		})
	})
}

func atomicityIntegrationBundle() []parser.ParsedDocument {
	// In the v1 service-led model the bundle is BusinessService → Process → Surface.
	//
	// Execution order (see orderedEntries): BusinessService (0) → Process (2)
	// → Surface (3). The Process.Create is the operation the failure-injection
	// wrapper targets, so the rollback scenario covers the BusinessService
	// commit being undone and the Surface tier never running.
	return []parser.ParsedDocument{
		{
			Kind: types.KindBusinessService,
			ID:   "bs-atomic-int",
			Doc: types.BusinessServiceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindBusinessService,
				Metadata:   types.DocumentMetadata{ID: "bs-atomic-int", Name: "Atomic Integration Service"},
				Spec:       types.BusinessServiceSpec{ServiceType: "internal", Status: "active"},
			},
		},
		{
			Kind: types.KindProcess,
			ID:   "proc-atomic-int",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-atomic-int", Name: "Atomic Integration Process"},
				Spec:       types.ProcessSpec{BusinessServiceID: "bs-atomic-int", Status: "active"},
			},
		},
		{
			Kind: types.KindSurface,
			ID:   "surf-atomic-int",
			Doc: types.SurfaceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindSurface,
				Metadata:   types.DocumentMetadata{ID: "surf-atomic-int", Name: "Atomic Integration Surface"},
				Spec: types.SurfaceSpec{
					Category:  "integration",
					RiskTier:  "low",
					Status:    "active",
					ProcessID: "proc-atomic-int",
				},
			},
		},
	}
}

func cleanupAtomicityIntegrationRows(t *testing.T, db *sql.DB) {
	t.Helper()
	// Children before parents to keep FK constraints happy.
	for _, stmt := range []string{
		`DELETE FROM decision_surfaces WHERE id = 'surf-atomic-int'`,
		`DELETE FROM processes WHERE process_id = 'proc-atomic-int'`,
		`DELETE FROM business_services WHERE business_service_id = 'bs-atomic-int'`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("cleanup %q: %v", stmt, err)
		}
	}
}

// TestApplyAtomicity_PostgresRealRollback verifies real Postgres
// rollback. A three-document bundle (Capability → Process → Surface)
// is applied with Process.Create forced to fail. After the apply
// returns the Capability row must NOT be visible in the capabilities
// table — because the real Postgres transaction was rolled back.
func TestApplyAtomicity_PostgresRealRollback(t *testing.T) {
	db := openTestDB(t)
	// Register cleanup + Close as a single Cleanup callback so the
	// data-cleanup query runs against an OPEN connection. A plain
	// `defer db.Close()` on the test body would fire before the
	// testing framework invokes t.Cleanup callbacks, producing
	// `sql: database is closed` on the cleanup query.
	t.Cleanup(func() {
		cleanupAtomicityIntegrationRows(t, db)
		db.Close()
	})
	cleanupAtomicityIntegrationRows(t, db)

	s, err := NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}

	failErr := errors.New("simulated repository failure during Process.Create")
	txRunner := &atomicityIntegrationTxRunner{
		store:          s,
		failProcessID:  "proc-atomic-int",
		failProcessErr: failErr,
	}

	// Outer (non-transactional) repos drive plan-time lookups.
	// Tx-scoped repos drive execute-time writes.
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Surfaces:         repos.Surfaces,
		Processes:        repos.Processes,
		Capabilities:     repos.Capabilities,
		BusinessServices: repos.BusinessServices,
		Agents:           repos.Agents,
		Profiles:         repos.Profiles,
		Grants:           repos.Grants,
		ControlAudit:     repos.ControlAudit,
		Tx:               txRunner,
	})

	ctx := context.Background()
	result := svc.Apply(ctx, atomicityIntegrationBundle(), "integration-test")

	if result.ApplyErrorCount() == 0 {
		t.Fatalf("expected an apply error from the injected Process.Create failure; got result=%+v", result)
	}

	// Assertion: the BusinessService that was written earlier in the same
	// transaction must NOT remain in Postgres. A raw SELECT outside
	// the transaction probes the committed state directly.
	var bsCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM business_services WHERE business_service_id = $1`,
		"bs-atomic-int",
	).Scan(&bsCount); err != nil {
		t.Fatalf("SELECT business_services: %v", err)
	}
	if bsCount != 0 {
		t.Errorf("real-postgres atomicity violated: BusinessService %q remained after mid-bundle failure (rows=%d). "+
			"This means the transaction did not roll back as expected.", "bs-atomic-int", bsCount)
	}

	// Defensive: the Process and Surface must also be absent. Process
	// is absent because its Create was rejected; Surface is absent
	// because the executor aborts before reaching later plan entries.
	var procCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM processes WHERE process_id = $1`,
		"proc-atomic-int",
	).Scan(&procCount); err != nil {
		t.Fatalf("SELECT processes: %v", err)
	}
	if procCount != 0 {
		t.Errorf("Process row unexpectedly persisted (%d); injection fake should have prevented any write", procCount)
	}
	var surfCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM decision_surfaces WHERE id = $1`,
		"surf-atomic-int",
	).Scan(&surfCount); err != nil {
		t.Fatalf("SELECT decision_surfaces: %v", err)
	}
	if surfCount != 0 {
		t.Errorf("Surface row unexpectedly persisted (%d); executor must abort before attempting later entries", surfCount)
	}
}

// TestApplyAtomicity_PostgresPositiveControl is the happy-path companion:
// the same bundle shape, with no failure injection, must commit every
// row to Postgres. If this test fails, the harness above is broken and
// the atomicity assertion cannot be trusted.
func TestApplyAtomicity_PostgresPositiveControl(t *testing.T) {
	db := openTestDB(t)
	// See TestApplyAtomicity_PostgresRealRollback for why cleanup and
	// Close are co-located in a single t.Cleanup rather than split.
	t.Cleanup(func() {
		cleanupAtomicityIntegrationRows(t, db)
		db.Close()
	})
	cleanupAtomicityIntegrationRows(t, db)

	s, err := NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Surfaces:         repos.Surfaces,
		Processes:        repos.Processes,
		Capabilities:     repos.Capabilities,
		BusinessServices: repos.BusinessServices,
		Agents:           repos.Agents,
		Profiles:         repos.Profiles,
		Grants:           repos.Grants,
		ControlAudit:     repos.ControlAudit,
		Tx:               NewApplyTxRunner(s), // the real production adapter
	})

	ctx := context.Background()
	result := svc.Apply(ctx, atomicityIntegrationBundle(), "integration-test")

	if result.ApplyErrorCount() != 0 {
		t.Fatalf("positive control: expected zero apply errors, got %d: %+v",
			result.ApplyErrorCount(), result.Results)
	}
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("positive control: expected zero validation errors, got %d: %+v",
			result.ValidationErrorCount(), result.ValidationErrors)
	}

	for _, check := range []struct {
		name, query, id string
	}{
		{"business_service", `SELECT COUNT(*) FROM business_services WHERE business_service_id = $1`, "bs-atomic-int"},
		{"process", `SELECT COUNT(*) FROM processes WHERE process_id = $1`, "proc-atomic-int"},
		{"surface", `SELECT COUNT(*) FROM decision_surfaces WHERE id = $1`, "surf-atomic-int"},
	} {
		var count int
		if err := db.QueryRowContext(ctx, check.query, check.id).Scan(&count); err != nil {
			t.Fatalf("positive control: SELECT %s: %v", check.name, err)
		}
		if count != 1 {
			t.Errorf("positive control: want 1 %s row committed, got %d", check.name, count)
		}
	}
}
