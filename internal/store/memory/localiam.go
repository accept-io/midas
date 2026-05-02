package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/accept-io/midas/internal/localiam"
)

// LocalUserRepo is the in-memory implementation of localiam.UserRepository.
// It is safe for concurrent access.
type LocalUserRepo struct {
	mu         sync.RWMutex
	byID       map[string]*localiam.User
	byUsername map[string]*localiam.User
}

func NewLocalUserRepo() *LocalUserRepo {
	return &LocalUserRepo{
		byID:       make(map[string]*localiam.User),
		byUsername: make(map[string]*localiam.User),
	}
}

func (r *LocalUserRepo) FindByID(_ context.Context, id string) (*localiam.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u := r.byID[id]
	if u == nil {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}

func (r *LocalUserRepo) FindByUsername(_ context.Context, username string) (*localiam.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u := r.byUsername[username]
	if u == nil {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}

func (r *LocalUserRepo) Create(_ context.Context, u *localiam.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byUsername[u.Username]; exists {
		return fmt.Errorf("localiam: username %q already exists", u.Username)
	}
	cp := *u
	r.byID[u.ID] = &cp
	r.byUsername[u.Username] = &cp
	return nil
}

func (r *LocalUserRepo) Update(_ context.Context, u *localiam.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.byID[u.ID]
	if !ok {
		return fmt.Errorf("localiam: user %q not found", u.ID)
	}
	// If username changed, update the username index.
	if existing.Username != u.Username {
		delete(r.byUsername, existing.Username)
	}
	cp := *u
	r.byID[u.ID] = &cp
	r.byUsername[u.Username] = &cp
	return nil
}

func (r *LocalUserRepo) Count(_ context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byID), nil
}

// LocalSessionRepo is the in-memory implementation of localiam.SessionRepository.
type LocalSessionRepo struct {
	mu    sync.RWMutex
	items map[string]*localiam.Session
}

func NewLocalSessionRepo() *LocalSessionRepo {
	return &LocalSessionRepo{items: make(map[string]*localiam.Session)}
}

func (r *LocalSessionRepo) Create(_ context.Context, s *localiam.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *s
	r.items[s.ID] = &cp
	return nil
}

func (r *LocalSessionRepo) FindByID(_ context.Context, id string) (*localiam.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s := r.items[id]
	if s == nil {
		return nil, nil
	}
	cp := *s
	return &cp, nil
}

func (r *LocalSessionRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, id)
	return nil
}

func (r *LocalSessionRepo) DeleteExpired(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	for id, s := range r.items {
		if now.After(s.ExpiresAt) {
			delete(r.items, id)
		}
	}
	return nil
}
