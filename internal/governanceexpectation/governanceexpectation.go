// Package governanceexpectation defines the GovernanceExpectation domain
// Kind: a declared rule of the form "in this scope, when these conditions
// match the arriving decision, this Surface is the one that should have
// governed it."
//
// A GovernanceExpectation has three structural components:
//
//   - Scope (ScopeKind + ScopeID): the structural-layer anchor under which
//     the expectation applies — Process, BusinessService, or Capability.
//     The matching engine indexes expectations by scope so that an
//     arriving decision can be checked against only the expectations
//     relevant to its resolved structural context.
//
//   - RequiredSurfaceID: the logical Surface that should have governed an
//     arriving decision when the expectation's conditions match in its
//     scope. References to structural entities are by logical ID, not
//     version-pinned, matching the envelope structural snapshot pattern
//     introduced by ADR-0001.
//
//   - Conditions: a typed discriminator (ConditionType) and a per-type
//     opaque payload (ConditionPayload). The closed enum ensures
//     additions are explicit code changes; the JSONB payload is
//     forward-compatible per type. The matching engine added in a later
//     issue interprets the payload per ConditionType.
//
// Lifecycle posture. GovernanceExpectations are intended to be
// review-forced by later control-plane apply work, by analogy with
// AuthorityProfile and DecisionSurface: applying a new version persists
// it in review status, and an explicit approval is required before the
// version becomes active and is considered for governance-coverage
// matching. The transition graph here is intentionally narrow — only
// review→active and active→deprecated are permitted, mirroring
// AuthorityProfile.CanTransitionTo. The status constants for draft and
// retired exist for schema-CHECK alignment with other Kinds; they are
// not reachable through the transition graph.
//
// This package only defines the domain Kind, the lifecycle status enum,
// the transition rules, the condition discriminator, the scope
// discriminator, and the Repository interface. Persistence
// implementations, control-plane apply integration, the approval API,
// the condition matching engine, coverage-event emission, and
// missing-surface detection are all scoped to subsequent issues.
package governanceexpectation

import (
	"context"
	"encoding/json"
	"time"
)

// ScopeKind identifies the structural-layer anchor an expectation applies
// under. The closed enum is enforced at the domain validator and (in a
// later issue) at the schema CHECK. There is deliberately no
// ScopeKindSurface value: the matching engine works at the Surface level
// only at evaluation time, and a Surface-scoped expectation would
// collapse into the trivial "did this Surface get invoked" check that
// RequiredSurfaceID already encodes.
type ScopeKind string

const (
	// ScopeKindProcess scopes an expectation to a single Process. The
	// matching engine considers this expectation when an arriving
	// decision resolves to a Surface inside this Process.
	ScopeKindProcess ScopeKind = "process"

	// ScopeKindBusinessService scopes an expectation to a single
	// BusinessService. The matching engine considers this expectation
	// when an arriving decision resolves to a Surface inside any
	// Process belonging to this BusinessService.
	ScopeKindBusinessService ScopeKind = "business_service"

	// ScopeKindCapability scopes an expectation to a single Capability.
	// The matching engine considers this expectation when an arriving
	// decision resolves to a Surface whose owning BusinessService is
	// linked to this Capability through the BusinessServiceCapability
	// junction.
	ScopeKindCapability ScopeKind = "capability"
)

// ConditionType is the typed discriminator on the per-expectation
// matching condition. The set is closed at the domain level; adding a
// new type is an explicit code change with its own design discussion,
// not a runtime configuration.
type ConditionType string

const (
	// ConditionTypeRiskCondition matches against the typed risk fields
	// of an arriving decision: consequence (type, amount, currency,
	// risk_rating) and confidence. The exact payload shape is
	// interpreted by the matching engine added in a later issue.
	ConditionTypeRiskCondition ConditionType = "risk_condition"
)

// ExpectationStatus mirrors the five status constants used by Surfaces
// and Profiles. The constant set aligns the schema CHECK across Kinds;
// reachability is constrained by the transition graph, not by the
// constant set. In particular, draft and retired are constants but are
// not reachable through CanTransitionTo or ValidateLifecycleTransition.
type ExpectationStatus string

