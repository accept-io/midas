// Package processcapability defines the ProcessCapability domain model.
package processcapability

import (
	"context"
	"time"
)

// ProcessCapability represents a link between a process and a capability.
type ProcessCapability struct {
	ProcessID    string
	CapabilityID string
	CreatedAt    time.Time
}

// ProcessCapabilityRepository defines persistence operations for process_capabilities.
type ProcessCapabilityRepository interface {
	Create(ctx context.Context, pc *ProcessCapability) error
	ListByProcessID(ctx context.Context, processID string) ([]*ProcessCapability, error)
	ListByCapabilityID(ctx context.Context, capabilityID string) ([]*ProcessCapability, error)
	Delete(ctx context.Context, processID, capabilityID string) error
}
