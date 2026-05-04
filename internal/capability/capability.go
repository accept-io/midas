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
	Create(ctx context.Context, c *Capability) error
	Update(ctx context.Context, c *Capability) error
}