const (
	ExpectationStatusDraft      ExpectationStatus = "draft"
	ExpectationStatusReview     ExpectationStatus = "review"
	ExpectationStatusActive     ExpectationStatus = "active"
	ExpectationStatusDeprecated ExpectationStatus = "deprecated"
	ExpectationStatusRetired    ExpectationStatus = "retired"
)

// GovernanceExpectation is a declared governance-coverage rule. The
// (ID, Version) composite key matches AuthorityProfile's versioning
// posture: a re-apply produces a new version of the same logical ID,
// and historical versions are retained so coverage events recorded
// against past evaluations can name which version was in effect.
type GovernanceExpectation struct {
	// Identity (composite logical-version key, like AuthorityProfile)
	ID      string
	Version int

	// Scope: where this expectation applies. ScopeKind + ScopeID
	// identify the structural anchor used by the matching engine to
	// discover expectations relevant to an arriving decision.
	ScopeKind ScopeKind
	ScopeID   string

	// RequiredSurfaceID names the logical Surface that should have
	// governed an arriving decision when this expectation's conditions
	// match in its scope. Logical ID; not version-pinned. Matches the
	// envelope structural snapshot pattern (ADR-0001) and the existing
	// authority_profiles.surface_id reference shape.
	RequiredSurfaceID string

	// Identity / docs
	Name        string
	Description string

	// Lifecycle (mirrors AuthorityProfile)
	Status         ExpectationStatus
	EffectiveDate  time.Time
	EffectiveUntil *time.Time
	RetiredAt      *time.Time

	// Matching condition. ConditionType is a closed enum;
	// ConditionPayload is per-type opaque JSON whose shape is
	// interpreted by the matching engine added in a later issue.
	ConditionType    ConditionType
	ConditionPayload json.RawMessage

	// Ownership (mirrors DecisionSurface)
	BusinessOwner  string
	TechnicalOwner string

	// Audit metadata (mirrors AuthorityProfile)
	CreatedAt  time.Time
	UpdatedAt  time.Time
	CreatedBy  string
	ApprovedBy string
	ApprovedAt *time.Time
}

// CanTransitionTo enforces the same narrow transition set as
// AuthorityProfile: review → active, active → deprecated. All other
// transitions return false. The companion package-level
// ValidateLifecycleTransition agrees with this method on every input.
func (e *GovernanceExpectation) CanTransitionTo(next ExpectationStatus) bool {
	switch e.Status {
	case ExpectationStatusReview:
		return next == ExpectationStatusActive
	case ExpectationStatusActive:
		return next == ExpectationStatusDeprecated
	default:
		return false
	}
}

// Repository defines persistence operations for GovernanceExpectation.
// Implementations live in internal/store/postgres and
// internal/store/memory, added in a subsequent issue. This package
// declares the contract only.
type Repository interface {
	// Create inserts a new (id, version) row. Returns an error if the
	// (id, version) pair already exists.
	Create(ctx context.Context, e *GovernanceExpectation) error

	// FindByID returns the latest version of an expectation by its
	// logical ID. Returns nil, nil when no expectation with that ID
	// exists. Mirrors AuthorityProfile.FindByID.
	FindByID(ctx context.Context, id string) (*GovernanceExpectation, error)

	// FindByIDAndVersion retrieves a specific (id, version) pair.
	// Returns nil, nil when the pair does not exist.
	FindByIDAndVersion(ctx context.Context, id string, version int) (*GovernanceExpectation, error)

	// ListVersions returns every version of an expectation in
	// version-DESC order (latest first). Mirrors
	// AuthorityProfile.ListVersions.
	ListVersions(ctx context.Context, id string) ([]*GovernanceExpectation, error)

	// Update persists the mutable lifecycle fields of an existing
	// (id, version) row: Status, ApprovedBy, ApprovedAt, RetiredAt,
	// UpdatedAt. Used by lifecycle transitions; non-lifecycle fields
	// are immutable per version.
	Update(ctx context.Context, e *GovernanceExpectation) error
}
