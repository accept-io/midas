package inference

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/accept-io/midas/internal/store/sqltx"
)

// PromoteRequest identifies the inferred entities to promote and the managed IDs to create.
type PromoteRequest struct {
	FromCapabilityID string
	FromProcessID    string
	ToCapabilityID   string
	ToProcessID      string
}

// PromoteResponse reports what the promotion created and migrated.
type PromoteResponse struct {
	FromCapabilityID string
	FromProcessID    string
	ToCapabilityID   string
	ToProcessID      string
	SurfacesMigrated int
}

// PromoteErr is the sentinel error type for client-visible promotion validation failures.
// Errors of this type map to HTTP 400/422 at the HTTP layer.
type PromoteErr string

func (e PromoteErr) Error() string { return string(e) }

// capabilityPromoter is the narrow interface required for capability promotion operations.
// *postgres.CapabilityRepo satisfies this interface.
type capabilityPromoter interface {
	// GetInferredMeta reads origin and managed for promotion validation.
	// Returns (false, "", false, nil) when the row does not exist.
	GetInferredMeta(ctx context.Context, id string) (exists bool, origin string, managed bool, err error)
	// GetReplaces reads the replaces column for the given ID.
	// Returns ("", false, nil) when the row does not exist or replaces is NULL.
	GetReplaces(ctx context.Context, id string) (replacesID string, ok bool, err error)
	// Exists reports whether a capability with the given ID exists.
	Exists(ctx context.Context, id string) (bool, error)
	// CreateManaged inserts a new managed capability within the given transaction.
	CreateManaged(ctx context.Context, db sqltx.DBTX, id, replacesID string) error
	// DeprecateInferred sets status='deprecated' for the given inferred capability.
	DeprecateInferred(ctx context.Context, db sqltx.DBTX, id string) error
}

// processPromoter is the narrow interface required for process promotion operations.
// *postgres.ProcessRepo satisfies this interface.
type processPromoter interface {
	// GetInferredMeta reads origin, managed, and capability_id for the given ID.
	// Returns (false, "", false, "", nil) when the row does not exist.
	GetInferredMeta(ctx context.Context, id string) (exists bool, origin string, managed bool, capabilityID string, err error)
	// GetReplaces reads the replaces column for the given ID.
	// Returns ("", false, nil) when the row does not exist or replaces is NULL.
	GetReplaces(ctx context.Context, id string) (replacesID string, ok bool, err error)
	// Exists reports whether a process with the given ID exists.
	Exists(ctx context.Context, id string) (bool, error)
	// CreateManaged inserts a new managed process within the given transaction.
	CreateManaged(ctx context.Context, db sqltx.DBTX, id, capabilityID, replacesID string) error
	// DeprecateInferred sets status='deprecated' for the given inferred process.
	DeprecateInferred(ctx context.Context, db sqltx.DBTX, id string) error
}

// surfaceMigrator is the narrow interface required for surface migration during promotion.
// *postgres.SurfaceRepo satisfies this interface.
type surfaceMigrator interface {
	// CountByProcessID returns the number of surfaces attached to the given process.
	CountByProcessID(ctx context.Context, processID string) (int, error)
	// MigrateProcess updates all surfaces from fromProcessID to toProcessID within
	// the given transaction. Returns the number of rows updated.
	MigrateProcess(ctx context.Context, db sqltx.DBTX, fromProcessID, toProcessID string) (int64, error)
}

// PromoteService promotes inferred structural entities to managed equivalents.
// It is safe for concurrent use.
type PromoteService struct {
	db    *sql.DB
	caps  capabilityPromoter
	procs processPromoter
	surfs surfaceMigrator
}

// NewPromoteService constructs a PromoteService. All arguments must be non-nil.
func NewPromoteService(db *sql.DB, caps capabilityPromoter, procs processPromoter, surfs surfaceMigrator) *PromoteService {
	return &PromoteService{
		db:    db,
		caps:  caps,
		procs: procs,
		surfs: surfs,
	}
}

