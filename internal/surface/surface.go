package surface

import (
	"context"
	"time"
)

// SurfaceStatus represents the lifecycle state of a DecisionSurface.
type SurfaceStatus string

const (
	SurfaceStatusActive   SurfaceStatus = "active"
	SurfaceStatusInactive SurfaceStatus = "inactive"
	SurfaceStatusDraft    SurfaceStatus = "draft"
)

// DecisionSurface defines what is governed.
// ID is the logical identifier across versions.
// It does not carry thresholds or policy configuration.
type DecisionSurface struct {
	ID             string
	Name           string
	Domain         string
	BusinessOwner  string
	TechnicalOwner string
	Status         SurfaceStatus
	Version        int
	EffectiveDate  time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// SurfaceRepository defines persistence operations for DecisionSurface.
// Implementations live in internal/store/postgres.
type SurfaceRepository interface {
	// FindByID returns the current surface for the logical surface ID.
	FindByID(ctx context.Context, id string) (*DecisionSurface, error)

	// FindActiveAt resolves the latest active version where effective_date <= at.
	FindActiveAt(ctx context.Context, id string, at time.Time) (*DecisionSurface, error)

	Create(ctx context.Context, s *DecisionSurface) error
	Update(ctx context.Context, s *DecisionSurface) error
	List(ctx context.Context) ([]*DecisionSurface, error)
}
