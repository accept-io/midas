package surface

import (
	"context"
	"time"
)

// ---------------------------------------------------------------------------
// Surface Lifecycle
// ---------------------------------------------------------------------------

// SurfaceStatus represents the lifecycle state of a DecisionSurface.
// Surfaces follow a controlled promotion path to ensure governance quality.
type SurfaceStatus string

const (
	// SurfaceStatusDraft: initial state, definition in progress
	SurfaceStatusDraft SurfaceStatus = "draft"

	// SurfaceStatusReview: submitted for approval, awaiting governance review
	SurfaceStatusReview SurfaceStatus = "review"

	// SurfaceStatusActive: approved and available for agent authorization
	SurfaceStatusActive SurfaceStatus = "active"

	// SurfaceStatusDeprecated: still operational but superseded, use discouraged
	SurfaceStatusDeprecated SurfaceStatus = "deprecated"

	// SurfaceStatusRetired: no longer operational, historical record only
	SurfaceStatusRetired SurfaceStatus = "retired"
)

// ---------------------------------------------------------------------------
// Decision Characteristics
// ---------------------------------------------------------------------------

// DecisionType classifies the strategic level of decisions on this surface.
// Drives default escalation thresholds and review requirements.
type DecisionType string

const (
	DecisionTypeStrategic   DecisionType = "strategic"   // Long-term, high-impact, board-level
	DecisionTypeTactical    DecisionType = "tactical"    // Medium-term, program-level
	DecisionTypeOperational DecisionType = "operational" // Day-to-day, routine
)

// ReversibilityClass captures whether decisions on this surface can be undone.
// Critical for consequence evaluation and escalation policy.
type ReversibilityClass string

const (
	ReversibilityReversible              ReversibilityClass = "reversible"
	ReversibilityConditionallyReversible ReversibilityClass = "conditionally_reversible"
	ReversibilityIrreversible            ReversibilityClass = "irreversible"
)

// ---------------------------------------------------------------------------
// Context Schema
//
// LIMITATION: This schema supports flat contexts and simple validation rules.
// Nested object schemas and recursive validation are intentionally not supported
// in v0.1.0. Use flat, top-level context fields or encode complex structures as
// single validated strings (e.g., JSON blobs with external validation).
// ---------------------------------------------------------------------------

// ContextSchema defines the structure and validation rules for the context
// map required to make decisions on this surface. This makes governance
// requirements explicit and machine-verifiable.
type ContextSchema struct {
	Fields []ContextField `json:"fields"`
}

// ContextField defines a single required or optional context attribute.
//
// INVARIANTS:
//   - Type and Validation must be compatible (see ValidationRule)
//   - Object and Array types are opaque (no nested schema in v0.1.0)
//   - Example, if provided, MUST conform to Type and Validation
type ContextField struct {
	Name        string          `json:"name"`
	Type        FieldType       `json:"type"`
	Required    bool            `json:"required"`
	Description string          `json:"description"`
	Validation  *ValidationRule `json:"validation,omitempty"`

	// Example demonstrates a valid value for this field.
	// Validators MUST verify Example conforms to Type and Validation.
	// Invalid examples MUST be rejected to prevent documentation decay.
	Example any `json:"example,omitempty"`
}

// FieldType categorizes the data type of a context field.
type FieldType string

const (
	FieldTypeString  FieldType = "string"
	FieldTypeNumber  FieldType = "number"
	FieldTypeBoolean FieldType = "boolean"
	FieldTypeObject  FieldType = "object" // Opaque: no nested schema validation in v0.1.0
	FieldTypeArray   FieldType = "array"  // Opaque: no item type validation in v0.1.0
)

// ValidationRule specifies constraints on field values.
//
// COMPATIBILITY MATRIX:
//   String fields: Pattern, MinLength, MaxLength, Enum
//   Number fields: Minimum, Maximum, ExclusiveMinimum, ExclusiveMaximum
//   Array fields:  MinItems, MaxItems
//   Boolean:       No validation rules
//   Object:        No validation rules (opaque in v0.1.0)
//
// Validators MUST reject incompatible rule combinations (e.g., Pattern on number).
type ValidationRule struct {
	// String validation
	Pattern   string   `json:"pattern,omitempty"`    // regex pattern
	MinLength *int     `json:"min_length,omitempty"` // minimum string length
	MaxLength *int     `json:"max_length,omitempty"` // maximum string length
	Enum      []string `json:"enum,omitempty"`       // allowed values (exact match)

	// Number validation
	Minimum          *float64 `json:"minimum,omitempty"`
	Maximum          *float64 `json:"maximum,omitempty"`
	ExclusiveMinimum bool     `json:"exclusive_minimum,omitempty"` // true: value > min; false: value >= min
	ExclusiveMaximum bool     `json:"exclusive_maximum,omitempty"` // true: value < max; false: value <= max

	// Array validation
	MinItems *int `json:"min_items,omitempty"`
	MaxItems *int `json:"max_items,omitempty"`
}

