package capability

import (
	"context"
	"time"
)

type Capability struct {
	ID                string
	Name              string
	Description       string
	Status            string
	Origin            string
	Managed           bool
	Replaces          string
	Owner             string
	ParentCapabilityID string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	CreatedBy         string
}

type CapabilityRepository interface {
	Exists(ctx context.Context, id string) (bool, error)
	GetByID(ctx context.Context, id string) (*Capability, error)
	List(ctx context.Context) ([]*Capability, error)
	Create(ctx context.Context, c *Capability) error
	Update(ctx context.Context, c *Capability) error
}
