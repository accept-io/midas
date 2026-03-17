package approval

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/surface"
)

var (
	ErrSurfaceNotFound   = errors.New("surface not found")
	ErrApprovalForbidden = errors.New("approval forbidden")
	ErrInvalidStatus     = errors.New("surface is not awaiting approval")
)

type SurfaceRepository interface {
	FindLatestByID(ctx context.Context, id string) (*surface.DecisionSurface, error)
	Update(ctx context.Context, s *surface.DecisionSurface) error
}

type Service struct {
	repo   SurfaceRepository
	policy Policy
}

func NewService(repo SurfaceRepository, policy Policy) *Service {
	return &Service{
		repo:   repo,
		policy: policy,
	}
}

// ApproveSurface promotes a surface from review to active.
func (s *Service) ApproveSurface(ctx context.Context, surfaceID string, submitter identity.Principal, approver identity.Principal) (*surface.DecisionSurface, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("approval repository not configured")
	}

	current, err := s.repo.FindLatestByID(ctx, surfaceID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrSurfaceNotFound
	}

	if current.Status != surface.SurfaceStatusReview && current.Status != surface.SurfaceStatusDraft {
		return nil, ErrInvalidStatus
	}

	if !CanApproveSurface(s.policy, submitter, approver, current) {
		return nil, ErrApprovalForbidden
	}

	now := time.Now().UTC()
	current.Status = surface.SurfaceStatusActive
	current.ApprovedBy = approver.ID
	current.ApprovedAt = &now

	// If not already set, activate immediately.
	if current.EffectiveFrom.IsZero() {
		current.EffectiveFrom = now
	}
	current.UpdatedAt = now

	if err := s.repo.Update(ctx, current); err != nil {
		return nil, err
	}

	return current, nil
}