// ---------------------------------------------------------------------------
// Consequence Type System
// ---------------------------------------------------------------------------

// ConsequenceType defines a category of impact that decisions on this surface
// can produce. Surfaces declare their consequence types; authority profiles
// set thresholds within those types.
type ConsequenceType struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	MeasureType MeasureType `json:"measure_type"`

	// Financial consequences
	Currency string `json:"currency,omitempty"` // ISO 4217 code (USD, EUR, GBP, etc.)

	// Temporal consequences (stored as hours for JSON-friendliness)
	DurationUnit DurationUnit `json:"duration_unit,omitempty"` // hours, days, months, years

	// Risk rating consequences
	RatingScale []string `json:"rating_scale,omitempty"` // e.g., [low, medium, high, critical]

	// Impact scope consequences
	ScopeScale []string `json:"scope_scale,omitempty"` // e.g., [individual, team, organization, public]

	// Validation bounds
	MinValue *float64 `json:"min_value,omitempty"`
	MaxValue *float64 `json:"max_value,omitempty"`
}

// MeasureType categorizes how consequence magnitude is measured.
type MeasureType string

const (
	MeasureTypeFinancial   MeasureType = "financial"    // monetary impact
	MeasureTypeTemporal    MeasureType = "temporal"     // time commitment/duration
	MeasureTypeRiskRating  MeasureType = "risk_rating"  // categorical risk level
	MeasureTypeImpactScope MeasureType = "impact_scope" // breadth of effect
	MeasureTypeCustom      MeasureType = "custom"       // domain-specific measure
)

// DurationUnit specifies the time unit for temporal consequences.
type DurationUnit string

const (
	DurationUnitHours  DurationUnit = "hours"
	DurationUnitDays   DurationUnit = "days"
	DurationUnitMonths DurationUnit = "months"
	DurationUnitYears  DurationUnit = "years"
)

// ---------------------------------------------------------------------------
// Consequence - Typed consequence value for validation
//
// Consequences use a tagged union structure to support both numeric and
// categorical impact types. The Value field type must match the MeasureType
// of the referenced ConsequenceType.
// ---------------------------------------------------------------------------

// Consequence represents a typed impact declaration for a decision.
type Consequence struct {
	// TypeID references a ConsequenceType.ID in the surface definition.
	// Determines which Value variant is valid.
	TypeID string `json:"type_id"`

	// Value holds the consequence magnitude in a type-appropriate format.
	// The structure depends on the ConsequenceType.MeasureType:
	//   - Financial: NumericValue (monetary amount)
	//   - Temporal: NumericValue (duration in specified units)
	//   - RiskRating: CategoricalValue (rating level)
	//   - ImpactScope: CategoricalValue (scope level)
	//   - Custom: NumericValue or CategoricalValue (domain-specific)
	Value ConsequenceValue `json:"value"`
}

// ConsequenceValue is a tagged union for consequence magnitudes.
// Exactly one field must be non-nil. Validators enforce this.
type ConsequenceValue struct {
	// Numeric holds quantitative consequences (financial, temporal).
	// Must be non-nil for MeasureTypeFinancial and MeasureTypeTemporal.
	Numeric *NumericConsequence `json:"numeric,omitempty"`

	// Categorical holds qualitative consequences (risk rating, impact scope).
	// Must be non-nil for MeasureTypeRiskRating and MeasureTypeImpactScope.
	Categorical *CategoricalConsequence `json:"categorical,omitempty"`
}

// NumericConsequence represents quantitative impact (financial amount, duration).
type NumericConsequence struct {
	// Amount is the numeric magnitude.
	Amount float64 `json:"amount"`

	// Unit overrides the default unit from ConsequenceType (optional).
	// Example: ConsequenceType specifies "USD" but this consequence is "EUR"
	Unit string `json:"unit,omitempty"`
}

