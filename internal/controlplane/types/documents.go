package types

import "time"

// API version and resource kind constants for MIDAS control plane.
const (
	APIVersionV1 = "midas.accept.io/v1"

	KindSurface = "Surface"
	KindAgent   = "Agent"
	KindProfile = "Profile"
	KindGrant   = "Grant"
)

// Document is the common interface implemented by all control plane documents.
type Document interface {
	GetKind() string
	GetID() string
}

// DocumentMetadata contains common metadata fields for all control plane resources.
type DocumentMetadata struct {
	ID     string            `json:"id" yaml:"id"`
	Name   string            `json:"name,omitempty" yaml:"name,omitempty"`
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// ---------------------------------------------------------------------------
// Surface
// ---------------------------------------------------------------------------

// SurfaceDocument defines a governed action boundary.
type SurfaceDocument struct {
	APIVersion string           `json:"apiVersion" yaml:"apiVersion"`
	Kind       string           `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata `json:"metadata" yaml:"metadata"`
	Spec       SurfaceSpec      `json:"spec" yaml:"spec"`
}

// SurfaceSpec contains the specification for a decision surface.
//
// This schema is intentionally richer than the original MVP shape so that the
// control-plane document can represent a governed, versioned decision surface
// more faithfully.
type SurfaceSpec struct {
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Registry / classification
	Domain   string   `json:"domain,omitempty" yaml:"domain,omitempty"`
	Category string   `json:"category,omitempty" yaml:"category,omitempty"`   // financial | customer_data | compliance | operational
	RiskTier string   `json:"risk_tier,omitempty" yaml:"risk_tier,omitempty"` // high | medium | low
	Taxonomy []string `json:"taxonomy,omitempty" yaml:"taxonomy,omitempty"`
	Tags     []string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// Decision characteristics
	DecisionType       string  `json:"decision_type,omitempty" yaml:"decision_type,omitempty"`             // strategic | tactical | operational
	ReversibilityClass string  `json:"reversibility_class,omitempty" yaml:"reversibility_class,omitempty"` // reversible | conditionally_reversible | irreversible
	MinimumConfidence  float64 `json:"minimum_confidence,omitempty" yaml:"minimum_confidence,omitempty"`   // 0.0 - 1.0
	SubjectRequired    bool    `json:"subject_required,omitempty" yaml:"subject_required,omitempty"`

	// Policy integration
	PolicyPackage string `json:"policy_package,omitempty" yaml:"policy_package,omitempty"`
	PolicyVersion string `json:"policy_version,omitempty" yaml:"policy_version,omitempty"`
	FailureMode   string `json:"failure_mode,omitempty" yaml:"failure_mode,omitempty"` // closed | open

	// Ownership / governance
	BusinessOwner  string   `json:"business_owner,omitempty" yaml:"business_owner,omitempty"`
	TechnicalOwner string   `json:"technical_owner,omitempty" yaml:"technical_owner,omitempty"`
	Stakeholders   []string `json:"stakeholders,omitempty" yaml:"stakeholders,omitempty"`

	// Evidence / compliance
	MandatoryEvidence    []EvidenceRequirement `json:"mandatory_evidence,omitempty" yaml:"mandatory_evidence,omitempty"`
	AuditRetentionHours  int                   `json:"audit_retention_hours,omitempty" yaml:"audit_retention_hours,omitempty"`
	ComplianceFrameworks []string              `json:"compliance_frameworks,omitempty" yaml:"compliance_frameworks,omitempty"`

	// Runtime evaluation inputs
	RequiredContext  ContextSchema     `json:"required_context,omitempty" yaml:"required_context,omitempty"`
	ConsequenceTypes []ConsequenceType `json:"consequence_types,omitempty" yaml:"consequence_types,omitempty"`

	// Lifecycle
	Status             string     `json:"status,omitempty" yaml:"status,omitempty"` // draft | review | active | deprecated | retired
	EffectiveFrom      time.Time  `json:"effective_from,omitempty" yaml:"effective_from,omitempty"`
	EffectiveUntil     *time.Time `json:"effective_until,omitempty" yaml:"effective_until,omitempty"`
	DeprecationReason  string     `json:"deprecation_reason,omitempty" yaml:"deprecation_reason,omitempty"`
	SuccessorSurfaceID string     `json:"successor_surface_id,omitempty" yaml:"successor_surface_id,omitempty"`
	SuccessorVersion   int        `json:"successor_version,omitempty" yaml:"successor_version,omitempty"`

	// Documentation / references
	DocumentationURL   string            `json:"documentation_url,omitempty" yaml:"documentation_url,omitempty"`
	ExternalReferences map[string]string `json:"external_references,omitempty" yaml:"external_references,omitempty"`
}

// ContextSchema defines the structure and validation rules for the context map
// required to make decisions on this surface.
type ContextSchema struct {
	Fields []ContextField `json:"fields,omitempty" yaml:"fields,omitempty"`
}

// ContextField defines a single required or optional context attribute.
type ContextField struct {
	Name        string          `json:"name" yaml:"name"`
	Type        string          `json:"type" yaml:"type"` // string | number | boolean | object | array
	Required    bool            `json:"required,omitempty" yaml:"required,omitempty"`
	Description string          `json:"description,omitempty" yaml:"description,omitempty"`
	Validation  *ValidationRule `json:"validation,omitempty" yaml:"validation,omitempty"`
	Example     any             `json:"example,omitempty" yaml:"example,omitempty"`
}

// ValidationRule specifies constraints on field values.
type ValidationRule struct {
	// String validation
	Pattern   string   `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	MinLength *int     `json:"min_length,omitempty" yaml:"min_length,omitempty"`
	MaxLength *int     `json:"max_length,omitempty" yaml:"max_length,omitempty"`
	Enum      []string `json:"enum,omitempty" yaml:"enum,omitempty"`

	// Number validation
	Minimum          *float64 `json:"minimum,omitempty" yaml:"minimum,omitempty"`
	Maximum          *float64 `json:"maximum,omitempty" yaml:"maximum,omitempty"`
	ExclusiveMinimum bool     `json:"exclusive_minimum,omitempty" yaml:"exclusive_minimum,omitempty"`
	ExclusiveMaximum bool     `json:"exclusive_maximum,omitempty" yaml:"exclusive_maximum,omitempty"`

	// Array validation
	MinItems *int `json:"min_items,omitempty" yaml:"min_items,omitempty"`
	MaxItems *int `json:"max_items,omitempty" yaml:"max_items,omitempty"`
}

// ConsequenceType defines a category of impact that decisions on this surface
// can produce.
type ConsequenceType struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	MeasureType string `json:"measure_type" yaml:"measure_type"` // financial | temporal | risk_rating | impact_scope | custom`

	Currency     string   `json:"currency,omitempty" yaml:"currency,omitempty"`
	DurationUnit string   `json:"duration_unit,omitempty" yaml:"duration_unit,omitempty"` // hours | days | months | years
	RatingScale  []string `json:"rating_scale,omitempty" yaml:"rating_scale,omitempty"`
	ScopeScale   []string `json:"scope_scale,omitempty" yaml:"scope_scale,omitempty"`

	MinValue *float64 `json:"min_value,omitempty" yaml:"min_value,omitempty"`
	MaxValue *float64 `json:"max_value,omitempty" yaml:"max_value,omitempty"`
}

