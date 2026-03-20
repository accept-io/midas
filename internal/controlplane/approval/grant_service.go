package approval

import (
	"context"
	"errors"
	"time"

	"github.com/accept-io/midas/internal/authority"
)

// Grant lifecycle errors.
var (
	ErrGrantNotFound    = errors.New("grant not found")
	ErrGrantNotActive   = errors.New("grant is not active")
	ErrGrantNotSuspended = errors.New("grant is not suspended")
	ErrGrantRevoked     = errors.New("grant is permanently revoked")
	ErrInvalidGrantTransition = errors.New("invalid grant status transition")
)

// GrantRepository is the subset of authority.GrantRepository required for
// grant lifecycle operations.
type GrantRepository interface {
	FindByID(ctx context.Context, id string) (*authority.AuthorityGrant, error)
	Update(ctx context.Context, g *authority.AuthorityGrant) error
}

// GrantService orchestrates grant lifecycle governance: suspend, revoke, reinstate.
type GrantService struct {
	repo GrantRepository
}

// NewGrantService constructs a GrantService.
func NewGrantService(repo GrantRepository) *GrantService {
	return &GrantService{repo: repo}
}

// SuspendGrant transitions an active grant to suspended.
func (s *GrantService) SuspendGrant(ctx context.Context, grantID, suspendedBy, reason string) (*authority.AuthorityGrant, error) {
	g, err := s.repo.FindByID(ctx, grantID)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, ErrGrantNotFound
	}

	if g.Status == authority.GrantStatusRevoked {
		return nil, ErrGrantRevoked
	}
	if !g.CanTransitionTo(authority.GrantStatusSuspended) {
		return nil, ErrGrantNotActive
	}

	now := time.Now().UTC()
	g.Status = authority.GrantStatusSuspended
	g.SuspendedBy = suspendedBy
	g.SuspendedAt = &now
	g.SuspendReason = reason
	g.UpdatedAt = now

	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}

	return g, nil
}

// RevokeGrant transitions an active or suspended grant to revoked.
// Revocation is permanent and cannot be undone.
func (s *GrantService) RevokeGrant(ctx context.Context, grantID, revokedBy, reason string) (*authority.AuthorityGrant, error) {
	g, err := s.repo.FindByID(ctx, grantID)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, ErrGrantNotFound
	}

	if g.Status == authority.GrantStatusRevoked {
		return nil, ErrGrantRevoked
	}
	if !g.CanTransitionTo(authority.GrantStatusRevoked) {
		return nil, ErrInvalidGrantTransition
	}

	now := time.Now().UTC()
	g.Status = authority.GrantStatusRevoked
	g.RevokedBy = revokedBy
	g.RevokedAt = &now
	g.RevokeReason = reason
	g.UpdatedAt = now

	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}

	return g, nil
}

// ReinstateGrant transitions a suspended grant back to active.
// Only suspended grants may be reinstated; revoked grants are permanent.
func (s *GrantService) ReinstateGrant(ctx context.Context, grantID, reinstatedBy string) (*authority.AuthorityGrant, error) {
	g, err := s.repo.FindByID(ctx, grantID)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, ErrGrantNotFound
	}

	if g.Status == authority.GrantStatusRevoked {
		return nil, ErrGrantRevoked
	}
	if !g.CanTransitionTo(authority.GrantStatusActive) {
		return nil, ErrGrantNotSuspended
	}

	now := time.Now().UTC()
	g.Status = authority.GrantStatusActive
	// Clear suspension fields on reinstate
	g.SuspendedBy = ""
	g.SuspendedAt = nil
	g.SuspendReason = ""
	g.UpdatedAt = now

	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}

	return g, nil
}