// CategoricalConsequence represents qualitative impact (risk level, scope).
type CategoricalConsequence struct {
	// Level is the categorical value (e.g., "high", "critical", "organization").
	// Must be a member of ConsequenceType.RatingScale or ScopeScale.
	Level string `json:"level"`

	// Ordinal is the zero-based position in the scale (optional, for ordering).
	// Example: ["low", "medium", "high"] → "medium" has Ordinal = 1
	// Validators can populate this automatically from the scale.
	Ordinal *int `json:"ordinal,omitempty"`
}

// ---------------------------------------------------------------------------
// Evidence Requirements
// ---------------------------------------------------------------------------

// EvidenceRequirement specifies documentation or justification that must
// accompany decisions on this surface.
type EvidenceRequirement struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"` // mandatory vs recommended
	Format      string `json:"format"`   // structured_data, document_reference, attestation
}

// ---------------------------------------------------------------------------
// Policy Integration
// ---------------------------------------------------------------------------

// FailureMode determines orchestrator behavior when policy evaluation fails.
type FailureMode string

const (
	// FailureModeOpen: policy failure allows decision to proceed (logs warning)
	FailureModeOpen FailureMode = "open"

	// FailureModeClosed: policy failure escalates decision (safe default)
	FailureModeClosed FailureMode = "closed"
)

// ---------------------------------------------------------------------------
// DecisionSurface - The Complete Model
//
// FIELD MUTABILITY (complete exhaustive contract):
//
// IMMUTABLE AFTER ACTIVATION (Status == Active):
//   - ID
//   - Version
//   - RequiredContext
//   - ConsequenceTypes
//   - MinimumConfidence
//   - PolicyPackage
//   - PolicyVersion
//   - Domain
//   - Category
//   - Taxonomy
//   - DecisionType
//   - ReversibilityClass
//   - MandatoryEvidence
//   - SubjectRequired
//   - CreatedAt
//   - CreatedBy
//
// MUTABLE AFTER ACTIVATION:
//   - Name (clarifications, typo fixes)
//   - Description (documentation improvements)
//   - Tags (ad-hoc categorization)
//   - FailureMode (operational policy changes)
//   - AuditRetentionHours (compliance updates)
//   - ComplianceFrameworks (regulatory additions)
//   - Status (lifecycle progression)
//   - EffectiveUntil (retirement scheduling)
//   - DeprecationReason (deprecation documentation)
//   - SuccessorSurfaceID (migration path)
//   - SuccessorVersion (migration path)
//   - BusinessOwner (ownership transfer)
//   - TechnicalOwner (ownership transfer)
//   - Stakeholders (interest updates)
//   - DocumentationURL (doc link updates)
//   - ExternalReferences (related system updates)
//   - UpdatedAt (automatically set on update)
//   - ApprovedBy (can be set when transitioning to active)
//   - ApprovedAt (can be set when transitioning to active)
//
// VERSIONING INVARIANT:
//   Only ONE active version per logical surface ID at any instant in time.
//   Active windows (EffectiveFrom, EffectiveUntil) MUST NOT overlap.
//   Repository validation enforces non-overlapping active ranges.
// ---------------------------------------------------------------------------

