package postgres

import (
	"context"

	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/store"
)

// ApplyTxRunner adapts *Store into apply.TxRunner. The scoped
// *apply.RepositorySet passed to fn is backed by transaction-scoped
// repositories for every state-changing kind, so a roll-back propagates
// through real Postgres.
//
// ControlAudit and Tx are deliberately not set on the scoped set:
//
//   - ControlAudit is excluded per ADR-041b (control audit sits outside
//     the state-change transaction). The apply.Service buffers audit
//     records during the loop and flushes them through its own outer
//     controlAuditRepo after commit; it never consults the scoped
//     ControlAudit.
//
//   - Tx is excluded because the loop inside WithTx is already running
//     inside a transaction; a nested TxRunner would be meaningless and
//     is ignored by the apply executor.
type ApplyTxRunner struct {
	store *Store
}

// NewApplyTxRunner constructs an adapter bound to s. s must be non-nil.
func NewApplyTxRunner(s *Store) *ApplyTxRunner {
	return &ApplyTxRunner{store: s}
}

// WithTx runs fn inside a Postgres transaction. On a nil return from fn
// the transaction commits; on any other return the transaction rolls
// back and the error is returned unchanged.
func (a *ApplyTxRunner) WithTx(
	ctx context.Context,
	operation string,
	fn func(*apply.RepositorySet) error,
) error {
	return a.store.WithTx(ctx, operation, func(repos *store.Repositories) error {
		return fn(&apply.RepositorySet{
			Surfaces:                repos.Surfaces,
			Agents:                  repos.Agents,
			Profiles:                repos.Profiles,
			Grants:                  repos.Grants,
			Processes:               repos.Processes,
			Capabilities:            repos.Capabilities,
			BusinessServices:        repos.BusinessServices,
			ProcessCapabilities:     repos.ProcessCapabilities,
			ProcessBusinessServices: repos.ProcessBusinessServices,
		})
	})
}

// Compile-time check that ApplyTxRunner satisfies apply.TxRunner.
var _ apply.TxRunner = (*ApplyTxRunner)(nil)
