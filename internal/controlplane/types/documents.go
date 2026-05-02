package types

import "time"

// API version and resource kind constants for MIDAS control plane.
const (
	APIVersionV1 = "midas.accept.io/v1"

	KindSurface                     = "Surface"
	KindAgent                       = "Agent"
	KindProfile                     = "Profile"
	KindGrant                       = "Grant"
	KindCapability                  = "Capability"
	KindProcess                     = "Process"
	KindBusinessService             = "BusinessService"
	KindBusinessServiceCapability   = "BusinessServiceCapability"
	KindBusinessServiceRelationship = "BusinessServiceRelationship"
	KindGovernanceExpectation       = "GovernanceExpectation"
	KindAISystem                    = "AISystem"
	KindAISystemVersion             = "AISystemVersion"
	KindAISystemBinding             = "AISystemBinding"
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

	// Process link
	ProcessID string `json:"process_id,omitempty" yaml:"process_id,omitempty"`
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

// ---------------------------------------------------------------------------
// Capability
// ---------------------------------------------------------------------------

type CapabilityDocument struct {
	APIVersion string           `json:"apiVersion" yaml:"apiVersion"`
	Kind       string           `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata `json:"metadata" yaml:"metadata"`
	Spec       CapabilitySpec   `json:"spec" yaml:"spec"`
}

type CapabilitySpec struct {
	Description        string `json:"description,omitempty" yaml:"description,omitempty"`
	Status             string `json:"status" yaml:"status"`
	Owner              string `json:"owner,omitempty" yaml:"owner,omitempty"`
	ParentCapabilityID string `json:"parent_capability_id,omitempty" yaml:"parent_capability_id,omitempty"`
}

func (c CapabilityDocument) GetKind() string { return c.Kind }
func (c CapabilityDocument) GetID() string   { return c.Metadata.ID }

// ---------------------------------------------------------------------------
// Process
// ---------------------------------------------------------------------------

type ProcessDocument struct {
	APIVersion string           `json:"apiVersion" yaml:"apiVersion"`
	Kind       string           `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata `json:"metadata" yaml:"metadata"`
	Spec       ProcessSpec      `json:"spec" yaml:"spec"`
}

type ProcessSpec struct {
	Description       string `json:"description,omitempty" yaml:"description,omitempty"`
	Status            string `json:"status" yaml:"status"`
	Owner             string `json:"owner,omitempty" yaml:"owner,omitempty"`
	BusinessServiceID string `json:"business_service_id" yaml:"business_service_id"`
	ParentProcessID   string `json:"parent_process_id,omitempty" yaml:"parent_process_id,omitempty"`
}

func (p ProcessDocument) GetKind() string { return p.Kind }
func (p ProcessDocument) GetID() string   { return p.Metadata.ID }

// ---------------------------------------------------------------------------
// BusinessService
// ---------------------------------------------------------------------------

type BusinessServiceDocument struct {
	APIVersion string              `json:"apiVersion" yaml:"apiVersion"`
	Kind       string              `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata    `json:"metadata" yaml:"metadata"`
	Spec       BusinessServiceSpec `json:"spec" yaml:"spec"`
}

type BusinessServiceSpec struct {
	Description     string `json:"description,omitempty" yaml:"description,omitempty"`
	ServiceType     string `json:"service_type" yaml:"service_type"`
	RegulatoryScope string `json:"regulatory_scope,omitempty" yaml:"regulatory_scope,omitempty"`
	Status          string `json:"status" yaml:"status"`
	OwnerID         string `json:"owner_id,omitempty" yaml:"owner_id,omitempty"`
}

func (b BusinessServiceDocument) GetKind() string { return b.Kind }
func (b BusinessServiceDocument) GetID() string   { return b.Metadata.ID }

// ---------------------------------------------------------------------------
// BusinessServiceCapability
// ---------------------------------------------------------------------------
//
// BusinessServiceCapability declares an M:N link between a BusinessService and
// a Capability — the canonical Capability ↔ BusinessService relationship in
// the v1 service-led structural model. The metadata.id is a synthetic
// control-plane handle used for bundle identity and duplicate detection; it
// is not stored in the business_service_capabilities table.
//
// Per ADR-XXX, junction rows have no lifecycle: the spec carries only the two
// foreign references. No origin/managed/replaces/status fields.

type BusinessServiceCapabilityDocument struct {
	APIVersion string                        `json:"apiVersion" yaml:"apiVersion"`
	Kind       string                        `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata              `json:"metadata" yaml:"metadata"`
	Spec       BusinessServiceCapabilitySpec `json:"spec" yaml:"spec"`
}

// BusinessServiceCapabilitySpec identifies the BusinessService ↔ Capability link.
// Both fields are required.
type BusinessServiceCapabilitySpec struct {
	BusinessServiceID string `json:"business_service_id" yaml:"business_service_id"`
	CapabilityID      string `json:"capability_id" yaml:"capability_id"`
}

func (bsc BusinessServiceCapabilityDocument) GetKind() string { return bsc.Kind }
func (bsc BusinessServiceCapabilityDocument) GetID() string   { return bsc.Metadata.ID }

// ---------------------------------------------------------------------------
// BusinessServiceRelationship (Epic 1, PR 1)
// ---------------------------------------------------------------------------
//
// BusinessServiceRelationship declares a directed link between two
// BusinessServices. The relationship_type is one of {depends_on, supports,
// part_of}. The metadata.id is the synthetic control-plane handle used for
// bundle identity and duplicate detection.
//
// Like BusinessServiceCapability, this is a junction with no lifecycle of
// its own — no status, no effective dates, no review/approval. Apply
// creates the relationship directly in the active state. Description is
// the only mutable field.

type BusinessServiceRelationshipDocument struct {
	APIVersion string                          `json:"apiVersion" yaml:"apiVersion"`
	Kind       string                          `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata                `json:"metadata" yaml:"metadata"`
	Spec       BusinessServiceRelationshipSpec `json:"spec" yaml:"spec"`
}

// BusinessServiceRelationshipSpec carries the directed link payload. Both
// IDs are required and must reference distinct business services. The
// relationship_type must be one of {depends_on, supports, part_of}.
type BusinessServiceRelationshipSpec struct {
	SourceBusinessServiceID string `json:"source_business_service_id" yaml:"source_business_service_id"`
	TargetBusinessServiceID string `json:"target_business_service_id" yaml:"target_business_service_id"`
	RelationshipType        string `json:"relationship_type" yaml:"relationship_type"`
	Description             string `json:"description,omitempty" yaml:"description,omitempty"`
}

func (bsr BusinessServiceRelationshipDocument) GetKind() string { return bsr.Kind }
func (bsr BusinessServiceRelationshipDocument) GetID() string   { return bsr.Metadata.ID }

// ---------------------------------------------------------------------------
// GovernanceExpectation
// ---------------------------------------------------------------------------
//
// GovernanceExpectation is a declared coverage rule of the form "in this
// scope, when these conditions match the arriving decision, this Surface
// is the one that should have governed it." It is versioned per logical
// ID, persisted in `review` status by control-plane apply (forced by the
// mapper, mirroring Surface and Profile), and only becomes load-bearing
// for the matching engine once approved (out of #52 scope — see #51 epic).
//
// Apply support in #52 deliberately accepts only ScopeKind="process".
// The other two ScopeKind values defined by the domain
// (`business_service`, `capability`) are rejected by the validator with
// an explicit "not supported by control-plane apply yet" message; they
// will be enabled by #53 alongside the matching engine that needs the
// extra traversal validation.
//
// condition_payload is opaque to apply: a YAML map decoded into
// map[string]any and JSON-marshalled by the mapper for JSONB
// persistence. Per-type payload schema validation is the matching
// engine's responsibility (#53).

type GovernanceExpectationDocument struct {
	APIVersion string                    `json:"apiVersion" yaml:"apiVersion"`
	Kind       string                    `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata          `json:"metadata" yaml:"metadata"`
	Spec       GovernanceExpectationSpec `json:"spec" yaml:"spec"`
}

type GovernanceExpectationSpec struct {
	Description       string                         `json:"description,omitempty" yaml:"description,omitempty"`
	ScopeKind         string                         `json:"scope_kind" yaml:"scope_kind"`                                   // process | business_service | capability — only "process" accepted by apply in #52
	ScopeID           string                         `json:"scope_id" yaml:"scope_id"`                                       // logical ID of the structural anchor
	RequiredSurfaceID string                         `json:"required_surface_id" yaml:"required_surface_id"`                 // logical Surface ID; not version-pinned
	ConditionType     string                         `json:"condition_type" yaml:"condition_type"`                           // closed enum; today only "risk_condition"
	ConditionPayload  map[string]any                 `json:"condition_payload,omitempty" yaml:"condition_payload,omitempty"` // opaque to apply
	BusinessOwner     string                         `json:"business_owner" yaml:"business_owner"`
	TechnicalOwner    string                         `json:"technical_owner" yaml:"technical_owner"`
	Lifecycle         GovernanceExpectationLifecycle `json:"lifecycle,omitempty" yaml:"lifecycle,omitempty"`
}

// GovernanceExpectationLifecycle mirrors ProfileLifecycle's posture: dates
// are RFC3339 strings parsed in the mapper, not time.Time. The `version`
// field is informational; the apply planner authors the persisted version
// (1 for first apply, latest+1 for re-apply).
type GovernanceExpectationLifecycle struct {
	Status         string `json:"status,omitempty" yaml:"status,omitempty"`                   // accepted but persistence is always 'review'
	EffectiveFrom  string `json:"effective_from,omitempty" yaml:"effective_from,omitempty"`   // RFC3339
	EffectiveUntil string `json:"effective_until,omitempty" yaml:"effective_until,omitempty"` // RFC3339
	Version        int    `json:"version,omitempty" yaml:"version,omitempty"`                 // informational; planner authoritative
}

func (g GovernanceExpectationDocument) GetKind() string { return g.Kind }
func (g GovernanceExpectationDocument) GetID() string   { return g.Metadata.ID }

// ---------------------------------------------------------------------------
// AISystem (Epic 1, PR 2)
// ---------------------------------------------------------------------------
//
// AISystem registers the governance subject — the model behind a runtime
// Agent. Apply is status-honouring (no review-forcing), mirroring
// BusinessService and Agent posture: whatever spec.status the bundle
// declares is persisted directly. Allowed values: active | deprecated |
// retired (closed enum).
//
// Origin (manual | inferred) and Replaces (logical ID of a predecessor)
// follow the existing capability/process pattern. ExternalRef and
// risk-classification fields are deliberately excluded from PR 2.

type AISystemDocument struct {
	APIVersion string           `json:"apiVersion" yaml:"apiVersion"`
	Kind       string           `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata `json:"metadata" yaml:"metadata"`
	Spec       AISystemSpec     `json:"spec" yaml:"spec"`
}

type AISystemSpec struct {
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Owner       string `json:"owner,omitempty" yaml:"owner,omitempty"`
	Vendor      string `json:"vendor,omitempty" yaml:"vendor,omitempty"`
	SystemType  string `json:"system_type,omitempty" yaml:"system_type,omitempty"`
	Status      string `json:"status,omitempty" yaml:"status,omitempty"` // active | deprecated | retired
	Origin      string `json:"origin,omitempty" yaml:"origin,omitempty"` // manual | inferred (defaults to manual)
	Replaces    string `json:"replaces,omitempty" yaml:"replaces,omitempty"`
}

func (a AISystemDocument) GetKind() string { return a.Kind }
func (a AISystemDocument) GetID() string   { return a.Metadata.ID }

// ---------------------------------------------------------------------------
// AISystemVersion (Epic 1, PR 2)
// ---------------------------------------------------------------------------
//
// AISystemVersion is a versioned snapshot of an AISystem (model artifact,
// hash, endpoint, compliance frameworks). Apply is status-honouring:
// spec.status is persisted verbatim. The (ai_system_id, version) tuple
// is the composite identity; metadata.id is the synthetic control-plane
// handle used for bundle identity and duplicate detection.
//
// Lifecycle dates are RFC3339 strings parsed in the mapper, mirroring
// ProfileLifecycle / GovernanceExpectationLifecycle posture. Status enum:
// review | active | deprecated | retired.

type AISystemVersionDocument struct {
	APIVersion string              `json:"apiVersion" yaml:"apiVersion"`
	Kind       string              `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata    `json:"metadata" yaml:"metadata"`
	Spec       AISystemVersionSpec `json:"spec" yaml:"spec"`
}

type AISystemVersionSpec struct {
	AISystemID           string   `json:"ai_system_id" yaml:"ai_system_id"`
	Version              int      `json:"version" yaml:"version"`
	ReleaseLabel         string   `json:"release_label,omitempty" yaml:"release_label,omitempty"`
	ModelArtifact        string   `json:"model_artifact,omitempty" yaml:"model_artifact,omitempty"`
	ModelHash            string   `json:"model_hash,omitempty" yaml:"model_hash,omitempty"`
	Endpoint             string   `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	Status               string   `json:"status,omitempty" yaml:"status,omitempty"`                   // review | active | deprecated | retired
	EffectiveFrom        string   `json:"effective_from,omitempty" yaml:"effective_from,omitempty"`   // RFC3339
	EffectiveUntil       string   `json:"effective_until,omitempty" yaml:"effective_until,omitempty"` // RFC3339
	RetiredAt            string   `json:"retired_at,omitempty" yaml:"retired_at,omitempty"`           // RFC3339
	ComplianceFrameworks []string `json:"compliance_frameworks,omitempty" yaml:"compliance_frameworks,omitempty"`
	DocumentationURL     string   `json:"documentation_url,omitempty" yaml:"documentation_url,omitempty"`
}

func (v AISystemVersionDocument) GetKind() string { return v.Kind }
func (v AISystemVersionDocument) GetID() string   { return v.Metadata.ID }

// ---------------------------------------------------------------------------
// AISystemBinding (Epic 1, PR 2)
// ---------------------------------------------------------------------------
//
// AISystemBinding is the immediate-apply junction linking an AISystem
// (optionally pinned to a specific AISystemVersion) to one or more
// existing MIDAS context entities. Mirrors BusinessServiceRelationship
// and BusinessServiceCapability posture: no Status, no EffectiveFrom,
// no review. At least one of business_service_id, capability_id,
// process_id, or surface_id must be set.
//
// AISystemVersion is *int (pointer) so that "no pinned version" is
// distinguishable from "version 0" at the document layer.
// surface_id has no FK at the schema layer because surfaces are
// versioned (composite key); bindings reference the logical surface ID.

type AISystemBindingDocument struct {
	APIVersion string              `json:"apiVersion" yaml:"apiVersion"`
	Kind       string              `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata    `json:"metadata" yaml:"metadata"`
	Spec       AISystemBindingSpec `json:"spec" yaml:"spec"`
}

type AISystemBindingSpec struct {
	AISystemID        string `json:"ai_system_id" yaml:"ai_system_id"`
	AISystemVersion   *int   `json:"ai_system_version,omitempty" yaml:"ai_system_version,omitempty"`
	BusinessServiceID string `json:"business_service_id,omitempty" yaml:"business_service_id,omitempty"`
	CapabilityID      string `json:"capability_id,omitempty" yaml:"capability_id,omitempty"`
	ProcessID         string `json:"process_id,omitempty" yaml:"process_id,omitempty"`
	SurfaceID         string `json:"surface_id,omitempty" yaml:"surface_id,omitempty"`
	Role              string `json:"role,omitempty" yaml:"role,omitempty"`
	Description       string `json:"description,omitempty" yaml:"description,omitempty"`
}

func (b AISystemBindingDocument) GetKind() string { return b.Kind }
func (b AISystemBindingDocument) GetID() string   { return b.Metadata.ID }