// DecisionSurface is the central abstraction in MIDAS governance. It represents
// a boundary where autonomous agents make decisions - the "what" being governed.
//
// Surfaces are:
//   - Versioned: changes create new versions, preserving history
//   - Declarative: all governance requirements expressed as data
//   - Self-contained: a surface definition is sufficient to evaluate authority
//   - Discoverable: surfaces form a navigable taxonomy
//
// Example surfaces:
//   - "lending-secured-origination" (approve/deny secured loans)
//   - "payments-instant-execution" (execute real-time payments)
//   - "fraud-account-suspension" (suspend accounts for fraud)
//   - "prescription-schedule-ii" (prescribe controlled substances)
type DecisionSurface struct {
	// ---------------------------------------------------------------------------
	// CORE EVALUATION FIELDS
	// These fields drive runtime authority evaluation logic.
	// ---------------------------------------------------------------------------

	// ID is the logical surface identifier, stable across versions.
	// Format: kebab-case, e.g. "lending-secured-origination"
	ID string `json:"id"`

	// Version is the numeric version of this surface definition.
	// Versions are immutable once active. Composite key: (ID, Version)
	Version int `json:"version"`

	// RequiredContext defines the structured context schema for decisions.
	// Orchestrator validates request context against this schema.
	// IMMUTABLE after activation.
	RequiredContext ContextSchema `json:"required_context"`

	// ConsequenceTypes declares what kinds of impact decisions on this surface produce.
	// Authority profiles set thresholds within these types.
	// IMMUTABLE after activation.
	ConsequenceTypes []ConsequenceType `json:"consequence_types"`

	// MinimumConfidence is a surface-level confidence floor.
	// MUST be in range [0.0, 1.0]. Authority profiles cannot set confidence
	// thresholds below this value.
	// IMMUTABLE after activation.
	MinimumConfidence float64 `json:"minimum_confidence"`

	// PolicyPackage is the Rego package path or policy bundle identifier.
	// If empty, no policy evaluation is performed for this surface.
	// IMMUTABLE after activation.
	PolicyPackage string `json:"policy_package,omitempty"`

	// PolicyVersion pins the policy bundle version for this surface version.
	// IMMUTABLE after activation.
	PolicyVersion string `json:"policy_version,omitempty"`

	// FailureMode controls behavior when policy evaluation fails.
	// MUTABLE: operational policy can change.
	FailureMode FailureMode `json:"failure_mode"`

	// ---------------------------------------------------------------------------
	// REGISTRY & METADATA FIELDS
	// These fields support discovery, lifecycle, ownership, and compliance.
	// ---------------------------------------------------------------------------

	// Name is the human-readable surface name.
	// MUTABLE: clarifications and typo fixes allowed.
	Name string `json:"name"`

	// Description explains what decisions this surface governs.
	// MUTABLE: documentation improvements allowed.
	Description string `json:"description"`

	// Domain is the high-level business area (financial_services, healthcare, etc.)
	// IMMUTABLE after activation.
	Domain string `json:"domain"`

	// Category is the functional area within the domain (lending, payments, prescriptions)
	// IMMUTABLE after activation.
	Category string `json:"category,omitempty"`

	// Taxonomy is a hierarchical path for discovery and grouping.
	// Example: ["financial", "lending", "secured", "origination"]
	// IMMUTABLE after activation.
	Taxonomy []string `json:"taxonomy,omitempty"`

	// Tags are free-form labels for ad-hoc categorization.
	// MUTABLE: tags can be added/removed post-activation.
	Tags []string `json:"tags,omitempty"`

	// DecisionType classifies the strategic level (strategic/tactical/operational)
	// IMMUTABLE after activation.
	DecisionType DecisionType `json:"decision_type"`

	// ReversibilityClass captures undo-ability (reversible/conditionally_reversible/irreversible)
	// IMMUTABLE after activation.
	ReversibilityClass ReversibilityClass `json:"reversibility_class"`

	// MandatoryEvidence lists documentation requirements for decisions.
	// IMMUTABLE after activation.
	MandatoryEvidence []EvidenceRequirement `json:"mandatory_evidence,omitempty"`

	// AuditRetentionHours specifies how long governance envelopes must be retained.
	// Stored as integer hours for JSON-friendliness (8760 = 1 year, 87600 = 10 years).
	//
	// VALIDATION:
	//   - Zero (0) means use system default retention
	//   - Positive values must be >= 24 (minimum 1 day)
	//   - Negative values are INVALID
	//
	// MUTABLE: compliance requirements can change.
	AuditRetentionHours int `json:"audit_retention_hours,omitempty"`

	// SubjectRequired mandates that every decision must identify a subject.
	// Enables subject-based audit trails and compliance reporting.
	// IMMUTABLE after activation.
	SubjectRequired bool `json:"subject_required"`

	// ComplianceFrameworks lists applicable regulatory/compliance requirements.
	// Examples: ["SOX", "GDPR", "HIPAA", "PCI-DSS"]
	// MUTABLE: frameworks can be added as requirements evolve.
	ComplianceFrameworks []string `json:"compliance_frameworks,omitempty"`

	// Status is the current lifecycle state (draft/review/active/deprecated/retired)
	// MUTABLE: surfaces progress through lifecycle.
	Status SurfaceStatus `json:"status"`

	// EffectiveFrom is when this surface version becomes available for use.
	// IMMUTABLE after activation.
	EffectiveFrom time.Time `json:"effective_from"`

	// EffectiveUntil is when this version is retired (nil means no expiration).
	// MUTABLE: can be set to schedule deprecation/retirement.
	EffectiveUntil *time.Time `json:"effective_until,omitempty"`

	// DeprecationReason explains why this surface was deprecated (if applicable).
	// MUTABLE: set when deprecating.
	DeprecationReason string `json:"deprecation_reason,omitempty"`

	// SuccessorSurfaceID points to the replacement surface (if deprecated/retired).
	// MUTABLE: set when deprecating to guide migration.
	SuccessorSurfaceID string `json:"successor_surface_id,omitempty"`

	// SuccessorVersion points to the replacement surface version.
	// MUTABLE: set when deprecating to guide migration.
	SuccessorVersion int `json:"successor_version,omitempty"`

	// BusinessOwner is accountable for governance decisions on this surface.
	// MUTABLE: ownership can transfer.
	BusinessOwner string `json:"business_owner"`

	// TechnicalOwner maintains the surface definition and integration.
	// MUTABLE: ownership can transfer.
	TechnicalOwner string `json:"technical_owner"`

	// Stakeholders are additional parties interested in this surface.
	// MUTABLE: stakeholders can change.
	Stakeholders []string `json:"stakeholders,omitempty"`

	// DocumentationURL links to detailed surface documentation.
	// MUTABLE: documentation can be updated.
	DocumentationURL string `json:"documentation_url,omitempty"`

	// ExternalReferences links to related systems, policies, or frameworks.
	// Example: {"jira": "GOVERN-1234", "confluence": "https://..."}
	// MUTABLE: references can be updated.
	ExternalReferences map[string]string `json:"external_references,omitempty"`

	// CreatedAt is when this surface version was created.
	// IMMUTABLE: set once on creation.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when this surface version was last modified.
	// MUTABLE: automatically updated on every Update call.
	UpdatedAt time.Time `json:"updated_at"`

	// CreatedBy identifies who created this surface version.
	// IMMUTABLE: set once on creation.
	CreatedBy string `json:"created_by,omitempty"`

	// ApprovedBy identifies who approved this surface for activation.
	// MUTABLE: set when transitioning to Status == Active.
	ApprovedBy string `json:"approved_by,omitempty"`

	// ApprovedAt is when this surface was approved for activation.
	// MUTABLE: set when transitioning to Status == Active.
	ApprovedAt *time.Time `json:"approved_at,omitempty"`
}