// EvidenceRequirement specifies documentation or justification that must
// accompany decisions on this surface.
type EvidenceRequirement struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Format      string `json:"format,omitempty" yaml:"format,omitempty"` // structured_data | document_reference | attestation
}

// GetKind returns the resource kind.
func (s SurfaceDocument) GetKind() string { return s.Kind }

// GetID returns the resource ID.
func (s SurfaceDocument) GetID() string { return s.Metadata.ID }

// ---------------------------------------------------------------------------
// Agent
// ---------------------------------------------------------------------------

// AgentDocument defines a non-human or system identity.
type AgentDocument struct {
	APIVersion string           `json:"apiVersion" yaml:"apiVersion"`
	Kind       string           `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata `json:"metadata" yaml:"metadata"`
	Spec       AgentSpec        `json:"spec" yaml:"spec"`
}

// AgentSpec contains the specification for an agent.
type AgentSpec struct {
	Type    string       `json:"type,omitempty" yaml:"type,omitempty"` // llm_agent | workflow | automation | copilot | rpa
	Runtime AgentRuntime `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	Status  string       `json:"status,omitempty" yaml:"status,omitempty"` // active | inactive
}

// AgentRuntime contains runtime metadata for an agent.
type AgentRuntime struct {
	Model    string `json:"model,omitempty" yaml:"model,omitempty"`
	Version  string `json:"version,omitempty" yaml:"version,omitempty"`
	Provider string `json:"provider,omitempty" yaml:"provider,omitempty"`
}

// GetKind returns the resource kind.
func (a AgentDocument) GetKind() string { return a.Kind }

// GetID returns the resource ID.
func (a AgentDocument) GetID() string { return a.Metadata.ID }

// ---------------------------------------------------------------------------
// Profile
// ---------------------------------------------------------------------------

