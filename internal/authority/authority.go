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

// ProfileStatus represents the lifecycle state of an AuthorityProfile.
// Schema v2.1: Added to mirror DecisionSurface lifecycle management.
type ProfileStatus string

const (
	ProfileStatusDraft      ProfileStatus = "draft"
	ProfileStatusReview     ProfileStatus = "review"
	ProfileStatusActive     ProfileStatus = "active"
	ProfileStatusDeprecated ProfileStatus = "deprecated"
	ProfileStatusRetired    ProfileStatus = "retired"
)

// AuthorityProfile defines how much authority is granted for a given surface.
// ID is the logical identifier across versions.
// Thresholds and policy configuration live here, not on the surface or grant.
type AuthorityProfile struct {
	ID          string
	Version     int
	SurfaceID   string
	Name        string
	Description string

	// Schema v2.1: Lifecycle management
	Status         ProfileStatus
	EffectiveDate  time.Time
	EffectiveUntil *time.Time // nil = no expiration
	RetiredAt      *time.Time

	// Authority thresholds
	ConfidenceThreshold  float64
	ConsequenceThreshold Consequence

	// Policy integration
	PolicyReference string

	// Governance semantics
	EscalationMode      EscalationMode
	FailMode            FailMode
	RequiredContextKeys []string

	// Metadata
	CreatedAt  time.Time
	UpdatedAt  time.Time
	CreatedBy  string
	ApprovedBy string
	ApprovedAt *time.Time
}

// GrantStatus represents the lifecycle state of an AuthorityGrant.
type GrantStatus string

const (
	GrantStatusActive    GrantStatus = "active"
	GrantStatusSuspended GrantStatus = "suspended" // Schema v2.1: Added suspended state
	GrantStatusRevoked   GrantStatus = "revoked"
)

// AuthorityGrant is the thin link between an Agent and an AuthorityProfile.
// It carries no governance semantics beyond the link itself.
type AuthorityGrant struct {
	ID        string
	AgentID   string
	ProfileID string // Logical profile ID (not versioned)

	GrantedBy string
	Status    GrantStatus

	// Temporal scope
	EffectiveDate time.Time
	ExpiresAt     *time.Time // nil = no expiration

	// Revocation tracking
	RevokedAt *time.Time
	RevokedBy string

	// Metadata
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ProfileRepository is the persistence interface for AuthorityProfile.
// Implementations live in internal/store/postgres.
type ProfileRepository interface {
	// FindByID returns the latest version of a profile by its logical ID.
	// Use FindByIDAndVersion to retrieve a specific version.
	FindByID(ctx context.Context, id string) (*AuthorityProfile, error)

	// FindByIDAndVersion retrieves a specific profile version.
	FindByIDAndVersion(ctx context.Context, id string, version int) (*AuthorityProfile, error)

	// FindActiveAt resolves the active version where:
	//   - status = 'active'
	//   - effective_date <= at
	//   - (effective_until IS NULL OR effective_until > at)
	// Schema v2.1: Now also checks status field, not just dates.
	FindActiveAt(ctx context.Context, id string, at time.Time) (*AuthorityProfile, error)

	ListBySurface(ctx context.Context, surfaceID string) ([]*AuthorityProfile, error)

	// ListVersions returns all versions of a profile ordered by version DESC.
	ListVersions(ctx context.Context, id string) ([]*AuthorityProfile, error)

	Create(ctx context.Context, p *AuthorityProfile) error
	Update(ctx context.Context, p *AuthorityProfile) error
}

// GrantRepository is the persistence interface for AuthorityGrant.
// Implementations live in internal/store/postgres.
type GrantRepository interface {
	FindByID(ctx context.Context, id string) (*AuthorityGrant, error)

	// FindActiveByAgentAndProfile returns the active grant linking agentID to profileID, if one exists.
	// Schema v2.1: Checks status='active' AND effective_date <= now AND (expires_at IS NULL OR expires_at > now)
	FindActiveByAgentAndProfile(ctx context.Context, agentID, profileID string) (*AuthorityGrant, error)

	ListByAgent(ctx context.Context, agentID string) ([]*AuthorityGrant, error)

	Create(ctx context.Context, g *AuthorityGrant) error

	// Revoke marks a grant as revoked and records revocation metadata.
	// Schema v2.1: Sets status='revoked', revoked_at=now, revoked_by=revokerID
	Revoke(ctx context.Context, id string, revokedBy string) error

	// Suspend temporarily disables a grant without full revocation.
	// Schema v2.1: Sets status='suspended'
	Suspend(ctx context.Context, id string) error

	// Reactivate restores a suspended grant.
	// Schema v2.1: Sets status='active' (only valid from suspended state)
	Reactivate(ctx context.Context, id string) error
}
