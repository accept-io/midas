package process

import (
	"context"
	"time"
)

type Process struct {
	ID                string
	Name              string
	ParentProcessID   string
	BusinessServiceID string
	Description       string
	Status            string
	Origin            string
	Managed           bool
	Replaces          string
	Owner             string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	CreatedBy         string
}

type ProcessRepository interface {
	Exists(ctx context.Context, id string) (bool, error)
	GetByID(ctx context.Context, id string) (*Process, error)
	List(ctx context.Context) ([]*Process, error)
	// ListByBusinessService returns the processes whose
	// business_service_id matches the given ID, ordered by process ID.
	// Returns an empty slice when no processes match. Added in Epic 1
	// PR 4 to support the governance map read service without a full
	// table scan + in-memory filter on the caller side.
	ListByBusinessService(ctx context.Context, businessServiceID string) ([]*Process, error)
	Create(ctx context.Context, p *Process) error
	Update(ctx context.Context, p *Process) error
}
