package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/sqltx"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &Store{db: db}, nil
}

// Repositories returns repositories bound to the base DB connection.
// Use this for read operations that do not require a transaction.
func (s *Store) Repositories() (*store.Repositories, error) {
	return newRepositories(s.db)
}

// WithTx executes fn with repositories bound to a transaction.
func (s *Store) WithTx(ctx context.Context, fn func(*store.Repositories) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	repos, err := newRepositories(tx)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return errors.Join(err, rbErr)
		}
		return err
	}

	if err := fn(repos); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return errors.Join(err, rbErr)
		}
		return err
	}

	return tx.Commit()
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