// Promote atomically promotes an inferred capability/process pair to managed equivalents,
// migrates all attached surfaces to the new process, and deprecates the old inferred entities.
//
// Pre-transaction validation:
//   - all four IDs must be non-empty and from != to
//   - from capability must exist with origin=inferred
//   - from process must exist with origin=inferred and capability_id = from.capability_id
//   - to capability must not already exist
//   - to process must not already exist
//   - no cycle in the capability or process replaces chains
//
// Transaction (all-or-nothing):
//  1. create new managed capability (replaces = from.capability_id)
//  2. create new managed process (replaces = from.process_id, capability_id = to.capability_id)
//  3. migrate all surfaces from from.process_id → to.process_id
//  4. verify migrated row count matches pre-tx count (concurrent-modification guard)
//  5. deprecate from.capability_id
//  6. deprecate from.process_id
//  7. commit
func (s *PromoteService) Promote(ctx context.Context, req PromoteRequest) (PromoteResponse, error) {
	if err := validatePromoteRequest(req); err != nil {
		return PromoteResponse{}, err
	}

	// Validate from capability exists and is inferred.
	capExists, capOrigin, _, err := s.caps.GetInferredMeta(ctx, req.FromCapabilityID)
	if err != nil {
		return PromoteResponse{}, fmt.Errorf("capability lookup failed: %w", err)
	}
	if !capExists {
		return PromoteResponse{}, PromoteErr(fmt.Sprintf("capability %q does not exist", req.FromCapabilityID))
	}
	if capOrigin != "inferred" {
		return PromoteResponse{}, PromoteErr(fmt.Sprintf("capability %q has origin=%q; only inferred capabilities can be promoted", req.FromCapabilityID, capOrigin))
	}

	// Validate from process exists, is inferred, and belongs to from capability.
	procExists, procOrigin, _, procCapID, err := s.procs.GetInferredMeta(ctx, req.FromProcessID)
	if err != nil {
		return PromoteResponse{}, fmt.Errorf("process lookup failed: %w", err)
	}
	if !procExists {
		return PromoteResponse{}, PromoteErr(fmt.Sprintf("process %q does not exist", req.FromProcessID))
	}
	if procOrigin != "inferred" {
		return PromoteResponse{}, PromoteErr(fmt.Sprintf("process %q has origin=%q; only inferred processes can be promoted", req.FromProcessID, procOrigin))
	}
	if procCapID != req.FromCapabilityID {
		return PromoteResponse{}, PromoteErr(fmt.Sprintf("process %q belongs to capability %q, not %q", req.FromProcessID, procCapID, req.FromCapabilityID))
	}

	// Validate to capability does not already exist.
	toCapExists, err := s.caps.Exists(ctx, req.ToCapabilityID)
	if err != nil {
		return PromoteResponse{}, fmt.Errorf("target capability lookup failed: %w", err)
	}
	if toCapExists {
		return PromoteResponse{}, PromoteErr(fmt.Sprintf("capability %q already exists", req.ToCapabilityID))
	}

	// Validate to process does not already exist.
	toProcExists, err := s.procs.Exists(ctx, req.ToProcessID)
	if err != nil {
		return PromoteResponse{}, fmt.Errorf("target process lookup failed: %w", err)
	}
	if toProcExists {
		return PromoteResponse{}, PromoteErr(fmt.Sprintf("process %q already exists", req.ToProcessID))
	}

	// Cycle detection on capability and process replaces chains.
	if err := checkReplacesChain(ctx, s.caps.GetReplaces, req.FromCapabilityID, req.ToCapabilityID); err != nil {
		return PromoteResponse{}, err
	}
	if err := checkReplacesChain(ctx, s.procs.GetReplaces, req.FromProcessID, req.ToProcessID); err != nil {
		return PromoteResponse{}, err
	}

	// Count surfaces attached to from process before opening the transaction.
	expectedCount, err := s.surfs.CountByProcessID(ctx, req.FromProcessID)
	if err != nil {
		return PromoteResponse{}, fmt.Errorf("surface count failed: %w", err)
	}

	// Execute the promotion transaction.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PromoteResponse{}, fmt.Errorf("begin transaction: %w", err)
	}

	surfacesMigrated, err := s.promoteInTx(ctx, tx, req, expectedCount)
	if err != nil {
		_ = tx.Rollback()
		return PromoteResponse{}, err
	}

	if err := tx.Commit(); err != nil {
		return PromoteResponse{}, fmt.Errorf("commit transaction: %w", err)
	}

	return PromoteResponse{
		FromCapabilityID: req.FromCapabilityID,
		FromProcessID:    req.FromProcessID,
		ToCapabilityID:   req.ToCapabilityID,
		ToProcessID:      req.ToProcessID,
		SurfacesMigrated: surfacesMigrated,
	}, nil
}

