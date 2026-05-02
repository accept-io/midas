package aisystem

import (
	"context"
	"time"
)

// AISystemBinding is the immediate-apply junction linking an AISystem
// (optionally pinned to a specific AISystemVersion) to one or more
// existing MIDAS context entities. At least one of BusinessServiceID,
// CapabilityID, ProcessID, or SurfaceID must be set
// (chk_ai_bindings_at_least_one_target).
//
// Cross-reference consistency rules (validated at apply time, not by
// the schema):
//
//  1. (surface_id, process_id) must agree if both set:
//     surfaces.process_id must equal binding.process_id.
//  2. (process_id, business_service_id) must agree if both set:
//     processes.business_service_id must equal binding.business_service_id.
//  3. (business_service_id, capability_id) must be linked through
//     business_service_capabilities.
//  4. (process_id, capability_id) must be transitively linked via the
//     process's business service through business_service_capabilities.
//  5. If AISystemVersion is set, the (AISystemID, Version) pair must
//     exist (in bundle or persisted store).
//
// SurfaceID has no FK at the schema level because surfaces are
// versioned (composite PK). Bindings reference the logical surface ID
// only.
type AISystemBinding struct {
	ID              string
	AISystemID      string
	AISystemVersion *int

	// At least one of the four context fields below must be set. They
	// are independent — a binding may target any non-empty subset.
	BusinessServiceID string
	CapabilityID      string
	ProcessID         string
	SurfaceID         string

	Role        string
	Description string

	CreatedAt time.Time
	CreatedBy string
}

// HasContextReference reports whether at least one of the four context
// fields is set. Mirrors chk_ai_bindings_at_least_one_target.
func (b *AISystemBinding) HasContextReference() bool {
	return b.BusinessServiceID != "" ||
		b.CapabilityID != "" ||
		b.ProcessID != "" ||
		b.SurfaceID != ""
}

// BindingRepository is the persistence interface for AISystemBinding.
//
// Ordering contract: List* return rows ordered by CreatedAt DESC then
// ID ASC, matching the BSR convention.
//
// Bindings have no triple-uniqueness rule: multiple bindings of the
// same AI system to the same context are permitted as long as their
// IDs (and typically their roles) differ. This is a deliberate
// departure from the BSR triple-uniqueness pattern — different roles
// of the same system against the same context are first-class.
type BindingRepository interface {
	Create(ctx context.Context, b *AISystemBinding) error
	GetByID(ctx context.Context, id string) (*AISystemBinding, error)
	List(ctx context.Context) ([]*AISystemBinding, error)
	ListByAISystem(ctx context.Context, aiSystemID string) ([]*AISystemBinding, error)
	ListByBusinessService(ctx context.Context, bsID string) ([]*AISystemBinding, error)
	ListByCapability(ctx context.Context, capID string) ([]*AISystemBinding, error)
	ListByProcess(ctx context.Context, procID string) ([]*AISystemBinding, error)
	ListBySurface(ctx context.Context, surfID string) ([]*AISystemBinding, error)
	Delete(ctx context.Context, id string) error
}
