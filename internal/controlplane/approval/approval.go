package approval

import (
	"strings"

	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/surface"
)

// Policy defines the standalone approval rules for governed artefacts.
type Policy struct {
	RequireDifferentApprover bool
}

// DefaultPolicy returns the MVP maker-checker policy.
func DefaultPolicy() Policy {
	return Policy{
		RequireDifferentApprover: true,
	}
}

// CanApproveSurface returns true if approver is allowed to approve the surface.
func CanApproveSurface(policy Policy, submitter identity.Principal, approver identity.Principal, s *surface.DecisionSurface) bool {
	if s == nil {
		return false
	}

	if approver.ID == "" {
		return false
	}

	if policy.RequireDifferentApprover && submitter.ID != "" && strings.EqualFold(submitter.ID, approver.ID) {
		return false
	}

	// Admin can always approve.
	if approver.HasRole(identity.RoleAdmin) {
		return true
	}

	// Approver role is required for owner-based approval.
	if !approver.HasRole(identity.RoleApprover) {
		return false
	}

	// Owner match against declared governance owners.
	if strings.EqualFold(strings.TrimSpace(approver.ID), strings.TrimSpace(s.BusinessOwner)) {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(approver.ID), strings.TrimSpace(s.TechnicalOwner)) {
		return true
	}

	return false
}
