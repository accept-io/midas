package process

import (
	"context"
	"time"
)

type Process struct {
	ID                 string
	Name               string
	CapabilityID       string
	ParentProcessID    string
	BusinessServiceID  string
	Description        string
	Status             string
	Owner              string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CreatedBy          string
}

type ProcessRepository interface {
	Exists(ctx context.Context, id string) (bool, error)
	GetByID(ctx context.Context, id string) (*Process, error)
	List(ctx context.Context) ([]*Process, error)
	ListByCapabilityID(ctx context.Context, capabilityID string) ([]*Process, error)
	Create(ctx context.Context, p *Process) error
	Update(ctx context.Context, p *Process) error
}