func (s *PromoteService) promoteInTx(ctx context.Context, tx *sql.Tx, req PromoteRequest, expectedCount int) (int, error) {
	if err := s.caps.CreateManaged(ctx, tx, req.ToCapabilityID, req.FromCapabilityID); err != nil {
		return 0, fmt.Errorf("create managed capability: %w", err)
	}

	if err := s.procs.CreateManaged(ctx, tx, req.ToProcessID, req.ToCapabilityID, req.FromProcessID); err != nil {
		return 0, fmt.Errorf("create managed process: %w", err)
	}

	migrated, err := s.surfs.MigrateProcess(ctx, tx, req.FromProcessID, req.ToProcessID)
	if err != nil {
		return 0, fmt.Errorf("migrate surfaces: %w", err)
	}

	if int(migrated) != expectedCount {
		return 0, fmt.Errorf("surface migration mismatch: expected %d rows updated, got %d (concurrent modification?)", expectedCount, migrated)
	}

	if err := s.caps.DeprecateInferred(ctx, tx, req.FromCapabilityID); err != nil {
		return 0, fmt.Errorf("deprecate inferred capability: %w", err)
	}

	if err := s.procs.DeprecateInferred(ctx, tx, req.FromProcessID); err != nil {
		return 0, fmt.Errorf("deprecate inferred process: %w", err)
	}

	return int(migrated), nil
}

// validatePromoteRequest checks that all four IDs are present and from != to.
func validatePromoteRequest(req PromoteRequest) error {
	if req.FromCapabilityID == "" {
		return PromoteErr("from_capability_id is required")
	}
	if req.FromProcessID == "" {
		return PromoteErr("from_process_id is required")
	}
	if req.ToCapabilityID == "" {
		return PromoteErr("to_capability_id is required")
	}
	if req.ToProcessID == "" {
		return PromoteErr("to_process_id is required")
	}
	if req.FromCapabilityID == req.ToCapabilityID {
		return PromoteErr("from_capability_id and to_capability_id must differ")
	}
	if req.FromProcessID == req.ToProcessID {
		return PromoteErr("from_process_id and to_process_id must differ")
	}
	return nil
}

// checkReplacesChain walks the replaces chain starting from startID and verifies
// that targetID does not appear in it. Bounded by maxDepth to guard against
// existing corrupt circular chains in persisted data.
func checkReplacesChain(
	ctx context.Context,
	getReplaces func(ctx context.Context, id string) (string, bool, error),
	startID, targetID string,
) error {
	const maxDepth = 100
	current := startID
	for i := 0; i < maxDepth; i++ {
		next, ok, err := getReplaces(ctx, current)
		if err != nil {
			return fmt.Errorf("cycle detection: reading replaces for %q: %w", current, err)
		}
		if !ok {
			return nil // chain terminated cleanly
		}
		if next == targetID {
			return PromoteErr(fmt.Sprintf("promotion would create a replaces cycle: %q is already in the chain of %q", targetID, startID))
		}
		current = next
	}
	return fmt.Errorf("replaces chain from %q exceeds maximum depth (%d); data may be corrupt", startID, maxDepth)
}