// ProfileDocument defines an authority policy envelope.
type ProfileDocument struct {
	APIVersion string           `json:"apiVersion" yaml:"apiVersion"`
	Kind       string           `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata `json:"metadata" yaml:"metadata"`
	Spec       ProfileSpec      `json:"spec" yaml:"spec"`
}

// ProfileSpec contains the specification for an authority profile.
type ProfileSpec struct {
	SurfaceID         string                   `json:"surface_id" yaml:"surface_id"`
	Authority         ProfileAuthority         `json:"authority,omitempty" yaml:"authority,omitempty"`
	InputRequirements ProfileInputRequirements `json:"input_requirements,omitempty" yaml:"input_requirements,omitempty"`
	Policy            ProfilePolicy            `json:"policy,omitempty" yaml:"policy,omitempty"`
	Lifecycle         ProfileLifecycle         `json:"lifecycle,omitempty" yaml:"lifecycle,omitempty"`
}

// ProfileAuthority defines the authority limits for a profile.
type ProfileAuthority struct {
	DecisionConfidenceThreshold float64              `json:"decision_confidence_threshold,omitempty" yaml:"decision_confidence_threshold,omitempty"`
	ConsequenceThreshold        ConsequenceThreshold `json:"consequence_threshold,omitempty" yaml:"consequence_threshold,omitempty"`
}

// ConsequenceThreshold defines limits on decision consequences.
type ConsequenceThreshold struct {
	Type       string  `json:"type,omitempty" yaml:"type,omitempty"`               // monetary | data_access | risk_rating
	Amount     float64 `json:"amount,omitempty" yaml:"amount,omitempty"`           // for monetary type
	Currency   string  `json:"currency,omitempty" yaml:"currency,omitempty"`       // for monetary type
	RiskRating string  `json:"risk_rating,omitempty" yaml:"risk_rating,omitempty"` // for risk_rating type
}

// ProfileInputRequirements defines required context for decisions.
type ProfileInputRequirements struct {
	RequiredContext []string `json:"required_context,omitempty" yaml:"required_context,omitempty"`
}

// ProfilePolicy defines policy evaluation settings.
type ProfilePolicy struct {
	Reference string `json:"reference,omitempty" yaml:"reference,omitempty"` // e.g. rego://payments/auto_approve_v1
	FailMode  string `json:"fail_mode,omitempty" yaml:"fail_mode,omitempty"` // closed | open
}

// ProfileLifecycle defines lifecycle metadata for a profile.
type ProfileLifecycle struct {
	Status         string `json:"status,omitempty" yaml:"status,omitempty"`                   // active | inactive | deprecated
	EffectiveFrom  string `json:"effective_from,omitempty" yaml:"effective_from,omitempty"`   // RFC3339 timestamp
	EffectiveUntil string `json:"effective_until,omitempty" yaml:"effective_until,omitempty"` // RFC3339 timestamp
	Version        int    `json:"version,omitempty" yaml:"version,omitempty"`
}

// GetKind returns the resource kind.
func (p ProfileDocument) GetKind() string { return p.Kind }

// GetID returns the resource ID.
func (p ProfileDocument) GetID() string { return p.Metadata.ID }

// ---------------------------------------------------------------------------
// Grant
// ---------------------------------------------------------------------------

// GrantDocument assigns a profile to an agent for a time period.
type GrantDocument struct {
	APIVersion string           `json:"apiVersion" yaml:"apiVersion"`
	Kind       string           `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata `json:"metadata" yaml:"metadata"`
	Spec       GrantSpec        `json:"spec" yaml:"spec"`
}

// GrantSpec contains the specification for a grant.
type GrantSpec struct {
	AgentID        string            `json:"agent_id" yaml:"agent_id"`
	ProfileID      string            `json:"profile_id" yaml:"profile_id"`
	GrantedBy      string            `json:"granted_by,omitempty" yaml:"granted_by,omitempty"`
	GrantedAt      string            `json:"granted_at,omitempty" yaml:"granted_at,omitempty"`
	EffectiveFrom  string            `json:"effective_from,omitempty" yaml:"effective_from,omitempty"`
	EffectiveUntil string            `json:"effective_until,omitempty" yaml:"effective_until,omitempty"`
	Status         string            `json:"status,omitempty" yaml:"status,omitempty"` // active | suspended | revoked | expired
	Metadata       map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// GetKind returns the resource kind.
func (g GrantDocument) GetKind() string { return g.Kind }

// GetID returns the resource ID.
func (g GrantDocument) GetID() string { return g.Metadata.ID }
