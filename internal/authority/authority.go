package authority

import (
	"context"
	"time"

	"github.com/accept-io/midas/internal/value"
)

// Consequence is a typed value object representing the impact threshold of authority.
// Exactly one variant is populated based on Type.
type Consequence struct {
	Type value.ConsequenceType

	// Monetary variant
	Amount   float64
	Currency string

	// Risk rating variant
	RiskRating value.RiskRating
}

// EscalationMode controls how an out-of-authority decision is handled.
type EscalationMode string

const (
	EscalationModeAuto   EscalationMode = "auto"
	EscalationModeManual EscalationMode = "manual"
)

// FailMode controls what happens when the policy evaluator returns an error.
type FailMode string

const (
	FailModeOpen   FailMode = "open"
	FailModeClosed FailMode = "closed"
)

// AuthorityProfile defines how much authority is granted for a given surface.
// ID is the logical identifier across versions.
// Thresholds and policy configuration live here, not on the surface or grant.
type AuthorityProfile struct {
	ID                   string
	SurfaceID            string
	Name                 string
	ConfidenceThreshold  float64
	ConsequenceThreshold Consequence
	PolicyReference      string
	EscalationMode       EscalationMode
	FailMode             FailMode
	RequiredContextKeys  []string
	Version              int
	EffectiveDate        time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// GrantStatus represents the lifecycle state of an AuthorityGrant.
type GrantStatus string

const (
	GrantStatusActive  GrantStatus = "active"
	GrantStatusRevoked GrantStatus = "revoked"
	GrantStatusExpired GrantStatus = "expired"
)

// AuthorityGrant is the thin link between an Agent and an AuthorityProfile.
// It carries no governance semantics beyond the link itself.
type AuthorityGrant struct {
	ID            string
	AgentID       string
	ProfileID     string
	GrantedBy     string
	EffectiveDate time.Time
	Status        GrantStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ProfileRepository is the persistence interface for AuthorityProfile.
// Implementations live in internal/store/postgres.
type ProfileRepository interface {
	FindByID(ctx context.Context, id string) (*AuthorityProfile, error)

	// FindActiveAt resolves the latest active version where effective_date <= at.
	FindActiveAt(ctx context.Context, id string, at time.Time) (*AuthorityProfile, error)

	ListBySurface(ctx context.Context, surfaceID string) ([]*AuthorityProfile, error)

	Create(ctx context.Context, p *AuthorityProfile) error
	Update(ctx context.Context, p *AuthorityProfile) error
}

// GrantRepository is the persistence interface for AuthorityGrant.
// Implementations live in internal/store/postgres.
type GrantRepository interface {
	FindByID(ctx context.Context, id string) (*AuthorityGrant, error)

	// FindActiveByAgentAndProfile returns the active grant linking agentID to profileID, if one exists.
	FindActiveByAgentAndProfile(ctx context.Context, agentID, profileID string) (*AuthorityGrant, error)

	ListByAgent(ctx context.Context, agentID string) ([]*AuthorityGrant, error)

	Create(ctx context.Context, g *AuthorityGrant) error
	Revoke(ctx context.Context, id string) error
}
