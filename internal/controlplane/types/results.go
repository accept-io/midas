package types

// PlanEntryAction mirrors the apply.ApplyAction values for use in the HTTP
// plan response. It is intentionally a plain string type so the types package
// remains free of a dependency on the apply package.
type PlanEntryAction string

const (
	PlanEntryActionCreate    PlanEntryAction = "create"
	PlanEntryActionConflict  PlanEntryAction = "conflict"
	PlanEntryActionInvalid   PlanEntryAction = "invalid"
	PlanEntryActionUnchanged PlanEntryAction = "unchanged"
)

// PlanEntryDecisionSource records how the planner resolved the action for an
// entry. The value is informational and intended for dry-run callers.
type PlanEntryDecisionSource string

const (
	// PlanEntryDecisionSourcePersistedState means the action was determined by
	// inspecting persisted state via a repository lookup.
	PlanEntryDecisionSourcePersistedState PlanEntryDecisionSource = "persisted_state"

	// PlanEntryDecisionSourceBundleDependency means the action was determined
	// by resolving a cross-document reference against another entry in the same
	// bundle rather than persisted state.
	PlanEntryDecisionSourceBundleDependency PlanEntryDecisionSource = "bundle_dependency"

	// PlanEntryDecisionSourceValidation means the action was determined by a
	// structural or referential-integrity validation failure.
	PlanEntryDecisionSourceValidation PlanEntryDecisionSource = "validation"
)

// PlanEntry describes the planned action for a single document in a dry-run
// plan response. It carries all fields needed for a caller to understand what
// would happen and why.
type PlanEntry struct {
	// Kind is the document kind (Surface, Agent, Profile, Grant).
	Kind string `json:"kind"`

	// ID is the document metadata.id.
	ID string `json:"id"`

	// Action is the intended operation if the bundle were applied.
	Action PlanEntryAction `json:"action"`

	// DocumentIndex is the 1-based position in the original bundle.
	DocumentIndex int `json:"document_index"`

	// Message provides human-readable context for conflict and invalid entries.
	Message string `json:"message,omitempty"`

	// DecisionSource records how the planner arrived at the action.
	DecisionSource PlanEntryDecisionSource `json:"decision_source,omitempty"`

	// ValidationErrors holds structured validation errors for this entry.
	// Populated when Action is invalid.
	ValidationErrors []ValidationError `json:"validation_errors,omitempty"`
}

// PlanResult is the structured response returned by a dry-run plan request.
// It describes what would happen for each document in the bundle without
// persisting anything.
type PlanResult struct {
	// Entries describes the planned action for each document in bundle order.
	Entries []PlanEntry `json:"entries"`

	// WouldApply is true when the plan contains no invalid entries and at
	// least one create entry — i.e., applying this bundle would succeed and
	// produce at least one new resource.
	WouldApply bool `json:"would_apply"`

	// InvalidCount is the number of entries with action invalid.
	InvalidCount int `json:"invalid_count"`

	// ConflictCount is the number of entries with action conflict.
	ConflictCount int `json:"conflict_count"`

	// CreateCount is the number of entries with action create.
	CreateCount int `json:"create_count"`
}

// ValidationError represents a single validation failure for a control plane resource.
type ValidationError struct {
	Kind          string `json:"kind"`                     // Surface | Agent | Profile | Grant
	ID            string `json:"id"`                       // metadata.id from the document
	Field         string `json:"field,omitempty"`          // e.g. spec.surface_id
	Message       string `json:"message"`                  // human-readable error description
	DocumentIndex int    `json:"document_index,omitempty"` // 1-based position in a multi-document bundle
}

// ResourceStatus represents the outcome of applying a single resource.
type ResourceStatus string

const (
	ResourceStatusCreated   ResourceStatus = "created"
	ResourceStatusConflict  ResourceStatus = "conflict"
	ResourceStatusError     ResourceStatus = "error"
	ResourceStatusUnchanged ResourceStatus = "unchanged"
)

// ResourceResult represents the outcome of applying a single resource.
type ResourceResult struct {
	Kind    string         `json:"kind"`              // Surface | Agent | Profile | Grant
	ID      string         `json:"id"`                // metadata.id from the document
	Status  ResourceStatus `json:"status"`            // created | conflict | error | unchanged
	Message string         `json:"message,omitempty"` // additional context
}