// ---------------------------------------------------------------------------
// Repository Interface
// ---------------------------------------------------------------------------

// SurfaceRepository defines persistence operations for DecisionSurface.
// Implementations must enforce version immutability and lifecycle constraints.
type SurfaceRepository interface {
	// Create persists a new surface version. Returns error if (ID, Version) exists.
	Create(ctx context.Context, s *DecisionSurface) error

	// Update modifies mutable fields on an existing surface version.
	// See FIELD MUTABILITY contract in DecisionSurface struct documentation.
	//
	// Implementations MUST:
	//   - Reject updates to IMMUTABLE fields after Status == Active
	//   - Automatically set UpdatedAt to current timestamp
	//   - Preserve CreatedAt, CreatedBy (never change)
	//
	// Returns error if attempting to mutate immutable fields.
	Update(ctx context.Context, s *DecisionSurface) error

	// FindLatestByID returns the latest version (highest version number) for
	// the logical surface ID, regardless of status.
	FindLatestByID(ctx context.Context, id string) (*DecisionSurface, error)

	// FindByIDVersion returns a specific surface version.
	FindByIDVersion(ctx context.Context, id string, version int) (*DecisionSurface, error)

	// FindActiveAt resolves the active surface version at a given timestamp.
	// Returns the active version where EffectiveFrom <= at < EffectiveUntil.
	// Returns error if multiple active versions exist (invariant violation).
	FindActiveAt(ctx context.Context, id string, at time.Time) (*DecisionSurface, error)

	// ListVersions returns all versions of a surface, ordered by version descending.
	ListVersions(ctx context.Context, id string) ([]*DecisionSurface, error)

	// ListAll returns the latest version of each logical surface, regardless of status.
	// "Latest" is determined by MAX(version) grouped by ID.
	// Implementations MUST ensure consistent version ordering.
	ListAll(ctx context.Context) ([]*DecisionSurface, error)

	// ListByStatus returns surfaces (latest version only) with the given status.
	// "Latest" is determined by MAX(version) grouped by ID.
	// Implementations MUST ensure consistent version ordering.
	ListByStatus(ctx context.Context, status SurfaceStatus) ([]*DecisionSurface, error)

	// ListByDomain returns surfaces (latest version only) in the given domain.
	// "Latest" is determined by MAX(version) grouped by ID.
	// Implementations MUST ensure consistent version ordering.
	ListByDomain(ctx context.Context, domain string) ([]*DecisionSurface, error)

	// Search finds surfaces (latest version only) matching criteria.
	// "Latest" is determined by MAX(version) grouped by ID.
	// Implementations MUST ensure consistent version ordering.
	Search(ctx context.Context, criteria SearchCriteria) ([]*DecisionSurface, error)
}

