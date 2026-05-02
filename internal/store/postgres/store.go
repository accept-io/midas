package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/sqltx"
)

type Store struct {
	db      *sql.DB
	metrics store.TransactionRecorder
}

func NewStore(db *sql.DB, metrics store.TransactionRecorder) (*Store, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	if metrics == nil {
		metrics = store.NoOpTransactionRecorder{}
	}
	return &Store{
		db:      db,
		metrics: metrics,
	}, nil
}

// Repositories returns repositories bound to the base DB connection.
// Use this for read operations that do not require a transaction.
func (s *Store) Repositories() (*store.Repositories, error) {
	return newRepositories(s.db)
}

// WithTx executes fn with repositories bound to a transaction.
// operation should describe the business workflow (e.g., "evaluation", "review", "admin_update").
func (s *Store) WithTx(ctx context.Context, operation string, fn func(*store.Repositories) error) (err error) {
	start := time.Now()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("tx_begin_failed",
			"operation", operation,
			"error", err,
		)
		s.metrics.IncrementTransactionError(operation, "begin")
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			duration := time.Since(start)
			slog.Error("tx_panic_recovered",
				"operation", operation,
				"duration_ms", duration.Milliseconds(),
				"panic_value", fmt.Sprintf("%v", p),
			)
			s.metrics.IncrementTransactionError(operation, "panic")
			s.metrics.IncrementTransactionRollback(operation)
			s.metrics.RecordTransactionDuration(operation, "panic", duration)

			// Attempt rollback and log if it fails
			if rbErr := tx.Rollback(); rbErr != nil {
				slog.Error("tx_rollback_failed_after_panic",
					"operation", operation,
					"rollback_error", rbErr,
				)
			}
			panic(p)
		}
	}()

	repos, err := newRepositories(tx)
	if err != nil {
		slog.Error("tx_repository_factory_failed",
			"operation", operation,
			"error", err,
		)
		s.metrics.IncrementTransactionError(operation, "repository_factory")

		if rbErr := tx.Rollback(); rbErr != nil {
			duration := time.Since(start)
			slog.Error("tx_rollback_failed_after_factory_error",
				"operation", operation,
				"factory_error", err,
				"rollback_error", rbErr,
			)
			s.metrics.IncrementTransactionError(operation, "rollback_after_factory_error")
			s.metrics.RecordTransactionDuration(operation, "rollback_error", duration)
			return errors.Join(err, rbErr)
		}

		duration := time.Since(start)
		s.metrics.IncrementTransactionRollback(operation)
		s.metrics.RecordTransactionDuration(operation, "rollback", duration)
		return err
	}

	if err := fn(repos); err != nil {
		// Callback returned error - may be business logic or repository failure
		s.metrics.IncrementTransactionError(operation, "callback_returned_error")

		if rbErr := tx.Rollback(); rbErr != nil {
			duration := time.Since(start)
			slog.Error("tx_rollback_failed_after_callback_error",
				"operation", operation,
				"callback_error", err,
				"rollback_error", rbErr,
			)
			s.metrics.IncrementTransactionError(operation, "rollback_after_callback_error")
			s.metrics.RecordTransactionDuration(operation, "rollback_error", duration)
			return errors.Join(err, rbErr)
		}

		duration := time.Since(start)
		s.metrics.IncrementTransactionRollback(operation)
		s.metrics.RecordTransactionDuration(operation, "rollback", duration)
		return err
	}

	if err := tx.Commit(); err != nil {
		duration := time.Since(start)
		slog.Error("tx_commit_failed",
			"operation", operation,
			"duration_ms", duration.Milliseconds(),
			"error", err,
		)
		s.metrics.IncrementTransactionError(operation, "commit")
		s.metrics.RecordTransactionDuration(operation, "commit_error", duration)
		return err
	}

	duration := time.Since(start)
	s.metrics.IncrementTransactionCommit(operation)
	s.metrics.RecordTransactionDuration(operation, "commit", duration)
	return nil
}

func newRepositories(db sqltx.DBTX) (*store.Repositories, error) {
	caps, err := NewCapabilityRepo(db)
	if err != nil {
		return nil, err
	}

	procs, err := NewProcessRepo(db)
	if err != nil {
		return nil, err
	}

	surfaces, err := NewSurfaceRepo(db)
	if err != nil {
		return nil, err
	}

	agents, err := NewAgentRepo(db)
	if err != nil {
		return nil, err
	}

	profiles, err := NewProfileRepo(db)
	if err != nil {
		return nil, err
	}

	grants, err := NewGrantRepo(db)
	if err != nil {
		return nil, err
	}

	envelopes, err := NewEnvelopeRepo(db)
	if err != nil {
		return nil, err
	}

	outboxRepo, err := NewOutboxRepo(db)
	if err != nil {
		return nil, err
	}

	auditRepo := audit.NewPostgresRepository(db)

	controlAuditRepo, err := NewControlAuditRepo(db)
	if err != nil {
		return nil, err
	}

	adminAuditRepo, err := NewAdminAuditRepo(db)
	if err != nil {
		return nil, err
	}

	localUsers, err := NewLocalUserRepo(db)
	if err != nil {
		return nil, err
	}

	localSessions, err := NewLocalSessionRepo(db)
	if err != nil {
		return nil, err
	}

	businessServices, err := NewBusinessServiceRepo(db)
	if err != nil {
		return nil, err
	}

	bsCaps, err := NewBusinessServiceCapabilityRepo(db)
	if err != nil {
		return nil, err
	}

	bsRelationships, err := NewBusinessServiceRelationshipRepo(db)
	if err != nil {
		return nil, err
	}

	governanceExpectations, err := NewGovernanceExpectationRepo(db)
	if err != nil {
		return nil, err
	}

	aiSystems, err := NewAISystemRepo(db)
	if err != nil {
		return nil, err
	}

	aiSystemVersions, err := NewAISystemVersionRepo(db)
	if err != nil {
		return nil, err
	}

	aiSystemBindings, err := NewAISystemBindingRepo(db)
	if err != nil {
		return nil, err
	}

	return &store.Repositories{
		Capabilities:                 caps,
		Processes:                    procs,
		Surfaces:                     surfaces,
		Agents:                       agents,
		Profiles:                     profiles,
		Grants:                       grants,
		Envelopes:                    envelopes,
		Audit:                        auditRepo,
		ControlAudit:                 controlAuditRepo,
		AdminAudit:                   adminAuditRepo,
		Outbox:                       outboxRepo,
		LocalUsers:                   localUsers,
		LocalSessions:                localSessions,
		BusinessServices:             businessServices,
		BusinessServiceCapabilities:  bsCaps,
		BusinessServiceRelationships: bsRelationships,
		GovernanceExpectations:       governanceExpectations,
		AISystems:                    aiSystems,
		AISystemVersions:             aiSystemVersions,
		AISystemBindings:             aiSystemBindings,
	}, nil
}