// ApplyResult summarizes the outcome of applying a bundle of control plane resources.
type ApplyResult struct {
	Results          []ResourceResult  `json:"results"`
	ValidationErrors []ValidationError `json:"validation_errors,omitempty"`
}

// ---------------------------------------------------------------------------
// ApplyResult Builder Methods
// ---------------------------------------------------------------------------

// AddCreated records a successfully created resource.
func (r *ApplyResult) AddCreated(kind, id string) {
	r.Results = append(r.Results, ResourceResult{
		Kind:   kind,
		ID:     id,
		Status: ResourceStatusCreated,
	})
}

// AddConflict records a resource that already exists.
func (r *ApplyResult) AddConflict(kind, id, message string) {
	r.Results = append(r.Results, ResourceResult{
		Kind:    kind,
		ID:      id,
		Status:  ResourceStatusConflict,
		Message: message,
	})
}

// AddError records a resource that failed to apply due to an execution error.
func (r *ApplyResult) AddError(kind, id, message string) {
	r.Results = append(r.Results, ResourceResult{
		Kind:    kind,
		ID:      id,
		Status:  ResourceStatusError,
		Message: message,
	})
}

// AddFieldError records a validation error for a specific field.
func (r *ApplyResult) AddFieldError(kind, id, field, message string) {
	r.ValidationErrors = append(r.ValidationErrors, ValidationError{
		Kind:    kind,
		ID:      id,
		Field:   field,
		Message: message,
	})
}

// AddValidationError records a document-level validation error (not field-specific).
func (r *ApplyResult) AddValidationError(kind, id, message string) {
	r.ValidationErrors = append(r.ValidationErrors, ValidationError{
		Kind:    kind,
		ID:      id,
		Message: message,
	})
}

// AddUnchanged records a resource that was already in the desired state.
func (r *ApplyResult) AddUnchanged(kind, id string) {
	r.Results = append(r.Results, ResourceResult{
		Kind:   kind,
		ID:     id,
		Status: ResourceStatusUnchanged,
	})
}

// ---------------------------------------------------------------------------
// ApplyResult Query Methods
// ---------------------------------------------------------------------------

// TotalCount returns the total number of resources processed.
func (r ApplyResult) TotalCount() int {
	return len(r.Results)
}

// CreatedCount returns the number of created resources.
func (r ApplyResult) CreatedCount() int {
	count := 0
	for _, res := range r.Results {
		if res.Status == ResourceStatusCreated {
			count++
		}
	}
	return count
}

// ConflictCount returns the number of conflicting resources.
func (r ApplyResult) ConflictCount() int {
	count := 0
	for _, res := range r.Results {
		if res.Status == ResourceStatusConflict {
			count++
		}
	}
	return count
}

// ApplyErrorCount returns the number of apply-time resource errors.
func (r ApplyResult) ApplyErrorCount() int {
	count := 0
	for _, res := range r.Results {
		if res.Status == ResourceStatusError {
			count++
		}
	}
	return count
}

// ValidationErrorCount returns the number of validation errors.
func (r ApplyResult) ValidationErrorCount() int {
	return len(r.ValidationErrors)
}

// UnchangedCount returns the number of unchanged resources.
func (r ApplyResult) UnchangedCount() int {
	count := 0
	for _, res := range r.Results {
		if res.Status == ResourceStatusUnchanged {
			count++
		}
	}
	return count
}

// HasValidationErrors returns true if validation errors occurred.
func (r ApplyResult) HasValidationErrors() bool {
	return len(r.ValidationErrors) > 0
}

// IsValid returns true if no validation errors occurred.
func (r ApplyResult) IsValid() bool {
	return !r.HasValidationErrors()
}

// Success returns true if all resources were successfully applied.
//
// Phase 7 (MVP): Conflicts are treated as failures because apply is create-only.
// Phase 8+: When idempotent apply is implemented, this may treat unchanged
// resources as successful.
func (r ApplyResult) Success() bool {
	if r.HasValidationErrors() {
		return false
	}
	for _, res := range r.Results {
		if res.Status == ResourceStatusError || res.Status == ResourceStatusConflict {
			return false
		}
	}
	return true
}

// PartialSuccess returns true if at least one resource succeeded and no validation errors occurred.
func (r ApplyResult) PartialSuccess() bool {
	return r.IsValid() && r.CreatedCount() > 0
}
