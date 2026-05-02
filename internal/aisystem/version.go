package aisystem

import (
	"context"
	"time"

	"github.com/accept-io/midas/internal/externalref"
)

// Allowed AISystemVersion status values. The set is identical to the
// authority/control lifecycle states, but AISystemVersion is
// status-honouring (registry/evidence context, not authority artefact)
// — apply preserves whatever the bundle declares. There is no
// review-forced activation, no ApprovedBy/ApprovedAt, and no approval
// endpoint.
const (
	AISystemVersionStatusReview     = "review"
	AISystemVersionStatusActive     = "active"
	AISystemVersionStatusDeprecated = "deprecated"
	AISystemVersionStatusRetired    = "retired"
)

// IsValidAISystemVersionStatus reports whether s is one of the canonical
// AISystemVersion status values. Mirrors the schema CHECK on
// ai_system_versions.status.
func IsValidAISystemVersionStatus(s string) bool {
	switch s {
	case AISystemVersionStatusReview, AISystemVersionStatusActive,
		AISystemVersionStatusDeprecated, AISystemVersionStatusRetired:
		return true
	}
	return false
}

// AISystemVersion is a versioned snapshot of an AISystem. Composite
// primary key (AISystemID, Version). Versions are write-once on the
// (id, version) tuple; only mutable column is Status (and its companion
// EffectiveUntil / RetiredAt timestamps).
type AISystemVersion struct {
	AISystemID    string
	Version       int
	ReleaseLabel  string
	ModelArtifact string
	ModelHash     string
	Endpoint      string
	Status        string

	// EffectiveFrom is required and must be <= EffectiveUntil when both
	// are set (chk_ai_versions_effective_range).
	EffectiveFrom  time.Time
	EffectiveUntil *time.Time
	RetiredAt      *time.Time

	// ComplianceFrameworks is a free-form set of strings (e.g.
	// "iso-42001"). Stored as JSONB at the schema level; never nil at
	// the Go layer (use empty slice for "no frameworks").
	ComplianceFrameworks []string

	DocumentationURL string

	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy string

	// ExternalRef is optional structured metadata about the version in
	// an external system (Epic 1, PR 3). Nil when no external reference
	// is recorded.
	ExternalRef *externalref.ExternalRef
}

// VersionRepository is the persistence interface for AISystemVersion.
//
// Ordering contract: ListBySystem returns versions ordered by Version
// DESC (latest first), matching the partial-index pattern used by
// Surface/Profile.
type VersionRepository interface {
	Create(ctx context.Context, ver *AISystemVersion) error
	GetByIDAndVersion(ctx context.Context, aiSystemID string, version int) (*AISystemVersion, error)
	ListBySystem(ctx context.Context, aiSystemID string) ([]*AISystemVersion, error)

	// GetActiveBySystem returns the highest-Version row whose status is
	// 'active' for the given AI system. Returns nil, nil when no active
	// version exists. EffectiveFrom / EffectiveUntil bounds are not
	// applied here — callers needing point-in-time resolution can
	// filter the returned row.
	GetActiveBySystem(ctx context.Context, aiSystemID string) (*AISystemVersion, error)

	Update(ctx context.Context, ver *AISystemVersion) error
}
