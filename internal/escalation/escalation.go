// Package escalation defines the EscalationTarget structural entity.
//
// An EscalationTarget is a governed, versioned configuration object that
// declares WHERE an authority escalation goes when an AuthorityProfile
// escalates a decision. D31k-impl-1 introduces the target as a first-
// class entity so operators can model the routing decision (role, agent,
// surface, or external system) explicitly rather than implicitly through
// downstream conventions.
//
// Scope (D31k-impl-1):
//
//   - One AuthorityProfile may reference at most one EscalationTarget via
//     AuthorityProfile.EscalationTargetID. The reference is to the
//     logical id only — version selection is the runtime resolver's job,
//     mirroring how AuthorityProfile and FailModePolicy versioning work.
//   - EscalationTarget is independent of EscalationMode (auto / manual)
//     which is preserved as an additive complement: Mode describes the
//     behaviour at the moment of escalation; Target describes the
//     destination.
//   - Multi-step escalation chains, EscalationPolicy, EscalationRule, and
//     Authority Graph projection of targets are explicitly deferred.
//
// Layering: this package is structural domain, not runtime. Production
// code in internal/escalation must not import internal/decision or any
// other runtime package. Repositories live in internal/store/{memory,
// postgres}; the runtime resolver lives in internal/decision.
package escalation

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Kind classifies the destination of an escalation.
//
//   - role:     identity role name (Handle = role identifier,
//               e.g. "governance.approver").
//   - agent:    a registered agent (Handle = agent id).
//   - surface:  a decision surface (Handle = surface id).
//   - external: an external system or URI identifier (Handle = the
//               external identifier).
//
// Cross-context existence (e.g. that an agent or surface with the
// referenced handle actually exists) is NOT validated at the
// repository layer — repositories must not create cross-context
// dependencies. Higher-level services or apply-time validators may
// optionally check existence; D31k-impl-1 defers this.
type Kind string

const (
	KindRole     Kind = "role"
	KindAgent    Kind = "agent"
	KindSurface  Kind = "surface"
	KindExternal Kind = "external"
)

// Status is the lifecycle state of an EscalationTarget version. The
// five values mirror authority.AuthorityProfile, surface.DecisionSurface,
// and failmode.FailModePolicyStatus — every governed configuration
// entity in MIDAS follows the same draft → review → active →
// deprecated → retired progression.
type Status string

const (
	StatusDraft      Status = "draft"
	StatusReview     Status = "review"
	StatusActive     Status = "active"
	StatusDeprecated Status = "deprecated"
	StatusRetired    Status = "retired"
)

// ErrInvalidEscalationTarget is wrapped by Validate for any shape-level
// validation failure. Callers should use errors.Is to discriminate
// validation errors from infrastructure errors.
var ErrInvalidEscalationTarget = errors.New("invalid escalation target")

// EscalationTarget is a governed, versioned escalation target. The
// composite (ID, Version) is the primary key in persistence and the
// natural identity for runtime resolution: FindActiveAt picks the
// highest active version whose effective window covers the moment of
// escalation.
type EscalationTarget struct {
	ID          string
	Version     int
	Name        string
	Description string

	// Kind discriminates Handle's meaning. See the Kind comment for
	// per-variant semantics.
	Kind   Kind
	Handle string

	// Status is the lifecycle state of this version. Only Status=active
	// versions participate in runtime resolution.
	Status Status

	// EffectiveDate is the inclusive lower bound of the version's
	// validity window. EffectiveUntil is the exclusive upper bound;
	// when nil, the version has no expiry.
	EffectiveDate  time.Time
	EffectiveUntil *time.Time

	// Ownership metadata mirrors the convention used by every other
	// governed configuration entity in MIDAS.
	BusinessOwner  string
	TechnicalOwner string

	CreatedAt  time.Time
	UpdatedAt  time.Time
	CreatedBy  string
	ApprovedBy string
	ApprovedAt *time.Time
}

// IsValid reports whether k is one of the canonical Kind values.
func (k Kind) IsValid() bool {
	switch k {
	case KindRole, KindAgent, KindSurface, KindExternal:
		return true
	}
	return false
}

// IsValid reports whether s is one of the canonical Status values.
func (s Status) IsValid() bool {
	switch s {
	case StatusDraft, StatusReview, StatusActive, StatusDeprecated, StatusRetired:
		return true
	}
	return false
}

