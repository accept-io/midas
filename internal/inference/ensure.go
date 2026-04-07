package inference

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/accept-io/midas/internal/store/sqltx"
)

// InferenceResult reports the deterministic IDs used for an inferred structural chain
// and whether each entity was newly created or already existed with correct semantics.
type InferenceResult struct {
	CapabilityID      string
	ProcessID         string
	SurfaceID         string
	CapabilityCreated bool
	ProcessCreated    bool
	SurfaceCreated    bool
}

// capabilityEnsurer is a narrow interface satisfied by *postgres.CapabilityRepo.
type capabilityEnsurer interface {
	EnsureInferred(ctx context.Context, db sqltx.DBTX, id string) (bool, error)
}

// processEnsurer is a narrow interface satisfied by *postgres.ProcessRepo.
type processEnsurer interface {
	EnsureInferred(ctx context.Context, db sqltx.DBTX, processID, capabilityID string) (bool, error)
}

// surfaceEnsurer is a narrow interface satisfied by *postgres.SurfaceRepo.
type surfaceEnsurer interface {
	EnsureInferred(ctx context.Context, db sqltx.DBTX, surfaceID, processID string) (bool, error)
}

// Service provides transactional inference operations. It is safe for concurrent use.
type Service struct {
	db    *sql.DB
	caps  capabilityEnsurer
	procs processEnsurer
	surfs surfaceEnsurer
}

// NewService creates a new inference Service. All arguments must be non-nil.
func NewService(db *sql.DB, caps capabilityEnsurer, procs processEnsurer, surfs surfaceEnsurer) *Service {
	return &Service{
		db:    db,
		caps:  caps,
		procs: procs,
		surfs: surfs,
	}
}

// EnsureInferredStructure validates surfaceID, derives the inferred capability and
// process IDs using PR2 helpers, then atomically ensures all three entities exist in
// the database with origin=inferred and managed=false.
//
// The operation is:
//   - transactional: all three inserts are committed together or not at all
//   - idempotent: safe to call repeatedly with the same surfaceID
//   - concurrency-safe: duplicate concurrent calls produce at most one row each,
//     via ON CONFLICT DO NOTHING on deterministic primary keys
//
// If ValidateSurfaceID returns an error no transaction is started.
// If any step fails the full chain is rolled back.
func (s *Service) EnsureInferredStructure(ctx context.Context, surfaceID string) (InferenceResult, error) {
	if err := ValidateSurfaceID(surfaceID); err != nil {
		return InferenceResult{}, fmt.Errorf("invalid surface ID: %w", err)
	}

	capID, procID := InferStructure(surfaceID)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return InferenceResult{}, fmt.Errorf("begin transaction: %w", err)
	}

	result, err := s.ensureChainInTx(ctx, tx, surfaceID, capID, procID)
	if err != nil {
		_ = tx.Rollback()
		return InferenceResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return InferenceResult{}, fmt.Errorf("commit transaction: %w", err)
	}
	return result, nil
}

func (s *Service) ensureChainInTx(ctx context.Context, tx *sql.Tx, surfaceID, capID, procID string) (InferenceResult, error) {
	capCreated, err := s.caps.EnsureInferred(ctx, tx, capID)
	if err != nil {
		return InferenceResult{}, err
	}

	procCreated, err := s.procs.EnsureInferred(ctx, tx, procID, capID)
	if err != nil {
		return InferenceResult{}, err
	}

	surfCreated, err := s.surfs.EnsureInferred(ctx, tx, surfaceID, procID)
	if err != nil {
		return InferenceResult{}, err
	}

	return InferenceResult{
		CapabilityID:      capID,
		ProcessID:         procID,
		SurfaceID:         surfaceID,
		CapabilityCreated: capCreated,
		ProcessCreated:    procCreated,
		SurfaceCreated:    surfCreated,
	}, nil
}
