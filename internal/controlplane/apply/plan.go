package apply

import (
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

// ---------------------------------------------------------------------------
// Dry-run output enrichments (Issue #37)
//
// These types are additive extensions to the existing plan output. They are
// not consulted by the apply executor and have no effect on apply semantics.
// Warnings never affect would_apply, invalid_count, or conflict_count.
// ---------------------------------------------------------------------------

// WarningCode is a machine-stable identifier for a plan-time warning. The
// set is intentionally small. Add new codes explicitly; do not derive them
// from free text.
type WarningCode string

const (
	// WarningRefSurfaceTerminal fires when a Profile references a Surface
	// whose latest version is in a terminal lifecycle state (deprecated or
	// retired). The reference is accepted but flagged for review.
	WarningRefSurfaceTerminal WarningCode = "REF_SURFACE_TERMINAL"

	// WarningRefProfileTerminal fires when a Grant references a Profile
	// whose latest version is in a terminal lifecycle state.
	WarningRefProfileTerminal WarningCode = "REF_PROFILE_TERMINAL"

	// WarningRefProcessTerminal fires when a Surface references a Process
	// whose status is deprecated (the Process model has no retired state).
	WarningRefProcessTerminal WarningCode = "REF_PROCESS_TERMINAL"

	// WarningRefCapabilityTerminal fires when a Process references a
	// Capability whose status is deprecated.
	WarningRefCapabilityTerminal WarningCode = "REF_CAPABILITY_TERMINAL"
)

// WarningSeverity classifies the urgency of a warning. This PR uses a single
// value; the type is a string alias to leave room for future additions
// without breaking the wire shape.
type WarningSeverity string

const (
	WarningSeverityWarning WarningSeverity = "warning"
)

// PlanWarning is an advisory signal attached to a plan entry. It is
// informational: it does not change the entry's action, does not count
// toward invalid_count, and does not affect would_apply.
type PlanWarning struct {
	// Code is a machine-stable identifier. See WarningCode constants.
	Code WarningCode

	// Severity is currently always WarningSeverityWarning.
	Severity WarningSeverity

	// Message is a human-readable explanation.
	Message string

	// Field is the optional spec path of the referring field on the entry
	// (e.g. "spec.surface_id"). Empty when the warning is not field-scoped.
	Field string

	// RelatedKind and RelatedID identify the referenced resource the warning
	// speaks about. Both are empty when the warning does not refer to
	// another resource.
	RelatedKind string
	RelatedID   string
}

// CreateKind classifies a create-action entry relative to persisted state.
// It is populated only when Action == ApplyActionCreate.
type CreateKind string

const (
	// CreateKindNew means the planner found no prior row for this resource.
	CreateKindNew CreateKind = "new"

	// CreateKindNewVersion means the planner found an existing versioned
	// lineage; the create will append a new version (Surface or Profile).
	CreateKindNewVersion CreateKind = "new_version"
)

// FieldDiff describes a single changed field between the persisted
// active-state object and the proposed new version.
type FieldDiff struct {
	// Field is the dotted path of the changed field (e.g. "spec.minimum_confidence").
	Field string

	// Before is the value currently in persisted state. Rendered as a JSON
	// value; may be a string, number, bool, array, or object depending on
	// the field.
	Before any

	// After is the proposed value in the new version.
	After any
}

// PlanDiff carries field-level changes between the active-state baseline
// and the proposed new version. Emitted only for CreateKindNewVersion on
// Surface and Profile entries in this release.
type PlanDiff struct {
	// Fields is the ordered list of changed fields. Empty when the
	// proposed version is structurally equal to the active baseline on
	// every compared field.
	Fields []FieldDiff
}

// ApplyAction describes the intended outcome for a single resource in a plan.
type ApplyAction string

const (
	// ApplyActionCreate indicates the resource does not yet exist in persisted
	// state in its target form and will be created by the executor.
	ApplyActionCreate ApplyAction = "create"

	// ApplyActionUnchanged indicates the resource is already represented in
	// persisted state such that applying it would produce no effective change.
	// The executor skips persistence for unchanged entries.
	//
	// Surface documents are never planned as unchanged: the governance model
	// creates a new versioned record on every valid submission. Unchanged is
	// available for other resource types when repository-backed inspection
	// supports it.
	ApplyActionUnchanged ApplyAction = "unchanged"

	// ApplyActionConflict indicates the resource collides with persisted state
	// in a way that apply cannot silently resolve. The executor records a
	// conflict result without persisting the resource.
	//
	// For surfaces, a conflict is raised when the latest persisted version is
	// already in review status, because applying again before the pending
	// governance review is resolved would create an ambiguous state.
	ApplyActionConflict ApplyAction = "conflict"

	// ApplyActionInvalid indicates the resource failed validation. Invalid
	// entries cause the entire bundle to be rejected; no resources are persisted.
	ApplyActionInvalid ApplyAction = "invalid"
)

// DecisionSource records how the planner resolved the action for an entry.
// It tells callers whether the decision came from a persisted-state lookup,
// same-bundle dependency resolution, or a local validation failure.
type DecisionSource string

const (
	// DecisionSourcePersistedState indicates the action was determined by
	// inspecting persisted state via a repository lookup.
	DecisionSourcePersistedState DecisionSource = "persisted_state"

	// DecisionSourceBundleDependency indicates the action was determined by
	// resolving a reference against another entry in the same bundle rather
	// than persisted state.
	DecisionSourceBundleDependency DecisionSource = "bundle_dependency"

	// DecisionSourceValidation indicates the action was determined by
	// structural or referential-integrity validation failure without any
	// repository lookup being relevant to the outcome.
	DecisionSourceValidation DecisionSource = "validation"
)

// ApplyPlanEntry describes the planned action for a single document.
type ApplyPlanEntry struct {
	// Kind is the document kind (Surface, Agent, Profile, Grant).
	Kind string

	// ID is the document metadata.id.
	ID string

	// Action is the intended operation.
	Action ApplyAction

	// DocumentIndex is the 1-based position in the original bundle.
	DocumentIndex int

	// ValidationErrors holds structured validation errors for this entry.
	// Only populated when Action is ApplyActionInvalid.
	ValidationErrors []types.ValidationError

	// Message provides additional human-readable context, populated for
	// conflict and invalid entries, and for profile version-create entries.
	Message string

	// DecisionSource records how the planner arrived at the action. This
	// field is informational and intended for dry-run callers that need to
	// understand the rationale for each planned action.
	DecisionSource DecisionSource

	// NewVersion is the version number that the executor will assign when
	// persisting this entry. It is only meaningful for Profile entries with
	// Action == ApplyActionCreate; it is zero for all other resource kinds.
	//
	// Version 1 means the profile is being created for the first time.
	// Version > 1 means a new version is being appended to an existing
	// profile lineage.
	NewVersion int

	// CreateKind classifies the create. Populated only when
	// Action == ApplyActionCreate. See CreateKind constants.
	CreateKind CreateKind

	// Warnings carries advisory warnings attached to this entry. Never
	// affects apply or the entry's action.
	Warnings []PlanWarning

	// Diff carries field-level changes between the persisted active-state
	// object and the proposed new version. Populated only for
	// CreateKindNewVersion on Surface and Profile entries.
	Diff *PlanDiff

	// Doc is the underlying parsed document, available for the executor phase.
	Doc parser.ParsedDocument
}

// AddWarning appends a warning to this entry. Convenience helper that
// preserves the single construction site for plan warnings.
func (e *ApplyPlanEntry) AddWarning(w PlanWarning) {
	e.Warnings = append(e.Warnings, w)
}

// ApplyPlan holds the full set of planned actions for a bundle before execution.
type ApplyPlan struct {
	Entries []ApplyPlanEntry
}

// HasInvalid returns true if any entry has an invalid action.
func (p ApplyPlan) HasInvalid() bool {
	for _, e := range p.Entries {
		if e.Action == ApplyActionInvalid {
			return true
		}
	}
	return false
}

// HasConflict returns true if any entry has a conflict action.
func (p ApplyPlan) HasConflict() bool {
	for _, e := range p.Entries {
		if e.Action == ApplyActionConflict {
			return true
		}
	}
	return false
}
