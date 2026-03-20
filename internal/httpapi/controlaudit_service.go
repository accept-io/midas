package httpapi

import (
	"context"

	"github.com/accept-io/midas/internal/controlaudit"
)

// ControlAuditReadService satisfies the controlAuditService interface by
// delegating to a controlaudit.Repository.
type ControlAuditReadService struct {
	repo controlaudit.Repository
}

// NewControlAuditReadService constructs a ControlAuditReadService.
// repo must be non-nil.
func NewControlAuditReadService(repo controlaudit.Repository) *ControlAuditReadService {
	return &ControlAuditReadService{repo: repo}
}

// ListAudit returns control-plane audit records matching the filter, newest first.
func (s *ControlAuditReadService) ListAudit(ctx context.Context, f controlaudit.ListFilter) ([]*controlaudit.ControlAuditRecord, error) {
	return s.repo.List(ctx, f)
}
