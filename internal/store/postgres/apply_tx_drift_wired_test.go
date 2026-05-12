package postgres

// apply_tx_drift_wired_test.go — Drift-1c-fix source pin for the
// transaction-scoped apply.RepositorySet built by ApplyTxRunner.WithTx.
//
// Critical wiring path: every Postgres-backed control-plane apply call
// runs inside this transactional scope. If DriftDefinitions is omitted
// here, the inner Service is handed a nil DriftDefinitionRepository
// for the duration of the transaction even when the outer Service was
// constructed with a non-nil one — silently degrading the apply path
// to validation-only inside the tx.
//
// Drift-1b's repository parity tests already cover the deeper
// persistence behaviour (Create, FindByID, FindByIDAndVersion, etc.)
// against a live Postgres. This source pin guards specifically against
// a regression of the literal struct in apply_tx.go's WithTx function.

import (
	"os"
	"strings"
	"testing"
)

func TestApplyTx_WiringIncludesDriftDefinitions(t *testing.T) {
	src, err := os.ReadFile("apply_tx.go")
	if err != nil {
		t.Fatalf("read apply_tx.go: %v", err)
	}
	wantSubstr := "DriftDefinitions:             repos.DriftDefinitions,"
	if !strings.Contains(string(src), wantSubstr) {
		t.Errorf("internal/store/postgres/apply_tx.go must wire DriftDefinitions into the transactional apply.RepositorySet; missing literal:\n  %s", wantSubstr)
	}
}
