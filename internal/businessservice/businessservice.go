// Package businessservice defines the BusinessService domain model.
package businessservice

import (
	"context"
	"time"

	"github.com/accept-io/midas/internal/externalref"
)

// ServiceType classifies what kind of service a BusinessService is.
type ServiceType string

const (
	ServiceTypeCustomerFacing ServiceType = "customer_facing"
	ServiceTypeInternal       ServiceType = "internal"
	ServiceTypeTechnical      ServiceType = "technical"
)

// BusinessService represents what an organization delivers.
type BusinessService struct {
	ID              string
	Name            string
	Description     string
	ServiceType     ServiceType
	RegulatoryScope string
	Status          string
	Origin          string
	Managed         bool
	Replaces        string
	OwnerID         string
	CreatedAt       time.Time
	UpdatedAt       time.Time

	// ExternalRef is optional structured metadata about the entity in an
	// external system (Epic 1, PR 3). Nil when no external reference is
	// recorded. Carries no lifecycle behaviour and does not gate apply.
	ExternalRef *externalref.ExternalRef
}

// BusinessServiceRepository defines persistence operations for business services.
type BusinessServiceRepository interface {
	Exists(ctx context.Context, id string) (bool, error)
	GetByID(ctx context.Context, id string) (*BusinessService, error)
	List(ctx context.Context) ([]*BusinessService, error)
	Create(ctx context.Context, s *BusinessService) error
	Update(ctx context.Context, s *BusinessService) error
}
