package memory

import (
	"context"

	"github.com/accept-io/midas/internal/store"
)

type Store struct {
	repos *store.Repositories
}

func NewStore() *Store {
	return &Store{
		repos: NewRepositories(),
	}
}

func (s *Store) Repositories() (*store.Repositories, error) {
	return s.repos, nil
}

func (s *Store) WithTx(ctx context.Context, fn func(*store.Repositories) error) error {
	return fn(s.repos)
}

func NewStoreWithRepositories(repos *store.Repositories) *Store {
	return &Store{repos: repos}
}
