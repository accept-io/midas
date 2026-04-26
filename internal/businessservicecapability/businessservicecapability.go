// Package businessservicecapability defines the BusinessServiceCapability
// domain model — the canonical Capability ↔ BusinessService relationship in
// the v1 service-led structural model.
//
// A BusinessService is realised by one or more Capabilities, and a
// Capability can be realised by multiple BusinessServices. This is a pure
// relationship: it has no lifecycle of its own beyond the lifecycle of the
// participating BusinessService and Capability rows. Junction rows therefore
// carry no origin/managed/replaces/status fields by design.
package businessservicecapability

import (
	"context"
	"time"
)

// BusinessServiceCapability links a BusinessService to a Capability.
type BusinessServiceCapability struct {
	BusinessServiceID string
	CapabilityID      string
	CreatedAt         time.Time
}

// BusinessServiceCapabilityRepository defines persistence operations for
// the business_service_capabilities junction.
type BusinessServiceCapabilityRepository interface {
	Create(ctx context.Context, bsc *BusinessServiceCapability) error
	Exists(ctx context.Context, businessServiceID, capabilityID string) (bool, error)
	ListByBusinessServiceID(ctx context.Context, businessServiceID string) ([]*BusinessServiceCapability, error)
	ListByCapabilityID(ctx context.Context, capabilityID string) ([]*BusinessServiceCapability, error)
	Delete(ctx context.Context, businessServiceID, capabilityID string) error
}