// SearchCriteria defines filters for surface discovery.
//
// MATCHING SEMANTICS:
//   - Domain: exact match (case-insensitive)
//   - Category: exact match (case-insensitive)
//   - Tags: ANY match (surface has at least one of the specified tags)
//   - Taxonomy: prefix match (surface taxonomy starts with specified path)
//   - Status: ANY match (surface status is one of the specified values)
//
// Multiple criteria are combined with AND logic.
// Empty criteria match all surfaces.
type SearchCriteria struct {
	Domain   string          `json:"domain,omitempty"`
	Category string          `json:"category,omitempty"`
	Tags     []string        `json:"tags,omitempty"`     // ANY match
	Taxonomy []string        `json:"taxonomy,omitempty"` // Prefix match
	Status   []SurfaceStatus `json:"status,omitempty"`   // ANY match (OR)
}

// ---------------------------------------------------------------------------
// Validation Interface
//
// Validation is split into two layers:
//   1. Structural validation: pure domain logic, no persistence required
//   2. Repository-backed validation: requires lookup, enforces global invariants
// ---------------------------------------------------------------------------

// SurfaceValidator enforces structural and business-rule constraints.
// This validator operates on the surface object itself without requiring
// persistence access.
type SurfaceValidator interface {
	// ValidateSurface checks structural validity and business rules.
	//
	// STRUCTURAL CHECKS:
	//   - Non-empty required fields (ID, Name, Domain, BusinessOwner, etc.)
	//   - MinimumConfidence in range [0.0, 1.0]
	//   - AuditRetentionHours: zero or >= 24 (negative is invalid)
	//   - Compatible ValidationRule for each ContextField type
	//   - Example conforms to ContextField Type and Validation
	//   - ConsequenceType bounds: MinValue <= MaxValue
	//
	// DOES NOT CHECK:
	//   - Active version overlap (requires repository access)
	//   - Lifecycle transition validity (separate method)
	ValidateSurface(ctx context.Context, s *DecisionSurface) error

	// ValidateContext verifies that a context map conforms to the surface's schema.
	// Checks:
	//   - All required fields are present
	//   - Field types match schema
	//   - Values satisfy validation rules
	ValidateContext(ctx context.Context, s *DecisionSurface, context map[string]any) error

	// ValidateConsequence checks that a consequence conforms to surface types.
	//
	// VALIDATION RULES:
	//   1. TypeID must reference a valid ConsequenceType.ID in the surface
	//   2. Exactly one of Value.Numeric or Value.Categorical must be set
	//   3. Value variant must match ConsequenceType.MeasureType:
	//        - Financial/Temporal → Numeric required
	//        - RiskRating/ImpactScope → Categorical required
	//        - Custom → either allowed (surface defines which)
	//   4. For Numeric:
	//        - Amount must be within [MinValue, MaxValue] if bounds set
	//        - Unit, if provided, should be compatible with Currency/DurationUnit
	//   5. For Categorical:
	//        - Level must be member of RatingScale or ScopeScale
	//        - Ordinal, if provided, must match Level's position in scale
	//
	// Returns error if consequence does not conform to surface declaration.
	ValidateConsequence(ctx context.Context, s *DecisionSurface, consequence Consequence) error

	// ValidateTransition checks if a lifecycle status change is allowed.
	// Allowed transitions:
	//   draft → review → active → deprecated → retired
	//   draft → retired (cancel without activation)
	//   review → draft (return for revision)
	ValidateTransition(ctx context.Context, from, to SurfaceStatus) error
}

// SurfaceRepositoryValidator enforces repository-backed invariants.
// Constructed with a repository dependency via NewSurfaceRepositoryValidator.
type SurfaceRepositoryValidator interface {
	// ValidateActiveOverlap checks that activating this surface version would
	// not create overlapping active windows with other versions.
	//
	// INVARIANT: At most one active version per surface ID at any instant.
	//
	// Implementation:
	//   1. Query all versions of surface.ID with Status == Active
	//   2. For each active version V:
	//        - Check if [s.EffectiveFrom, s.EffectiveUntil) overlaps [V.EffectiveFrom, V.EffectiveUntil)
	//   3. Return error if any overlap detected
	//
	// Returns error if overlap would occur.
	ValidateActiveOverlap(ctx context.Context, s *DecisionSurface) error
}