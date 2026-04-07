// Package processbusinessservice defines the ProcessBusinessService domain model.
//
// ProcessBusinessService represents a link in the many-to-many relationship between
// a Process and a BusinessService. A Process may support multiple BusinessServices,
// and a BusinessService may be supported by multiple Processes.
//
// This is additive to the existing process.BusinessServiceID N:1 field, which
// remains valid and backward compatible.
package processbusinessservice

import (
	"context"
	"time"
)

// ProcessBusinessService represents a link between a process and a business service.
type ProcessBusinessService struct {
	ProcessID         string
	BusinessServiceID string
	CreatedAt         time.Time
}

// ProcessBusinessServiceRepository defines persistence operations for process_business_services.
type ProcessBusinessServiceRepository interface {
	Create(ctx context.Context, pbs *ProcessBusinessService) error
	ListByProcessID(ctx context.Context, processID string) ([]*ProcessBusinessService, error)
	ListByBusinessServiceID(ctx context.Context, businessServiceID string) ([]*ProcessBusinessService, error)
	Delete(ctx context.Context, processID, businessServiceID string) error
}
