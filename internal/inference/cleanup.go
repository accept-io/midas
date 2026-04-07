package inference

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/store/sqltx"
)

// CleanupResult reports the IDs of entities deleted by a cleanup run.
type CleanupResult struct {
	ProcessesDeleted    []string `json:"processes_deleted"`
	CapabilitiesDeleted []string `json:"capabilities_deleted"`
}

// processCleanup is the narrow interface for process cleanup operations.
// *postgres.ProcessRepo satisfies this interface.
type processCleanup interface {
	// FindEligibleForCleanup returns the IDs of deprecated inferred processes
	// that are safe to delete as of the given cutoff time.
	FindEligibleForCleanup(ctx context.Context, db sqltx.DBTX, cutoff time.Time) ([]string, error)
	// DeleteByIDs deletes processes with the given IDs within the transaction.
	// No-op when ids is empty.
	DeleteByIDs(ctx context.Context, db sqltx.DBTX, ids []string) error
}

// capabilityCleanup is the narrow interface for capability cleanup operations.
// *postgres.CapabilityRepo satisfies this interface.
type capabilityCleanup interface {
	// FindEligibleForCleanup returns the IDs of deprecated inferred capabilities
	// that are safe to delete as of the given cutoff time.
	FindEligibleForCleanup(ctx context.Context, db sqltx.DBTX, cutoff time.Time) ([]string, error)
	// DeleteByIDs deletes capabilities with the given IDs within the transaction.
	// No-op when ids is empty.
	DeleteByIDs(ctx context.Context, db sqltx.DBTX, ids []string) error
}

// CleanupService deletes deprecated inferred entities that are no longer referenced.
// It is safe for concurrent use.
type CleanupService struct {
	db    *sql.DB
	procs processCleanup
	caps  capabilityCleanup
}

// NewCleanupService constructs a CleanupService. All arguments must be non-nil.
func NewCleanupService(db *sql.DB, procs processCleanup, caps capabilityCleanup) *CleanupService {
	return &CleanupService{db: db, procs: procs, caps: caps}
}

// CleanupInferredEntities deletes deprecated inferred processes and capabilities
// that are no longer referenced, running all deletions inside a single transaction.
//
// cutoff controls which entities are eligible by age:
//   - zero time (time.Time{}) → all otherwise-eligible entities regardless of age
//   - non-zero              → only entities whose updated_at < cutoff
//
// Deletion order: processes first, then capabilities. This order is required
// because capabilities cannot be deleted while processes still reference them via
// capability_id. Deleting eligible processes first clears the path for capability
// deletion within the same transaction.
//
// Eligibility rules for processes (all must hold):
//  1. origin = 'inferred'
//  2. managed = false
//  3. status = 'deprecated'
//  4. updated_at < cutoff (or cutoff is zero)
//  5. no decision_surface references this process via process_id
//  6. no other process references this via replaces
//  7. no other process references this via parent_process_id
//
// Note on envelope protection: envelopes reference surfaces (not processes directly).
// Rule 5 (no surfaces reference the process) provides transitive envelope protection:
// if no surface points to this process, no envelope can reference it through the
// surface chain. decision_surfaces.process_id is mutable (updated by promotion), so
// a join-based envelope-process query would not reliably detect historical linkage.
// Rules 5 and 6 together cover all correctness cases.
//
// Eligibility rules for capabilities (all must hold):
//  1. origin = 'inferred'
//  2. managed = false
//  3. status = 'deprecated'
//  4. updated_at < cutoff (or cutoff is zero)
//  5. no process references this capability via capability_id
//  6. no other capability references this via replaces
//  7. no other capability references this via parent_capability_id
func (s *CleanupService) CleanupInferredEntities(ctx context.Context, cutoff time.Time) (CleanupResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CleanupResult{}, fmt.Errorf("begin transaction: %w", err)
	}

	result, err := s.cleanupInTx(ctx, tx, cutoff)
	if err != nil {
		_ = tx.Rollback()
		return CleanupResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return CleanupResult{}, fmt.Errorf("commit transaction: %w", err)
	}
	return result, nil
}

func (s *CleanupService) cleanupInTx(ctx context.Context, tx *sql.Tx, cutoff time.Time) (CleanupResult, error) {
	// Step 1: find and delete eligible processes.
	procIDs, err := s.procs.FindEligibleForCleanup(ctx, tx, cutoff)
	if err != nil {
		return CleanupResult{}, fmt.Errorf("find eligible processes: %w", err)
	}
	if err := s.procs.DeleteByIDs(ctx, tx, procIDs); err != nil {
		return CleanupResult{}, fmt.Errorf("delete eligible processes: %w", err)
	}

	// Step 2: find and delete eligible capabilities.
	// Runs after process deletion so that capabilities only held by now-deleted
	// processes become eligible within the same transaction.
	capIDs, err := s.caps.FindEligibleForCleanup(ctx, tx, cutoff)
	if err != nil {
		return CleanupResult{}, fmt.Errorf("find eligible capabilities: %w", err)
	}
	if err := s.caps.DeleteByIDs(ctx, tx, capIDs); err != nil {
		return CleanupResult{}, fmt.Errorf("delete eligible capabilities: %w", err)
	}

	// Normalise to empty slices so callers never receive nil.
	if procIDs == nil {
		procIDs = []string{}
	}
	if capIDs == nil {
		capIDs = []string{}
	}

	return CleanupResult{
		ProcessesDeleted:    procIDs,
		CapabilitiesDeleted: capIDs,
	}, nil
}
