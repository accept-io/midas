package apply

import (
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

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

	// Doc is the underlying parsed document, available for the executor phase.
	Doc parser.ParsedDocument
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
