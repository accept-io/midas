// Package failmode defines the FailModePolicy structural entity.
//
// A FailModePolicy is a governed, versioned policy object that declares the
// maximum permitted runtime degradation behaviour for evaluation failures by
// correctness class. Per D27j, FailModePolicy is the future first-class
// substrate for fail-mode posture; D27j-impl-1a introduces the entity in
// closed-only form with no runtime consultation.
//
// Layering: this package is structural domain, not runtime. Production code
// in internal/failmode must not import internal/decision. The five
// CorrectnessClass values share string identity with decision.FailureClass,
// but the type lives here as a local view; a drift-detection test imports
// the runtime package to assert the two sets remain equal.
//
// Closed-only invariant (D27j-impl-1a): only PermittedModeClosed (and
// PermittedModeNotApplicable for the input class) is admitted by the
// validator. Soft and open admittance is a deliberate later change gated on
// Option B durable-secondary-audit prerequisites.
package failmode

import (
	"context"
	"time"
)

// FailModePolicyStatus is the lifecycle state of a FailModePolicy version.
// The enumeration matches the authority/control artefact lifecycle used by
// AuthorityProfile and DecisionSurface (draft → review → active →
// deprecated → retired).
type FailModePolicyStatus string

const (
	FailModePolicyStatusDraft      FailModePolicyStatus = "draft"
	FailModePolicyStatusReview     FailModePolicyStatus = "review"
	FailModePolicyStatusActive     FailModePolicyStatus = "active"
	FailModePolicyStatusDeprecated FailModePolicyStatus = "deprecated"
	FailModePolicyStatusRetired    FailModePolicyStatus = "retired"
)

// CorrectnessClass groups runtime evaluation failures by governance
// correctness posture. The five values mirror decision.FailureClass; they
// are duplicated here so structural domain does not depend on runtime
// evaluation. A drift-detection test guards the two sets against divergence.
type CorrectnessClass string

const (
	CorrectnessClassGovernanceIntegrity CorrectnessClass = "governance_integrity"
	CorrectnessClassPersistence         CorrectnessClass = "persistence"
	CorrectnessClassInput               CorrectnessClass = "input"
	CorrectnessClassResource            CorrectnessClass = "resource"
	CorrectnessClassConsistency         CorrectnessClass = "consistency"
)

// PermittedMode is the runtime degradation a FailModePolicy rule permits
// for a given correctness class. D27j-impl-1a admits only Closed and
// NotApplicable through the validator; Soft and Open are reserved for a
// later tranche gated on Option B prerequisites.
type PermittedMode string

const (
	PermittedModeClosed        PermittedMode = "closed"
	PermittedModeSoft          PermittedMode = "soft"
	PermittedModeOpen          PermittedMode = "open"
	PermittedModeNotApplicable PermittedMode = "not_applicable"
)

// FailModePolicy is a governed, versioned fail-mode policy. Composite
// (ID, Version) primary key; lifecycle and ownership shape mirror
// authority.AuthorityProfile.
type FailModePolicy struct {
	ID          string
	Version     int
	Name        string
	Description string

	Status         FailModePolicyStatus
	EffectiveDate  time.Time
	EffectiveUntil *time.Time
	RetiredAt      *time.Time

	BusinessOwner  string
	TechnicalOwner string

	// Rules is exhaustive over the five CorrectnessClass values per the
	// D27j-impl-1a invariant. Persisted as JSONB at the schema level.
	Rules []FailModePolicyRule

	Origin   string // "manual" | "inferred"; default "manual"
	Managed  bool   // default true
	Replaces string // optional logical predecessor; must not equal ID

	SuccessorPolicyID string
	SuccessorVersion  int

	CreatedAt  time.Time
	UpdatedAt  time.Time
	CreatedBy  string
	ApprovedBy string
	ApprovedAt *time.Time
}

// FailModePolicyRule binds a CorrectnessClass to a PermittedMode with an
// optional free-form Reason. Per the D27j-impl-1a closed-only invariant the
// validator admits only Closed (for non-input classes) and NotApplicable
// (for input).
type FailModePolicyRule struct {
	CorrectnessClass CorrectnessClass
	PermittedMode    PermittedMode
	Reason           string
}

// CanTransitionTo reports whether a FailModePolicy may transition from its
// current Status to next. The transition map mirrors the authority/control
// lifecycle posture:
//
//   - draft      → review or retired
//   - review     → active, draft, or retired
//   - active     → deprecated or retired
//   - deprecated → retired
//   - retired    → terminal
func (p *FailModePolicy) CanTransitionTo(next FailModePolicyStatus) bool {
	switch p.Status {
	case FailModePolicyStatusDraft:
		return next == FailModePolicyStatusReview || next == FailModePolicyStatusRetired
	case FailModePolicyStatusReview:
		return next == FailModePolicyStatusActive ||
			next == FailModePolicyStatusDraft ||
			next == FailModePolicyStatusRetired
	case FailModePolicyStatusActive:
		return next == FailModePolicyStatusDeprecated || next == FailModePolicyStatusRetired
	case FailModePolicyStatusDeprecated:
		return next == FailModePolicyStatusRetired
	default:
		// retired is terminal; unknown statuses cannot transition.
		return false
	}
}

// PolicyRepository is the persistence interface for FailModePolicy.
// Semantics mirror authority.ProfileRepository:
//
//   - FindByID returns the latest version (highest Version) regardless of
//     status. Returns nil, nil when no rows exist for the id.
//   - FindByIDAndVersion returns nil, nil when the (id, version) pair does
//     not exist.
//   - FindActiveAt resolves status='active' AND effective_date <= at AND
//     (effective_until IS NULL OR effective_until > at); on multiple matches
//     (an invariant violation) it returns the highest version.
//   - ListVersions returns versions descending by Version; empty slice (not
//     nil) for an unknown id.
//   - Create appends a version; the caller assigns Version.
//   - Update replaces the matching (id, version) row in place. Memory-repo
//     posture is silent no-op on missing row (matching authority.ProfileRepo);
//     Postgres-repo posture returns a wrapped error when no row is updated.
//
// Repositories do not call Validate. Callers and tests call it explicitly.
type PolicyRepository interface {
	Create(ctx context.Context, p *FailModePolicy) error
	Update(ctx context.Context, p *FailModePolicy) error
	FindByID(ctx context.Context, id string) (*FailModePolicy, error)
	FindByIDAndVersion(ctx context.Context, id string, version int) (*FailModePolicy, error)
	FindActiveAt(ctx context.Context, id string, at time.Time) (*FailModePolicy, error)
	ListVersions(ctx context.Context, id string) ([]*FailModePolicy, error)
}
