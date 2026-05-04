package capability

import (
	"context"
	"time"

	"github.com/accept-io/midas/internal/externalref"
)

type Capability struct {
	ID                 string
	Name               string
	Description        string
	Status             string
	Origin             string
	Managed            bool
	Replaces           string
	Owner              string
	ParentCapabilityID string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CreatedBy          string

	// ExternalRef is optional structured metadata about the entity in
	// an external system (Phase 0B-2 — extends the Epic 1 PR 3 pattern
	// to Capability). Nil when no external reference is recorded.
	// Carries no lifecycle behaviour and does not gate apply.
	ExternalRef *externalref.ExternalRef
}

type CapabilityRepository interface {
	Exists(ctx context.Context, id string) (bool, error)
	GetByID(ctx context.Context, id string) (*Capability, error)
	List(ctx context.Context) ([]*Capability, error)
	// ListByParentCapabilityID returns the direct children of the given
	// parent capability. Recursive descendants are NOT returned — a
	// caller that wants the full subtree must walk the hierarchy
	// itself. Returns an empty (non-nil) slice when no children exist
	// or when parentID is empty; the method does not validate that the
	// parent itself exists. Results are ordered by capability_id
	// ascending so consumers see a deterministic list.
	ListByParentCapabilityID(ctx context.Context, parentID string) ([]*Capability, error)
	Create(ctx context.Context, c *Capability) error
	Update(ctx context.Context, c *Capability) error
}
