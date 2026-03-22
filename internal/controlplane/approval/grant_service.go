package approval

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlaudit"
	"github.com/accept-io/midas/internal/outbox"
)

// Grant lifecycle errors.
var (
	ErrGrantNotFound          = errors.New("grant not found")
	ErrGrantNotActive         = errors.New("grant is not active")
	ErrGrantNotSuspended      = errors.New("grant is not suspended")
	ErrGrantRevoked           = errors.New("grant is permanently revoked")
	ErrInvalidGrantTransition = errors.New("invalid grant status transition")
)

// GrantRepository is the subset of authority.GrantRepository required for
// grant lifecycle operations.
type GrantRepository interface {
	FindByID(ctx context.Context, id string) (*authority.AuthorityGrant, error)
	Update(ctx context.Context, g *authority.AuthorityGrant) error
}

// GrantService orchestrates grant lifecycle governance: suspend, revoke, reinstate.
//
// If a controlaudit.Repository is provided (via NewGrantServiceFull), a control-plane
// audit record is appended after each successful lifecycle transition.
//
// If an outbox.Repository is provided, a grant lifecycle outbox event is appended
// in the same call sequence as the repository Update. For transactional atomicity,
// the GrantRepository and the outbox.Repository must be bound to the same database
// transaction by the caller.
type GrantService struct {
	repo         GrantRepository
	outbox       outbox.Repository       // nil-safe: no event emitted if nil
	controlAudit controlaudit.Repository // nil-safe: no audit record if nil
}

// NewGrantService constructs a GrantService without outbox or audit emission.
func NewGrantService(repo GrantRepository) *GrantService {
	return &GrantService{repo: repo}
}

// NewGrantServiceFull constructs a GrantService with outbox event emission and
// control-plane audit recording. Either outboxRepo or controlAuditRepo may be nil;
// nil repositories are no-ops.
func NewGrantServiceFull(repo GrantRepository, outboxRepo outbox.Repository, controlAuditRepo controlaudit.Repository) *GrantService {
	return &GrantService{
		repo:         repo,
		outbox:       outboxRepo,
		controlAudit: controlAuditRepo,
	}
}

// appendControlAudit appends a control-plane audit record. It is a no-op when
// the controlAudit repository is nil.
func (s *GrantService) appendControlAudit(ctx context.Context, rec *controlaudit.ControlAuditRecord) {
	if s.controlAudit == nil {
		return
	}
	_ = s.controlAudit.Append(ctx, rec)
}

// appendOutboxEvent constructs and appends an outbox event. It is a no-op when
// s.outbox is nil.
func (s *GrantService) appendOutboxEvent(
	ctx context.Context,
	eventType outbox.EventType,
	grantID string,
	payload []byte,
) error {
	if s.outbox == nil {
		return nil
	}
	ev, err := outbox.New(
		eventType,
		"grant",
		grantID,
		"midas.grants",
		grantID,
		payload,
	)
	if err != nil {
		return fmt.Errorf("construct outbox event %s: %w", eventType, err)
	}
	return s.outbox.Append(ctx, ev)
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

	payload, err := outbox.BuildGrantSuspendedEvent(g.ID, g.AgentID, g.ProfileID, suspendedBy, reason)
	if err != nil {
		return nil, fmt.Errorf("build outbox payload grant.suspended: %w", err)
	}
	if err := s.appendOutboxEvent(ctx, outbox.EventGrantSuspended, g.ID, payload); err != nil {
		return nil, fmt.Errorf("outbox append grant.suspended: %w", err)
	}

	s.appendControlAudit(ctx, controlaudit.NewGrantSuspendedRecord(suspendedBy, g.ID, reason))

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

	payload, err := outbox.BuildGrantRevokedEvent(g.ID, g.AgentID, g.ProfileID, revokedBy, reason)
	if err != nil {
		return nil, fmt.Errorf("build outbox payload grant.revoked: %w", err)
	}
	if err := s.appendOutboxEvent(ctx, outbox.EventGrantRevoked, g.ID, payload); err != nil {
		return nil, fmt.Errorf("outbox append grant.revoked: %w", err)
	}

	s.appendControlAudit(ctx, controlaudit.NewGrantRevokedRecord(revokedBy, g.ID, reason))

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

	payload, err := outbox.BuildGrantReinstatedEvent(g.ID, g.AgentID, g.ProfileID, reinstatedBy)
	if err != nil {
		return nil, fmt.Errorf("build outbox payload grant.reinstated: %w", err)
	}
	if err := s.appendOutboxEvent(ctx, outbox.EventGrantReinstated, g.ID, payload); err != nil {
		return nil, fmt.Errorf("outbox append grant.reinstated: %w", err)
	}

	s.appendControlAudit(ctx, controlaudit.NewGrantReinstatedRecord(reinstatedBy, g.ID))

	return g, nil
}
