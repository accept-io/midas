// Package failmode defines the FailModePolicy structural entity.
//
// A FailModePolicy is a governed, versioned policy object that declares the
// maximum permitted runtime degradation behaviour for evaluation failures by
// correctness class. Per D27j, FailModePolicy is the first-class substrate
// for fail-mode posture.
//
// Layering: this package is structural domain, not runtime. Production code
// in internal/failmode must not import internal/decision. The five
// CorrectnessClass values share string identity with decision.FailureClass,
// but the type lives here as a local view; a drift-detection test imports
// the runtime package to assert the two sets remain equal.
//
// Three-axis rule model (D29b). Each FailModePolicyRule binds a
// CorrectnessClass to three orthogonal axes:
//
//   Axis A — PermittedMode    (posture / declared maximum degradation)
//   Axis B — EnforcementState (whether the posture is recorded only,
//                              computed in dry-run, or enforced)
//   Axis C — Outcome          (what runtime decision the policy would
//                              produce if enforcement fires)
//
// D29b admits soft and open postures at declaration time but the runtime
// resolver continues to record observability metadata only — no rule
// contents are consulted by the orchestrator. The three-axis matrix is
// enforced by Validate: valid combinations differ per posture; see
// validate.go for the table.
//
// Backward compatibility. Existing closed-only policies (and JSONB rows
// without the new keys) read back as EnforcementStateEvidenceOnly plus a
// posture-implied Outcome, leaving the runtime behaviour byte-identical
// to pre-D29b. Defaults are applied during repository deserialisation and
// during Validate (against a non-mutating defaulted copy).
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

// PermittedMode is Axis A of the rule: the declared maximum runtime
// degradation permitted for a given correctness class.
//
//   - closed:         no degradation is permitted; runtime must not relax.
//   - soft:           evidence-only proceed is permitted; future enforcement
//                     may permit with explicit evidence.
//   - open:           proceed is permitted, but never silently — evidence
//                     is still recorded.
//   - not_applicable: structurally not applicable to this policy. Valid
//                     only for the input correctness class.
//
// closed does not mean "deny". It means "do not relax". The actual runtime
// outcome under closed posture is governed by EnforcementState (Axis B) and
// Outcome (Axis C). D29b admits all four values at the declaration layer;
// the runtime continues to consult nothing.
type PermittedMode string

const (
	PermittedModeClosed        PermittedMode = "closed"
	PermittedModeSoft          PermittedMode = "soft"
	PermittedModeOpen          PermittedMode = "open"
	PermittedModeNotApplicable PermittedMode = "not_applicable"
)

// EnforcementState is Axis B of the rule: whether the declared posture is
// recorded only, computed in dry-run, or enforced at runtime.
//
//   - evidence_only: resolver attaches the policy; runtime outcome is
//                    unchanged. This is the D29b default for omitted /
//                    zero values and the only state the orchestrator
//                    currently consults.
//   - dry_run:       reserved for a future tranche. Future runtime may
//                    compute the would-be enforcement decision and record
//                    it without applying the outcome.
//   - enforced:      reserved for a future tranche. Future runtime may
//                    apply the configured Outcome.
//
// D29b admits and persists this enum but does NOT introduce dry-run or
// enforcement logic. The runtime path is unchanged.
type EnforcementState string

const (
	EnforcementStateEvidenceOnly EnforcementState = "evidence_only"
	EnforcementStateDryRun       EnforcementState = "dry_run"
	EnforcementStateEnforced     EnforcementState = "enforced"
)

// Outcome is Axis C of the rule: the runtime decision the policy would
// produce if enforcement fires.
//
//   - deny:                  future enforcement may reject the decision.
//   - escalate:              future enforcement may escalate the decision.
//   - permit_with_evidence:  future enforcement may allow the decision
//                            but record explicit evidence.
//   - manual_review:         future enforcement may place the decision
//                            into a manual-review posture.
//
// D29b admits and persists this enum but does NOT apply outcomes. The
// runtime path is unchanged. Outcome is ignored while EnforcementState is
// evidence_only.
type Outcome string

const (
	OutcomeDeny               Outcome = "deny"
	OutcomeEscalate           Outcome = "escalate"
	OutcomePermitWithEvidence Outcome = "permit_with_evidence"
	OutcomeManualReview       Outcome = "manual_review"
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

// FailModePolicyRule binds a CorrectnessClass to a three-axis posture
// (Axis A = PermittedMode, Axis B = EnforcementState, Axis C = Outcome)
// with an optional free-form Reason.
//
// Defaulting (D29b). EnforcementState and Outcome are optional at the
// document and JSONB layers; omitted/zero values resolve to:
//
//   EnforcementState: empty → evidence_only
//   Outcome (when PermittedMode is):
//     closed         → escalate
//     soft           → permit_with_evidence
//     open           → permit_with_evidence
//     not_applicable → escalate (ignored while evidence_only)
//
// Defaults are applied non-mutatingly inside Validate (against a copy)
// and at the Postgres deserialisation layer so legacy JSONB rows load
// with effective values.
type FailModePolicyRule struct {
	CorrectnessClass CorrectnessClass
	PermittedMode    PermittedMode
	EnforcementState EnforcementState
	Outcome          Outcome
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
