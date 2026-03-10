package postgres

import (
	"context"
	"database/sql"
	"errors"
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
func (s *Store) WithTx(ctx context.Context, operation string, fn func(*store.Repositories) error) (err error) {
	start := time.Now()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.metrics.IncrementTransactionError(operation, "begin")
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			s.metrics.IncrementTransactionError(operation, "panic")
			s.metrics.IncrementTransactionRollback(operation)
			s.metrics.RecordTransactionDuration(operation, "panic", time.Since(start))
			_ = tx.Rollback()
			panic(p)
		}
	}()

	repos, err := newRepositories(tx)
	if err != nil {
		s.metrics.IncrementTransactionError(operation, "repository_factory")
		if rbErr := tx.Rollback(); rbErr != nil {
			s.metrics.IncrementTransactionError(operation, "rollback_after_factory_error")
			s.metrics.RecordTransactionDuration(operation, "rollback_error", time.Since(start))
			return errors.Join(err, rbErr)
		}
		s.metrics.IncrementTransactionRollback(operation)
		s.metrics.RecordTransactionDuration(operation, "rollback", time.Since(start))
		return err
	}

	if err := fn(repos); err != nil {
		s.metrics.IncrementTransactionError(operation, "callback")
		if rbErr := tx.Rollback(); rbErr != nil {
			s.metrics.IncrementTransactionError(operation, "rollback_after_callback_error")
			s.metrics.RecordTransactionDuration(operation, "rollback_error", time.Since(start))
			return errors.Join(err, rbErr)
		}
		s.metrics.IncrementTransactionRollback(operation)
		s.metrics.RecordTransactionDuration(operation, "rollback", time.Since(start))
		return err
	}

	if err := tx.Commit(); err != nil {
		s.metrics.IncrementTransactionError(operation, "commit")
		s.metrics.RecordTransactionDuration(operation, "commit_error", time.Since(start))
		return err
	}

	s.metrics.IncrementTransactionCommit(operation)
	s.metrics.RecordTransactionDuration(operation, "commit", time.Since(start))
	return nil
}

func newRepositories(db sqltx.DBTX) (*store.Repositories, error) {
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

	auditRepo := audit.NewPostgresRepository(db)

	return &store.Repositories{
		Surfaces:  surfaces,
		Agents:    agents,
		Profiles:  profiles,
		Grants:    grants,
		Envelopes: envelopes,
		Audit:     auditRepo,
	}, nil
}