// Validate reports whether the EscalationTarget shape is consistent.
// Repository implementations should call Validate at construction-time
// boundaries (Create/Update) so persistence does not silently store
// malformed records.
//
// Validation rules:
//
//   - ID must be non-empty.
//   - Version must be > 0.
//   - Name must be non-empty.
//   - Kind must be a canonical value.
//   - Handle must be non-empty.
//   - Status must be a canonical value.
//   - EffectiveDate must be non-zero.
//   - EffectiveUntil, when present, must be strictly after EffectiveDate.
//
// Ownership and audit fields are advisory; their absence is not a
// shape error. The repository layer does not enforce status-transition
// rules — that posture mirrors AuthorityProfile and FailModePolicy.
func (t *EscalationTarget) Validate() error {
	if t == nil {
		return fmt.Errorf("%w: nil target", ErrInvalidEscalationTarget)
	}
	if t.ID == "" {
		return fmt.Errorf("%w: id is empty", ErrInvalidEscalationTarget)
	}
	if t.Version <= 0 {
		return fmt.Errorf("%w: version %d must be > 0", ErrInvalidEscalationTarget, t.Version)
	}
	if t.Name == "" {
		return fmt.Errorf("%w: name is empty", ErrInvalidEscalationTarget)
	}
	if !t.Kind.IsValid() {
		return fmt.Errorf("%w: kind %q is not canonical", ErrInvalidEscalationTarget, string(t.Kind))
	}
	if t.Handle == "" {
		return fmt.Errorf("%w: handle is empty", ErrInvalidEscalationTarget)
	}
	if !t.Status.IsValid() {
		return fmt.Errorf("%w: status %q is not canonical", ErrInvalidEscalationTarget, string(t.Status))
	}
	if t.EffectiveDate.IsZero() {
		return fmt.Errorf("%w: effective_date is zero", ErrInvalidEscalationTarget)
	}
	if t.EffectiveUntil != nil && !t.EffectiveUntil.After(t.EffectiveDate) {
		return fmt.Errorf("%w: effective_until must be after effective_date", ErrInvalidEscalationTarget)
	}
	return nil
}

// CanTransitionTo reports whether the target may transition from its
// current Status to next. The transition map mirrors the same
// lifecycle posture used by AuthorityProfile, DecisionSurface, and
// FailModePolicy:
//
//   - draft      → review or retired
//   - review     → active, draft, or retired
//   - active     → deprecated or retired
//   - deprecated → retired
//   - retired    → terminal
func (t *EscalationTarget) CanTransitionTo(next Status) bool {
	switch t.Status {
	case StatusDraft:
		return next == StatusReview || next == StatusRetired
	case StatusReview:
		return next == StatusActive || next == StatusDraft || next == StatusRetired
	case StatusActive:
		return next == StatusDeprecated || next == StatusRetired
	case StatusDeprecated:
		return next == StatusRetired
	default:
		return false
	}
}

// Repository is the persistence interface for EscalationTarget. Read
// semantics mirror failmode.PolicyRepository and authority.ProfileRepository:
//
//   - FindByID returns the latest version (highest Version) regardless of
//     status. Returns nil, nil when no rows exist for the id.
//   - FindByIDAndVersion returns nil, nil when the (id, version) pair
//     does not exist.
//   - FindActiveAt resolves Status==active AND EffectiveDate <= at AND
//     (EffectiveUntil IS NULL OR EffectiveUntil > at); on multiple
//     matches (an invariant violation) the highest Version is
//     returned.
//   - List returns one row per logical id, the latest version of each,
//     sorted by id ascending. Use it as the read source for
//     /v1/escalation-targets.
//   - ListVersions returns versions descending by Version; empty slice
//     (not nil) for an unknown id.
//   - Create appends a version; the caller assigns Version.
//   - Update replaces the matching (id, version) row in place. Memory
//     posture is silent no-op on missing row (matching
//     authority.ProfileRepo); Postgres posture returns a wrapped error
//     when no row is updated.
//
// Repositories do not call Validate. Callers and tests call it
// explicitly when they want shape enforcement at the boundary.
type Repository interface {
	Create(ctx context.Context, target *EscalationTarget) error
	Update(ctx context.Context, target *EscalationTarget) error
	FindByID(ctx context.Context, id string) (*EscalationTarget, error)
	FindByIDAndVersion(ctx context.Context, id string, version int) (*EscalationTarget, error)
	FindActiveAt(ctx context.Context, id string, at time.Time) (*EscalationTarget, error)
	List(ctx context.Context) ([]*EscalationTarget, error)
	ListVersions(ctx context.Context, id string) ([]*EscalationTarget, error)
}
